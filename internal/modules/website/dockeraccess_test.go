package website

import (
	"strings"
	"testing"
)

// Jalur jaringan terbuka BUKAN jaminan container bisa masuk: MySQL masih
// menolak akun yang hostnya hanya 'localhost'. Ini kegagalan yang paling
// sering disalahartikan sebagai masalah password, jadi harus dilaporkan.
func TestComputeEnabledMemperingatkanAkunLocalhostSaja(t *testing.T) {
	st := &DBDockerAccessStatus{
		Engine:             "mysql",
		BindAllInterfaces:  true,
		FirewallDetected:   "ufw",
		FirewallRuleActive: true,
		LocalOnlyUsers:     []string{"webuser", "acmeapp"},
	}
	st.computeEnabled()

	// Jalur jaringannya memang sudah benar — Enabled tidak diturunkan.
	if !st.Enabled {
		t.Fatal("jalur jaringan sudah terbuka, Enabled seharusnya tetap true")
	}
	if !strings.Contains(st.Message, "webuser") || !strings.Contains(st.Message, "acmeapp") {
		t.Fatalf("akun yang bermasalah harus disebutkan: %q", st.Message)
	}
	if !strings.Contains(st.Message, "is not allowed") {
		t.Fatalf("pesan harus menyebut gejala aslinya supaya mudah dikenali: %q", st.Message)
	}
}

func TestComputeEnabledTanpaAkunBermasalahTidakMemperingatkan(t *testing.T) {
	st := &DBDockerAccessStatus{
		Engine:             "mysql",
		BindAllInterfaces:  true,
		FirewallDetected:   "ufw",
		FirewallRuleActive: true,
	}
	st.computeEnabled()
	if st.Message != "" {
		t.Fatalf("tanpa akun bermasalah tidak boleh ada peringatan: %q", st.Message)
	}
}

// Kalau jalur jaringannya sendiri masih tertutup, peringatan akun tidak
// ditambahkan — masalah yang lebih mendasar dulu yang harus diselesaikan,
// dan menumpuk dua pesan justru mengaburkan mana yang perlu dikerjakan.
func TestComputeEnabledTidakMenumpukPesanSaatJaringanMasihTertutup(t *testing.T) {
	st := &DBDockerAccessStatus{
		Engine:             "mysql",
		BindAllInterfaces:  false,
		FirewallDetected:   "ufw",
		FirewallRuleActive: false,
		LocalOnlyUsers:     []string{"webuser"},
	}
	st.computeEnabled()
	if st.Enabled {
		t.Fatal("bind-address belum 0.0.0.0, tidak boleh dianggap enabled")
	}
	if strings.Contains(st.Message, "webuser") {
		t.Fatalf("peringatan akun jangan muncul sebelum jaringannya terbuka: %q", st.Message)
	}
}

// pg_hba TIDAK boleh mematok md5: PostgreSQL 14+ menyimpan password sebagai
// scram-sha-256 dan baris md5 akan menolak login walau passwordnya benar.
func TestPgHbaMengikutiPasswordEncryption(t *testing.T) {
	script := dbDockerAccessApplyScript("postgresql", "apt")
	if !strings.Contains(script, "SHOW password_encryption") {
		t.Fatalf("metode auth harus dibaca dari server:\n%s", script)
	}
	if !strings.Contains(script, "scram-sha-256") {
		t.Fatalf("scram-sha-256 harus didukung:\n%s", script)
	}
	// Pastikan nilainya benar-benar dipakai, bukan sekadar dibaca lalu
	// baris pg_hba tetap ditulis "md5".
	if strings.Contains(script, "            md5\n'") {
		t.Fatalf("baris pg_hba masih mematok md5:\n%s", script)
	}
}

// Network Docker dengan subnet kustom (10.x / 192.168.x dari
// default-address-pools atau compose) harus ikut diizinkan — dulu yang
// dipatok hanya 172.16.0.0/12, sehingga container di network seperti itu
// tidak pernah bisa connect apa pun yang ditekan.
func TestAksesDockerMemakaiSubnetTerdeteksi(t *testing.T) {
	for _, engine := range []string{"postgresql", "mysql"} {
		script := dbDockerAccessApplyScript(engine, "apt")
		if !strings.Contains(script, "docker network inspect") {
			t.Fatalf("%s: subnet harus dideteksi dari server:\n%s", engine, script)
		}
		if !strings.Contains(script, `for S in $SUBNETS`) {
			t.Fatalf("%s: aturan harus dipasang per subnet terdeteksi:\n%s", engine, script)
		}
	}
	pg := dbDockerAccessApplyScript("postgresql", "apt")
	if !strings.Contains(pg, "strip_docker_hba") || !strings.Contains(pg, "# "+dockerAccessMarker+`\n' "$S" "$AUTH"`) {
		t.Fatalf("pg_hba harus disinkronkan ulang per subnet:\n%s", pg)
	}
	// Menerapkan ulang tidak boleh lagi dilewati hanya karena marker lama ada.
	if strings.Contains(pg, `! grep -q "`+dockerAccessMarker+`"`) {
		t.Fatalf("baris pg_hba lama tidak boleh menghalangi sinkronisasi:\n%s", pg)
	}
}

func TestHbaMissingSubnets(t *testing.T) {
	subnets := []string{"172.16.0.0/12", "10.200.0.0/24", "192.168.50.0/24"}
	// format lama hanya punya 172.16.0.0/12
	missing := hbaMissingSubnets(subnets, []string{"172.16.0.0/12"})
	if len(missing) != 2 || missing[0] != "10.200.0.0/24" || missing[1] != "192.168.50.0/24" {
		t.Fatalf("subnet kustom harus dilaporkan belum tercakup, dapat %v", missing)
	}
	if m := hbaMissingSubnets(subnets, []string{"172.16.0.0/12", "10.0.0.0/8", "192.168.50.0/24"}); len(m) != 0 {
		t.Fatalf("semua tercakup (termasuk lewat CIDR lebih longgar), dapat %v", m)
	}
	if cidrContains("172.17.0.0/16", "172.16.0.0/12") {
		t.Fatal("CIDR sempit tidak boleh dianggap mencakup yang lebih longgar")
	}
}

// Sejak remote access bisa membuat listener '*', listener terbuka saja
// tidak boleh lagi dibaca "container bisa connect".
func TestStatusDockerPostgresMemeriksaPgHba(t *testing.T) {
	out := "BIND=*\nDOCKERNET=172.16.0.0/12\nDOCKERNET=10.200.0.0/24\nHBADOCKER=172.16.0.0/12\n"
	st := parseDockerAccessStatus("postgresql", out)
	st.FirewallDetected = "none"
	st.computeEnabled()
	if st.Enabled {
		t.Fatal("pg_hba belum mencakup 10.200.0.0/24 — belum boleh enabled")
	}
	if !strings.Contains(st.Message, "10.200.0.0/24") || !strings.Contains(st.Message, "no pg_hba.conf entry") {
		t.Fatalf("pesan harus menyebut subnet dan gejalanya: %q", st.Message)
	}
	ok := parseDockerAccessStatus("postgresql", out+"HBADOCKER=10.200.0.0/24\n")
	ok.FirewallDetected = "none"
	ok.computeEnabled()
	if !ok.Enabled || ok.Message != "" {
		t.Fatalf("semua lapisan terbuka — harus enabled tanpa pesan: %+v", ok)
	}
	// MySQL tidak punya pg_hba
	my := parseDockerAccessStatus("mysql", "BIND=0.0.0.0\nLOCALUSER=mariadb.sys\n")
	my.FirewallDetected = "none"
	my.computeEnabled()
	if !my.Enabled || len(my.LocalOnlyUsers) != 0 {
		t.Fatalf("MySQL: enabled & akun sistem disaring, dapat %+v", my)
	}
}
