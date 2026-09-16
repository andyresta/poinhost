package firewall

import (
	"strings"
	"testing"
)

// Keluaran `ufw status numbered` yang ditiru di sini mencakup kasus yang
// gampang salah baca: kolom "To" berisi spasi ("22/tcp (v6)"), aturan dengan
// dan tanpa komentar, rentang port, app profile yang bukan port, serta aturan
// buatan fitur lain di aplikasi ini (bermarker).
const ufwSample = `Status: active

     To                         Action      From
     --                         ------      ----
[ 1] 22/tcp                     ALLOW IN    Anywhere                   # SSH masuk
[ 2] 80/tcp                     ALLOW IN    Anywhere
[ 3] 3306                       ALLOW IN    172.17.0.0/16              # poinhost-docker-access
[ 4] 8000:9000/udp              ALLOW IN    10.0.0.0/8
[ 5] OpenSSH                    ALLOW IN    Anywhere
[ 6] 23/tcp                     DENY IN     Anywhere
[ 7] 22/tcp (v6)                ALLOW IN    Anywhere (v6)              # SSH masuk
`

func TestParseUFW(t *testing.T) {
	rules := parseUFW(ufwSample)
	if len(rules) != 7 {
		t.Fatalf("harus 7 aturan, dapat %d: %+v", len(rules), rules)
	}

	if rules[0].Port != "22" || rules[0].Protocol != "tcp" || rules[0].Action != "allow" {
		t.Fatalf("aturan SSH salah baca: %+v", rules[0])
	}
	if rules[0].Source != "" {
		t.Fatalf("\"Anywhere\" harus jadi sumber kosong, dapat %q", rules[0].Source)
	}
	if rules[0].Comment != "SSH masuk" {
		t.Fatalf("komentar salah: %q", rules[0].Comment)
	}

	// Aturan milik fitur lain harus dikenali supaya UI bisa memperingatkan.
	if rules[2].Owner != "poinhost-docker-access" {
		t.Fatalf("pemilik aturan tidak terdeteksi: %+v", rules[2])
	}
	if rules[2].Source != "172.17.0.0/16" {
		t.Fatalf("sumber CIDR salah: %+v", rules[2])
	}

	// Rentang port + protokol udp.
	if rules[3].Port != "8000:9000" || rules[3].Protocol != "udp" {
		t.Fatalf("rentang port salah baca: %+v", rules[3])
	}

	// App profile bukan port — tidak boleh dianggap port, dan tidak boleh
	// punya ID hapus (spesifikasinya tidak bisa disusun ulang).
	if rules[4].Port != "" || rules[4].ID != "" {
		t.Fatalf("app profile seharusnya tanpa port dan tanpa ID: %+v", rules[4])
	}

	if rules[5].Action != "deny" {
		t.Fatalf("DENY salah baca: %+v", rules[5])
	}

	// Baris (v6) ditampilkan terpisah dan ditandai.
	if !rules[6].IPv6 || rules[6].Port != "22" {
		t.Fatalf("baris v6 salah baca: %+v", rules[6])
	}
}

// ID hapus disusun sebagai SPESIFIKASI, bukan nomor baris — nomor bergeser
// setiap ada penghapusan lain.
func TestUFWRuleIDIsASpecNotALineNumber(t *testing.T) {
	rules := parseUFW(ufwSample)
	if got := rules[2].ID; got != "allow from 172.17.0.0/16 to any port 3306" {
		t.Fatalf("ID aturan salah bentuk: %q", got)
	}
	for _, r := range rules {
		if r.ID == "" {
			continue
		}
		if err := validateUFWDeleteSpec(r.ID); err != nil {
			t.Fatalf("ID hasil parse harus lolos validasi hapus: %q → %v", r.ID, err)
		}
	}
}

const firewalldSample = `PORT=80/tcp 443/tcp 8000-9000/udp
RICH=rule family="ipv4" source address="172.17.0.0/16" port port="3306" protocol="tcp" accept
RICH=rule family="ipv4" port port="23" protocol="tcp" drop
RICH=rule family="ipv6" source address="2001:db8::/32" port port="443" protocol="tcp" accept
`

func TestParseFirewalld(t *testing.T) {
	rules := parseFirewalld(firewalldSample)
	if len(rules) != 6 {
		t.Fatalf("harus 6 aturan (3 port + 3 rich), dapat %d: %+v", len(rules), rules)
	}

	if rules[0].Port != "80" || rules[0].Protocol != "tcp" || rules[0].Action != "allow" {
		t.Fatalf("port biasa salah baca: %+v", rules[0])
	}
	// Rentang firewalld pakai "-", dinormalkan ke ":" agar sama dengan ufw.
	if rules[2].Port != "8000:9000" {
		t.Fatalf("rentang port firewalld harus dinormalkan: %+v", rules[2])
	}

	rich := rules[3]
	if rich.Source != "172.17.0.0/16" || rich.Port != "3306" || rich.Protocol != "tcp" {
		t.Fatalf("rich rule salah baca: %+v", rich)
	}
	// protocol="..." muncul dua kali di rich rule; yang diambil harus tcp,
	// bukan nilai family.
	if rules[4].Action != "deny" {
		t.Fatalf("rich rule drop harus jadi deny: %+v", rules[4])
	}
	if !rules[5].IPv6 {
		t.Fatalf("rich rule ipv6 harus ditandai: %+v", rules[5])
	}
}

// Deteksi backend HARUS berdasar yang aktif, bukan yang terpasang: satu
// server bisa punya ufw dan firewalld sekaligus.
func TestParseListPicksActiveBackendNotInstalledOne(t *testing.T) {
	stdout := `INSTALLED=ufw
INSTALLED=firewalld
INSTALLED=nftables
BACKEND=ufw
DEFAULT=Default: deny (incoming), allow (outgoing), disabled (routed)
__RULES__
[ 1] 22/tcp                     ALLOW IN    Anywhere
`
	out := parseList(stdout, 22)
	if out.Status.Backend != "ufw" || !out.Status.Active {
		t.Fatalf("backend aktif salah: %+v", out.Status)
	}
	if len(out.Status.Installed) != 3 {
		t.Fatalf("daftar terpasang harus tetap lengkap: %+v", out.Status.Installed)
	}
	if out.Status.DefaultIncoming != "deny" || out.Status.DefaultOutgoing != "allow" {
		t.Fatalf("kebijakan default salah baca: %+v", out.Status)
	}
	if !out.Status.Editable || !out.Status.OwnerDetectable {
		t.Fatalf("ufw harus editable dan asal aturannya terdeteksi: %+v", out.Status)
	}
}

func TestParseListFirewalldCannotDetectOwner(t *testing.T) {
	out := parseList("INSTALLED=firewalld\nBACKEND=firewalld\nZONE=public\n__RULES__\nPORT=80/tcp\n", 22)
	if out.Status.Backend != "firewalld" || !out.Status.Editable {
		t.Fatalf("firewalld harus editable: %+v", out.Status)
	}
	// Rich rule tidak punya kolom komentar — ini keterbatasan nyata yang
	// dilaporkan apa adanya, bukan ditutupi.
	if out.Status.OwnerDetectable {
		t.Fatal("asal aturan tidak mungkin dipastikan di firewalld")
	}
}

func TestParseListNftablesIsReadOnly(t *testing.T) {
	out := parseList("INSTALLED=nftables\nBACKEND=nftables\n__RULES__\n", 22)
	if out.Status.Editable {
		t.Fatal("nftables mentah tidak boleh bisa diubah dari sini")
	}
}

func TestParseListInactiveFirewall(t *testing.T) {
	out := parseList("INSTALLED=ufw\nBACKEND=ufw\nINACTIVE=1\n__RULES__\n", 22)
	if out.Status.Active {
		t.Fatal("firewall terpasang tapi mati harus dilaporkan tidak aktif")
	}
	if out.Status.SSHAllowed {
		t.Fatal("tanpa aturan apa pun, SSH tidak boleh dianggap sudah diizinkan")
	}
}

// Penjaga anti-terkunci: inilah yang mencegah `ufw enable` memutus sesi SSH.
func TestSSHAllowedGuard(t *testing.T) {
	cases := []struct {
		name  string
		rules []Rule
		port  int
		want  bool
	}{
		{"port persis", []Rule{{Port: "22", Protocol: "tcp", Action: "allow"}}, 22, true},
		{"tercakup rentang", []Rule{{Port: "20:30", Protocol: "tcp", Action: "allow"}}, 22, true},
		{"port lain", []Rule{{Port: "80", Protocol: "tcp", Action: "allow"}}, 22, false},
		{"port SSH tidak baku", []Rule{{Port: "2222", Protocol: "tcp", Action: "allow"}}, 2222, true},
		{"aturannya deny", []Rule{{Port: "22", Protocol: "tcp", Action: "deny"}}, 22, false},
		{"protokol udp saja", []Rule{{Port: "22", Protocol: "udp", Action: "allow"}}, 22, false},
		{"tanpa aturan", nil, 22, false},
	}
	for _, c := range cases {
		if got := sshAllowed(c.rules, c.port); got != c.want {
			t.Fatalf("%s: sshAllowed = %v, mau %v", c.name, got, c.want)
		}
	}
}

// Skrip pengaktifan WAJIB membuka SSH lebih dulu, baru menyalakan firewall.
// Urutan terbalik akan mengunci user dari servernya sendiri.
func TestEnableScriptsOpenSSHBeforeEnabling(t *testing.T) {
	ufw := ufwEnableScript(2222)
	allowAt := strings.Index(ufw, "allow to any port 2222")
	enableAt := strings.Index(ufw, "--force enable")
	if allowAt < 0 || enableAt < 0 {
		t.Fatalf("skrip ufw tidak lengkap:\n%s", ufw)
	}
	if allowAt > enableAt {
		t.Fatalf("aturan SSH harus SEBELUM enable:\n%s", ufw)
	}

	fwd := firewalldEnableScript(2222)
	offlineAt := strings.Index(fwd, "firewall-offline-cmd --add-port=2222/tcp")
	startAt := strings.Index(fwd, "systemctl enable --now firewalld")
	if offlineAt < 0 || startAt < 0 {
		t.Fatalf("skrip firewalld tidak lengkap:\n%s", fwd)
	}
	if offlineAt > startAt {
		t.Fatalf("port SSH harus ditulis SEBELUM firewalld dinyalakan:\n%s", fwd)
	}
}

// ufw akan menggantung menunggu jawaban interaktif tanpa --force.
func TestUFWEnableIsNonInteractive(t *testing.T) {
	if !strings.Contains(ufwEnableScript(22), "ufw --force enable") {
		t.Fatal("ufw enable harus memakai --force agar tidak menunggu prompt")
	}
}

// Semua nilai yang ikut jadi perintah shell divalidasi lewat whitelist.
func TestInputValidationRejectsInjection(t *testing.T) {
	for _, bad := range []string{"80; rm -rf /", "$(id)", "`id`", "0", "70000", "", "80 80"} {
		if _, err := normalizePort(bad); err == nil {
			t.Fatalf("port %q seharusnya ditolak", bad)
		}
	}
	for _, bad := range []string{"tcp; reboot", "icmp", "$(id)"} {
		if _, err := normalizeProtocol(bad); err == nil {
			t.Fatalf("protokol %q seharusnya ditolak", bad)
		}
	}
	// Nama host ditolak: resolusinya bisa berubah diam-diam sehingga aturan
	// firewall jadi tidak deterministik.
	for _, bad := range []string{"example.com", "10.0.0.1; reboot", "$(id)"} {
		if _, err := normalizeSource(bad); err == nil {
			t.Fatalf("sumber %q seharusnya ditolak", bad)
		}
	}
	for _, bad := range []string{"drop; reboot", "reject"} {
		if _, err := normalizeAction(bad); err == nil {
			t.Fatalf("aksi %q seharusnya ditolak", bad)
		}
	}
	if _, err := normalizeComment("bagus'; rm -rf /"); err == nil {
		t.Fatal("komentar dengan kutip seharusnya ditolak")
	}
}

func TestNormalizeAcceptsValidInput(t *testing.T) {
	if p, err := normalizePort("8000-9000"); err != nil || p != "8000:9000" {
		t.Fatalf("rentang gaya firewalld harus dinormalkan: %q %v", p, err)
	}
	if s, err := normalizeSource("any"); err != nil || s != "" {
		t.Fatalf("\"any\" harus jadi sumber kosong: %q %v", s, err)
	}
	if a, err := normalizeAction(""); err != nil || a != "allow" {
		t.Fatalf("aksi kosong harus default allow: %q %v", a, err)
	}
}

// ID aturan datang dari frontend; string sembarang tidak boleh menumpang.
func TestRuleIDValidationRejectsForgedInput(t *testing.T) {
	for _, bad := range []string{
		"allow to any port 22; reboot",
		"allow from example.com to any port 22",
		"$(id)",
		"",
	} {
		if err := validateUFWDeleteSpec(bad); err == nil {
			t.Fatalf("spesifikasi ufw %q seharusnya ditolak", bad)
		}
	}
	for _, bad := range []string{
		"rich:rule family=\"ipv4\"; reboot",
		"rich:bukan rule",
		"port:22/tcp; reboot",
		"port:abc/tcp",
		"sembarang",
	} {
		if _, err := parseFirewalldRuleID(bad, ""); err == nil {
			t.Fatalf("ID firewalld %q seharusnya ditolak", bad)
		}
	}
	if _, err := parseFirewalldRuleID("port:3306/tcp", ""); err != nil {
		t.Fatalf("ID port yang sah ditolak: %v", err)
	}
}

// Keluaran `ufw show added` — dipakai saat ufw terpasang tapi MATI. Mencakup
// bentuk ringkas maupun lengkap, komentar berisi spasi, dan aksi limit yang
// tidak dikelola dari sini.
const ufwAddedSample = `ADDED=Added user rules (see 'ufw status' for running firewall):
ADDED=ufw allow 22/tcp
ADDED=ufw allow 8080
ADDED=ufw deny 23/tcp
ADDED=ufw allow from 172.17.0.0/16 to any port 3306 proto tcp comment 'poinhost-docker-access'
ADDED=ufw allow from 10.0.0.0/8 to any port 8000:9000 proto udp comment 'rentang internal'
ADDED=ufw limit ssh
`

func TestParseUFWAdded(t *testing.T) {
	rules := parseUFWAdded(ufwAddedSample)
	if len(rules) != 6 {
		t.Fatalf("harus 6 aturan, dapat %d: %+v", len(rules), rules)
	}

	// Bentuk ringkas "22/tcp".
	if rules[0].Port != "22" || rules[0].Protocol != "tcp" || rules[0].Action != "allow" {
		t.Fatalf("bentuk ringkas salah baca: %+v", rules[0])
	}
	// Ringkas tanpa protokol.
	if rules[1].Port != "8080" || rules[1].Protocol != "" {
		t.Fatalf("port tanpa protokol salah baca: %+v", rules[1])
	}
	if rules[2].Action != "deny" {
		t.Fatalf("deny salah baca: %+v", rules[2])
	}

	// Bentuk lengkap + komentar + pemilik.
	r := rules[3]
	if r.Source != "172.17.0.0/16" || r.Port != "3306" || r.Protocol != "tcp" {
		t.Fatalf("bentuk lengkap salah baca: %+v", r)
	}
	if r.Owner != "poinhost-docker-access" {
		t.Fatalf("pemilik tidak terdeteksi: %+v", r)
	}

	// Komentar berisi spasi harus utuh, tidak terpotong saat baris dipecah.
	if rules[4].Comment != "rentang internal" || rules[4].Port != "8000:9000" {
		t.Fatalf("komentar berspasi / rentang salah baca: %+v", rules[4])
	}

	// "limit ssh": bukan allow/deny dan tanpa port angka — tidak boleh punya
	// ID hapus.
	if rules[5].ID != "" {
		t.Fatalf("aturan limit tidak boleh bisa dihapus dari sini: %+v", rules[5])
	}

	// Semua ID yang terbentuk harus lolos validasi hapus.
	for _, r := range rules {
		if r.ID == "" {
			continue
		}
		if err := validateUFWDeleteSpec(r.ID); err != nil {
			t.Fatalf("ID %q ditolak validasi: %v", r.ID, err)
		}
	}
}

// Regresi: ruleset nftables yang terisi karena Docker TIDAK boleh dianggap
// firewall aktif, dan tidak boleh menutupi ufw yang terpasang tapi mati.
func TestInactiveUFWIsPreferredOverDockerNftables(t *testing.T) {
	stdout := `INSTALLED=ufw
INSTALLED=nftables
BACKEND=ufw
INACTIVE=1
__RULES__
ADDED=Added user rules (see 'ufw status' for running firewall):
ADDED=ufw allow 22/tcp
`
	out := parseList(stdout, 22)
	if out.Status.Backend != "ufw" {
		t.Fatalf("ufw terpasang harus menang atas nftables bikinan Docker: %+v", out.Status)
	}
	if out.Status.Active {
		t.Fatal("ufw yang mati tidak boleh dilaporkan aktif")
	}
	// Ini yang membuat fitur terasa berfungsi: aturan tetap terlihat walau
	// firewall belum menyala.
	if len(out.Rules) != 1 || out.Rules[0].Port != "22" {
		t.Fatalf("aturan tersimpan harus tetap tampil saat ufw mati: %+v", out.Rules)
	}
	if !out.Status.Editable {
		t.Fatal("ufw yang mati tetap harus bisa diatur (tambah/hapus aturan)")
	}
	if !out.Status.SSHAllowed {
		t.Fatal("aturan port 22 harus terhitung sebagai SSH sudah diizinkan")
	}
}

// --- firewalld: service dan zone ---

// Di RHEL/CentOS akses lazimnya dibuka lewat NAMA SERVICE, bukan nomor port.
// Tanpa membaca --list-services, panel akan tampak kosong padahal SSH/HTTP
// sudah diizinkan — dan penjaga SSH akan salah melaporkan "belum diizinkan".
const firewalldServiceSample = `PORT=8080/tcp
SVC=ssh|22/tcp
SVC=http|80/tcp
SVC=dhcpv6-client|546/udp
RICH=rule family="ipv4" source address="10.0.0.0/8" port port="3306" protocol="tcp" accept
`

func TestParseFirewalldReadsServices(t *testing.T) {
	rules := parseFirewalld(firewalldServiceSample)
	if len(rules) != 5 {
		t.Fatalf("harus 5 aturan (1 port + 3 service + 1 rich), dapat %d: %+v", len(rules), rules)
	}

	var ssh *Rule
	for i := range rules {
		if rules[i].Service == "ssh" {
			ssh = &rules[i]
		}
	}
	if ssh == nil {
		t.Fatalf("service ssh tidak terbaca: %+v", rules)
	}
	if ssh.ServicePorts != "22/tcp" || ssh.Action != "allow" {
		t.Fatalf("service ssh salah baca: %+v", ssh)
	}
	if ssh.ID != "service:ssh" {
		t.Fatalf("ID service harus bisa dipakai menghapus: %q", ssh.ID)
	}
}

// Inilah bug yang membuat panel memunculkan peringatan terkunci secara keliru
// di RHEL: SSH dibuka lewat service, bukan port.
func TestSSHAllowedCountsFirewalldServices(t *testing.T) {
	rules := parseFirewalld(firewalldServiceSample)
	if !sshAllowed(rules, 22) {
		t.Fatal("service ssh (22/tcp) harus terhitung mengizinkan port 22")
	}
	// Port SSH tidak baku tidak boleh ikut dianggap terbuka oleh service ssh.
	if sshAllowed(rules, 2222) {
		t.Fatal("service ssh tidak boleh dianggap membuka port 2222")
	}
}

func TestParseListReadsZones(t *testing.T) {
	out := parseList("INSTALLED=firewalld\nBACKEND=firewalld\nZONE=public\nZONES=block dmz drop external home internal public trusted work\n__RULES__\nSVC=ssh|22/tcp\n", 22)
	if out.Status.Zone != "public" {
		t.Fatalf("zone aktif salah: %q", out.Status.Zone)
	}
	if len(out.Status.Zones) != 9 {
		t.Fatalf("daftar zone salah: %+v", out.Status.Zones)
	}
}

// Perintah harus menyertakan --zone saat zone dipilih, dan TIDAK menyertakannya
// saat memakai zone default (supaya firewalld memakai default servernya).
func TestZoneIsThreadedIntoCommands(t *testing.T) {
	withZone := firewalldAddScript("80", "tcp", "", "allow", "internal")
	if !strings.Contains(withZone, "--zone=internal --add-port=80/tcp") {
		t.Fatalf("zone tidak masuk ke perintah tambah:\n%s", withZone)
	}
	def := firewalldAddScript("80", "tcp", "", "allow", "")
	if strings.Contains(def, "--zone=") {
		t.Fatalf("zone default tidak boleh memaksa --zone:\n%s", def)
	}

	del, err := parseFirewalldRuleID("service:ssh", "dmz")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(del, "--zone=dmz --remove-service=ssh") {
		t.Fatalf("hapus service tidak memakai zone:\n%s", del)
	}
}

func TestZoneAndServiceValidation(t *testing.T) {
	for _, bad := range []string{"public; reboot", "$(id)", strings.Repeat("a", 40)} {
		if _, err := normalizeZone(bad); err == nil {
			t.Fatalf("zone %q seharusnya ditolak", bad)
		}
	}
	if z, err := normalizeZone(""); err != nil || z != "" {
		t.Fatalf("zone kosong harus diterima sebagai default: %q %v", z, err)
	}
	for _, bad := range []string{"ssh; reboot", "$(id)", ""} {
		if _, err := normalizeServiceName(bad); err == nil {
			t.Fatalf("service %q seharusnya ditolak", bad)
		}
	}
}

// --- Deteksi subnet Docker + jalan pintas "Izinkan Docker ke database" ---

// Subnet DIDETEKSI, bukan diasumsikan. Contoh ini meniru server nyata: delapan
// network, dua di antaranya tanpa IPAM (host/none), satu memakai rentang di
// LUAR 172.16/12 — persis kasus yang membuat rentang hardcoded gagal diam-diam.
const dockerNetSample = `DOCKERNET=bridge|172.17.0.0/16|172.17.0.1
DOCKERNET=devpoin_default|172.22.0.0/16|172.22.0.1
DOCKERNET=deploy_default|172.18.0.0/16|172.18.0.1
DOCKERNET=proyek_lama|10.55.0.0/16|10.55.0.1
DOCKERNET=ipv6net|fd00::/64|fd00::1
DOCKERNET=bridge|172.17.0.0/16|172.17.0.1
`

func TestParseDockerSubnets(t *testing.T) {
	got := parseDockerSubnets(dockerNetSample)
	if len(got) != 4 {
		t.Fatalf("harus 4 subnet IPv4 unik, dapat %d: %+v", len(got), got)
	}
	// Rentang di luar 172.16/12 WAJIB ikut terdeteksi — inilah alasan
	// deteksi ini ada.
	var adaNonDefault bool
	for _, s := range got {
		if s.Subnet == "10.55.0.0/16" {
			adaNonDefault = true
		}
		if s.Subnet == "fd00::/64" {
			t.Fatal("subnet IPv6 tidak boleh ikut: aturan yang dipasang berbentuk IPv4")
		}
	}
	if !adaNonDefault {
		t.Fatalf("subnet di luar rentang default Docker harus terdeteksi: %+v", got)
	}
	if got[0].Gateway != "172.17.0.1" {
		t.Fatalf("gateway salah baca: %+v", got[0])
	}
}

// Containment harus PENUH, bukan sekadar beririsan. Aturan sempit yang
// dianggap mencakup subnet luas akan membuat UI melaporkan "aman" padahal
// sebagian container tetap terblokir.
func TestCidrCoversButuhCakupanPenuh(t *testing.T) {
	if !cidrCovers("172.16.0.0/12", "172.22.0.0/16") {
		t.Fatal("rentang luas harus mencakup subnet di dalamnya")
	}
	if !cidrCovers("172.17.0.0/16", "172.17.0.0/16") {
		t.Fatal("subnet yang sama persis harus tercakup")
	}
	if cidrCovers("172.17.0.0/16", "172.16.0.0/12") {
		t.Fatal("aturan sempit TIDAK boleh dianggap mencakup rentang yang lebih luas")
	}
	if cidrCovers("10.0.0.0/8", "172.17.0.0/16") {
		t.Fatal("rentang yang tidak beririsan tidak boleh tercakup")
	}
	if cidrCovers("", "172.17.0.0/16") {
		t.Fatal("aturan tanpa sumber (publik) bukan izin khusus Docker")
	}
}

func TestMarkCoveragePerSubnet(t *testing.T) {
	subnets := []DockerSubnet{
		{Network: "bridge", Subnet: "172.17.0.0/16"},
		{Network: "lama", Subnet: "10.55.0.0/16"},
	}
	// Hanya rentang Docker default yang diizinkan — subnet 10.55 tertinggal.
	rules := []Rule{
		{Port: "3306", Protocol: "tcp", Source: "172.16.0.0/12", Action: "allow"},
		{Port: "5432", Protocol: "tcp", Source: "172.16.0.0/12", Action: "allow"},
	}
	if markCoverage(subnets, rules) {
		t.Fatal("masih ada subnet yang belum tercakup, tidak boleh dilaporkan selesai")
	}
	if !subnets[0].Covered {
		t.Fatal("subnet 172.17 seharusnya tercakup")
	}
	if subnets[1].Covered {
		t.Fatal("subnet 10.55 TIDAK tercakup dan harus ditandai begitu")
	}

	// Lengkapi yang tertinggal.
	rules = append(rules,
		Rule{Port: "3306", Protocol: "tcp", Source: "10.55.0.0/16", Action: "allow"},
		Rule{Port: "5432", Protocol: "tcp", Source: "10.55.0.0/16", Action: "allow"})
	if !markCoverage(subnets, rules) {
		t.Fatal("seluruh subnet sudah tercakup, seharusnya selesai")
	}

	// Satu port saja tidak cukup.
	hanyaMySQL := []Rule{{Port: "3306", Protocol: "tcp", Source: "172.16.0.0/12", Action: "allow"}}
	if markCoverage([]DockerSubnet{{Subnet: "172.17.0.0/16"}}, hanyaMySQL) {
		t.Fatal("baru MySQL yang diizinkan, PostgreSQL belum")
	}

	// Tanpa Docker sama sekali bukan berarti "sudah terpasang".
	if markCoverage(nil, rules) {
		t.Fatal("tanpa network Docker tidak boleh dilaporkan sudah terpasang")
	}
}

// Skrip hanya memasang aturan untuk yang BELUM tercakup, dan memakai marker
// bersama dengan modul Database.
func TestDockerDBScriptHanyaYangKurang(t *testing.T) {
	ufw := ufwDockerDBScript([]string{"172.22.0.0/16|3306", "10.55.0.0/16|5432"})
	for _, want := range []string{
		"ufw allow from 172.22.0.0/16 to any port 3306 proto tcp comment 'poinhost-docker-access'",
		"ufw allow from 10.55.0.0/16 to any port 5432 proto tcp comment 'poinhost-docker-access'",
	} {
		if !strings.Contains(ufw, want) {
			t.Fatalf("skrip ufw kurang %q:\n%s", want, ufw)
		}
	}
	if strings.Contains(ufw, "172.16.0.0/12") {
		t.Fatalf("tidak boleh memasang rentang yang diasumsikan:\n%s", ufw)
	}

	fwd := firewalldDockerDBScript([]string{"172.22.0.0/16|3306"}, "internal")
	if !strings.Contains(fwd, "--zone=internal") {
		t.Fatalf("zone tidak diteruskan:\n%s", fwd)
	}
	if !strings.Contains(fwd, `source address="172.22.0.0/16"`) {
		t.Fatalf("subnet hasil deteksi tidak dipakai:\n%s", fwd)
	}
	if !strings.HasSuffix(strings.TrimSpace(fwd), "firewall-cmd --reload") {
		t.Fatalf("firewalld wajib reload di akhir:\n%s", fwd)
	}
}

// Status yang dibaca UI: subnet hasil deteksi dan cakupannya, dalam satu
// round-trip yang sama dengan pembacaan aturan.
func TestParseListMenyertakanSubnetDocker(t *testing.T) {
	stdout := `DOCKERNET=bridge|172.17.0.0/16|172.17.0.1
DOCKERNET=devpoin_default|172.22.0.0/16|172.22.0.1
INSTALLED=ufw
BACKEND=ufw
DEFAULT=Default: deny (incoming), allow (outgoing), deny (routed)
__RULES__
[ 1] 22/tcp                     ALLOW IN    Anywhere
[ 2] 3306/tcp                   ALLOW IN    172.16.0.0/12              # poinhost-docker-access
[ 3] 5432/tcp                   ALLOW IN    172.16.0.0/12              # poinhost-docker-access
`
	out := parseList(stdout, 22)
	if len(out.Status.DockerSubnets) != 2 {
		t.Fatalf("subnet Docker harus ikut terbaca: %+v", out.Status.DockerSubnets)
	}
	if !out.Status.DockerDBAllowed {
		t.Fatalf("kedua subnet tercakup 172.16/12, seharusnya selesai: %+v", out.Status)
	}

	// Subnet di luar rentang default membuat statusnya belum selesai.
	stdout2 := strings.Replace(stdout, "DOCKERNET=devpoin_default|172.22.0.0/16|172.22.0.1",
		"DOCKERNET=proyek_lama|10.55.0.0/16|10.55.0.1", 1)
	out2 := parseList(stdout2, 22)
	if out2.Status.DockerDBAllowed {
		t.Fatal("subnet 10.55 belum diizinkan, tidak boleh dilaporkan selesai")
	}
}
