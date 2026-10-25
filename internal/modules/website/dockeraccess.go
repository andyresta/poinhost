package website

import (
	"net"
	"strconv"
	"strings"
	"time"
)

// Docker di host yang sama mengarahkan host.docker.internal ke IP host,
// tapi itu saja TIDAK CUKUP: MySQL/PostgreSQL versi paket distro bawaan
// SELALU dipasang cuma mendengarkan di 127.0.0.1 (loopback) — koneksi dari
// container (datang lewat interface bridge Docker, BUKAN loopback) ditolak
// di level TCP sebelum sempat autentikasi sama sekali, apa pun grant user/
// role-nya. Ini alasan sebenarnya kenapa homepoin butuh setting manual
// meski host.docker.internal sudah benar di sisi Docker — bukan soal
// Docker-nya, tapi bind-address/listen_addresses database itu sendiri.
//
// dockerBridgeCIDR mencakup rentang alamat bridge network Docker BAWAAN:
// docker0 (172.17.0.0/16) maupun jaringan bridge kustom yang dibuat
// `docker network create`/docker-compose (dialokasikan Docker secara
// default dari pool 172.18.0.0/16 s.d. 172.31.0.0/16). Dipakai untuk
// membatasi akses HANYA dari jaringan Docker lokal di mesin yang sama —
// beda mendasar dari "remote access" ala homepoin (pgmanager) yang berarti
// dibuka untuk klien EKSTERNAL/internet.
const dockerBridgeCIDR = "172.16.0.0/12"

// dockerAccessMarker menandai baris/aturan yang poinhost tulis sendiri, di
// config file maupun rule firewall — supaya idempotent (tidak dobel kalau
// dijalankan berkali-kali) dan gampang dicari/dihapus manual kalau perlu.
const dockerAccessMarker = "poinhost-docker-access"

// DBDockerAccessStatus status akses Docker->database saat ini.
type DBDockerAccessStatus struct {
	Engine string `json:"engine"`
	// BindAllInterfaces: bind-address 0.0.0.0 (MySQL) atau listen_addresses
	// '*' (PostgreSQL) — kalau false, container Docker TIDAK akan bisa
	// connect sama sekali (ditolak di level TCP).
	BindAllInterfaces bool `json:"bindAllInterfaces"`
	// FirewallDetected: "ufw" | "firewalld" | "none" — firewall AKTIF yang
	// terdeteksi di server ini (poinhost tidak memasang firewall baru,
	// cuma menambah aturan ke yang sudah berjalan).
	FirewallDetected string `json:"firewallDetected,omitempty"`
	// FirewallRuleActive: aturan allow poinhost (scope dockerBridgeCIDR)
	// sudah terpasang di firewall yang terdeteksi.
	FirewallRuleActive bool `json:"firewallRuleActive"`
	// Enabled: KESIMPULAN akhir "container Docker benar-benar bisa connect".
	// Sengaja dihitung di backend, bukan dirakit ulang di UI: dulu UI memakai
	// syarat `bindAllInterfaces && firewallRuleActive`, sehingga di server
	// TANPA firewall (FirewallDetected "none", FirewallRuleActive selalu
	// false karena tidak ada tempat memasang aturan) statusnya bilang "sudah
	// bisa connect" TAPI tombol "Aktifkan" tetap muncul — persis
	// keambiguan yang dilaporkan. Kalau tidak ada firewall, ya tidak ada
	// yang perlu dibuka.
	Enabled bool   `json:"enabled"`
	Message string `json:"message,omitempty"`
	// LocalOnlyUsers daftar user MySQL yang HANYA punya host 'localhost' /
	// '127.0.0.1'. Ini celah yang paling sering menipu: bind-address sudah
	// 0.0.0.0 dan firewall sudah terbuka, tapi MySQL tetap menolak koneksi
	// dari container karena akunnya tidak punya host yang cocok. Gejalanya
	// "Host 'x.x.x.x' is not allowed", bukan timeout — jadi mudah dikira
	// masalah password.
	//
	// SENGAJA hanya dilaporkan, TIDAK diubah otomatis: menambah host '%' pada
	// sebuah akun adalah keputusan keamanan milik user, bukan efek samping
	// dari menekan tombol.
	LocalOnlyUsers []string `json:"localOnlyUsers,omitempty"`
	// DockerSubnets sumber yang dipakai fitur ini: pool bawaan Docker plus
	// subnet network Docker yang terdeteksi di luar pool itu.
	DockerSubnets []string `json:"dockerSubnets,omitempty"`
	// HbaMissing (PostgreSQL) subnet Docker yang belum punya baris pg_hba —
	// container dari sana ditolak "no pg_hba.conf entry" walau listener dan
	// firewall sudah terbuka. Biasanya network yang dibuat SESUDAH fitur ini
	// diaktifkan; menekan "Aktifkan" lagi menyinkronkannya.
	HbaMissing []string `json:"hbaMissing,omitempty"`
}

func (st *DBDockerAccessStatus) computeEnabled() {
	firewallBlocks := st.FirewallDetected != "" && st.FirewallDetected != "none" && !st.FirewallRuleActive
	st.Enabled = st.BindAllInterfaces && !firewallBlocks && len(st.HbaMissing) == 0

	if st.BindAllInterfaces && !firewallBlocks && len(st.HbaMissing) > 0 {
		st.Message = strings.TrimSpace(st.Message + " Listener dan firewall sudah terbuka, TAPI pg_hba.conf belum mengizinkan network Docker berikut sehingga container ditolak (\"no pg_hba.conf entry\"): " +
			strings.Join(st.HbaMissing, ", ") + ". Tekan \"Aktifkan\" untuk menyinkronkan.")
	}

	// Jalur TCP yang terbuka belum berarti container BISA masuk: MySQL masih
	// akan menolak akun yang hostnya cuma 'localhost'. Ini dilaporkan sebagai
	// peringatan, bukan diam-diam menurunkan Enabled — jalur jaringannya
	// memang sudah benar, yang kurang ada di sisi akun.
	if st.Enabled && len(st.LocalOnlyUsers) > 0 {
		st.Message = strings.TrimSpace(st.Message + " Jalur jaringan sudah terbuka, TAPI akun MySQL berikut hanya punya host localhost sehingga koneksi dari container tetap akan ditolak (\"Host ... is not allowed\"): " +
			strings.Join(st.LocalOnlyUsers, ", ") + ". Tambahkan host '%' atau host jaringan Docker pada akun itu kalau memang dipakai dari container.")
	}
}

func mysqlDockerConfPath(pm string) string {
	if pm == "apt" {
		return "/etc/mysql/mariadb.conf.d/99-poinhost-docker.cnf"
	}
	return "/etc/my.cnf.d/99-poinhost-docker.cnf"
}

// dockerSubnetsScript mengisi $SUBNETS dengan sumber yang diizinkan:
// dockerBridgeCIDR (pool bawaan Docker, sehingga network baru yang dibuat
// NANTI dari pool itu otomatis ikut) PLUS setiap subnet IPv4 network Docker
// yang TERDETEKSI di luar rentang itu.
//
// Dulu hanya dockerBridgeCIDR yang dipatok. Network dengan subnet kustom
// (`default-address-pools` di daemon.json, atau compose dengan `ipam` sendiri
// seperti 10.x / 192.168.x) lalu tidak pernah bisa connect: firewall
// memblokirnya, dan untuk PostgreSQL pg_hba juga menolaknya — dan karena
// status firewall SUDAH dihitung dari subnet terdeteksi, tombol "Aktifkan"
// tampil terus tanpa pernah bisa memperbaikinya.
const dockerSubnetsScript = `SUBNETS="` + dockerBridgeCIDR + `"
if command -v docker >/dev/null 2>&1; then
  for S in $(docker network ls -q 2>/dev/null | xargs -r docker network inspect -f '{{range .IPAM.Config}}{{.Subnet}} {{end}}' 2>/dev/null); do
    echo "$S" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+$' || continue
    A=${S%%.*}; R=${S#*.}; B=${R%%.*}
    if [ "$A" = 172 ] && [ "$B" -ge 16 ] && [ "$B" -le 31 ]; then continue; fi
    case " $SUBNETS " in *" $S "*) ;; *) SUBNETS="$SUBNETS $S" ;; esac
  done
fi
`

// firewallEnsureScript menambah aturan allow SEMPIT (hanya dari $SUBNETS,
// BUKAN 0.0.0.0/0) ke firewall yang TERDETEKSI SEDANG AKTIF — tidak pernah
// memasang/mengaktifkan firewall baru (itu keputusan besar tersendiri, di
// luar cakupan fitur ini). Butuh dockerSubnetsScript dijalankan lebih dulu.
func firewallEnsureScript(port int) string {
	p := strconv.Itoa(port)
	return `if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi "^Status: active"; then
  for S in $SUBNETS; do
    ufw allow from "$S" to any port ` + p + ` proto tcp comment '` + dockerAccessMarker + `' >/dev/null 2>&1 || true
  done
  echo "FIREWALL=ufw"
elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state 2>/dev/null | grep -qi running; then
  for S in $SUBNETS; do
    firewall-cmd --permanent --add-rich-rule="rule family=\"ipv4\" source address=\"$S\" port port=\"` + p + `\" protocol=\"tcp\" accept" >/dev/null 2>&1 || true
  done
  firewall-cmd --reload >/dev/null 2>&1 || true
  echo "FIREWALL=firewalld"
else
  echo "FIREWALL=none"
fi`
}

// pgHbaStripDockerFn membuang SEMUA baris pg_hba milik akses Docker — format
// lama (komentar marker di baris sendiri + satu baris host sesudahnya, plus
// baris kosong yang dulu ikut ditulis di depannya) maupun format baru (baris
// host berkomentar marker di ujungnya). Ditulis balik lewat `cat >` supaya
// pemilik & izin file pg_hba tidak berubah.
const pgHbaStripDockerFn = `strip_docker_hba() {
  awk -v m="# ` + dockerAccessMarker + `" '
    { l[NR] = $0 }
    END {
      for (i = 1; i <= NR; i++) {
        if (index(l[i], m) == 1) { s[i] = 1; s[i + 1] = 1; if (i > 1 && l[i - 1] == "") s[i - 1] = 1 }
        else if (index(l[i], "` + dockerAccessMarker + `") > 0) s[i] = 1
      }
      for (i = 1; i <= NR; i++) if (!(i in s)) print l[i]
    }' "$HBA" > "$HBA.poinhost.tmp" && cat "$HBA.poinhost.tmp" > "$HBA" && rm -f "$HBA.poinhost.tmp"
}
`

// dbDockerAccessApplyScript membuat MySQL/PostgreSQL bisa diakses dari
// container Docker DI HOST YANG SAMA — dipanggil otomatis di akhir
// instalasi (lihat dbInstallScript) DAN tersedia manual lewat
// EnsureDBDockerAccess. Ketiga lapisan diurus sekaligus: listener, firewall,
// dan (PostgreSQL) pg_hba — sehingga ini satu-satunya tombol yang perlu
// ditekan. Menjalankannya ulang = SINKRONISASI: baris pg_hba milik fitur ini
// ditulis ulang dari subnet yang terdeteksi saat itu, jadi network Docker
// yang dibuat belakangan ikut tercakup.
//
// PostgreSQL punya lapisan proteksi KEDUA yang independen dari firewall
// OS: pg_hba.conf menolak sumber IP yang tidak cocok di level protokolnya
// sendiri (sebelum autentikasi), jadi tetap aman walau firewall OS tidak
// aktif. MySQL TIDAK punya mekanisme setara (host user MySQL pakai pola
// LIKE seperti '%', bukan CIDR asli) — makanya untuk MySQL, firewall OS
// yang aktif benar-benar jadi lapisan proteksi utama. Kalau tidak
// terdeteksi aktif, bind-address TETAP diubah (sesuai yang diminta), tapi
// hasilnya (lihat EnsureDBDockerAccess) membawa pesan peringatan eksplisit
// — bukan diam-diam meninggalkan port terbuka tanpa pemberitahuan.
func dbDockerAccessApplyScript(engine, pm string) string {
	if engine == "mysql" {
		path := mysqlDockerConfPath(pm)
		return `set -e
` + dockerSubnetsScript + `mkdir -p "$(dirname ` + path + `)"
cat > ` + path + ` <<'POINHOSTEOF'
# ` + dockerAccessMarker + ` — supaya container Docker di host yang sama bisa
# connect ke MySQL/MariaDB ini (mis. lewat host.docker.internal). Jangan
# diedit manual — file ini ditimpa ulang tiap kali fitur ini dijalankan.
[mysqld]
bind-address = 0.0.0.0
POINHOSTEOF
BIND=$(` + mysqlBindProbe + `)
case "$BIND" in
  0.0.0.0|'*'|::) ;;
  *) systemctl restart mariadb 2>/dev/null || systemctl restart mysql 2>/dev/null || true ;;
esac
` + firewallEnsureScript(3306) + `
echo "POINHOST_DOCKER_ACCESS_DONE"
`
	}
	return `set -e
` + pgRestartFn + pgHbaStripDockerFn + dockerSubnetsScript + `HBA=$(sudo -u postgres psql -tAc "SHOW hba_file;" | tr -d '[:space:]')
# Metode auth MENGIKUTI password_encryption server, bukan "md5" yang dipatok
# mati. PostgreSQL 14+ menyimpan password sebagai scram-sha-256, dan baris
# pg_hba bertuliskan md5 akan menolak login walau passwordnya benar —
# kegagalan yang terbaca seperti "password salah" padahal konfigurasinya yang
# keliru, jadi sangat memakan waktu untuk dilacak.
AUTH=$(sudo -u postgres psql -tAc "SHOW password_encryption;" 2>/dev/null | tr -d '[:space:]')
case "$AUTH" in
  scram-sha-256|md5) ;;
  *) AUTH=scram-sha-256 ;;
esac
if [ -n "$HBA" ] && [ -f "$HBA" ]; then
  strip_docker_hba
  [ -s "$HBA" ] && [ -n "$(tail -c1 "$HBA")" ] && echo >> "$HBA"
  for S in $SUBNETS; do
    printf 'host    all    all    %s    %s    # ` + dockerAccessMarker + `\n' "$S" "$AUTH" >> "$HBA"
  done
fi
LISTEN=$(sudo -u postgres psql -tAc "SHOW listen_addresses;" | tr -d '[:space:]')
case "$LISTEN" in
  *'*'*|*0.0.0.0*) sudo -u postgres psql -tAc "SELECT pg_reload_conf();" >/dev/null ;;
  *)
    sudo -u postgres psql -c "ALTER SYSTEM SET listen_addresses = '*';" >/dev/null
    pg_restart ;;
esac
` + firewallEnsureScript(5432) + `
echo "POINHOST_DOCKER_ACCESS_DONE"
`
}

// firewallRemoveScript mencabut aturan allow dari $SUBNETS untuk port ini —
// kebalikan firewallEnsureScript (juga aturan yang dulu dipasang tombol
// Docker di panel Firewall, yang bentuknya sama). Aturan lain milik user
// tidak disentuh.
func firewallRemoveScript(port int) string {
	p := strconv.Itoa(port)
	return `if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi "^Status: active"; then
  for S in $SUBNETS; do
    ufw --force delete allow from "$S" to any port ` + p + ` proto tcp >/dev/null 2>&1 || true
  done
elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state 2>/dev/null | grep -qi running; then
  for S in $SUBNETS; do
    firewall-cmd --permanent --remove-rich-rule="rule family=\"ipv4\" source address=\"$S\" port port=\"` + p + `\" protocol=\"tcp\" accept" >/dev/null 2>&1 || true
  done
  firewall-cmd --reload >/dev/null 2>&1 || true
fi`
}

// dbDockerAccessRevertScript mengembalikan database ke keadaan HANYA bisa
// diakses dari host itu sendiri (localhost) — kebalikan persis dari
// dbDockerAccessApplyScript, dan hanya membongkar yang poinhost pasang
// sendiri (file config bermarker, baris pg_hba bermarker, aturan firewall
// bermarker). Konfigurasi lain milik user tidak ikut diubah.
func dbDockerAccessRevertScript(engine, pm string) string {
	if engine == "mysql" {
		path := mysqlDockerConfPath(pm)
		return `set -e
` + dockerSubnetsScript + `rm -f ` + path + `
systemctl restart mariadb 2>/dev/null || systemctl restart mysql 2>/dev/null || true
` + firewallRemoveScript(3306) + `
echo "POINHOST_DOCKER_ACCESS_REVERTED"
`
	}
	// listen_addresses hanya dikembalikan ke default kalau tidak ada aturan
	// remote access (remoteaccess.go) yang masih membutuhkannya — mematikan
	// akses Docker tidak boleh ikut memutus klien remote.
	return `set -e
` + pgRestartFn + pgHbaStripDockerFn + dockerSubnetsScript + `HBA=$(sudo -u postgres psql -tAc "SHOW hba_file;" | tr -d '[:space:]')
if [ -n "$HBA" ] && [ -f "$HBA" ]; then
  strip_docker_hba
fi
if [ -z "$HBA" ] || ! grep -q "^# ` + remoteAccessMarker + ` id=" "$HBA" 2>/dev/null; then
  sudo -u postgres psql -c "ALTER SYSTEM RESET listen_addresses;" >/dev/null
  pg_restart
else
  sudo -u postgres psql -tAc "SELECT pg_reload_conf();" >/dev/null
fi
` + firewallRemoveScript(5432) + `
echo "POINHOST_DOCKER_ACCESS_REVERTED"
`
}

func dbDockerAccessStatusScript(engine string) string {
	if engine == "mysql" {
		// Lewat unix socket lokal (`mysql -u root`, TANPA `-h`) — bukan
		// TCP ke 127.0.0.1. Lihat komentar panjang di runMySQL (database.go):
		// TCP ke 127.0.0.1 gagal konsisten (MariaDB mencocokkan koneksi loopback
		// itu ke akun `root@localhost`, bukan `root@127.0.0.1`, meski password
		// benar), dan bind_address sendiri cuma nilai variabel server — bisa
		// dibaca lewat KONEKSI APA PUN yang berhasil, tidak perlu tes lewat TCP
		// sungguhan untuk tahu nilainya.
		return `BIND=$(mysql -u root --batch --skip-column-names -e "SHOW VARIABLES LIKE 'bind_address';" 2>/dev/null | awk '{print $2}')
echo "BIND=$BIND"
mysql -u root --batch --skip-column-names -e "SELECT user FROM mysql.user GROUP BY user HAVING SUM(host NOT IN ('localhost','127.0.0.1','::1')) = 0;" 2>/dev/null | sed 's/^/LOCALUSER=/'
`
	}
	// pg_hba ikut dibaca: listen_addresses '*' saja tidak berarti container
	// bisa masuk — apalagi sejak remote access (remoteaccess.go) juga bisa
	// membuat listener '*' tanpa satu pun baris pg_hba untuk Docker.
	return dockerSubnetsScript + `LISTEN=$(sudo -u postgres psql -tAc "SHOW listen_addresses;" 2>/dev/null | tr -d '[:space:]')
echo "BIND=$LISTEN"
for S in $SUBNETS; do echo "DOCKERNET=$S"; done
HBA=$(sudo -u postgres psql -tAc "SHOW hba_file;" 2>/dev/null | tr -d '[:space:]')
if [ -n "$HBA" ] && [ -f "$HBA" ]; then
  awk '
    /^# ` + dockerAccessMarker + `/ { legacy = 1; next }
    legacy && $1 == "host" { print "HBADOCKER=" $4; legacy = 0; next }
    { legacy = 0 }
    $1 == "host" && /` + dockerAccessMarker + `/ { print "HBADOCKER=" $4 }
  ' "$HBA"
fi
exit 0
`
}

// cidrContains true kalau jaringan outer mencakup SELURUH inner.
func cidrContains(outer, inner string) bool {
	_, o, err := net.ParseCIDR(outer)
	if err != nil {
		return false
	}
	_, in, err := net.ParseCIDR(inner)
	if err != nil {
		return false
	}
	oOnes, oBits := o.Mask.Size()
	iOnes, iBits := in.Mask.Size()
	return oBits == iBits && oOnes <= iOnes && o.Contains(in.IP)
}

// hbaMissingSubnets subnet Docker yang belum tercakup baris pg_hba mana pun.
func hbaMissingSubnets(subnets, hba []string) []string {
	var missing []string
	for _, sn := range subnets {
		covered := false
		for _, h := range hba {
			if cidrContains(h, sn) {
				covered = true
				break
			}
		}
		if !covered {
			missing = append(missing, sn)
		}
	}
	return missing
}

// parseDockerAccessStatus dipisah supaya bisa diuji tanpa SSH.
func parseDockerAccessStatus(engine, stdout string) *DBDockerAccessStatus {
	st := &DBDockerAccessStatus{Engine: engine}
	var hba []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "BIND="):
			v := strings.TrimPrefix(line, "BIND=")
			st.BindAllInterfaces = v == "0.0.0.0" || strings.Contains(v, "*")
		case strings.HasPrefix(line, "FIREWALL="):
			st.FirewallDetected = strings.TrimPrefix(line, "FIREWALL=")
		case strings.HasPrefix(line, "FWRULE="):
			st.FirewallRuleActive = strings.TrimPrefix(line, "FWRULE=") == "1"
		case strings.HasPrefix(line, "LOCALUSER="):
			if u := strings.TrimSpace(strings.TrimPrefix(line, "LOCALUSER=")); u != "" && !mysqlSystemAccount(u) {
				st.LocalOnlyUsers = append(st.LocalOnlyUsers, u)
			}
		case strings.HasPrefix(line, "DOCKERNET="):
			st.DockerSubnets = append(st.DockerSubnets, strings.TrimPrefix(line, "DOCKERNET="))
		case strings.HasPrefix(line, "HBADOCKER="):
			hba = append(hba, strings.TrimPrefix(line, "HBADOCKER="))
		}
	}
	if engine != "mysql" {
		st.HbaMissing = hbaMissingSubnets(st.DockerSubnets, hba)
	}
	return st
}

// GetDBDockerAccessStatus membaca status akses Docker->database saat ini —
// TANPA mengubah apa pun di server.
func (s *Service) GetDBDockerAccessStatus(serverID, engine string) (*DBDockerAccessStatus, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	res, err := s.run(access, dbDockerAccessStatusScript(engine), 15*time.Second)
	if err != nil {
		return nil, err
	}

	st := parseDockerAccessStatus(engine, res.Stdout)
	// Status firewall DITANYAKAN ke modul firewall, bukan ditebak dari
	// komentar aturan. Versi lama mencari marker "poinhost-docker-access" di
	// keluaran `ufw status`, sehingga aturan sah yang ditulis lewat jalur lain
	// (panel Firewall, atau manual oleh user) tidak dikenali — panel ini lalu
	// melaporkan "terblokir" padahal container benar-benar bisa connect.
	// Sekarang yang dinilai adalah EFEKNYA: apakah seluruh subnet Docker yang
	// terdeteksi tercakup aturan allow untuk port ini.
	port := 3306
	if engine != "mysql" {
		port = 5432
	}
	if s.firewall != nil {
		allowed, backend, ferr := s.firewall.PortAllowedFromDocker(serverID, port)
		if ferr == nil {
			st.FirewallDetected = backend
			st.FirewallRuleActive = allowed
		}
		// Kalau pengecekan firewall gagal, status firewall dibiarkan kosong
		// daripada diisi tebakan — computeEnabled memperlakukan "tidak ada
		// firewall terdeteksi" sebagai tidak ada yang memblokir.
	}

	st.computeEnabled()
	return st, nil
}

// EnsureDBDockerAccess mengaktifkan akses Docker->database untuk instalasi
// yang SUDAH ADA sebelumnya (instalasi baru lewat wizard sudah otomatis,
// lihat dbInstallScript). Aman dijalankan berulang.
func (s *Service) EnsureDBDockerAccess(serverID, engine string) (*DBDockerAccessStatus, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	distro, err := s.detectDistro(access)
	if err != nil {
		return nil, err
	}

	s.mutex.Lock(serverID)
	_, err = s.run(access, dbDockerAccessApplyScript(engine, distro.PackageManager), 30*time.Second)
	s.mutex.Unlock(serverID)
	if err != nil {
		return nil, err
	}

	st, err := s.GetDBDockerAccessStatus(serverID, engine)
	if err != nil {
		return nil, err
	}
	if engine == "mysql" && st.FirewallDetected == "none" {
		st.Message = "Bind-address sudah diubah ke 0.0.0.0, TAPI tidak ada firewall (ufw/firewalld) aktif yang terdeteksi di server ini — artinya port MySQL kini bisa dijangkau siapa pun yang punya akses jaringan ke server ini, bukan cuma Docker. Aktifkan ufw (atau firewalld), lalu jalankan ini lagi supaya aksesnya benar-benar dibatasi ke jaringan Docker saja."
	}
	return st, nil
}

// DisableDBDockerAccess menutup kembali akses Docker->database: database
// balik hanya mendengarkan localhost, dan aturan firewall yang dipasang
// poinhost dicabut. Container Docker setelah ini TIDAK bisa connect lagi.
func (s *Service) DisableDBDockerAccess(serverID, engine string) (*DBDockerAccessStatus, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	distro, err := s.detectDistro(access)
	if err != nil {
		return nil, err
	}

	s.mutex.Lock(serverID)
	_, err = s.run(access, dbDockerAccessRevertScript(engine, distro.PackageManager), 30*time.Second)
	s.mutex.Unlock(serverID)
	if err != nil {
		return nil, err
	}
	return s.GetDBDockerAccessStatus(serverID, engine)
}
