package website

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// Modul Database (dari tab "Database" di menu Website) TERNYATA — sama
// seperti di homepoin — bukan benar-benar domain-scoped: ini cuma utilitas
// ringan "sediakan database/user/grants" untuk MySQL/PostgreSQL, kebetulan
// bisa dibuka dari tab satu domain untuk kenyamanan. Field Domain di
// request cuma breadcrumb UI, tidak pernah dipakai memfilter apa pun di
// sini. Ini TERPISAH dari database browser server-wide (tabel/baris/query)
// yang jauh lebih besar — modul itu di luar scope port ini.
//
// Beda penting dari homepoin: karena semua perintah SQL di sini dijalankan
// via SSH exec LANGSUNG di server target (bukan tunnel TCP dari proses
// homepoin yang terpisah), tidak perlu sama sekali membuka akses remote
// MySQL/PostgreSQL (bind ke 0.0.0.0, edit pg_hba.conf, buka firewall) —
// itu semua dilewati di sini, jauh lebih sederhana dan tidak menambah luas
// permukaan serangan server target tanpa alasan.

// DBEngineStatus status instalasi & service MySQL/PostgreSQL di server.
type DBEngineStatus struct {
	Engine         string `json:"engine"` // "mysql" | "postgresql"
	Installed      bool   `json:"installed"`
	Active         bool   `json:"active"`
	Enabled        bool   `json:"enabled"`
	Version        string `json:"version,omitempty"`
	DistroID       string `json:"distroId,omitempty"`
	DistroName     string `json:"distroName,omitempty"`
	PackageManager string `json:"packageManager,omitempty"`
	CanInstall     bool   `json:"canInstall"`
	// RepoConfigured: repo resmi vendor (MariaDB repo / PostgreSQL PGDG)
	// sudah terpasang di server ini — hasil dari instalasi versi PINNED
	// sebelumnya (lihat dbversion.go). Bukan syarat instalasi baru (repo
	// dipasang otomatis sekali kalau perlu), cuma info status.
	RepoConfigured bool `json:"repoConfigured"`
}

// DBDatabaseInfo satu database di server.
type DBDatabaseInfo struct {
	Name    string `json:"name"`
	Charset string `json:"charset,omitempty"`
}

// DBUserInfo satu user database — kalau user MySQL yang sama terdaftar
// dengan beberapa host (mis. 'usera'@'localhost' dan 'usera'@'127.0.0.1'),
// digabung jadi SATU baris di sini (Host jadi gabungan dipisah koma untuk
// tampilan, Hosts tetap menyimpan daftar aslinya satu-satu — dibutuhkan
// operasi per-host seperti reapply grants/credential/Explore, yang di
// MySQL memang terikat host tertentu). PostgreSQL tidak punya konsep host
// per role sama sekali (Hosts selalu kosong).
type DBUserInfo struct {
	Username string   `json:"username"`
	Host     string   `json:"host,omitempty"`  // gabungan Hosts dipisah ", " — MySQL saja
	Hosts    []string `json:"hosts,omitempty"` // daftar host asli satu-satu — MySQL saja
	// Databases: nama-nama database yang bisa diakses user/role ini (hasil
	// introspeksi grants yang SUDAH diterapkan — bukan cuma yang dipilih
	// terakhir kali lewat form Buat User/Reapply Grants). AllDatabases true
	// kalau user/role ini punya akses ke SEMUA database (MySQL: grant
	// *.*; PostgreSQL: didekati dengan "role muncul di semua database yang
	// terdaftar" — role Postgres tidak punya grant global tunggal seperti
	// *.* MySQL).
	Databases    []string `json:"databases,omitempty"`
	AllDatabases bool     `json:"allDatabases,omitempty"`
	// Privileges: key privilege (lihat DBPrivilegeOptions) yang BENAR-BENAR
	// dipegang user ini sekarang, hasil introspeksi grant aktual — dipakai
	// dialog "Ubah privilege" supaya centangnya berangkat dari kondisi asli,
	// bukan dari tebakan/nilai default form.
	Privileges []string `json:"privileges,omitempty"`
}

// DBPrivilegeOption satu opsi privilege yang bisa dipilih user (dipetakan
// ke grant SQL konkret berbeda per engine, lihat privilegeSQL).
type DBPrivilegeOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// DBPrivilegeOptions daftar privilege tetap yang ditawarkan UI.
func DBPrivilegeOptions() []DBPrivilegeOption {
	return []DBPrivilegeOption{
		{Key: "read", Label: "Baca (SELECT)"},
		{Key: "write", Label: "Tulis (INSERT/UPDATE/DELETE)"},
		{Key: "create", Label: "Buat tabel (CREATE)"},
		{Key: "alter", Label: "Ubah struktur (ALTER)"},
		{Key: "drop", Label: "Hapus tabel (DROP)"},
		{Key: "execute", Label: "Jalankan fungsi/prosedur (EXECUTE)"},
	}
}

// DBCreateDatabaseRequest membuat database baru. Owner (opsional) = user
// yang langsung dijadikan pemilik/pemegang akses penuh database ini:
// PostgreSQL memakai OWNER asli (CREATE DATABASE ... OWNER), MySQL tidak
// punya konsep pemilik database sama sekali sehingga didekati dengan GRANT
// ALL ON <db>.* ke user tsb (lihat DBCreateDatabase).
type DBCreateDatabaseRequest struct {
	ServerID  string `json:"serverId"`
	Engine    string `json:"engine"`
	Domain    string `json:"domain,omitempty"` // breadcrumb UI saja
	Name      string `json:"name"`
	Encoding  string `json:"encoding,omitempty"`
	Owner     string `json:"owner,omitempty"`
	OwnerHost string `json:"ownerHost,omitempty"` // MySQL saja; kosong = semua host user itu
}

// DBDatabaseUserRequest memberi/mencabut akses satu user ke satu database
// (fitur "assign user" di daftar database) — dipakai DBGrantDatabaseUser
// dan DBRevokeDatabaseUser untuk KEDUA engine.
type DBDatabaseUserRequest struct {
	ServerID   string   `json:"serverId"`
	Engine     string   `json:"engine"`
	Database   string   `json:"database"`
	Username   string   `json:"username"`
	Host       string   `json:"host,omitempty"` // MySQL saja; kosong = semua host user itu
	Privileges []string `json:"privileges,omitempty"`
}

// DBCreateUserRequest membuat user database baru + grants awal.
type DBCreateUserRequest struct {
	ServerID   string   `json:"serverId"`
	Engine     string   `json:"engine"`
	Domain     string   `json:"domain,omitempty"`
	Username   string   `json:"username"`
	Password   string   `json:"password"`
	Host       string   `json:"host,omitempty"` // MySQL saja, default "%"
	AllDBs     bool     `json:"allDbs"`
	Databases  []string `json:"databases,omitempty"`
	Privileges []string `json:"privileges"`
	// SaveCredential: simpan password ini ke vault lokal sekalian (opsional,
	// dicentang user) supaya user langsung bisa di-Explore tanpa masukkan
	// ulang password — didukung untuk MySQL (mysqlexplore.go) maupun
	// PostgreSQL (pgexplore.go).
	SaveCredential bool `json:"saveCredential,omitempty"`
}

// DBGrantsRequest menerapkan ulang grants untuk user yang sudah ada.
type DBGrantsRequest struct {
	ServerID   string   `json:"serverId"`
	Engine     string   `json:"engine"`
	Username   string   `json:"username"`
	Host       string   `json:"host,omitempty"`
	AllDBs     bool     `json:"allDbs"`
	Databases  []string `json:"databases,omitempty"`
	Privileges []string `json:"privileges"`
}

func normalizeDBEngine(raw string) (string, error) {
	switch raw {
	case "mysql", "postgresql":
		return raw, nil
	default:
		return "", errFmt("engine database tidak dikenal: %s (pakai mysql atau postgresql)", raw)
	}
}

var dbNameRE = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
var dbUserRE = regexp.MustCompile(`^[A-Za-z0-9_]{1,32}$`)

func normalizeDBName(raw string) (string, error) {
	n := strings.TrimSpace(raw)
	if !dbNameRE.MatchString(n) {
		return "", errFmt("nama database tidak valid (huruf/angka/_ saja, maks 64 karakter)")
	}
	return n, nil
}

func normalizeDBUsername(raw string) (string, error) {
	n := strings.TrimSpace(raw)
	if !dbUserRE.MatchString(n) {
		return "", errFmt("username database tidak valid (huruf/angka/_ saja, maks 32 karakter)")
	}
	return n, nil
}

// mysqlPrivilegeSQL memetakan key privilege ke daftar hak MySQL.
func mysqlPrivilegeSQL(keys []string) string {
	set := map[string]string{
		"read": "SELECT", "write": "INSERT, UPDATE, DELETE", "create": "CREATE",
		"alter": "ALTER", "drop": "DROP", "execute": "EXECUTE",
	}
	var parts []string
	for _, k := range keys {
		if v, ok := set[k]; ok {
			parts = append(parts, v)
		}
	}
	if len(parts) == 0 {
		return "USAGE"
	}
	return strings.Join(parts, ", ")
}

// Privilege PostgreSQL TIDAK seragam lintas jenis objek — inilah sumber bug
// nyata `invalid privilege type CREATE for relation`: CREATE itu hak
// SCHEMA/DATABASE, bukan hak tabel; EXECUTE hak FUNGSI; dan sequence cuma
// kenal USAGE/SELECT/UPDATE. Jadi satu daftar privilege tidak bisa
// ditempelkan ke semua statement GRANT seperti di MySQL — tiap jenis objek
// punya pemetaannya sendiri di bawah ini.
//
// ALTER dan DROP sengaja tidak dipetakan ke apa pun: PostgreSQL tidak punya
// grant untuk itu sama sekali (keduanya melekat pada KEPEMILIKAN objek),
// jadi satu-satunya cara memberikannya adalah menjadikan user pemilik
// database/schema — lihat opsi pemilik di DBCreateDatabase.
func postgresPrivilegeParts(keys []string, set map[string][]string, fallback string) string {
	seen := map[string]bool{}
	var parts []string
	for _, k := range keys {
		for _, p := range set[k] {
			if !seen[p] {
				seen[p] = true
				parts = append(parts, p)
			}
		}
	}
	if len(parts) == 0 {
		return fallback
	}
	return strings.Join(parts, ", ")
}

// postgresTablePrivilegeSQL — hak yang sah untuk tabel/view saja.
func postgresTablePrivilegeSQL(keys []string) string {
	return postgresPrivilegeParts(keys, map[string][]string{
		"read":  {"SELECT"},
		"write": {"INSERT", "UPDATE", "DELETE"},
		"drop":  {"TRUNCATE"},
	}, "SELECT")
}

// postgresSequencePrivilegeSQL — sequence hanya menerima USAGE/SELECT/UPDATE.
// USAGE+UPDATE dibutuhkan supaya nextval() jalan untuk kolom serial saat
// user diberi hak tulis.
func postgresSequencePrivilegeSQL(keys []string) string {
	return postgresPrivilegeParts(keys, map[string][]string{
		"read":  {"SELECT"},
		"write": {"USAGE", "UPDATE"},
	}, "SELECT")
}

func hasPrivilegeKey(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}

const dbStatusScriptMySQL = `BIN=$(command -v mysql 2>/dev/null || command -v mariadb 2>/dev/null || true)
echo "BIN=$BIN"
if [ -n "$BIN" ]; then "$BIN" --version 2>&1 | head -1; fi
echo "ACTIVE=$(systemctl is-active mysql 2>/dev/null || systemctl is-active mariadb 2>/dev/null || echo unknown)"
echo "ENABLED=$(systemctl is-enabled mysql 2>/dev/null || systemctl is-enabled mariadb 2>/dev/null || echo unknown)"
if [ -f /etc/apt/sources.list.d/mariadb.list ] || [ -f /etc/yum.repos.d/mariadb.repo ]; then echo "REPO=1"; fi`

// dbStatusScriptPostgres versi (mayor) diekstrak dari `psql --version` lalu
// dipakai mencoba nama service ter-versi ala PGDG dulu (`postgresql-16`,
// dst — dipakai instalasi PINNED lewat repo PGDG di RHEL/dnf, lihat
// dbversion.go) sebelum fallback ke nama generik `postgresql` (dipakai
// instalasi bawaan distro/apt, nama service-nya selalu generik apa pun
// versinya) — supaya status tetap akurat untuk KEDUA jalur instalasi.
//
// Debian/Ubuntu (postgresql-common) beda lagi: cluster sebenarnya jalan
// sebagai instance TEMPLATE `postgresql@<ver>-<cluster>` (default cluster
// "main"), dan unit generik `postgresql.service` cuma wrapper — kalau
// cluster itu diaktifkan langsung (mis. lewat pg_ctlcluster / start saat
// boot) tanpa pernah "start"/"restart" via unit wrapper-nya, `systemctl
// is-active postgresql` bisa balas "unknown"/inactive walau cluster-nya
// BENAR-BENAR jalan — makanya dicoba juga nama instance eksplisit
// (`postgresql@$VER-main`), lalu fallback paling longgar: cek ada/tidaknya
// instance `postgresql@*` mana pun yang aktif (menutup kasus nama cluster
// bukan "main" atau VER gagal terbaca).
const dbStatusScriptPostgres = `BIN=$(command -v psql 2>/dev/null || true)
echo "BIN=$BIN"
VER=""
if [ -n "$BIN" ]; then
  VLINE=$("$BIN" --version 2>&1 | head -1)
  echo "$VLINE"
  VER=$(echo "$VLINE" | grep -oE '[0-9]+' | head -1)
fi
ACTIVE=$(systemctl is-active postgresql-$VER 2>/dev/null || systemctl is-active postgresql@$VER-main 2>/dev/null || systemctl is-active postgresql 2>/dev/null || echo unknown)
if [ "$ACTIVE" != "active" ] && systemctl list-units --type=service --state=running --no-legend --plain 'postgresql@*' 2>/dev/null | grep -q .; then ACTIVE=active; fi
echo "ACTIVE=$ACTIVE"
ENABLED=$(systemctl is-enabled postgresql-$VER 2>/dev/null || systemctl is-enabled postgresql@$VER-main 2>/dev/null || systemctl is-enabled postgresql 2>/dev/null || echo unknown)
if [ "$ENABLED" != "enabled" ] && systemctl list-unit-files --type=service --state=enabled --no-legend --plain 'postgresql@*' 2>/dev/null | grep -q .; then ENABLED=enabled; fi
echo "ENABLED=$ENABLED"
if [ -f /etc/apt/sources.list.d/pgdg.list ] || rpm -q pgdg-redhat-repo >/dev/null 2>&1; then echo "REPO=1"; fi`

// DBStatus membaca status instalasi & service satu engine database.
func (s *Service) DBStatus(serverID, engine string) (*DBEngineStatus, error) {
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
	statusScript := dbStatusScriptMySQL
	if engine == "postgresql" {
		statusScript = dbStatusScriptPostgres
	}
	res, err := s.run(access, statusScript, 15*time.Second)
	if err != nil {
		return nil, err
	}
	st := &DBEngineStatus{
		Engine: engine, DistroID: distro.ID, DistroName: distro.Name,
		PackageManager: distro.PackageManager, CanInstall: distro.PackageManager != "",
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "BIN="):
			st.Installed = strings.TrimPrefix(line, "BIN=") != ""
		case strings.HasPrefix(line, "ACTIVE="):
			st.Active = strings.TrimPrefix(line, "ACTIVE=") == "active"
		case strings.HasPrefix(line, "ENABLED="):
			st.Enabled = strings.TrimPrefix(line, "ENABLED=") == "enabled"
		case line == "REPO=1":
			st.RepoConfigured = true
		case st.Version == "" && line != "" && !strings.Contains(line, "="):
			st.Version = line
		}
	}
	return st, nil
}

// DBStart mengaktifkan service database yang sudah terpasang tapi tidak
// aktif. PostgreSQL butuh perlakuan khusus: instalasi PINNED lewat repo
// PGDG di RHEL/dnf (lihat dbversion.go) memberi nama service TER-VERSI
// (`postgresql-16`, dst — PGDG sengaja begitu supaya beberapa versi bisa
// hidup berdampingan), beda dari instalasi bawaan distro/apt yang selalu
// memakai nama generik `postgresql` apa pun versinya. Skrip di bawah coba
// nama ter-versi dulu (diekstrak dari `psql --version`), fallback ke nama
// generik — jadi jalan untuk KEDUA jalur instalasi tanpa perlu tahu lebih
// dulu jalur mana yang dipakai.
func (s *Service) DBStart(serverID, engine string) (*DBEngineStatus, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	var script string
	if engine == "postgresql" {
		script = `set -e
BIN=$(command -v psql 2>/dev/null || true)
VER=""
if [ -n "$BIN" ]; then VER=$("$BIN" --version 2>&1 | grep -oE '[0-9]+' | head -1); fi
systemctl enable postgresql-$VER 2>/dev/null || systemctl enable postgresql 2>/dev/null || true
systemctl start postgresql-$VER 2>&1 || systemctl start postgresql 2>&1 || systemctl restart postgresql 2>&1`
	} else {
		script = `set -e
systemctl enable mariadb 2>/dev/null || systemctl enable mysql 2>/dev/null || true
systemctl start mariadb 2>&1 || systemctl start mysql 2>&1 || systemctl restart mariadb 2>&1`
	}
	if _, err := s.run(access, script, 20*time.Second); err != nil {
		return nil, err
	}
	return s.DBStatus(serverID, engine)
}

// dbInstallScript skrip instalasi per engine+package manager. MariaDB
// dipakai sebagai pengganti MySQL di apt/dnf/yum (drop-in compatible,
// tersedia langsung dari repo distro tanpa perlu repo pihak ketiga).
//
// version kosong berarti "bawaan distro" (perilaku asli, tanpa repo pihak
// ketiga apa pun — versi ikut apa pun yang jadi default di repo OS
// tersebut, TIDAK selalu versi terbaru). version diisi (mis. "16" untuk
// PostgreSQL, "10.11" untuk MariaDB) berarti PINNED lewat repo resmi
// vendor — lihat dbversion.go.
func dbInstallScript(engine, pm, version string) (string, bool) {
	// dockerAccess dijalankan di AKHIR instalasi (bukan langkah terpisah
	// yang bisa lupa dipanggil) — supaya "instalasi MySQL/PostgreSQL" dan
	// "bisa langsung diakses dari container Docker di host yang sama"
	// selalu satu paket yang sama, sesuai yang diminta: user tidak perlu
	// tahu ada langkah tambahan sama sekali. Skrip yang SAMA persis dipakai
	// EnsureDBDockerAccess untuk instalasi yang sudah ada sebelumnya (lihat
	// dockeraccess.go) — jangan duplikasi logikanya di sini.
	dockerAccess := dbDockerAccessApplyScript(engine, pm)

	if version != "" {
		return dbVersionedInstallScript(engine, pm, version, dockerAccess)
	}

	if engine == "mysql" {
		switch pm {
		case "apt":
			return `set -e
` + aptWaitLock + `
apt-get install -y mariadb-server
systemctl enable mariadb
systemctl start mariadb
` + dockerAccess + `
echo ">> MariaDB (MySQL) terpasang, siap diakses dari container Docker di host ini."
`, true
		case "dnf", "yum":
			return `set -e
` + pm + ` install -y mariadb-server
systemctl enable mariadb
systemctl start mariadb
` + dockerAccess + `
echo ">> MariaDB (MySQL) terpasang, siap diakses dari container Docker di host ini."
`, true
		}
		return "", false
	}
	// postgresql
	switch pm {
	case "apt":
		return `set -e
` + aptWaitLock + `
apt-get install -y postgresql
systemctl enable postgresql
systemctl start postgresql
` + dockerAccess + `
echo ">> PostgreSQL terpasang, siap diakses dari container Docker di host ini."
`, true
	case "dnf", "yum":
		return `set -e
` + pm + ` install -y postgresql-server postgresql
postgresql-setup --initdb 2>/dev/null || /usr/bin/postgresql-setup initdb 2>/dev/null || true
systemctl enable postgresql
systemctl start postgresql
` + dockerAccess + `
echo ">> PostgreSQL terpasang, siap diakses dari container Docker di host ini."
`, true
	default:
		return "", false
	}
}

// runMySQL menjalankan SQL sebagai root lewat UNIX SOCKET lokal (`mysql -u
// root`, TANPA `-h 127.0.0.1`) — BUKAN salah ketik. Awalnya memakai `-h
// 127.0.0.1` (TCP) + user `root`@`127.0.0.1` ber-password kosong yang
// dibuat khusus saat instalasi, tapi itu GAGAL konsisten di server nyata:
// MariaDB/MySQL menganggap koneksi TCP ke 127.0.0.1 datang dari host
// "localhost" (lewat /etc/hosts + resolusi nama, `skip_name_resolve` OFF
// adalah default), lalu mencocokkan ke akun `root@localhost` (auth
// `unix_socket`, SELALU menolak koneksi TCP) alih-alih `root@127.0.0.1`
// yang sudah dibuat — bukan `root@127.0.0.1` yang dicoba sama sekali.
// Dikonfirmasi reproduksi nyata (instal MariaDB 10.11 asli, jalankan
// bootstrap `CREATE USER 'root'@'127.0.0.1'` persis seperti skrip
// instalasi, tetap `ERROR 1698: Access denied for user 'root'@'localhost'`
// walau password sudah benar). Root cause sebenarnya bug lama di SELURUH
// fitur admin MySQL modul ini (List/Create Database, List/Create User,
// Grants — semua lewat fungsi ini — bukan cuma tombol "Aktifkan akses dari
// Docker" yang melaporkannya). Socket lokal (dijalankan sebagai root lewat
// SSH exec, konteks trust yang sama dipakai di seluruh modul lain) bekerja
// tanpa syarat apa pun — tidak perlu akun `root@127.0.0.1` berpassword
// kosong sama sekali (dihapus dari skrip instalasi, lihat dbInstallScript/
// dbVersionedInstallScript — akun TCP-reachable berpassword kosong itu
// sendiri sebenarnya beban keamanan yang tidak perlu).
// mysqlAdminCommand membentuk perintah shell yang dipakai runMySQL — fungsi
// murni terpisah supaya bisa di-assert langsung di test (lihat
// database_test.go) bahwa perintah ini TIDAK PERNAH balik memakai `-h
// 127.0.0.1` (regresi ke bug yang sudah diperbaiki).
func mysqlAdminCommand(sql string) string {
	return "mysql -u root --batch --skip-column-names -e " + shellQuote(sql)
}

func (s *Service) runMySQL(access *websiteAccess, sql string) (string, error) {
	res, err := s.run(access, mysqlAdminCommand(sql), 20*time.Second)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = "perintah MySQL gagal"
		}
		return "", mapWebsiteError(errFmt("%s", msg))
	}
	return res.Stdout, nil
}

func (s *Service) runPostgres(access *websiteAccess, db, sql string) (string, error) {
	if db == "" {
		db = "postgres"
	}
	// -F TAB WAJIB ada. Dalam mode unaligned (-A), pemisah kolom bawaan psql
	// adalah PIPA, bukan tab — sementara parser di modul ini memecah baris
	// dengan tab (sama seperti keluaran `mysql --batch`). Tanpa flag ini,
	// seluruh baris masuk sebagai kolom pertama, sehingga mis. nama role
	// terbaca "acmeapp|OWNER" dan tidak cocok dengan user mana pun —
	// gejalanya kolom relasi user↔database tampil kosong padahal datanya ada.
	cmd := "sudo -u postgres psql -v ON_ERROR_STOP=1 -d " + shellQuote(db) +
		" -At -F " + shellQuote("\t") + " -c " + shellQuote(sql)
	res, err := s.run(access, cmd, 20*time.Second)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = "perintah PostgreSQL gagal"
		}
		return "", mapWebsiteError(errFmt("%s", msg))
	}
	return res.Stdout, nil
}

// DBListDatabases mengembalikan daftar database di server (menyaring
// database sistem bawaan).
func (s *Service) DBListDatabases(serverID, engine string) ([]DBDatabaseInfo, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	out := make([]DBDatabaseInfo, 0)
	if engine == "mysql" {
		res, err := s.runMySQL(access, "SHOW DATABASES")
		if err != nil {
			return nil, err
		}
		skip := map[string]bool{"information_schema": true, "mysql": true, "performance_schema": true, "sys": true}
		for _, line := range strings.Split(res, "\n") {
			name := strings.TrimSpace(line)
			if name == "" || skip[name] {
				continue
			}
			out = append(out, DBDatabaseInfo{Name: name})
		}
		return out, nil
	}
	res, err := s.runPostgres(access, "", "SELECT datname FROM pg_database WHERE datistemplate = false AND datname != 'postgres'")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(res, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		out = append(out, DBDatabaseInfo{Name: name})
	}
	return out, nil
}

// mysqlHostsForUser mengembalikan semua host yang terdaftar untuk satu
// username MySQL — dipakai operasi yang secara UI berlaku "untuk user itu"
// (assign/revoke akses database, set owner) padahal grant MySQL selalu
// per-(user,host): tanpa ini, user yang punya 'x'@'localhost' dan
// 'x'@'127.0.0.1' cuma kebagian di salah satu host saja dan aksesnya
// terasa "kadang jalan kadang tidak" tergantung cara aplikasi connect.
func (s *Service) mysqlHostsForUser(access *websiteAccess, username string) ([]string, error) {
	res, err := s.runMySQL(access, "SELECT Host FROM mysql.user WHERE User = '"+mysqlEscape(username)+"'")
	if err != nil {
		return nil, err
	}
	hosts := make([]string, 0)
	for _, line := range strings.Split(res, "\n") {
		h := strings.TrimSpace(line)
		if h != "" {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		return nil, errFmt("user %q tidak ditemukan di server ini", username)
	}
	return hosts, nil
}

// resolveMySQLTargetHosts memilih host mana yang jadi sasaran satu operasi:
// host eksplisit kalau diisi, atau SEMUA host user itu kalau dikosongkan.
func (s *Service) resolveMySQLTargetHosts(access *websiteAccess, username, host string) ([]string, error) {
	if h := strings.TrimSpace(host); h != "" {
		return []string{h}, nil
	}
	return s.mysqlHostsForUser(access, username)
}

// DBCreateDatabase membuat database baru, opsional langsung dengan pemilik
// (req.Owner). MySQL tidak punya konsep owner database — didekati dengan
// GRANT ALL PRIVILEGES ON <db>.* ke user tsb (di semua host-nya kalau
// OwnerHost dikosongkan). PostgreSQL memakai OWNER asli, DAN sekalian
// menutup CONNECT dari PUBLIC lalu memberikannya HANYA ke owner — tanpa itu
// PostgreSQL secara bawaan mengizinkan SEMUA role connect ke database baru,
// sehingga "database milik user A" tidak benar-benar terbatas ke A (dan
// daftar database per-user jadi tidak berarti apa-apa). Revoke ini hanya
// untuk database yang BARU dibuat di sini — database lama tidak disentuh.
func (s *Service) DBCreateDatabase(req DBCreateDatabaseRequest) error {
	engine, err := normalizeDBEngine(req.Engine)
	if err != nil {
		return err
	}
	name, err := normalizeDBName(req.Name)
	if err != nil {
		return err
	}
	owner := strings.TrimSpace(req.Owner)
	if owner != "" {
		if owner, err = normalizeDBUsername(owner); err != nil {
			return err
		}
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}
	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)

	if engine == "mysql" {
		charset := strings.TrimSpace(req.Encoding)
		if charset == "" {
			charset = "utf8mb4"
		}
		if _, err := s.runMySQL(access, "CREATE DATABASE `"+name+"` CHARACTER SET `"+charset+"`"); err != nil {
			return err
		}
		if owner == "" {
			return nil
		}
		hosts, err := s.resolveMySQLTargetHosts(access, owner, req.OwnerHost)
		if err != nil {
			return err
		}
		for _, h := range hosts {
			if _, err := s.runMySQL(access, "GRANT ALL PRIVILEGES ON `"+name+"`.* TO '"+owner+"'@'"+mysqlEscape(h)+"'"); err != nil {
				return err
			}
		}
		_, err = s.runMySQL(access, "FLUSH PRIVILEGES")
		return err
	}

	create := `CREATE DATABASE "` + name + `"`
	if owner != "" {
		create += ` OWNER "` + owner + `"`
	}
	if _, err := s.runPostgres(access, "", create); err != nil {
		return err
	}
	if owner == "" {
		return nil
	}
	scope := `REVOKE CONNECT ON DATABASE "` + name + `" FROM PUBLIC; ` +
		`GRANT CONNECT ON DATABASE "` + name + `" TO "` + owner + `"`
	if _, err := s.runPostgres(access, "", scope); err != nil {
		return err
	}
	// Schema public di database baru: pastikan owner benar-benar bisa bikin
	// tabel di sana (di PostgreSQL <15 schema public dimiliki postgres dan
	// owner database tidak otomatis dapat CREATE).
	_, err = s.runPostgres(access, name, `ALTER SCHEMA public OWNER TO "`+owner+`"`)
	return err
}

// DBDropDatabase menghapus satu database beserta seluruh isinya. PERMANEN
// — pemanggil (UI) yang wajib mengkonfirmasi dulu.
//
// PostgreSQL menolak DROP DATABASE selama masih ada koneksi ke database
// itu, dan poinhost sendiri kemungkinan besar MASIH memegang koneksi
// Explore ke sana (lihat dbconnpool.go), jadi: koneksi cache dibuang dulu,
// lalu dipakai WITH (FORCE) (PostgreSQL 13+) untuk memutus sisa sesi lain;
// kalau server-nya lebih tua dan menolak sintaks itu, diulang tanpa FORCE.
func (s *Service) DBDropDatabase(serverID, engine, name string) error {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return err
	}
	dbName, err := normalizeDBName(name)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}
	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	if engine == "mysql" {
		_, err := s.runMySQL(access, "DROP DATABASE `"+dbName+"`")
		return err
	}

	s.evictDBConnsWithPrefix(dbConnCacheKeyPrefix("postgresql", serverID, ""))
	if _, err := s.runPostgres(access, "", `DROP DATABASE "`+dbName+`" WITH (FORCE)`); err != nil {
		if _, retryErr := s.runPostgres(access, "", `DROP DATABASE "`+dbName+`"`); retryErr != nil {
			return retryErr
		}
	}
	return nil
}

// DBDropUser menghapus satu user database. Untuk MySQL, host kosong berarti
// SEMUA host user itu (baris user di UI memang sudah digabung per-host).
//
// PostgreSQL menolak DROP ROLE selama role itu masih memiliki objek atau
// memegang privilege di suatu database. Menghapus objeknya jelas TIDAK
// boleh dilakukan diam-diam (itu data user), jadi yang dipakai resep aman
// standar PostgreSQL di tiap database: REASSIGN OWNED (kepemilikan pindah
// ke postgres, tabelnya TETAP ADA) lalu DROP OWNED (tinggal mencabut
// privilege, karena sesudah reassign role ini sudah tidak memiliki apa pun).
func (s *Service) DBDropUser(serverID, engine, username, host string) error {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return err
	}
	user, err := normalizeDBUsername(username)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}
	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	// Kredensial lokal + koneksi Explore untuk user ini tidak ada gunanya
	// lagi begitu user-nya hilang di server.
	s.evictDBConnsWithPrefix(dbConnCacheKeyPrefix(engine, serverID, user))

	if engine == "mysql" {
		hosts, err := s.resolveMySQLTargetHosts(access, user, host)
		if err != nil {
			return err
		}
		for _, h := range hosts {
			if _, err := s.runMySQL(access, "DROP USER '"+user+"'@'"+mysqlEscape(h)+"'"); err != nil {
				return err
			}
			_ = s.ForgetDBCredential(serverID, engine, user, h)
		}
		_, err = s.runMySQL(access, "FLUSH PRIVILEGES")
		return err
	}

	databases, err := s.DBListDatabases(serverID, "postgresql")
	if err != nil {
		return err
	}
	for _, d := range databases {
		stmts := `REASSIGN OWNED BY "` + user + `" TO postgres; DROP OWNED BY "` + user + `"`
		if _, err := s.runPostgres(access, d.Name, stmts); err != nil {
			return err
		}
	}
	if _, err := s.runPostgres(access, "", `DROP ROLE "`+user+`"`); err != nil {
		return err
	}
	_ = s.ForgetDBCredential(serverID, engine, user, "")
	return nil
}

// DBGrantDatabaseUser memberi satu user akses ke satu database ("assign
// user" di daftar database). Privileges kosong = akses baca+tulis standar.
func (s *Service) DBGrantDatabaseUser(req DBDatabaseUserRequest) error {
	engine, err := normalizeDBEngine(req.Engine)
	if err != nil {
		return err
	}
	dbName, err := normalizeDBName(req.Database)
	if err != nil {
		return err
	}
	username, err := normalizeDBUsername(req.Username)
	if err != nil {
		return err
	}
	privileges := req.Privileges
	if len(privileges) == 0 {
		privileges = []string{"read", "write"}
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}
	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)

	if engine == "mysql" {
		hosts, err := s.resolveMySQLTargetHosts(access, username, req.Host)
		if err != nil {
			return err
		}
		priv := mysqlPrivilegeSQL(privileges)
		for _, h := range hosts {
			if _, err := s.runMySQL(access, "GRANT "+priv+" ON `"+dbName+"`.* TO '"+username+"'@'"+mysqlEscape(h)+"'"); err != nil {
				return err
			}
		}
		_, err = s.runMySQL(access, "FLUSH PRIVILEGES")
		return err
	}

	// applyGrants sudah sekalian memberi CONNECT ke database sasaran.
	return s.applyGrants(access, engine, username, "", false, []string{dbName}, privileges)
}

// DBRevokeDatabaseUser mencabut akses satu user dari satu database.
func (s *Service) DBRevokeDatabaseUser(req DBDatabaseUserRequest) error {
	engine, err := normalizeDBEngine(req.Engine)
	if err != nil {
		return err
	}
	dbName, err := normalizeDBName(req.Database)
	if err != nil {
		return err
	}
	username, err := normalizeDBUsername(req.Username)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}
	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)

	if engine == "mysql" {
		hosts, err := s.resolveMySQLTargetHosts(access, username, req.Host)
		if err != nil {
			return err
		}
		for _, h := range hosts {
			if _, err := s.runMySQL(access, "REVOKE ALL PRIVILEGES ON `"+dbName+"`.* FROM '"+username+"'@'"+mysqlEscape(h)+"'"); err != nil {
				return err
			}
		}
		_, err = s.runMySQL(access, "FLUSH PRIVILEGES")
		return err
	}

	stmts := []string{
		`ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON TABLES FROM "` + username + `"`,
		`ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON SEQUENCES FROM "` + username + `"`,
		`REVOKE ALL ON ALL TABLES IN SCHEMA public FROM "` + username + `"`,
		`REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM "` + username + `"`,
		`REVOKE ALL ON SCHEMA public FROM "` + username + `"`,
	}
	if _, err := s.runPostgres(access, dbName, strings.Join(stmts, "; ")); err != nil {
		return err
	}
	_, err = s.runPostgres(access, "", `REVOKE CONNECT ON DATABASE "`+dbName+`" FROM "`+username+`"`)
	return err
}

var mysqlGranteeRE = regexp.MustCompile(`^'(.*)'@'(.*)'$`)

// mysqlGlobalGrantQuery mencari user yang punya privilege GLOBAL (ON *.*).
// Filter USAGE-nya WAJIB — lihat komentar mysqlUserDatabaseAccess dan
// TestMySQLGlobalGrantDetectionExcludesUsage.
const mysqlGlobalGrantQuery = "SELECT DISTINCT GRANTEE, PRIVILEGE_TYPE FROM information_schema.USER_PRIVILEGES WHERE PRIVILEGE_TYPE <> 'USAGE'"

// privilegeKeyForSQL memetakan nama privilege dari katalog server BALIK ke
// key yang dipakai UI (lihat DBPrivilegeOptions) — arah kebalikan dari
// mysqlPrivilegeSQL/postgresTablePrivilegeSQL. Dipakai supaya dialog "Ubah
// privilege" bisa menampilkan centang sesuai kondisi nyata di server.
// Berlaku untuk kedua engine: nama privilege SQL-nya memang sama.
func privilegeKeyForSQL(name string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "SELECT":
		return "read", true
	case "INSERT", "UPDATE", "DELETE":
		return "write", true
	case "CREATE":
		return "create", true
	case "ALTER":
		return "alter", true
	case "DROP", "TRUNCATE":
		return "drop", true
	case "EXECUTE":
		return "execute", true
	}
	return "", false
}

// sortPrivilegeKeys mengurutkan key privilege mengikuti urutan tampilan di
// UI (DBPrivilegeOptions), bukan alfabetis — supaya daftarnya terbaca sama
// di mana pun ditampilkan.
func sortPrivilegeKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for _, opt := range DBPrivilegeOptions() {
		if set[opt.Key] {
			out = append(out, opt.Key)
		}
	}
	return out
}

// parseMySQLGrantee mem-parsing kolom GRANTEE information_schema (format
// `'user'@'host'`, kutip tunggal di dalam string aslinya di-escape jadi `”`
// oleh MySQL sendiri) jadi (username, host) terpisah.
func parseMySQLGrantee(grantee string) (username, host string, ok bool) {
	m := mysqlGranteeRE.FindStringSubmatch(grantee)
	if m == nil {
		return "", "", false
	}
	unescape := func(s string) string { return strings.ReplaceAll(s, "''", "'") }
	return unescape(m[1]), unescape(m[2]), true
}

// mysqlUserDatabaseAccess mengembalikan (a) set username yang punya grant
// GLOBAL (ON *.*, berlaku ke semua database) dan (b) map username -> set
// nama database yang punya grant EKSPLISIT di database itu (union lintas
// semua host user tsb — ditampilkan sebagai satu daftar per username,
// konsisten dengan DBUserInfo yang sudah digabung per-host). Query ringan
// terhadap information_schema, BUKAN "SHOW GRANTS" per user (yang perlu
// satu round-trip exec per user — tidak scalable kalau user banyak).
//
// PENTING (bug nyata yang pernah terjadi): USER_PRIVILEGES TIDAK boleh
// dibaca apa adanya sebagai "punya akses semua database". SETIAP user MySQL
// selalu punya baris `USAGE ON *.*` di sana — itu representasi "tidak punya
// privilege apa-apa", bukan akses global. Tanpa filter ini SEMUA user
// tampil "Semua database". Yang benar-benar berarti global cuma privilege
// selain USAGE.
func (s *Service) mysqlUserDatabaseAccess(access *websiteAccess) (allDBUsers map[string]bool, dbByUser map[string]map[string]bool, privByUser map[string]map[string]bool, err error) {
	allDBUsers = map[string]bool{}
	dbByUser = map[string]map[string]bool{}
	privByUser = map[string]map[string]bool{}

	notePriv := func(username, privilegeType string) {
		if key, ok := privilegeKeyForSQL(privilegeType); ok {
			if privByUser[username] == nil {
				privByUser[username] = map[string]bool{}
			}
			privByUser[username][key] = true
		}
	}

	globalRes, err := s.runMySQL(access, mysqlGlobalGrantQuery)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, line := range strings.Split(globalRes, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		username, _, ok := parseMySQLGrantee(parts[0])
		if !ok {
			continue
		}
		allDBUsers[username] = true
		if len(parts) == 2 {
			notePriv(username, parts[1])
		}
	}

	// Grant level-database (mysql.db) DAN level-tabel (mysql.tables_priv) —
	// dua-duanya berarti "user ini bisa masuk ke database tsb".
	schemaRes, err := s.runMySQL(access, `
		SELECT DISTINCT GRANTEE, TABLE_SCHEMA, PRIVILEGE_TYPE FROM information_schema.SCHEMA_PRIVILEGES WHERE PRIVILEGE_TYPE <> 'USAGE'
		UNION
		SELECT DISTINCT GRANTEE, TABLE_SCHEMA, PRIVILEGE_TYPE FROM information_schema.TABLE_PRIVILEGES WHERE PRIVILEGE_TYPE <> 'USAGE'`)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, line := range strings.Split(schemaRes, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		username, _, ok := parseMySQLGrantee(parts[0])
		if !ok {
			continue
		}
		if dbByUser[username] == nil {
			dbByUser[username] = map[string]bool{}
		}
		dbByUser[username][parts[1]] = true
		if len(parts) == 3 {
			notePriv(username, parts[2])
		}
	}
	return allDBUsers, dbByUser, privByUser, nil
}

// pgRoleAccessSQL mencari role yang punya hak apa pun di SATU database:
// pemilik database, pemilik/penerima grant di schema public, atau penerima
// grant di tabel/view mana pun. Sengaja lewat aclexplode di katalog (bukan
// information_schema.role_table_grants) karena view information_schema
// hanya menampilkan baris yang grantor/grantee-nya "currently enabled
// role" — dijalankan sebagai postgres hasilnya bisa tidak lengkap; katalog
// pg_class/pg_namespace selalu lengkap apa adanya.
// Kolom kedua = privilege_type (kosong untuk baris kepemilikan: pemilik
// otomatis punya semua hak, ditandai khusus oleh pemanggil).
const pgRoleAccessSQL = `
SELECT DISTINCT pg_get_userbyid(a.grantee), a.privilege_type
  FROM pg_namespace n, LATERAL aclexplode(n.nspacl) a
 WHERE n.nspname = 'public' AND a.grantee <> 0
UNION
SELECT DISTINCT pg_get_userbyid(a.grantee), a.privilege_type
  FROM pg_class c, LATERAL aclexplode(c.relacl) a
 WHERE c.relkind IN ('r','v','m','p','S') AND a.grantee <> 0
UNION
SELECT DISTINCT pg_get_userbyid(a.grantee), a.privilege_type
  FROM pg_proc p, LATERAL aclexplode(p.proacl) a
 WHERE a.grantee <> 0
UNION
SELECT pg_get_userbyid(n.nspowner), '' FROM pg_namespace n WHERE n.nspname = 'public'
UNION
SELECT pg_get_userbyid(d.datdba), '' FROM pg_database d WHERE d.datname = current_database()`

// postgresUserDatabaseAccess mengembalikan map rolname -> set nama database
// yang bisa dikelola role itu. PostgreSQL TIDAK punya grant global tunggal
// setara *.* MySQL, dan koneksi psql selalu terikat SATU database, jadi
// satu-satunya cara tahu "role ini bisa akses database mana saja" adalah
// tanya LANGSUNG ke tiap database (satu exec per database).
//
// CONNECT sengaja TIDAK dipakai sebagai sinyal: secara bawaan PostgreSQL
// memberi CONNECT ke PUBLIC untuk semua database, jadi kalau itu dipakai
// SEMUA role akan tampak bisa akses SEMUA database (persis masalah yang
// dilaporkan di sisi MySQL). Yang dipakai: kepemilikan + grant eksplisit.
func (s *Service) postgresUserDatabaseAccess(access *websiteAccess, databases []DBDatabaseInfo) (dbByUser, privByUser map[string]map[string]bool, err error) {
	dbByUser = map[string]map[string]bool{}
	privByUser = map[string]map[string]bool{}
	for _, d := range databases {
		res, err := s.runPostgres(access, d.Name, pgRoleAccessSQL)
		if err != nil {
			return nil, nil, err
		}
		for _, line := range strings.Split(res, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			role := strings.TrimSpace(parts[0])
			if role == "" || role == "postgres" {
				continue
			}
			if dbByUser[role] == nil {
				dbByUser[role] = map[string]bool{}
			}
			dbByUser[role][d.Name] = true

			if privByUser[role] == nil {
				privByUser[role] = map[string]bool{}
			}
			privilegeType := ""
			if len(parts) == 2 {
				privilegeType = strings.TrimSpace(parts[1])
			}
			if privilegeType == "" {
				// Baris kepemilikan: pemilik schema/database praktis bisa
				// melakukan apa pun di sana.
				for _, opt := range DBPrivilegeOptions() {
					privByUser[role][opt.Key] = true
				}
				continue
			}
			if key, ok := privilegeKeyForSQL(privilegeType); ok {
				privByUser[role][key] = true
			}
		}
	}
	return dbByUser, privByUser, nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DBListUsers mengembalikan daftar user database (bukan user sistem/role
// bawaan) — satu baris per username. Untuk MySQL, user yang sama terdaftar
// di beberapa host (mis. 'usera'@'localhost' dan 'usera'@'127.0.0.1')
// DIGABUNG jadi satu baris (lihat DBUserInfo), dan setiap baris disertai
// daftar database yang benar-benar bisa diakses (bukan cuma yang dipilih
// terakhir kali di form — introspeksi grants aktual saat ini), untuk KEDUA
// engine.
func (s *Service) DBListUsers(serverID, engine string) ([]DBUserInfo, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	out := make([]DBUserInfo, 0)
	if engine == "mysql" {
		res, err := s.runMySQL(access, "SELECT User, Host FROM mysql.user WHERE User NOT IN ('root','mysql.sys','mysql.session','mysql.infoschema','mariadb.sys') AND User != ''")
		if err != nil {
			return nil, err
		}
		hostsByUser := map[string]map[string]bool{}
		order := make([]string, 0)
		for _, line := range strings.Split(res, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) != 2 {
				continue
			}
			username, host := parts[0], parts[1]
			if hostsByUser[username] == nil {
				hostsByUser[username] = map[string]bool{}
				order = append(order, username)
			}
			hostsByUser[username][host] = true
		}

		allDBUsers, dbByUser, privByUser, err := s.mysqlUserDatabaseAccess(access)
		if err != nil {
			return nil, err
		}

		for _, username := range order {
			hosts := sortedKeys(hostsByUser[username])
			u := DBUserInfo{
				Username:     username,
				Host:         strings.Join(hosts, ", "),
				Hosts:        hosts,
				Databases:    sortedKeys(dbByUser[username]),
				AllDatabases: allDBUsers[username],
				Privileges:   sortPrivilegeKeys(privByUser[username]),
			}
			out = append(out, u)
		}
		return out, nil
	}

	res, err := s.runPostgres(access, "", "SELECT rolname FROM pg_roles WHERE rolname NOT LIKE 'pg\\_%' AND rolname != 'postgres' AND rolcanlogin = true")
	if err != nil {
		return nil, err
	}
	databases, err := s.DBListDatabases(serverID, "postgresql")
	if err != nil {
		return nil, err
	}
	dbByUser, privByUser, err := s.postgresUserDatabaseAccess(access, databases)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(res, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		dbs := sortedKeys(dbByUser[name])
		out = append(out, DBUserInfo{
			Username:     name,
			Databases:    dbs,
			AllDatabases: len(databases) > 0 && len(dbs) == len(databases),
			Privileges:   sortPrivilegeKeys(privByUser[name]),
		})
	}
	return out, nil
}

// DBCreateUser membuat user database baru + langsung menerapkan grants awal.
func (s *Service) DBCreateUser(req DBCreateUserRequest) error {
	engine, err := normalizeDBEngine(req.Engine)
	if err != nil {
		return err
	}
	username, err := normalizeDBUsername(req.Username)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.Password) == "" {
		return errFmt("password wajib diisi")
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}
	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)

	if engine == "mysql" {
		host := strings.TrimSpace(req.Host)
		if host == "" {
			host = "%"
		}
		sql := "CREATE USER IF NOT EXISTS '" + username + "'@'" + host + "' IDENTIFIED BY '" + mysqlEscape(req.Password) + "'; FLUSH PRIVILEGES;"
		if _, err := s.runMySQL(access, sql); err != nil {
			return err
		}
	} else {
		sql := `CREATE ROLE "` + username + `" LOGIN PASSWORD '` + pgEscape(req.Password) + `'`
		if _, err := s.runPostgres(access, "", sql); err != nil {
			return err
		}
	}
	if err := s.applyGrants(access, engine, username, req.Host, req.AllDBs, req.Databases, req.Privileges); err != nil {
		return err
	}
	if req.SaveCredential {
		// Best-effort — kegagalan simpan kredensial TIDAK membatalkan user
		// yang sudah berhasil dibuat; user masih bisa simpan manual lewat
		// SaveDBCredential nanti kalau ini gagal (mis. vault tidak siap).
		// Didukung untuk kedua engine sejak PostgreSQL Explore ditambahkan
		// (lihat pgexplore.go) — sebelumnya cuma MySQL.
		_, _ = s.SaveDBCredential(SaveDBCredentialRequest{
			ServerID: req.ServerID, Engine: engine, Username: username,
			Host: req.Host, Password: req.Password, Verify: false,
		})
	}
	return nil
}

// DBSetGrants menerapkan ulang grants untuk user yang sudah ada (tanpa mengubah password).
func (s *Service) DBSetGrants(req DBGrantsRequest) error {
	engine, err := normalizeDBEngine(req.Engine)
	if err != nil {
		return err
	}
	username, err := normalizeDBUsername(req.Username)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}
	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)
	return s.applyGrants(access, engine, username, req.Host, req.AllDBs, req.Databases, req.Privileges)
}

// applyGrants MENYETEL privilege user ke persis daftar yang diberikan —
// bukan sekadar menambahkan. Tiap sasaran dicabut dulu (REVOKE ALL) baru
// diberi ulang, karena GRANT saja tidak pernah bisa MENURUNKAN hak: tanpa
// revoke, menghapus centang "Tulis" lalu menerapkan ulang tidak berefek apa
// pun dan user tetap bisa menulis (bug nyata yang bikin fitur "terapkan
// ulang grants" terasa tidak berguna).
//
// REVOKE-nya sengaja best-effort: MySQL mengembalikan error kalau grant
// yang dicabut memang belum pernah ada (mis. user yang baru dibuat), dan
// itu bukan kegagalan — targetnya "akhirnya privilege user = daftar ini".
func (s *Service) applyGrants(access *websiteAccess, engine, username, host string, allDBs bool, databases, privileges []string) error {
	if engine == "mysql" {
		if host == "" {
			host = "%"
		}
		priv := mysqlPrivilegeSQL(privileges)
		if allDBs {
			_, _ = s.runMySQL(access, "REVOKE ALL PRIVILEGES ON *.* FROM '"+username+"'@'"+host+"';")
			_, err := s.runMySQL(access, "GRANT "+priv+" ON *.* TO '"+username+"'@'"+host+"'; FLUSH PRIVILEGES;")
			return err
		}
		for _, db := range databases {
			db, err := normalizeDBName(db)
			if err != nil {
				return err
			}
			_, _ = s.runMySQL(access, "REVOKE ALL PRIVILEGES ON `"+db+"`.* FROM '"+username+"'@'"+host+"';")
			if _, err := s.runMySQL(access, "GRANT "+priv+" ON `"+db+"`.* TO '"+username+"'@'"+host+"';"); err != nil {
				return err
			}
		}
		_, err := s.runMySQL(access, "FLUSH PRIVILEGES;")
		return err
	}

	// Satu daftar privilege dipecah per jenis objek — lihat catatan panjang
	// di postgresPrivilegeParts: menempelkan daftar yang sama ke tabel,
	// sequence, dan schema sekaligus persis yang dulu memicu error
	// `invalid privilege type CREATE for relation`.
	tablePriv := postgresTablePrivilegeSQL(privileges)
	seqPriv := postgresSequencePrivilegeSQL(privileges)
	schemaPriv := "USAGE"
	if hasPrivilegeKey(privileges, "create") {
		schemaPriv = "USAGE, CREATE"
	}

	targets := databases
	if allDBs {
		dbs, err := s.DBListDatabases(access.serverID, "postgresql")
		if err != nil {
			return err
		}
		targets = nil
		for _, d := range dbs {
			targets = append(targets, d.Name)
		}
	}
	for _, db := range targets {
		db, err := normalizeDBName(db)
		if err != nil {
			return err
		}
		// CONNECT diberikan eksplisit: database yang dibuat lewat poinhost
		// menutup CONNECT dari PUBLIC (lihat DBCreateDatabase), jadi tanpa
		// baris ini grant tabel/schema-nya benar tapi user tetap tidak bisa
		// masuk ke database-nya sama sekali.
		if _, err := s.runPostgres(access, "", `GRANT CONNECT ON DATABASE "`+db+`" TO "`+username+`"`); err != nil {
			return err
		}
		// Cabut dulu supaya hasil akhirnya PERSIS daftar privilege yang
		// diminta (GRANT saja tidak pernah bisa menurunkan hak). Di
		// PostgreSQL REVOKE atas hak yang belum pernah diberikan bukan
		// error, jadi aman juga untuk user yang baru dibuat.
		stmts := []string{
			`ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON TABLES FROM "` + username + `"`,
			`ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON SEQUENCES FROM "` + username + `"`,
			`ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON FUNCTIONS FROM "` + username + `"`,
			`REVOKE ALL ON ALL TABLES IN SCHEMA public FROM "` + username + `"`,
			`REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM "` + username + `"`,
			`REVOKE ALL ON ALL FUNCTIONS IN SCHEMA public FROM "` + username + `"`,
			`REVOKE ALL ON SCHEMA public FROM "` + username + `"`,

			`GRANT ` + schemaPriv + ` ON SCHEMA public TO "` + username + `"`,
			`GRANT ` + tablePriv + ` ON ALL TABLES IN SCHEMA public TO "` + username + `"`,
			`GRANT ` + seqPriv + ` ON ALL SEQUENCES IN SCHEMA public TO "` + username + `"`,
			`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ` + tablePriv + ` ON TABLES TO "` + username + `"`,
			`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ` + seqPriv + ` ON SEQUENCES TO "` + username + `"`,
		}
		if hasPrivilegeKey(privileges, "execute") {
			stmts = append(stmts,
				`GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO "`+username+`"`,
				`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT EXECUTE ON FUNCTIONS TO "`+username+`"`,
			)
		}
		if _, err := s.runPostgres(access, db, strings.Join(stmts, "; ")); err != nil {
			return err
		}
	}
	return nil
}

// mysqlEscape/pgEscape membungkus nilai literal SQL (password) — pemakaian
// SATU-nya di sini adalah literal string di antara kutip tunggal SQL, jadi
// cukup escape kutip tunggal SQL (bukan shell — shellQuote yang membungkus
// keseluruhan perintah exec sudah menangani lapisan shell-nya).
func mysqlEscape(s string) string { return strings.ReplaceAll(s, "'", "''") }
func pgEscape(s string) string    { return strings.ReplaceAll(s, "'", "''") }
