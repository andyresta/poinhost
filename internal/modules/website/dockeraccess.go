package website

import (
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
	FirewallRuleActive bool   `json:"firewallRuleActive"`
	Message            string `json:"message,omitempty"`
}

func mysqlDockerConfPath(pm string) string {
	if pm == "apt" {
		return "/etc/mysql/mariadb.conf.d/99-poinhost-docker.cnf"
	}
	return "/etc/my.cnf.d/99-poinhost-docker.cnf"
}

// firewallEnsureScript menambah SATU aturan allow sempit (hanya dari
// dockerBridgeCIDR, BUKAN 0.0.0.0/0) ke firewall yang TERDETEKSI SEDANG
// AKTIF — tidak pernah memasang/mengaktifkan firewall baru (itu keputusan
// besar tersendiri, di luar cakupan fitur ini), cuma menambah satu aturan
// sempit ke firewall yang memang sudah user pasang & jalankan.
func firewallEnsureScript(port int) string {
	p := strconv.Itoa(port)
	return `if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi "^Status: active"; then
  ufw allow from ` + dockerBridgeCIDR + ` to any port ` + p + ` proto tcp comment '` + dockerAccessMarker + `' >/dev/null 2>&1 || true
  echo "FIREWALL=ufw"
elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state 2>/dev/null | grep -qi running; then
  firewall-cmd --permanent --add-rich-rule="rule family=\"ipv4\" source address=\"` + dockerBridgeCIDR + `\" port port=\"` + p + `\" protocol=\"tcp\" accept" >/dev/null 2>&1 || true
  firewall-cmd --reload >/dev/null 2>&1 || true
  echo "FIREWALL=firewalld"
else
  echo "FIREWALL=none"
fi`
}

// firewallCheckScript mengecek apakah aturan poinhost untuk PORT INI
// spesifik sudah terpasang — bukan cuma "ada marker poinhost di suatu
// tempat" (kalau MySQL dan PostgreSQL sama-sama pernah diaktifkan, masing-
// masing punya aturan sendiri di port berbeda; mengecek marker saja tanpa
// port akan salah melaporkan "aktif" untuk engine yang aturannya belum
// pernah dipasang, cuma karena punya engine LAIN sudah).
func firewallCheckScript(port int) string {
	p := strconv.Itoa(port)
	return `if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi "^Status: active"; then
  echo "FIREWALL=ufw"
  ufw status verbose 2>/dev/null | grep "` + p + `" | grep -q "` + dockerAccessMarker + `" && echo "FWRULE=1" || echo "FWRULE=0"
elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state 2>/dev/null | grep -qi running; then
  echo "FIREWALL=firewalld"
  firewall-cmd --list-rich-rules 2>/dev/null | grep "port=\"` + p + `\"" | grep -q "` + dockerBridgeCIDR + `" && echo "FWRULE=1" || echo "FWRULE=0"
else
  echo "FIREWALL=none"
  echo "FWRULE=0"
fi`
}

// dbDockerAccessApplyScript membuat MySQL/PostgreSQL bisa diakses dari
// container Docker DI HOST YANG SAMA — dipanggil otomatis di akhir
// instalasi (lihat dbInstallScript) DAN tersedia manual lewat
// EnsureDBDockerAccess untuk instalasi yang sudah ada sebelumnya. Aman
// dijalankan berulang (idempotent): menimpa file config sendiri, mengecek
// marker sebelum menambah baris pg_hba.conf.
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
mkdir -p "$(dirname ` + path + `)"
cat > ` + path + ` <<'POINHOSTEOF'
# ` + dockerAccessMarker + ` — supaya container Docker di host yang sama bisa
# connect ke MySQL/MariaDB ini (mis. lewat host.docker.internal). Jangan
# diedit manual — file ini ditimpa ulang tiap kali fitur ini dijalankan.
[mysqld]
bind-address = 0.0.0.0
POINHOSTEOF
systemctl restart mariadb 2>/dev/null || systemctl restart mysql 2>/dev/null || true
` + firewallEnsureScript(3306) + `
echo "POINHOST_DOCKER_ACCESS_DONE"
`
	}
	return `set -e
sudo -u postgres psql -c "ALTER SYSTEM SET listen_addresses = '*';" >/dev/null
HBA=$(sudo -u postgres psql -tAc "SHOW hba_file;" | tr -d '[:space:]')
if [ -n "$HBA" ] && ! grep -q "` + dockerAccessMarker + `" "$HBA" 2>/dev/null; then
  printf '\n# ` + dockerAccessMarker + ` — izinkan koneksi dari container Docker di host yang sama\nhost    all             all             ` + dockerBridgeCIDR + `            md5\n' >> "$HBA"
fi
systemctl restart postgresql 2>/dev/null || true
` + firewallEnsureScript(5432) + `
echo "POINHOST_DOCKER_ACCESS_DONE"
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
` + firewallCheckScript(3306)
	}
	return `LISTEN=$(sudo -u postgres psql -tAc "SHOW listen_addresses;" 2>/dev/null | tr -d '[:space:]')
echo "BIND=$LISTEN"
` + firewallCheckScript(5432)
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

	st := &DBDockerAccessStatus{Engine: engine}
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "BIND="):
			v := strings.TrimPrefix(line, "BIND=")
			st.BindAllInterfaces = v == "0.0.0.0" || strings.Contains(v, "*")
		case strings.HasPrefix(line, "FIREWALL="):
			st.FirewallDetected = strings.TrimPrefix(line, "FIREWALL=")
		case strings.HasPrefix(line, "FWRULE="):
			st.FirewallRuleActive = strings.TrimPrefix(line, "FWRULE=") == "1"
		}
	}
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
