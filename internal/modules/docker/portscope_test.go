package docker

import "testing"

// Scope hanya dua: public dan private. Nilai lama "intranet"/"localhost"
// tetap diterima karena ada container yang terlanjur berlabel itu.
func TestNormalizePortScope(t *testing.T) {
	cases := map[string]string{
		"":          portScopePublic,
		"public":    portScopePublic,
		"PUBLIC":    portScopePublic,
		"private":   portScopePrivate,
		" Private ": portScopePrivate,
		// Nilai lama: "localhost" artinya persis sama dengan private.
		"localhost": portScopePrivate,
		// "intranet" dulu hanya DIJANJIKAN terbatas (aturan ufw di rantai
		// INPUT yang tidak pernah dilewati trafik Docker). Dipetakan ke
		// private, jadi proteksinya justru NAIK dari yang dulu dijanjikan.
		"intranet": portScopePrivate,
	}
	for in, want := range cases {
		got, err := normalizePortScope(in)
		if err != nil {
			t.Fatalf("normalizePortScope(%q) error: %v", in, err)
		}
		if got != want {
			t.Fatalf("normalizePortScope(%q) = %q, mau %q", in, got, want)
		}
	}

	for _, bad := range []string{"lan", "semua", "0.0.0.0", "private; reboot"} {
		if _, err := normalizePortScope(bad); err == nil {
			t.Fatalf("scope %q seharusnya ditolak", bad)
		}
	}
}

// Inilah inti perbaikannya: private ditegakkan lewat ALAMAT BIND, bukan
// aturan firewall. Bind ke 127.0.0.1 membuat Docker memasang DNAT hanya di
// loopback, sehingga portnya tidak terjangkau dari luar apa pun kondisi
// firewallnya — tidak ada lagi janji proteksi yang bergantung pada rantai
// iptables yang ternyata tidak dilewati trafik Docker.
func TestHostIPForScope(t *testing.T) {
	if got := hostIPForScope(portScopePrivate); got != "127.0.0.1" {
		t.Fatalf("private harus bind loopback, dapat %q", got)
	}
	if got := hostIPForScope(portScopePublic); got != "" {
		t.Fatalf("public harus bind semua interface (HostIP kosong), dapat %q", got)
	}
	// Nilai lama yang belum dinormalkan tidak boleh diam-diam jadi public.
	if got := hostIPForScope("intranet"); got != "" {
		t.Fatalf("nilai mentah harus dinormalkan dulu lewat normalizePortScope, dapat %q", got)
	}
}

func TestSyncPortScopeLabels(t *testing.T) {
	// Label port yang sudah tidak dipakai lagi harus dibuang, bukan menumpuk.
	labels := map[string]string{
		"poinhost.portscope.9999.tcp": "public",
		"com.docker.compose.service":  "app",
	}
	ports := []PortMapping{
		{HostPort: 8089, Protocol: "tcp", Scope: portScopePrivate},
		{HostPort: 5000, Protocol: "", Scope: ""},
	}
	got := syncPortScopeLabels(labels, ports)

	if _, ada := got["poinhost.portscope.9999.tcp"]; ada {
		t.Fatal("label port lama harus dibuang")
	}
	if got["poinhost.portscope.8089.tcp"] != portScopePrivate {
		t.Fatalf("scope private tidak tersimpan: %+v", got)
	}
	// Protokol kosong dianggap tcp, scope kosong dianggap public.
	if got["poinhost.portscope.5000.tcp"] != portScopePublic {
		t.Fatalf("default scope salah: %+v", got)
	}
	// Label milik pihak lain tidak boleh ikut terhapus.
	if got["com.docker.compose.service"] != "app" {
		t.Fatal("label non-poinhost tidak boleh disentuh")
	}
}

// Regresi: fitur ini TIDAK BOLEH lagi menulis aturan firewall. Aturan ufw
// hidup di rantai INPUT, sedangkan trafik ke port container yang di-publish
// lewat PREROUTING→FORWARD→DOCKER-USER dan tidak pernah menyentuh INPUT —
// jadi aturan seperti itu tidak menegakkan apa pun, cuma memberi kesan
// terlindungi.
func TestScopeTidakMenulisAturanFirewall(t *testing.T) {
	labels := syncPortScopeLabels(nil, []PortMapping{
		{HostPort: 8089, Protocol: "tcp", Scope: portScopePrivate},
	})
	for k, v := range labels {
		if k == "" || v == "" {
			t.Fatalf("label tidak lengkap: %q=%q", k, v)
		}
	}
	// hostIPForScope adalah SATU-SATUNYA mekanisme penegakan sekarang.
	if hostIPForScope(portScopePrivate) == "" {
		t.Fatal("private wajib punya alamat bind, kalau tidak tidak ada yang menegakkannya")
	}
}
