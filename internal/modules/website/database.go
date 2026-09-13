package website

import (
	"regexp"
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
}

// DBDatabaseInfo satu database di server.
type DBDatabaseInfo struct {
	Name    string `json:"name"`
	Charset string `json:"charset,omitempty"`
}

// DBUserInfo satu user database.
type DBUserInfo struct {
	Username string `json:"username"`
	Host     string `json:"host,omitempty"` // relevan untuk MySQL saja
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

// DBCreateDatabaseRequest membuat database baru.
type DBCreateDatabaseRequest struct {
	ServerID string `json:"serverId"`
	Engine   string `json:"engine"`
	Domain   string `json:"domain,omitempty"` // breadcrumb UI saja
	Name     string `json:"name"`
	Encoding string `json:"encoding,omitempty"`
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

// postgresPrivilegeSQL memetakan key privilege ke daftar hak PostgreSQL
// (dipakai per-tabel via ALL TABLES IN SCHEMA, dst).
func postgresPrivilegeSQL(keys []string) string {
	set := map[string]string{
		"read": "SELECT", "write": "INSERT, UPDATE, DELETE", "create": "CREATE",
		"alter": "", "drop": "", "execute": "EXECUTE",
	}
	var parts []string
	for _, k := range keys {
		if v, ok := set[k]; ok && v != "" {
			parts = append(parts, v)
		}
	}
	if len(parts) == 0 {
		return "SELECT"
	}
	return strings.Join(parts, ", ")
}

const dbStatusScriptMySQL = `BIN=$(command -v mysql 2>/dev/null || command -v mariadb 2>/dev/null || true)
echo "BIN=$BIN"
if [ -n "$BIN" ]; then "$BIN" --version 2>&1 | head -1; fi
echo "ACTIVE=$(systemctl is-active mysql 2>/dev/null || systemctl is-active mariadb 2>/dev/null || echo unknown)"
echo "ENABLED=$(systemctl is-enabled mysql 2>/dev/null || systemctl is-enabled mariadb 2>/dev/null || echo unknown)"`

const dbStatusScriptPostgres = `BIN=$(command -v psql 2>/dev/null || true)
echo "BIN=$BIN"
if [ -n "$BIN" ]; then "$BIN" --version 2>&1 | head -1; fi
echo "ACTIVE=$(systemctl is-active postgresql 2>/dev/null || echo unknown)"
echo "ENABLED=$(systemctl is-enabled postgresql 2>/dev/null || echo unknown)"`

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
		case st.Version == "" && line != "" && !strings.Contains(line, "="):
			st.Version = line
		}
	}
	return st, nil
}

// DBStart mengaktifkan service database yang sudah terpasang tapi tidak aktif.
func (s *Service) DBStart(serverID, engine string) (*DBEngineStatus, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	svc := "mariadb"
	if engine == "postgresql" {
		svc = "postgresql"
	}
	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)
	script := `set -e
systemctl enable ` + svc + ` 2>/dev/null || systemctl enable mysql 2>/dev/null || true
systemctl start ` + svc + ` 2>&1 || systemctl start mysql 2>&1 || systemctl restart ` + svc + ` 2>&1`
	if _, err := s.run(access, script, 20*time.Second); err != nil {
		return nil, err
	}
	return s.DBStatus(serverID, engine)
}

// dbInstallScript skrip instalasi per engine+package manager. MariaDB
// dipakai sebagai pengganti MySQL di apt/dnf/yum (drop-in compatible,
// tersedia langsung dari repo distro tanpa perlu repo pihak ketiga).
func dbInstallScript(engine, pm string) (string, bool) {
	// dockerAccess dijalankan di AKHIR instalasi (bukan langkah terpisah
	// yang bisa lupa dipanggil) — supaya "instalasi MySQL/PostgreSQL" dan
	// "bisa langsung diakses dari container Docker di host yang sama"
	// selalu satu paket yang sama, sesuai yang diminta: user tidak perlu
	// tahu ada langkah tambahan sama sekali. Skrip yang SAMA persis dipakai
	// EnsureDBDockerAccess untuk instalasi yang sudah ada sebelumnya (lihat
	// dockeraccess.go) — jangan duplikasi logikanya di sini.
	dockerAccess := dbDockerAccessApplyScript(engine, pm)

	if engine == "mysql" {
		switch pm {
		case "apt":
			return `set -e
` + aptWaitLock + `
apt-get install -y mariadb-server
systemctl enable mariadb
systemctl start mariadb
mysql -e "CREATE USER IF NOT EXISTS 'root'@'127.0.0.1' IDENTIFIED BY ''; GRANT ALL ON *.* TO 'root'@'127.0.0.1' WITH GRANT OPTION; FLUSH PRIVILEGES;" 2>/dev/null || true
` + dockerAccess + `
echo ">> MariaDB (MySQL) terpasang, siap diakses dari container Docker di host ini."
`, true
		case "dnf", "yum":
			return `set -e
` + pm + ` install -y mariadb-server
systemctl enable mariadb
systemctl start mariadb
mysql -e "CREATE USER IF NOT EXISTS 'root'@'127.0.0.1' IDENTIFIED BY ''; GRANT ALL ON *.* TO 'root'@'127.0.0.1' WITH GRANT OPTION; FLUSH PRIVILEGES;" 2>/dev/null || true
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

func (s *Service) runMySQL(access *websiteAccess, sql string) (string, error) {
	res, err := s.run(access, "mysql -h 127.0.0.1 -u root --batch --skip-column-names -e "+shellQuote(sql), 20*time.Second)
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
	cmd := "sudo -u postgres psql -v ON_ERROR_STOP=1 -d " + shellQuote(db) + " -At -c " + shellQuote(sql)
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

// DBCreateDatabase membuat database baru.
func (s *Service) DBCreateDatabase(req DBCreateDatabaseRequest) error {
	engine, err := normalizeDBEngine(req.Engine)
	if err != nil {
		return err
	}
	name, err := normalizeDBName(req.Name)
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
		charset := strings.TrimSpace(req.Encoding)
		if charset == "" {
			charset = "utf8mb4"
		}
		_, err := s.runMySQL(access, "CREATE DATABASE `"+name+"` CHARACTER SET `"+charset+"`")
		return err
	}
	_, err = s.runPostgres(access, "", `CREATE DATABASE "`+name+`"`)
	return err
}

// DBListUsers mengembalikan daftar user database (bukan user sistem/role bawaan).
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
		for _, line := range strings.Split(res, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) != 2 {
				continue
			}
			out = append(out, DBUserInfo{Username: parts[0], Host: parts[1]})
		}
		return out, nil
	}
	res, err := s.runPostgres(access, "", "SELECT rolname FROM pg_roles WHERE rolname NOT LIKE 'pg\\_%' AND rolname != 'postgres' AND rolcanlogin = true")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(res, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		out = append(out, DBUserInfo{Username: name})
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

func (s *Service) applyGrants(access *websiteAccess, engine, username, host string, allDBs bool, databases, privileges []string) error {
	if engine == "mysql" {
		if host == "" {
			host = "%"
		}
		priv := mysqlPrivilegeSQL(privileges)
		if allDBs {
			_, err := s.runMySQL(access, "GRANT "+priv+" ON *.* TO '"+username+"'@'"+host+"'; FLUSH PRIVILEGES;")
			return err
		}
		for _, db := range databases {
			db, err := normalizeDBName(db)
			if err != nil {
				return err
			}
			if _, err := s.runMySQL(access, "GRANT "+priv+" ON `"+db+"`.* TO '"+username+"'@'"+host+"';"); err != nil {
				return err
			}
		}
		_, err := s.runMySQL(access, "FLUSH PRIVILEGES;")
		return err
	}

	priv := postgresPrivilegeSQL(privileges)
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
		stmts := []string{
			`GRANT ALL ON SCHEMA public TO "` + username + `"`,
			`GRANT ` + priv + ` ON ALL TABLES IN SCHEMA public TO "` + username + `"`,
			`GRANT ` + priv + ` ON ALL SEQUENCES IN SCHEMA public TO "` + username + `"`,
			`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ` + priv + ` ON TABLES TO "` + username + `"`,
			`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ` + priv + ` ON SEQUENCES TO "` + username + `"`,
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
