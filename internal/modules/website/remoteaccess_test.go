package website

import (
	"strings"
	"testing"
)

func TestNormalizeRemoteSource(t *testing.T) {
	ok := map[string]string{
		"182.8.212.16":    "182.8.212.16/32", // pg_hba menolak IP tanpa mask
		" 182.8.212.16 ":  "182.8.212.16/32",
		"10.1.2.3/8":      "10.0.0.0/8",
		"0.0.0.0/0":       "0.0.0.0/0",
		"2001:db8::1":     "2001:db8::1/128",
		"2001:db8::/32":   "2001:db8::/32",
		"182.8.212.16/32": "182.8.212.16/32",
	}
	for in, want := range ok {
		got, err := normalizeRemoteSource(in)
		if err != nil || got != want {
			t.Errorf("normalizeRemoteSource(%q) = %q, %v; mau %q", in, got, err, want)
		}
	}
	// Semua nilai ini akhirnya masuk ke skrip shell & pg_hba — yang bukan
	// IP/CIDR murni wajib ditolak.
	for _, bad := range []string{"", "all", "samehost", "1.2.3.4; rm -rf /", "1.2.3.4/33", "host.example.com", "'1.2.3.4'"} {
		if _, err := normalizeRemoteSource(bad); err == nil {
			t.Errorf("sumber %q seharusnya ditolak", bad)
		}
	}
}

func TestNormalizeHbaName(t *testing.T) {
	if n, _ := normalizeHbaName("", dbNameRE, "db"); n != "all" {
		t.Fatalf("kosong harus jadi all, dapat %q", n)
	}
	if n, _ := normalizeHbaName("kelindoread", dbUserRE, "user"); n != "kelindoread" {
		t.Fatalf("nama sah harus diterima apa adanya, dapat %q", n)
	}
	for _, bad := range []string{"a b", "x'y", "+grp", "@file", "replication,all"} {
		if _, err := normalizeHbaName(bad, dbNameRE, "db"); err == nil {
			t.Errorf("%q seharusnya ditolak", bad)
		}
	}
}

func TestMarkerBolakBalik(t *testing.T) {
	line := remoteMarkerLine("ab12cd34", true, "182.8.212.16/32", "Navicat kantor")
	r, ok := parseRemoteMarker(line)
	if !ok || r.ID != "ab12cd34" || !r.FirewallManaged || r.Source != "182.8.212.16/32" || r.Note != "Navicat kantor" {
		t.Fatalf("marker tidak terbaca balik utuh: %q → %+v", line, r)
	}
	// Marker Docker access TIDAK boleh terbaca sebagai aturan remote.
	if _, ok := parseRemoteMarker("# poinhost-docker-access — izinkan koneksi dari container"); ok {
		t.Fatal("marker Docker tidak boleh dianggap aturan remote")
	}
}

func TestParseRemoteStatusPostgres(t *testing.T) {
	out := "BIND=*\n" +
		"RULE=# poinhost-remote-access id=ab12cd34 fw=1 note=Navicat\thost    all    kelindoread    182.8.212.16/32    scram-sha-256\n" +
		"RULE=# poinhost-remote-access id=deadbeef fw=0 note=\thost    appdb    all    10.0.0.0/8    md5\n" +
		// baris pasangan yang hilang/diedit manual → dilewati, bukan ditebak
		"RULE=# poinhost-remote-access id=00000001 fw=0 note=\t# komentar lain\n"
	st := parseRemoteStatus("postgresql", out)
	if !st.ListenAll || st.Port != 5432 {
		t.Fatalf("listener/port salah: %+v", st)
	}
	if len(st.Rules) != 2 {
		t.Fatalf("harus 2 aturan, dapat %+v", st.Rules)
	}
	r := st.Rules[0]
	if r.Username != "kelindoread" || r.Database != "all" || r.Source != "182.8.212.16/32" || r.Auth != "scram-sha-256" || !r.FirewallManaged || r.Note != "Navicat" {
		t.Fatalf("aturan pertama salah terbaca: %+v", r)
	}
}

func TestParseRemoteStatusMySQL(t *testing.T) {
	out := "BIND=127.0.0.1\nLOCALUSER=mariadb.sys\nLOCALUSER=mysql\nLOCALUSER=mysql.session\nLOCALUSER=root\n" +
		"RULE=# poinhost-remote-access id=ab12cd34 fw=0 src=182.8.212.16/32 note=\n"
	st := parseRemoteStatus("mysql", out)
	if len(st.LocalOnlyUsers) != 1 || st.LocalOnlyUsers[0] != "root" {
		t.Fatalf("akun sistem harus disaring, dapat %v", st.LocalOnlyUsers)
	}
	if st.ListenAll || st.Port != 3306 || len(st.Rules) != 1 || st.Rules[0].Source != "182.8.212.16/32" {
		t.Fatalf("status MySQL salah: %+v", st)
	}
	st.Rules[0].FirewallAllowed = true
	st.FirewallDetected = "none"
	st.finish()
	for _, want := range []string{"127.0.0.1", "Tidak ada firewall aktif", "root"} {
		if !strings.Contains(st.Message, want) {
			t.Errorf("pesan harus menyebut %q: %q", want, st.Message)
		}
	}
}

func TestFinishMemperingatkanSumberTerblokirDanSeluruhInternet(t *testing.T) {
	st := &DBRemoteAccessStatus{Engine: "postgresql", Port: 5432, ListenAll: true, FirewallDetected: "ufw",
		Rules: []DBRemoteRule{
			{ID: "ab12cd34", Source: "182.8.212.16/32", FirewallAllowed: false},
			{ID: "deadbeef", Source: "0.0.0.0/0", FirewallAllowed: true},
		}}
	st.finish()
	if !strings.Contains(st.Message, "182.8.212.16/32") || !strings.Contains(st.Message, "0.0.0.0/0") {
		t.Fatalf("pesan harus menyebut sumber terblokir & peringatan seluruh internet: %q", st.Message)
	}
	clean := &DBRemoteAccessStatus{Engine: "postgresql", Port: 5432, ListenAll: true,
		Rules: []DBRemoteRule{{ID: "ab12cd34", Source: "182.8.212.16/32", FirewallAllowed: true}}}
	clean.finish()
	if clean.Message != "" {
		t.Fatalf("konfigurasi sehat tidak boleh berpesan: %q", clean.Message)
	}
}

// Skrip PostgreSQL: auth mengikuti password_encryption, baris bermarker,
// validasi pg_hba_file_rules + rollback, dan reload/restart sesuai listener.
func TestPgRemoteAddScript(t *testing.T) {
	s := pgRemoteAddScript("ab12cd34", true, "all", "kelindoread", "182.8.212.16/32", "Navicat")
	for _, want := range []string{
		"SHOW password_encryption",
		"# poinhost-remote-access id=ab12cd34 fw=1 note=Navicat",
		"'kelindoread'", "'182.8.212.16/32'",
		"pg_hba_file_rules",
		"sed -i '/^# poinhost-remote-access id=ab12cd34 /,+1d'",
		"pg_reload_conf()",
		"ALTER SYSTEM SET listen_addresses = '*'",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("skrip tidak memuat %q", want)
		}
	}
}

// Mencabut aturan terakhir mengembalikan listen_addresses HANYA bila akses
// Docker juga tidak dipakai — dan sebaliknya mematikan Docker access tidak
// boleh mereset listener selama aturan remote masih ada.
func TestListenerTidakSalingMemutus(t *testing.T) {
	rm := pgRemoteRemoveScript("ab12cd34")
	if !strings.Contains(rm, dockerAccessMarker) || !strings.Contains(rm, "ALTER SYSTEM RESET listen_addresses") {
		t.Fatalf("skrip hapus harus memeriksa marker Docker sebelum reset listener:\n%s", rm)
	}
	dk := dbDockerAccessRevertScript("postgresql", "apt")
	i := strings.Index(dk, "^# "+remoteAccessMarker+" id=")
	j := strings.Index(dk, "ALTER SYSTEM RESET listen_addresses")
	if i < 0 || j < 0 || i > j {
		t.Fatalf("revert Docker harus memeriksa aturan remote sebelum reset listener:\n%s", dk)
	}
}

func TestMySQLRemoteScripts(t *testing.T) {
	add := mysqlRemoteAddScript("apt", "ab12cd34", false, "182.8.212.16/32", "")
	for _, want := range []string{"/etc/mysql/mariadb.conf.d/99-poinhost-remote.cnf", "bind-address = 0.0.0.0", "id=ab12cd34 fw=0 src=182.8.212.16/32"} {
		if !strings.Contains(add, want) {
			t.Errorf("skrip tambah MySQL tidak memuat %q", want)
		}
	}
	if strings.Contains(add, "99-poinhost-docker.cnf") {
		t.Fatal("remote access tidak boleh menulis ke file milik Docker access")
	}
	rm := mysqlRemoteRemoveScript("dnf", "ab12cd34")
	if !strings.Contains(rm, "/etc/my.cnf.d/99-poinhost-remote.cnf") || !strings.Contains(rm, "rm -f") {
		t.Fatalf("skrip hapus MySQL salah:\n%s", rm)
	}
}

func TestFirewallStillNeeded(t *testing.T) {
	rules := []DBRemoteRule{
		{ID: "aaaaaaaa", Source: "182.8.212.16/32"},
		{ID: "bbbbbbbb", Source: "182.8.212.16/32"},
		{ID: "cccccccc", Source: "10.0.0.0/8"},
	}
	if !firewallStillNeeded(rules, rules[0]) {
		t.Fatal("masih ada aturan lain bersumber sama — aturan firewall harus dipertahankan")
	}
	if firewallStillNeeded(rules, rules[2]) {
		t.Fatal("tidak ada aturan lain bersumber sama — aturan firewall boleh dicabut")
	}
}
