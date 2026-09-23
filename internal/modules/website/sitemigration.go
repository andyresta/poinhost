package website

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// File ini adalah seam khusus modul migrasi website antar server
// (internal/modules/sitexfer): satu-satunya titik di luar paket ini yang
// boleh membaca/menulis vhost, user database, akun SFTP, dan cron milik
// satu domain untuk dipindah ke server lain — supaya format vhost, script
// akun SFTP, dan format /etc/cron.d/poinhost tidak pernah diduplikasi di
// paket lain dan tidak bisa mendrift dari yang ditulis fitur Website biasa.
//
// Prinsip yang dipegang di sini: RAHASIA TIDAK PERNAH DIKETAHUI PLAINTEXT-NYA.
// Password user database dan akun SFTP dipindah dalam bentuk HASH yang
// sudah tersimpan di server asal (mysql.user / pg_authid / /etc/shadow),
// lalu dipasang apa adanya di tujuan — hasilnya password yang sama persis,
// tanpa poinhost perlu (atau bisa) tahu isinya.

// WrapStreamingCommand membungkus skrip yang MEMBACA DATA dari stdin sesi
// SSH (restore dump, `tar x`) lewat sudo tanpa merebut stdin itu — lihat
// wrapStreaming.
func (s *Service) WrapStreamingCommand(serverID, script string) (string, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return "", err
	}
	return access.wrapStreaming(script), nil
}

// RunRootScript menjalankan skrip pendek sebagai root (lewat lapisan sudo
// modul ini) dan mengembalikan stdout-nya — dipakai migrasi untuk langkah
// kecil (scan pemilik file, perbaikan kepemilikan, statistik verifikasi).
func (s *Service) RunRootScript(serverID, script string, timeout time.Duration) (string, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return "", err
	}
	res, err := s.run(access, script, timeout)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = fmt.Sprintf("perintah gagal (exit %d)", res.ExitCode)
		}
		return res.Stdout, mapWebsiteError(errFmt("%s", msg))
	}
	return res.Stdout, nil
}

// DomainsOnServer daftar vhost TERKINI (cache dibuang dulu) — dipakai
// migrasi untuk mendeteksi bentrok di server tujuan tepat sebelum menulis.
func (s *Service) DomainsOnServer(serverID string) ([]DomainInfo, error) {
	s.invalidateDomainCache(serverID)
	return s.listDomains(serverID)
}

// DomainBase folder yang dimiliki satu domain di disk: induk public_html
// untuk layout standar (/var/www/<domain>), atau document root itu sendiri
// untuk layout lain (mis. warisan homepoin/Plesk).
func DomainBase(info DomainInfo) string {
	return domainBaseFromRoot(info.Root)
}

// migratedVhostOptions vhost versi migrasi: SEMUA setelan domain ikut
// (root, proxy, aturan proxy per-path, status enabled), KECUALI dua hal
// yang memang harus disiapkan ulang di server tujuan:
//   - PHP dimatikan: socket PHP-FPM versi itu belum tentu ada di tujuan,
//     dan `nginx -t` TIDAK memeriksa socket — vhost-nya lolos tapi situsnya
//     502. File .php tetap aman karena vhost statis menolak .php (403).
//   - SSL dimatikan: file sertifikat /etc/letsencrypt/... tidak ada di
//     tujuan, dan `nginx -t` GAGAL kalau ssl_certificate menunjuk file yang
//     tidak ada — reload nginx untuk SEMUA situs di server tujuan ikut gagal.
//     Sertifikat diterbitkan ulang via certbot setelah DNS dipindah.
func migratedVhostOptions(info DomainInfo) *vhostOptions {
	opts := optionsFromInfo(info)
	opts.PHPVersion = ""
	opts.FastCGISocket = ""
	opts.SSLEnabled = false
	opts.SSLCertificate = ""
	opts.SSLCertificateKey = ""
	return opts
}

// ProvisionMigratedVhost menulis vhost domain hasil migrasi di server
// tujuan (lihat migratedVhostOptions). Berbeda dari provisionDomain: TIDAK
// menulis index.html dan TIDAK mengubah kepemilikan/permission document
// root — isinya sudah disalin apa adanya dari server asal.
func (s *Service) ProvisionMigratedVhost(serverID string, info DomainInfo) error {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}
	domain, err := NormalizeDomain(info.Domain)
	if err != nil {
		return err
	}
	content := buildVhostConfig(migratedVhostOptions(info))
	b64 := base64.StdEncoding.EncodeToString([]byte(content))
	target := configFilePath(domain, info.Enabled)

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	script := fmt.Sprintf(`set -e
ENABLED=%s
DISABLED=%s
TARGET=%s
if [ -f "$ENABLED" ] || [ -f "$DISABLED" ]; then echo "__poinhost_EXISTS__"; exit 1; fi
mkdir -p %s
trap 'rm -f "$TARGET"' ERR
echo %s | base64 -d > "$TARGET"
nginx -t
systemctl reload nginx || systemctl restart nginx
trap - ERR
`, shellQuote(configFilePath(domain, true)), shellQuote(configFilePath(domain, false)), shellQuote(target),
		shellQuote(info.Root), shellQuote(b64))
	res, err := s.run(access, script, 30*time.Second)
	if err != nil {
		return err
	}
	if strings.Contains(res.Stdout, "__poinhost_EXISTS__") {
		return errFmt("domain %s sudah ada di server tujuan", domain)
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = "gagal menulis vhost"
		}
		return mapWebsiteError(errFmt("%s", msg))
	}
	s.invalidateDomainCache(serverID)
	return nil
}

// ---------------------------------------------------------------------
// User database
// ---------------------------------------------------------------------

// DBUserExport satu user database (MySQL: satu pasangan user@host) beserta
// semua yang dibutuhkan untuk membuatnya ulang di server lain dengan
// password yang sama.
type DBUserExport struct {
	Engine   string `json:"engine"`
	Username string `json:"username"`
	Host     string `json:"host,omitempty"`
	// Databases database (dari yang dimigrasi) yang bisa diakses user ini.
	Databases []string `json:"databases"`
	// SkipReason diisi kalau user ini SENGAJA tidak dimigrasi otomatis
	// (hak global *.* / superuser) — hak seperti itu berlaku ke seluruh
	// server tujuan, bukan hanya ke database situs yang dipindah.
	SkipReason string `json:"skipReason,omitempty"`
	// Plugin autentikasi MySQL (caching_sha2_password, mysql_native_password,
	// ed25519, ...) dan jenis server asalnya ("mysql"/"mariadb") — hash
	// password hanya bisa dipindah apa adanya kalau server tujuan mengenal
	// plugin yang sama (lihat MySQLUserPortable).
	Plugin       string `json:"plugin,omitempty"`
	SourceFlavor string `json:"sourceFlavor,omitempty"`

	nativeHash string
	createSQL  string
	grantSQL   []string
	pgAttrs    []string
	pgPassword string
}

// mysqlQuote literal string MySQL lengkap dengan kutip — beda dari
// mysqlEscape, backslash IKUT di-escape (mode SQL bawaan MySQL
// memperlakukan backslash sebagai karakter escape di dalam string).
func mysqlQuote(s string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", "''") + "'"
}

func pgIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// unescapeMySQLBatch membalik escape keluaran `mysql --batch` (\n, \t, \\,
// \0) — SHOW CREATE USER / SHOW GRANTS dibaca lewat mode itu.
func unescapeMySQLBatch(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case '0':
			b.WriteByte(0)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// mysqlGrantDatabase membaca target sebuah baris SHOW GRANTS: nama database
// (sudah di-unescape dari `a\_b` jadi `a_b`), global=true untuk *.*, dan
// ok=false untuk baris yang tidak bertarget database (grant role, PROXY).
func mysqlGrantDatabase(grant string) (db string, global, ok bool) {
	g := strings.TrimSpace(grant)
	if !strings.HasPrefix(strings.ToUpper(g), "GRANT ") || strings.HasPrefix(strings.ToUpper(g), "GRANT PROXY ") {
		return "", false, false
	}
	up := strings.ToUpper(g)
	on := strings.Index(up, " ON ")
	to := strings.LastIndex(up, " TO ")
	if on < 0 || to < on {
		return "", false, false
	}
	target := strings.TrimSpace(g[on+4 : to])
	// Objek bertipe: "PROCEDURE `db`.`p`", "FUNCTION ..."
	for _, kw := range []string{"PROCEDURE ", "FUNCTION ", "TABLE "} {
		if strings.HasPrefix(strings.ToUpper(target), kw) {
			target = strings.TrimSpace(target[len(kw):])
		}
	}
	if strings.HasPrefix(target, "*") {
		return "", true, true
	}
	if strings.HasPrefix(target, "`") {
		end := strings.Index(target[1:], "`")
		if end < 0 {
			return "", false, false
		}
		db = target[1 : 1+end]
	} else {
		db, _, _ = strings.Cut(target, ".")
	}
	db = strings.NewReplacer(`\_`, "_", `\%`, "%").Replace(db)
	return db, false, true
}

func intersect(have []string, want map[string]bool) []string {
	out := make([]string, 0)
	for _, h := range have {
		if want[h] {
			out = append(out, h)
		}
	}
	sort.Strings(out)
	return out
}

// ExportDBUsers mengumpulkan user database yang punya akses ke salah satu
// database yang dimigrasi, beserta hash password dan grant-nya (grant
// dibatasi HANYA ke database yang dimigrasi — grant ke database lain di
// server asal tidak ikut, karena database itu tidak ada di tujuan).
func (s *Service) ExportDBUsers(serverID, engine string, databases []string) ([]DBUserExport, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(databases))
	for _, d := range databases {
		want[d] = true
	}
	users, err := s.DBListUsers(serverID, engine)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	out := make([]DBUserExport, 0)
	flavor := ""
	if engine == "mysql" {
		if flavor, err = s.mysqlFlavor(access); err != nil {
			return nil, err
		}
	}
	for _, u := range users {
		rel := intersect(u.Databases, want)
		if len(rel) == 0 {
			continue
		}
		if engine == "mysql" {
			for _, host := range u.Hosts {
				exp := DBUserExport{Engine: engine, Username: u.Username, Host: host, Databases: rel, SourceFlavor: flavor}
				if u.AllDatabases {
					exp.SkipReason = "punya hak ke semua database (*.*) — tidak dimigrasi otomatis"
					out = append(out, exp)
					continue
				}
				if err := s.fillMySQLUserExport(access, &exp, want); err != nil {
					return nil, fmt.Errorf("baca user %s@%s: %w", u.Username, host, err)
				}
				out = append(out, exp)
			}
			continue
		}
		exp := DBUserExport{Engine: engine, Username: u.Username, Databases: rel}
		if err := s.fillPostgresUserExport(access, &exp); err != nil {
			return nil, fmt.Errorf("baca role %s: %w", u.Username, err)
		}
		out = append(out, exp)
	}
	return out, nil
}

func (s *Service) fillMySQLUserExport(access *websiteAccess, exp *DBUserExport, want map[string]bool) error {
	who := mysqlQuote(exp.Username) + "@" + mysqlQuote(exp.Host)
	// Hash caching_sha2_password berisi byte biner; MySQL 8.0.17+ bisa
	// menampilkannya sebagai hex (aman lewat shell/batch). MariaDB tidak
	// mengenal variabel ini, dan hash-nya memang sudah teks — jadi cukup
	// diulang tanpa SET kalau ditolak.
	res, err := s.runMySQL(access, "SET SESSION print_identified_with_as_hex = ON; SHOW CREATE USER "+who)
	if err != nil {
		res, err = s.runMySQL(access, "SHOW CREATE USER "+who)
		if err != nil {
			return err
		}
	}
	exp.createSQL = unescapeMySQLBatch(strings.TrimSpace(res))
	if exp.createSQL == "" {
		return errFmt("SHOW CREATE USER kosong")
	}
	exp.Plugin, exp.nativeHash = parseIdentified(exp.createSQL)

	grants, err := s.runMySQL(access, "SHOW GRANTS FOR "+who)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(grants, "\n") {
		line = unescapeMySQLBatch(strings.TrimSpace(line))
		db, global, ok := mysqlGrantDatabase(line)
		if !ok {
			continue
		}
		if global {
			if !strings.HasPrefix(strings.ToUpper(line), "GRANT USAGE ON") {
				exp.SkipReason = "punya hak global (*.*) — tidak dimigrasi otomatis"
				return nil
			}
			continue
		}
		if want[db] {
			exp.grantSQL = append(exp.grantSQL, line)
		}
	}
	return nil
}

func (s *Service) fillPostgresUserExport(access *websiteAccess, exp *DBUserExport) error {
	res, err := s.runPostgres(access, "", "SELECT rolsuper, rolcreatedb, rolcreaterole, rolcanlogin, rolreplication, COALESCE(rolpassword, '') FROM pg_authid WHERE rolname = '"+pgEscape(exp.Username)+"'")
	if err != nil {
		return err
	}
	parts := strings.Split(strings.TrimRight(strings.TrimSpace(res), "\r"), "\t")
	if len(parts) < 6 {
		return errFmt("role tidak ditemukan")
	}
	if parts[0] == "t" {
		exp.SkipReason = "superuser — tidak dimigrasi otomatis"
		return nil
	}
	attrs := []string{}
	for i, kw := range []string{"", "CREATEDB", "CREATEROLE", "LOGIN", "REPLICATION"} {
		if i > 0 && parts[i] == "t" {
			attrs = append(attrs, kw)
		}
	}
	exp.pgAttrs = attrs
	exp.pgPassword = parts[5]
	return nil
}

// ImportDBUser membuat user di server tujuan. existed=true kalau user itu
// SUDAH ada di tujuan — dilewati utuh (password & grant tidak disentuh),
// sesuai kebijakan migrasi website: user yang sudah ada milik situs lain
// di server tujuan, menimpanya bisa memutus situs itu.
//
// password (opsional, MySQL saja) dipakai kalau hash-nya tidak portabel ke
// server tujuan (lihat MySQLUserPortable): user dibuat dengan password itu
// memakai plugin bawaan server tujuan.
func (s *Service) ImportDBUser(serverID string, u DBUserExport, password string) (existed bool, err error) {
	if u.SkipReason != "" {
		return false, errFmt("%s", u.SkipReason)
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return false, err
	}
	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	if u.Engine == "mysql" {
		n, err := s.runMySQL(access, "SELECT COUNT(*) FROM mysql.user WHERE User = "+mysqlQuote(u.Username)+" AND Host = "+mysqlQuote(u.Host))
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(n) != "0" {
			return true, nil
		}
		dstFlavor, err := s.mysqlFlavor(access)
		if err != nil {
			return false, err
		}
		who := mysqlQuote(u.Username) + "@" + mysqlQuote(u.Host)
		var create string
		switch {
		case password != "":
			create = "CREATE USER " + who + " IDENTIFIED BY " + mysqlQuote(password)
		case u.SourceFlavor == dstFlavor:
			create = u.createSQL
		case u.nativeHash != "":
			// Satu-satunya hash yang dikenal MySQL DAN MariaDB. Pernyataan
			// asli tidak dipakai: klausa MySQL 8 seperti PASSWORD HISTORY
			// adalah syntax error di MariaDB.
			create = "CREATE USER " + who + " IDENTIFIED BY PASSWORD " + mysqlQuote(u.nativeHash)
		default:
			return false, ErrDBUserNeedsPassword
		}
		if _, err := s.runMySQL(access, create); err != nil {
			return false, fmt.Errorf("buat user: %w", err)
		}
		for _, g := range u.grantSQL {
			if _, err := s.runMySQL(access, g); err != nil {
				return false, fmt.Errorf("terapkan grant: %w", err)
			}
		}
		return false, nil
	}

	n, err := s.runPostgres(access, "", "SELECT COUNT(*) FROM pg_roles WHERE rolname = '"+pgEscape(u.Username)+"'")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(n) != "0" {
		return true, nil
	}
	sql := "CREATE ROLE " + pgIdent(u.Username)
	if len(u.pgAttrs) > 0 {
		sql += " WITH " + strings.Join(u.pgAttrs, " ")
	}
	if u.pgPassword != "" {
		// Nilai berawalan SCRAM-SHA-256$ / md5 diterima PostgreSQL sebagai
		// hash jadi (tidak di-hash ulang) — password di tujuan identik.
		sql += " PASSWORD '" + pgEscape(u.pgPassword) + "'"
	}
	if _, err := s.runPostgres(access, "", sql); err != nil {
		return false, fmt.Errorf("buat role: %w", err)
	}
	return false, nil
}

// DBRoleExists memeriksa apakah role PostgreSQL / user MySQL (host apa pun) ada.
func (s *Service) DBRoleExists(serverID, engine, username string) (bool, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return false, err
	}
	var n string
	if engine == "mysql" {
		n, err = s.runMySQL(access, "SELECT COUNT(*) FROM mysql.user WHERE User = "+mysqlQuote(username))
	} else {
		n, err = s.runPostgres(access, "", "SELECT COUNT(*) FROM pg_roles WHERE rolname = '"+pgEscape(username)+"'")
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(n) != "0", nil
}

// DBDatabaseOwner pemilik database PostgreSQL (kosong untuk MySQL, yang
// tidak punya konsep pemilik database).
func (s *Service) DBDatabaseOwner(serverID, engine, database string) (string, error) {
	if engine != "postgresql" {
		return "", nil
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return "", err
	}
	res, err := s.runPostgres(access, "", "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = '"+pgEscape(database)+"'")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res), nil
}

// ---------------------------------------------------------------------
// Akun SFTP
// ---------------------------------------------------------------------

// SFTPAccountExport satu akun SFTP beserta hash password-nya.
type SFTPAccountExport struct {
	Username string `json:"username"`
	Domain   string `json:"domain"`
	Chroot   string `json:"chroot"`
	HomeDir  string `json:"homeDir"`
	Enabled  bool   `json:"enabled"`
	// HashScheme awalan skema hash ($y$ yescrypt, $6$ sha512, ...) — dipakai
	// cek kompatibilitas: server tujuan yang lebih tua tidak mengenal
	// yescrypt, dan login akun itu akan gagal di sana.
	HashScheme string `json:"hashScheme"`

	passwordHash string
}

func hashScheme(hash string) string {
	h := strings.TrimLeft(hash, "!")
	if !strings.HasPrefix(h, "$") {
		return ""
	}
	if i := strings.Index(h[1:], "$"); i >= 0 {
		return h[:i+2]
	}
	return ""
}

// ExportSFTPAccounts akun SFTP satu domain, termasuk hash /etc/shadow.
// Akun yang dinonaktifkan tetap terkunci di tujuan (awalan "!" pada hash
// ikut tersalin).
func (s *Service) ExportSFTPAccounts(serverID, domain string) ([]SFTPAccountExport, error) {
	list, err := s.ListSFTPAccounts(serverID, domain)
	if err != nil {
		return nil, err
	}
	if len(list.Accounts) == 0 {
		return []SFTPAccountExport{}, nil
	}
	var script strings.Builder
	script.WriteString("set +e\n")
	for _, a := range list.Accounts {
		fmt.Fprintf(&script, "printf 'H|%%s|%%s\\n' %s \"$(getent shadow %s | cut -d: -f2)\"\n", shellQuote(a.Username), shellQuote(a.Username))
	}
	out, err := s.RunRootScript(serverID, script.String(), 20*time.Second)
	if err != nil {
		return nil, err
	}
	hashes := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
		if len(parts) == 3 && parts[0] == "H" {
			hashes[parts[1]] = parts[2]
		}
	}
	res := make([]SFTPAccountExport, 0, len(list.Accounts))
	for _, a := range list.Accounts {
		h := hashes[a.Username]
		res = append(res, SFTPAccountExport{
			Username: a.Username, Domain: a.Domain, Chroot: a.Chroot, HomeDir: a.HomeDir,
			Enabled: a.Enabled, HashScheme: hashScheme(h), passwordHash: h,
		})
	}
	return res, nil
}

// ImportSFTPAccount membuat akun SFTP di tujuan dengan hash yang sama.
func (s *Service) ImportSFTPAccount(serverID string, a SFTPAccountExport) error {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}
	username, err := normalizeSFTPUsername(a.Username)
	if err != nil {
		return err
	}
	hash := a.passwordHash
	if hash == "" || hash == "*" {
		// Tanpa password sama sekali di asal — di tujuan dikunci, BUKAN
		// dibiarkan kosong (chpasswd -e dengan string kosong = tanpa password).
		hash = "!"
	}
	return s.provisionSFTPAccount(access, a.Domain, username, a.Chroot, a.HomeDir, hash, true)
}

// LinuxUserExists memeriksa nama user Linux di server.
func (s *Service) LinuxUserExists(serverID, username string) (bool, error) {
	out, err := s.RunRootScript(serverID, "id -u "+shellQuote(username)+" >/dev/null 2>&1 && echo yes || echo no", 15*time.Second)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "yes", nil
}

// CryptSchemeSupported mengecek apakah crypt(3) server mengenal skema hash
// tertentu, dengan memanggil crypt memakai contoh salt skema itu lewat
// perl (hampir selalu terpasang). "unknown" kalau perl tidak ada.
func (s *Service) CryptSchemeSupported(serverID, scheme string) (string, error) {
	if scheme == "" || scheme == "$1$" || scheme == "$5$" || scheme == "$6$" {
		return "yes", nil // didukung glibc sejak lama
	}
	salts := map[string]string{
		"$y$":  "$y$j9T$abcdefghijklmnopqrstuv$",
		"$gy$": "$gy$j9T$abcdefghijklmnopqrstuv$",
		"$2b$": "$2b$05$abcdefghijklmnopqrstuu",
		"$7$":  "$7$CU..../....abcdefghijklmnop$",
	}
	salt, ok := salts[scheme]
	if !ok {
		return "unknown", nil
	}
	script := "command -v perl >/dev/null 2>&1 || { echo unknown; exit 0; }\n" +
		"perl -e 'my $h = crypt(\"poinhost\", $ARGV[0]); print((defined $h && index($h, $ARGV[1]) == 0) ? \"yes\" : \"no\")' " +
		shellQuote(salt) + " " + shellQuote(scheme)
	out, err := s.RunRootScript(serverID, script, 15*time.Second)
	if err != nil {
		return "unknown", err
	}
	return strings.TrimSpace(out), nil
}

// ---------------------------------------------------------------------
// Cron
// ---------------------------------------------------------------------

// ExportCronJobs job cron poinhost milik domain-domain tertentu.
func (s *Service) ExportCronJobs(serverID string, domains []string) ([]CronJobInfo, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	_, jobs, _, err := s.readCronFile(access)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, d := range domains {
		want[d] = true
	}
	out := make([]CronJobInfo, 0)
	for _, j := range jobs {
		if want[j.Domain] {
			out = append(out, j)
		}
	}
	return out, nil
}

// ImportCronJobs menambahkan job ke /etc/cron.d/poinhost server tujuan,
// mempertahankan job & baris lain yang sudah ada. Job dengan ID yang sudah
// ada di tujuan dilewati (migrasi ulang tidak menggandakan job).
func (s *Service) ImportCronJobs(serverID string, jobs []CronJobInfo) (int, error) {
	if len(jobs) == 0 {
		return 0, nil
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return 0, err
	}
	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	_, existing, foreign, err := s.readCronFile(access)
	if err != nil {
		return 0, err
	}
	have := map[string]bool{}
	for _, j := range existing {
		have[j.ID] = true
	}
	added := 0
	for _, j := range jobs {
		if have[j.ID] {
			continue
		}
		existing = append(existing, j)
		added++
	}
	if added == 0 {
		return 0, nil
	}
	s.invalidateDomainCache(serverID)
	roots, err := s.domainRootsFor(serverID, existing)
	if err != nil {
		return 0, err
	}
	if _, err := s.run(access, "mkdir -p /var/log/poinhost-cron", 15*time.Second); err != nil {
		return 0, err
	}
	if err := s.writeCronFile(access, existing, foreign, roots); err != nil {
		return 0, err
	}
	return added, nil
}

// DBDatabaseACL pemilik + ACL level-database PostgreSQL (datacl mentah,
// kosong = default: PUBLIC boleh CONNECT/TEMP). pg_dump TIDAK membawa
// keduanya (hanya objek DI DALAM database), jadi dipindah terpisah.
func (s *Service) DBDatabaseACL(serverID, database string) (owner, acl string, err error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return "", "", err
	}
	res, err := s.runPostgres(access, "", "SELECT pg_get_userbyid(datdba), COALESCE(datacl::text, '') FROM pg_database WHERE datname = '"+pgEscape(database)+"'")
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(strings.TrimSpace(res), "\t", 2)
	owner = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		acl = strings.TrimSpace(parts[1])
	}
	return owner, acl, nil
}

type pgACLItem struct {
	grantee string // "" = PUBLIC
	privs   []string
}

// parsePGDatabaseACL mem-parsing datacl ("{=Tc/postgres,app=CTc/postgres}")
// jadi daftar grantee + privilege level database (CREATE/TEMPORARY/CONNECT).
func parsePGDatabaseACL(acl string) []pgACLItem {
	acl = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(acl), "{"), "}")
	if acl == "" {
		return nil
	}
	var out []pgACLItem
	for _, raw := range strings.Split(acl, ",") {
		var grantee, rest string
		if strings.HasPrefix(raw, `"`) {
			// Nama ber-kutip ganda, kutip di dalamnya digandakan ("a""b").
			i := 1
			var b strings.Builder
			for i < len(raw) {
				if raw[i] == '"' {
					if i+1 < len(raw) && raw[i+1] == '"' {
						b.WriteByte('"')
						i += 2
						continue
					}
					break
				}
				b.WriteByte(raw[i])
				i++
			}
			grantee = b.String()
			rest = strings.TrimPrefix(raw[min(i+1, len(raw)):], "=")
		} else {
			eq := strings.Index(raw, "=")
			if eq < 0 {
				continue
			}
			grantee, rest = raw[:eq], raw[eq+1:]
		}
		privStr, _, _ := strings.Cut(rest, "/")
		item := pgACLItem{grantee: grantee}
		for _, c := range privStr {
			switch c {
			case 'C':
				item.privs = append(item.privs, "CREATE")
			case 'T':
				item.privs = append(item.privs, "TEMPORARY")
			case 'c':
				item.privs = append(item.privs, "CONNECT")
			}
		}
		out = append(out, item)
	}
	return out
}

// DBApplyDatabaseACL menerapkan pemilik + ACL database dari server asal di
// server tujuan. Grantee yang role-nya tidak ada di tujuan dilewati dan
// dikembalikan supaya bisa dilaporkan.
func (s *Service) DBApplyDatabaseACL(serverID, database, owner, acl string) (missing []string, err error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	db := pgIdent(database)
	if owner != "" && owner != "postgres" {
		ok, err := s.DBRoleExists(serverID, "postgresql", owner)
		if err != nil {
			return nil, err
		}
		if ok {
			if _, err := s.runPostgres(access, "", "ALTER DATABASE "+db+" OWNER TO "+pgIdent(owner)); err != nil {
				return nil, err
			}
		} else {
			missing = append(missing, owner)
		}
	}
	items := parsePGDatabaseACL(acl)
	if len(items) == 0 {
		return missing, nil
	}
	stmts := []string{"REVOKE ALL ON DATABASE " + db + " FROM PUBLIC"}
	for _, it := range items {
		if len(it.privs) == 0 {
			continue
		}
		target := "PUBLIC"
		if it.grantee != "" {
			if it.grantee != "postgres" {
				ok, err := s.DBRoleExists(serverID, "postgresql", it.grantee)
				if err != nil {
					return missing, err
				}
				if !ok {
					missing = append(missing, it.grantee)
					continue
				}
			}
			target = pgIdent(it.grantee)
		}
		stmts = append(stmts, "GRANT "+strings.Join(it.privs, ", ")+" ON DATABASE "+db+" TO "+target)
	}
	_, err = s.runPostgres(access, "", strings.Join(stmts, "; "))
	return missing, err
}

// DBUserHostExists user MySQL persis user@host (PostgreSQL: role, host diabaikan).
func (s *Service) DBUserHostExists(serverID, engine, username, host string) (bool, error) {
	if engine != "mysql" {
		return s.DBRoleExists(serverID, engine, username)
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return false, err
	}
	n, err := s.runMySQL(access, "SELECT COUNT(*) FROM mysql.user WHERE User = "+mysqlQuote(username)+" AND Host = "+mysqlQuote(host))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(n) != "0", nil
}

// ErrDBUserNeedsPassword hash password user tidak bisa dipasang di server
// tujuan (mis. caching_sha2_password MySQL 8 → MariaDB, yang tidak mengenal
// plugin itu) — user hanya bisa dibuat kalau password aslinya diberikan.
var ErrDBUserNeedsPassword = errors.New("hash password user ini tidak dikenal server tujuan — isi password aslinya")

var (
	identifiedRE = regexp.MustCompile(`(?i)IDENTIFIED\s+(?:WITH|VIA)\s+'?([A-Za-z0-9_]+)'?(?:\s+(?:AS|USING)\s+(?:'([^']*)'|(0x[0-9A-Fa-f]+)))?`)
	byPasswordRE = regexp.MustCompile(`(?i)IDENTIFIED\s+BY\s+PASSWORD\s+'(\*[0-9A-Fa-f]{40})'`)
)

// parseIdentified membaca plugin autentikasi dari SHOW CREATE USER, dan
// hash mysql_native_password ("*" + 40 hex) kalau pluginnya itu — format
// MySQL (IDENTIFIED WITH 'plugin' AS '...') maupun MariaDB
// (IDENTIFIED BY PASSWORD '*...' / IDENTIFIED VIA plugin USING '...').
func parseIdentified(createSQL string) (plugin, nativeHash string) {
	if m := byPasswordRE.FindStringSubmatch(createSQL); m != nil {
		return "mysql_native_password", m[1]
	}
	m := identifiedRE.FindStringSubmatch(createSQL)
	if m == nil {
		return "", ""
	}
	plugin = strings.ToLower(m[1])
	if plugin == "mysql_native_password" && strings.HasPrefix(m[2], "*") && len(m[2]) == 41 {
		nativeHash = m[2]
	}
	return plugin, nativeHash
}

// MySQLUserPortable apakah user bisa dibuat di server tujuan (jenis
// dstFlavor) TANPA mengetahui password aslinya.
func MySQLUserPortable(u DBUserExport, dstFlavor string) bool {
	if u.Engine != "mysql" {
		return true
	}
	return u.SourceFlavor == dstFlavor || u.nativeHash != ""
}

func (s *Service) mysqlFlavor(access *websiteAccess) (string, error) {
	v, err := s.runMySQL(access, "SELECT VERSION()")
	if err != nil {
		return "", err
	}
	if strings.Contains(strings.ToLower(v), "mariadb") {
		return "mariadb", nil
	}
	return "mysql", nil
}

// DBMySQLFlavor jenis server MySQL: "mysql" atau "mariadb".
func (s *Service) DBMySQLFlavor(serverID string) (string, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return "", err
	}
	return s.mysqlFlavor(access)
}
