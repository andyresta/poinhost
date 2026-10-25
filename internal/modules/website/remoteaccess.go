package website

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"regexp"
	"strings"
	"time"
)

// Remote access database: klien EKSTERNAL (Navicat, DBeaver, aplikasi di
// server lain) connect langsung ke MySQL/PostgreSQL di server ini.
//
// Kasus yang memicu fitur ini: user membuka port 5432 "dari mana saja" di
// firewall, lalu Navicat tetap ditolak dengan
//
//	FATAL: no pg_hba.conf entry for host "182.8.212.16", user "...", database "..."
//
// Firewall hanya satu dari tiga lapisan. Supaya klien remote benar-benar bisa
// masuk, ketiganya harus terbuka:
//
//  1. Listener: listen_addresses (PostgreSQL) / bind-address (MySQL) bukan
//     cuma loopback.
//  2. Firewall OS (ufw/firewalld) mengizinkan IP klien ke port database.
//  3. Kontrol akses milik database sendiri: pg_hba.conf untuk PostgreSQL,
//     host akun (`user@host`) untuk MySQL.
//
// Fitur ini mengurus ketiganya per SUMBER (IP/CIDR) yang dipilih user —
// bukan sekadar "buka ke internet". Untuk PostgreSQL satu aturan = satu baris
// pg_hba.conf bermarker. MySQL tidak punya pg_hba (host akun MySQL adalah
// pola LIKE, bukan CIDR, dan diatur per user di tab Akses), jadi aturan
// MySQL mengurus lapisan 1 dan 2; akun yang hostnya cuma localhost
// dilaporkan, tidak diubah diam-diam.
//
// Semua yang ditulis bermarker remoteAccessMarker, terpisah dari marker
// Docker access, sehingga mencabut salah satunya tidak merusak yang lain.
const remoteAccessMarker = "poinhost-remote-access"

// DBRemoteRule satu sumber yang diizinkan connect dari luar.
type DBRemoteRule struct {
	ID string `json:"id"`
	// Source CIDR kanonik, mis. "182.8.212.16/32" atau "10.0.0.0/8".
	Source string `json:"source"`
	// Database & Username hanya PostgreSQL (kolom pg_hba). "all" = semua.
	Database string `json:"database,omitempty"`
	Username string `json:"username,omitempty"`
	// Auth metode pg_hba (scram-sha-256 / md5). Hanya PostgreSQL.
	Auth string `json:"auth,omitempty"`
	Note string `json:"note,omitempty"`
	// FirewallManaged true kalau aturan firewall untuk sumber ini DIPASANG
	// fitur ini — hanya aturan seperti ini yang ikut dicabut saat aturan
	// remote dihapus. Aturan firewall yang sudah ada sebelumnya (mis. "5432
	// dari mana saja" buatan user) tidak pernah disentuh.
	FirewallManaged bool `json:"firewallManaged"`
	// FirewallAllowed true kalau firewall yang aktif saat ini mengizinkan
	// sumber ini ke port database (atau tidak ada firewall aktif).
	FirewallAllowed bool `json:"firewallAllowed"`
}

// DBRemoteAccessStatus status remote access satu engine.
type DBRemoteAccessStatus struct {
	Engine string `json:"engine"`
	Port   int    `json:"port"`
	// ListenAll listen_addresses '*' / bind-address 0.0.0.0. Tanpa ini klien
	// remote ditolak di level TCP sebelum aturan mana pun dibaca.
	ListenAll   bool   `json:"listenAll"`
	ListenValue string `json:"listenValue,omitempty"`
	// FirewallDetected backend firewall aktif: "ufw" | "firewalld" |
	// "nftables" | "none" (kosong kalau pengecekan gagal).
	FirewallDetected string         `json:"firewallDetected,omitempty"`
	Rules            []DBRemoteRule `json:"rules"`
	// ClientIP alamat IP komputer ini SEBAGAIMANA TERLIHAT oleh server
	// (diambil dari $SSH_CLIENT sesi SSH poinhost). Dipakai UI sebagai isian
	// cepat "IP saya" — klien seperti Navicat di komputer yang sama akan
	// datang dari IP ini juga.
	ClientIP string `json:"clientIp,omitempty"`
	// LocalOnlyUsers akun MySQL yang hanya punya host localhost — tetap akan
	// ditolak ("Host ... is not allowed") walau jalur jaringan sudah terbuka.
	LocalOnlyUsers []string `json:"localOnlyUsers,omitempty"`
	Message        string   `json:"message,omitempty"`
}

// DBRemoteRuleRequest menambah satu sumber remote access.
type DBRemoteRuleRequest struct {
	ServerID string `json:"serverId"`
	Engine   string `json:"engine"`
	// Source IP ("182.8.212.16") atau CIDR ("10.0.0.0/8"). "0.0.0.0/0"
	// diterima (UI memberi peringatan keras), tapi tidak pernah jadi default.
	Source   string `json:"source"`
	Database string `json:"database,omitempty"`
	Username string `json:"username,omitempty"`
	Note     string `json:"note,omitempty"`
	// OpenFirewall: pasang juga aturan allow di ufw/firewalld yang aktif
	// kalau sumber ini belum diizinkan.
	OpenFirewall bool `json:"openFirewall"`
}

func dbPort(engine string) int {
	if engine == "mysql" {
		return 3306
	}
	return 5432
}

// normalizeRemoteSource mengubah IP/CIDR jadi CIDR kanonik. Satu IP jadi /32
// (IPv4) atau /128 (IPv6) — pg_hba menolak IP tanpa mask.
func normalizeRemoteSource(raw string) (string, error) {
	src := strings.TrimSpace(raw)
	if src == "" {
		return "", errFmt("sumber (IP atau CIDR) wajib diisi")
	}
	if strings.Contains(src, "/") {
		_, n, err := net.ParseCIDR(src)
		if err != nil {
			return "", errFmt("CIDR tidak valid — contoh yang benar: 182.8.212.16/32 atau 10.0.0.0/8")
		}
		return n.String(), nil
	}
	ip := net.ParseIP(src)
	if ip == nil {
		return "", errFmt("sumber harus berupa alamat IP atau CIDR, mis. 182.8.212.16")
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String() + "/32", nil
	}
	return ip.String() + "/128", nil
}

// isAnywhere true untuk 0.0.0.0/0 dan ::/0.
func isAnywhere(cidr string) bool {
	return cidr == "0.0.0.0/0" || cidr == "::/0"
}

// normalizeHbaName kolom database/user pg_hba: kosong = "all".
func normalizeHbaName(raw string, re *regexp.Regexp, what string) (string, error) {
	n := strings.TrimSpace(raw)
	if n == "" || n == "all" {
		return "all", nil
	}
	if !re.MatchString(n) {
		return "", errFmt("%s tidak valid (huruf/angka/_ saja)", what)
	}
	return n, nil
}

var remoteNoteRE = regexp.MustCompile(`^[A-Za-z0-9 _.\-]{0,40}$`)

func normalizeRemoteNote(raw string) (string, error) {
	n := strings.TrimSpace(raw)
	if !remoteNoteRE.MatchString(n) {
		return "", errFmt("catatan hanya boleh huruf, angka, spasi, dan _ . - (maks 40 karakter)")
	}
	return n, nil
}

var remoteRuleIDRE = regexp.MustCompile(`^[0-9a-f]{8}$`)

func newRemoteRuleID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func mysqlRemoteConfPath(pm string) string {
	if pm == "apt" {
		return "/etc/mysql/mariadb.conf.d/99-poinhost-remote.cnf"
	}
	return "/etc/my.cnf.d/99-poinhost-remote.cnf"
}

// remoteMarkerLine baris komentar penanda satu aturan. Formatnya
// `# <marker> id=<id> fw=<0|1> [src=<cidr>] note=<teks bebas>` — note
// selalu terakhir karena boleh berisi spasi.
func remoteMarkerLine(id string, fw bool, src, note string) string {
	f := "0"
	if fw {
		f = "1"
	}
	line := "# " + remoteAccessMarker + " id=" + id + " fw=" + f
	if src != "" {
		line += " src=" + src
	}
	return line + " note=" + note
}

// parseRemoteMarker kebalikan remoteMarkerLine.
func parseRemoteMarker(line string) (r DBRemoteRule, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(line), "# "+remoteAccessMarker+" ")
	if !found {
		return r, false
	}
	if i := strings.Index(rest, "note="); i >= 0 {
		r.Note = strings.TrimSpace(rest[i+len("note="):])
		rest = rest[:i]
	}
	for _, f := range strings.Fields(rest) {
		k, v, _ := strings.Cut(f, "=")
		switch k {
		case "id":
			r.ID = v
		case "fw":
			r.FirewallManaged = v == "1"
		case "src":
			r.Source = v
		}
	}
	return r, remoteRuleIDRE.MatchString(r.ID)
}

// parseHbaLine membaca baris `host <db> <user> <cidr> <auth>`.
func parseHbaLine(line string, r *DBRemoteRule) bool {
	f := strings.Fields(line)
	if len(f) < 5 || (f[0] != "host" && f[0] != "hostssl") {
		return false
	}
	r.Database, r.Username, r.Source, r.Auth = f[1], f[2], f[3], f[4]
	return true
}

// pgHbaPrelude membaca lokasi pg_hba.conf dan metode auth yang sesuai
// password_encryption server — alasan tidak mematok md5 ada di
// dbDockerAccessApplyScript.
const pgHbaPrelude = `HBA=$(sudo -u postgres psql -tAc "SHOW hba_file;" | tr -d '[:space:]')
if [ -z "$HBA" ] || [ ! -f "$HBA" ]; then echo "lokasi pg_hba.conf tidak terbaca" >&2; exit 1; fi
AUTH=$(sudo -u postgres psql -tAc "SHOW password_encryption;" 2>/dev/null | tr -d '[:space:]')
case "$AUTH" in
  scram-sha-256|md5) ;;
  *) AUTH=scram-sha-256 ;;
esac
`

// pgRestartFn me-restart cluster PostgreSQL yang SEDANG BERJALAN. Di
// Debian/Ubuntu `postgresql.service` hanya wrapper (oneshot), unit aslinya
// postgresql@16-main; di RHEL namanya postgresql-16. Mencari unit yang
// running lebih andal daripada menebak nama per distro. Setelah restart,
// ditunggu sampai server menerima koneksi lagi supaya pembacaan status
// berikutnya tidak gagal.
const pgRestartFn = `pg_restart() {
  U=$(systemctl list-units --type=service --state=running --no-legend --plain 'postgresql*' 2>/dev/null | awk '{print $1}' | grep -v '^postgresql.service$' | head -n1)
  if [ -n "$U" ]; then systemctl restart "$U"; else systemctl restart postgresql; fi
  for _ in $(seq 1 20); do
    sudo -u postgres psql -tAc "SELECT 1;" >/dev/null 2>&1 && return 0
    sleep 0.5
  done
  echo "PostgreSQL tidak kembali menerima koneksi setelah restart" >&2
  return 1
}
`

// pgRemoteAddScript menambah satu baris pg_hba bermarker, memastikan
// listen_addresses '*', lalu reload (atau restart kalau listener harus
// berubah — listen_addresses tidak bisa diubah lewat reload).
//
// Baris baru divalidasi lewat pg_hba_file_rules SEBELUM reload: kalau
// PostgreSQL menganggap baris itu error, baris dicabut lagi dan operasi
// gagal — pg_hba yang rusak membuat reload diabaikan dan restart berikutnya
// bisa membuat server menolak SEMUA koneksi.
func pgRemoteAddScript(id string, fw bool, db, user, src, note string) string {
	marker := remoteMarkerLine(id, fw, "", note)
	return `set -e
` + pgRestartFn + pgHbaPrelude + `# Tanpa baris kosong pembuka: penghapusan hanya membuang marker + baris
# aturannya, jadi baris kosong tambahan akan menumpuk tiap siklus tambah/hapus.
[ -s "$HBA" ] && [ -n "$(tail -c1 "$HBA")" ] && echo >> "$HBA"
printf '%s\nhost    %s    %s    %s    %s\n' ` +
		shellQuote(marker) + ` ` + shellQuote(db) + ` ` + shellQuote(user) + ` ` + shellQuote(src) + ` "$AUTH" >> "$HBA"
ERR=$(sudo -u postgres psql -tAc "SELECT line_number || ': ' || error FROM pg_hba_file_rules WHERE error IS NOT NULL;" 2>/dev/null || true)
if [ -n "$ERR" ]; then
  sed -i '/^# ` + remoteAccessMarker + ` id=` + id + ` /,+1d' "$HBA"
  echo "pg_hba.conf menolak aturan ini: $ERR" >&2
  exit 1
fi
LISTEN=$(sudo -u postgres psql -tAc "SHOW listen_addresses;" | tr -d '[:space:]')
case "$LISTEN" in
  *'*'*|*0.0.0.0*)
    sudo -u postgres psql -tAc "SELECT pg_reload_conf();" >/dev/null
    echo "PG=reloaded" ;;
  *)
    sudo -u postgres psql -c "ALTER SYSTEM SET listen_addresses = '*';" >/dev/null
    pg_restart
    echo "PG=restarted" ;;
esac
echo "POINHOST_REMOTE_ACCESS_DONE"
`
}

// pgRemoteRemoveScript mencabut satu aturan. Kalau tidak tersisa aturan
// poinhost apa pun (remote maupun Docker) listen_addresses dikembalikan ke
// default — kalau masih ada, cukup reload.
func pgRemoteRemoveScript(id string) string {
	return `set -e
` + pgRestartFn + pgHbaPrelude + `sed -i '/^# ` + remoteAccessMarker + ` id=` + id + ` /,+1d' "$HBA"
if ! grep -q "^# ` + remoteAccessMarker + ` id=" "$HBA" && ! grep -q "` + dockerAccessMarker + `" "$HBA"; then
  sudo -u postgres psql -c "ALTER SYSTEM RESET listen_addresses;" >/dev/null
  pg_restart
  echo "PG=restarted"
else
  sudo -u postgres psql -tAc "SELECT pg_reload_conf();" >/dev/null
  echo "PG=reloaded"
fi
echo "POINHOST_REMOTE_ACCESS_REMOVED"
`
}

// mysqlBindProbe membaca bind_address lewat unix socket (lihat komentar di
// dbDockerAccessStatusScript kenapa bukan TCP).
const mysqlBindProbe = `mysql -u root --batch --skip-column-names -e "SHOW VARIABLES LIKE 'bind_address';" 2>/dev/null | awk '{print $2}'`

const mysqlRestartCmd = `systemctl restart mariadb 2>/dev/null || systemctl restart mysql`

// mysqlRemoteAddScript membuat (sekali) file drop-in bind-address 0.0.0.0
// dan mencatat sumbernya sebagai komentar bermarker di file yang sama. File
// ini terpisah dari 99-poinhost-docker.cnf: mematikan akses Docker tidak
// menutup remote access, dan sebaliknya.
func mysqlRemoteAddScript(pm, id string, fw bool, src, note string) string {
	path := mysqlRemoteConfPath(pm)
	return `set -e
F=` + shellQuote(path) + `
mkdir -p "$(dirname "$F")"
if [ ! -f "$F" ]; then
  cat > "$F" <<'POINHOSTEOF'
# ` + remoteAccessMarker + ` — remote access MySQL/MariaDB dari klien eksternal.
# Dikelola poinhost: baris "id=" di bawah adalah sumber yang diizinkan. File
# ini dihapus otomatis saat aturan terakhir dicabut.
[mysqld]
bind-address = 0.0.0.0
POINHOSTEOF
fi
printf '%s\n' ` + shellQuote(remoteMarkerLine(id, fw, src, note)) + ` >> "$F"
BIND=$(` + mysqlBindProbe + `)
case "$BIND" in
  0.0.0.0|'*'|::) echo "MY=unchanged" ;;
  *) ` + mysqlRestartCmd + `; echo "MY=restarted" ;;
esac
echo "POINHOST_REMOTE_ACCESS_DONE"
`
}

func mysqlRemoteRemoveScript(pm, id string) string {
	path := mysqlRemoteConfPath(pm)
	return `set -e
F=` + shellQuote(path) + `
if [ -f "$F" ]; then
  sed -i '/^# ` + remoteAccessMarker + ` id=` + id + ` /d' "$F"
  if ! grep -q "^# ` + remoteAccessMarker + ` id=" "$F"; then
    rm -f "$F"
    ` + mysqlRestartCmd + `
    echo "MY=restarted"
  fi
fi
echo "POINHOST_REMOTE_ACCESS_REMOVED"
`
}

// remoteStatusScript membaca listener + aturan bermarker. Untuk MySQL kedua
// lokasi drop-in dicek sehingga tidak perlu deteksi distro di jalur baca.
func remoteStatusScript(engine string) string {
	if engine == "mysql" {
		return `echo "BIND=$(` + mysqlBindProbe + `)"
mysql -u root --batch --skip-column-names -e "SELECT user FROM mysql.user GROUP BY user HAVING SUM(host NOT IN ('localhost','127.0.0.1','::1')) = 0;" 2>/dev/null | sed 's/^/LOCALUSER=/'
for F in ` + mysqlRemoteConfPath("apt") + ` ` + mysqlRemoteConfPath("dnf") + `; do
  [ -f "$F" ] && grep "^# ` + remoteAccessMarker + ` id=" "$F" | sed 's/^/RULE=/'
done
exit 0
`
	}
	return `echo "BIND=$(sudo -u postgres psql -tAc "SHOW listen_addresses;" 2>/dev/null | tr -d '[:space:]')"
HBA=$(sudo -u postgres psql -tAc "SHOW hba_file;" 2>/dev/null | tr -d '[:space:]')
if [ -n "$HBA" ] && [ -f "$HBA" ]; then
  awk '/^# ` + remoteAccessMarker + ` id=/ { m = $0; if ((getline l) > 0) print "RULE=" m "\t" l }' "$HBA"
fi
exit 0
`
}

// mysqlSystemAccount akun internal MySQL/MariaDB yang memang selalu
// localhost-only dan tidak pernah dipakai klien — menyebutnya di peringatan
// hanya menambah noise.
func mysqlSystemAccount(u string) bool {
	return u == "mysql" || u == "mariadb.sys" || strings.HasPrefix(u, "mysql.")
}

// parseRemoteStatus dipisah supaya bisa diuji tanpa SSH.
func parseRemoteStatus(engine, stdout string) *DBRemoteAccessStatus {
	st := &DBRemoteAccessStatus{Engine: engine, Port: dbPort(engine), Rules: []DBRemoteRule{}}
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "BIND="):
			v := strings.TrimSpace(strings.TrimPrefix(line, "BIND="))
			st.ListenValue = v
			st.ListenAll = v == "0.0.0.0" || v == "::" || strings.Contains(v, "*") || strings.Contains(v, "0.0.0.0")
		case strings.HasPrefix(line, "LOCALUSER="):
			if u := strings.TrimSpace(strings.TrimPrefix(line, "LOCALUSER=")); u != "" && !mysqlSystemAccount(u) {
				st.LocalOnlyUsers = append(st.LocalOnlyUsers, u)
			}
		case strings.HasPrefix(line, "RULE="):
			body := strings.TrimPrefix(line, "RULE=")
			marker, hba, _ := strings.Cut(body, "\t")
			r, ok := parseRemoteMarker(marker)
			if !ok {
				continue
			}
			if engine != "mysql" && !parseHbaLine(hba, &r) {
				continue // baris pg_hba pasangannya hilang/diedit manual
			}
			if r.Source == "" {
				continue
			}
			st.Rules = append(st.Rules, r)
		}
	}
	return st
}

// finishRemoteStatus mengisi FirewallAllowed per aturan dan menyusun pesan
// peringatan.
func (st *DBRemoteAccessStatus) finish() {
	var msgs []string
	if len(st.Rules) > 0 && !st.ListenAll {
		msgs = append(msgs, "Database masih hanya mendengarkan "+st.ListenValue+" — klien remote ditolak di level TCP. Hapus lalu tambahkan lagi aturannya untuk menerapkan ulang.")
	}
	var blocked []string
	anywhere := false
	for _, r := range st.Rules {
		if !r.FirewallAllowed {
			blocked = append(blocked, r.Source)
		}
		if isAnywhere(r.Source) {
			anywhere = true
		}
	}
	if len(blocked) > 0 {
		msgs = append(msgs, "Firewall ("+st.FirewallDetected+") masih memblokir port "+itoa(st.Port)+" untuk: "+strings.Join(blocked, ", ")+".")
	}
	if anywhere {
		msgs = append(msgs, "Ada aturan untuk SELURUH internet (0.0.0.0/0). Database jadi sasaran brute-force password — batasi ke IP yang dipakai, atau gunakan SSH tunnel.")
	}
	if st.Engine == "mysql" && len(st.Rules) > 0 {
		if st.FirewallDetected == "none" {
			msgs = append(msgs, "Tidak ada firewall aktif: port MySQL kini bisa dijangkau siapa pun, bukan hanya sumber di daftar ini. MySQL tidak punya pg_hba — aktifkan ufw/firewalld supaya pembatasan sumber benar-benar berlaku.")
		}
		if len(st.LocalOnlyUsers) > 0 {
			msgs = append(msgs, "Akun MySQL berikut hanya punya host localhost sehingga tetap ditolak dari luar (\"Host ... is not allowed\"): "+strings.Join(st.LocalOnlyUsers, ", ")+". Buat akun dengan host '%' atau IP klien di tab Akses.")
		}
	}
	st.Message = strings.Join(msgs, " ")
}

// clientIP membaca IP komputer ini sebagaimana terlihat server, dari
// $SSH_CLIENT. Dijalankan TANPA sudo: sudo me-reset environment sehingga
// variabel itu hilang. Best-effort — kosong kalau tidak tersedia.
func (s *Service) clientIP(serverID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.executor.Exec(ctx, serverID, 10*time.Second, `printf '%s' "${SSH_CLIENT%% *}"`, false)
	if err != nil || res == nil {
		return ""
	}
	ip := net.ParseIP(strings.TrimSpace(res.Stdout))
	if ip == nil || ip.IsLoopback() {
		return ""
	}
	return ip.String()
}

// GetDBRemoteAccessStatus membaca status remote access — TANPA mengubah
// apa pun di server.
func (s *Service) GetDBRemoteAccessStatus(serverID, engine string) (*DBRemoteAccessStatus, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	res, err := s.run(access, remoteStatusScript(engine), 15*time.Second)
	if err != nil {
		return nil, err
	}
	st := parseRemoteStatus(engine, res.Stdout)

	// Tanpa modul firewall (atau kalau pengecekannya gagal) status firewall
	// dibiarkan tidak diketahui; aturan dianggap tidak diblokir daripada
	// memunculkan peringatan dari tebakan.
	for i := range st.Rules {
		st.Rules[i].FirewallAllowed = true
	}
	if s.firewall != nil {
		srcs := make([]string, len(st.Rules))
		for i, r := range st.Rules {
			srcs[i] = r.Source
		}
		if allowed, backend, ferr := s.firewall.PortAllowedFromSources(serverID, st.Port, srcs); ferr == nil {
			st.FirewallDetected = backend
			for i := range st.Rules {
				st.Rules[i].FirewallAllowed = allowed[i]
			}
		}
	}
	st.ClientIP = s.clientIP(serverID)
	st.finish()
	return st, nil
}

// AddDBRemoteRule mengizinkan satu sumber connect dari luar.
func (s *Service) AddDBRemoteRule(req DBRemoteRuleRequest) (*DBRemoteAccessStatus, error) {
	engine, err := normalizeDBEngine(req.Engine)
	if err != nil {
		return nil, err
	}
	src, err := normalizeRemoteSource(req.Source)
	if err != nil {
		return nil, err
	}
	note, err := normalizeRemoteNote(req.Note)
	if err != nil {
		return nil, err
	}
	db, user := "all", "all"
	if engine == "postgresql" {
		if db, err = normalizeHbaName(req.Database, dbNameRE, "nama database"); err != nil {
			return nil, err
		}
		if user, err = normalizeHbaName(req.Username, dbUserRE, "username"); err != nil {
			return nil, err
		}
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return nil, err
	}

	// Firewall dicek LEBIH DULU supaya penanda fw=1 hanya ditulis untuk
	// aturan firewall yang benar-benar dipasang fitur ini — itulah satu-
	// satunya aturan yang boleh ikut dicabut nanti.
	port := dbPort(engine)
	installFirewall := false
	fwNote := ""
	if req.OpenFirewall && s.firewall != nil {
		allowed, backend, ferr := s.firewall.PortAllowedFromSource(req.ServerID, port, src)
		switch {
		case ferr != nil:
			fwNote = "Status firewall tidak terbaca, aturan firewall tidak dipasang: " + ferr.Error()
		case allowed:
			// sudah terbuka (atau tidak ada firewall) — tidak ada yang ditambah
		case backend == "ufw" || backend == "firewalld":
			installFirewall = true
		default:
			fwNote = "Firewall " + backend + " tidak bisa diatur dari sini — izinkan port " + itoa(port) + " untuk " + src + " secara manual."
		}
	}

	id := newRemoteRuleID()
	var script string
	if engine == "mysql" {
		distro, err := s.detectDistro(access)
		if err != nil {
			return nil, err
		}
		script = mysqlRemoteAddScript(distro.PackageManager, id, installFirewall, src, note)
	} else {
		script = pgRemoteAddScript(id, installFirewall, db, user, src, note)
	}
	s.mutex.Lock(req.ServerID)
	_, err = s.run(access, script, 60*time.Second)
	s.mutex.Unlock(req.ServerID)
	if err != nil {
		return nil, err
	}

	// Di luar mutex: modul firewall mengunci server yang sama sendiri.
	if installFirewall {
		if ferr := s.firewall.AllowSourceToPort(req.ServerID, port, src); ferr != nil {
			fwNote = "Aturan database sudah dipasang, tapi aturan firewall gagal: " + ferr.Error()
		}
	}

	st, err := s.GetDBRemoteAccessStatus(req.ServerID, engine)
	if err != nil {
		return nil, err
	}
	if fwNote != "" {
		st.Message = strings.TrimSpace(fwNote + " " + st.Message)
	}
	return st, nil
}

// RemoveDBRemoteRule mencabut satu sumber. Aturan firewall ikut dicabut
// HANYA kalau dipasang fitur ini dan tidak ada aturan lain bersumber sama
// yang masih memakainya.
func (s *Service) RemoveDBRemoteRule(serverID, engine, id string) (*DBRemoteAccessStatus, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if !remoteRuleIDRE.MatchString(id) {
		return nil, errFmt("id aturan tidak valid")
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	res, err := s.run(access, remoteStatusScript(engine), 15*time.Second)
	if err != nil {
		return nil, err
	}
	before := parseRemoteStatus(engine, res.Stdout)
	var target *DBRemoteRule
	for i := range before.Rules {
		if before.Rules[i].ID == id {
			target = &before.Rules[i]
		}
	}
	if target == nil {
		return nil, errFmt("aturan remote access tidak ditemukan (mungkin sudah dihapus)")
	}

	var script string
	if engine == "mysql" {
		distro, err := s.detectDistro(access)
		if err != nil {
			return nil, err
		}
		script = mysqlRemoteRemoveScript(distro.PackageManager, id)
	} else {
		script = pgRemoteRemoveScript(id)
	}
	s.mutex.Lock(serverID)
	_, err = s.run(access, script, 60*time.Second)
	s.mutex.Unlock(serverID)
	if err != nil {
		return nil, err
	}

	fwNote := ""
	if target.FirewallManaged && s.firewall != nil && !firewallStillNeeded(before.Rules, *target) {
		if ferr := s.firewall.RemoveSourceFromPort(serverID, dbPort(engine), target.Source); ferr != nil {
			fwNote = "Aturan database sudah dicabut, tapi aturan firewall gagal dicabut: " + ferr.Error()
		}
	}

	st, err := s.GetDBRemoteAccessStatus(serverID, engine)
	if err != nil {
		return nil, err
	}
	if fwNote != "" {
		st.Message = strings.TrimSpace(fwNote + " " + st.Message)
	}
	return st, nil
}

// firewallStillNeeded true kalau aturan lain (selain target) bersumber sama
// masih ada — aturan firewall untuk sumber itu tetap dibutuhkannya.
func firewallStillNeeded(rules []DBRemoteRule, target DBRemoteRule) bool {
	for _, r := range rules {
		if r.ID != target.ID && r.Source == target.Source {
			return true
		}
	}
	return false
}
