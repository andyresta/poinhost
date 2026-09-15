package website

import "strings"
import "testing"

// Regresi bug nyata: mysql -h 127.0.0.1 -u root gagal konsisten di server
// sungguhan (dikonfirmasi via instalasi MariaDB 10.11 asli) — MariaDB
// mencocokkan koneksi TCP loopback itu ke akun root@localhost (auth
// unix_socket, SELALU menolak TCP), bukan root@127.0.0.1 yang dibuat
// khusus untuk ini, meski password benar. Lihat komentar panjang di
// runMySQL (database.go) untuk detail lengkap.
func TestMySQLAdminCommandUsesLocalSocketNotTCP(t *testing.T) {
	cmd := mysqlAdminCommand("SHOW DATABASES")
	if !strings.Contains(cmd, "mysql -u root") {
		t.Fatalf("expected local-socket mysql -u root, got: %s", cmd)
	}
	if strings.Contains(cmd, "-h 127.0.0.1") || strings.Contains(cmd, "-h127.0.0.1") {
		t.Fatalf("must not reintroduce -h 127.0.0.1 (TCP), which fails against real MariaDB: %s", cmd)
	}
}

func TestDBDockerAccessStatusScriptUsesLocalSocketForMySQL(t *testing.T) {
	script := dbDockerAccessStatusScript("mysql")
	if strings.Contains(script, "-h 127.0.0.1") {
		t.Fatalf("docker-access status check must not use -h 127.0.0.1 for MySQL: %s", script)
	}
	if !strings.Contains(script, "mysql -u root") {
		t.Fatalf("expected local-socket mysql -u root in status script: %s", script)
	}
}

// Bootstrap akun root@127.0.0.1 berpassword kosong (dulu dibuat supaya
// mysql -h 127.0.0.1 -u root bisa login) sudah tidak diperlukan lagi sejak
// admin ops pindah ke unix socket — dan sebaiknya memang tidak pernah
// dibuat lagi (akun root ber-password kosong yang bisa dijangkau TCP,
// meski cuma loopback, adalah beban keamanan yang tidak perlu).
func TestDBInstallScriptDoesNotCreateEmptyPasswordRootAt127(t *testing.T) {
	for _, pm := range []string{"apt", "dnf", "yum"} {
		script, ok := dbInstallScript("mysql", pm, "")
		if !ok {
			t.Fatalf("dbInstallScript(mysql, %s, \"\") not ok", pm)
		}
		if strings.Contains(script, "root'@'127.0.0.1'") {
			t.Fatalf("pm=%s: must not bootstrap root@127.0.0.1 anymore: %s", pm, script)
		}
	}
}

// Regresi bug nyata: SEMUA user tampil "Semua database" di UI. Penyebabnya
// information_schema.USER_PRIVILEGES dibaca apa adanya, padahal setiap user
// MySQL selalu punya baris `USAGE ON *.*` di sana — itu artinya "tidak
// punya privilege apa pun", bukan akses global. Query pendeteksi grant
// global WAJIB menyaring USAGE.
func TestMySQLGlobalGrantDetectionExcludesUsage(t *testing.T) {
	// Sumber kebenarannya query di mysqlUserDatabaseAccess; di sini dijaga
	// lewat konstanta yang sama supaya perubahan query tanpa filter ketahuan.
	const wantFilter = "PRIVILEGE_TYPE <> 'USAGE'"
	if !strings.Contains(mysqlGlobalGrantQuery, wantFilter) {
		t.Fatalf("query grant global harus menyaring USAGE, got: %s", mysqlGlobalGrantQuery)
	}
}

func TestParseMySQLGrantee(t *testing.T) {
	cases := []struct{ in, user, host string }{
		{"'alkana'@'localhost'", "alkana", "localhost"},
		{"'appdevpoin'@'127.0.0.1'", "appdevpoin", "127.0.0.1"},
		{"'devpoin'@'%'", "devpoin", "%"},
	}
	for _, c := range cases {
		user, host, ok := parseMySQLGrantee(c.in)
		if !ok || user != c.user || host != c.host {
			t.Fatalf("parseMySQLGrantee(%q) = (%q,%q,%v), mau (%q,%q,true)", c.in, user, host, ok, c.user, c.host)
		}
	}
	if _, _, ok := parseMySQLGrantee("bukan format grantee"); ok {
		t.Fatal("string non-GRANTEE seharusnya ditolak")
	}
}

// Regresi bug nyata: buat user PostgreSQL gagal dengan
// `ERROR: invalid privilege type CREATE for relation`. Penyebabnya SATU
// daftar privilege dipakai ulang untuk semua jenis objek, padahal di
// PostgreSQL CREATE itu hak schema/database (bukan tabel), EXECUTE hak
// fungsi, dan sequence cuma menerima USAGE/SELECT/UPDATE.
func TestPostgresPrivilegesAreSplitPerObjectType(t *testing.T) {
	all := []string{"read", "write", "create", "alter", "drop", "execute"}

	tablePriv := postgresTablePrivilegeSQL(all)
	for _, bad := range []string{"CREATE", "EXECUTE", "USAGE"} {
		if strings.Contains(tablePriv, bad) {
			t.Fatalf("hak tabel tidak boleh mengandung %s, got: %s", bad, tablePriv)
		}
	}
	for _, want := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
		if !strings.Contains(tablePriv, want) {
			t.Fatalf("hak tabel seharusnya mengandung %s, got: %s", want, tablePriv)
		}
	}

	seqPriv := postgresSequencePrivilegeSQL(all)
	for _, bad := range []string{"CREATE", "EXECUTE", "INSERT", "DELETE", "TRUNCATE"} {
		if strings.Contains(seqPriv, bad) {
			t.Fatalf("hak sequence tidak boleh mengandung %s, got: %s", bad, seqPriv)
		}
	}

	// Tanpa privilege apa pun tetap harus menghasilkan statement yang sah.
	if got := postgresTablePrivilegeSQL(nil); got != "SELECT" {
		t.Fatalf("fallback hak tabel = %q, mau SELECT", got)
	}
	if got := postgresSequencePrivilegeSQL([]string{"alter"}); got != "SELECT" {
		t.Fatalf("fallback hak sequence = %q, mau SELECT", got)
	}
	// Tidak boleh ada duplikat (mis. write memetakan ke beberapa hak).
	if got := postgresTablePrivilegeSQL([]string{"write", "write"}); got != "INSERT, UPDATE, DELETE" {
		t.Fatalf("hak tabel duplikat tidak dibersihkan: %q", got)
	}
}

// Paginasi hasil query WAJIB dilakukan server-side: menarik seluruh hasil
// lewat tunnel SSH lalu memotongnya di aplikasi tidak scalable untuk tabel
// besar. Baris ekstra (limit+1) dipakai mendeteksi "masih ada halaman lagi"
// tanpa COUNT(*) terpisah.
func TestPaginateSelectSQL(t *testing.T) {
	got, ok := paginateSelectSQL("SELECT * FROM users", 100, 200)
	if !ok {
		t.Fatal("SELECT seharusnya dipaginasi")
	}
	if !strings.Contains(got, "LIMIT 101") {
		t.Fatalf("harus minta limit+1 baris untuk deteksi halaman berikutnya, got: %s", got)
	}
	if !strings.Contains(got, "OFFSET 200") {
		t.Fatalf("offset hilang: %s", got)
	}

	// Titik koma di akhir harus dibuang — kalau ikut masuk subquery,
	// SQL-nya jadi tidak sah.
	got, _ = paginateSelectSQL("SELECT 1;", 10, 0)
	if strings.Contains(got, ";) AS") || strings.Contains(got, "; ) AS") {
		t.Fatalf("titik koma bocor ke dalam subquery: %s", got)
	}

	// Bentuk yang tidak sah dijadikan subquery MySQL dibiarkan apa adanya.
	for _, q := range []string{"SHOW TABLES", "DESCRIBE users", "EXPLAIN SELECT 1"} {
		if out, ok := paginateSelectSQL(q, 100, 0); ok || out != q {
			t.Fatalf("%q tidak boleh dibungkus subquery, got: %s", q, out)
		}
	}
	// limit 0 = tanpa paginasi (dipakai pemanggil lama).
	if out, ok := paginateSelectSQL("SELECT 1", 0, 0); ok || out != "SELECT 1" {
		t.Fatalf("limit 0 seharusnya tidak mengubah query, got: %s", out)
	}
}

// Regresi bug nyata yang dilaporkan: `show tables;` di kotak query
// PostgreSQL balas error mentah `unrecognized configuration parameter
// "tables"` — sintaks MySQL yang di PostgreSQL berarti "baca parameter
// konfigurasi bernama tables". Sekarang diterjemahkan ke padanannya dan
// benar-benar dijalankan, bukan cuma ditolak dengan saran.
func TestPGTranslateMySQLDialect(t *testing.T) {
	cases := map[string]string{
		"show tables;":            "pg_tables",
		"SHOW TABLES":             "pg_tables",
		"  show   tables  ":       "pg_tables",
		"SHOW DATABASES;":         "pg_database",
		"SHOW SCHEMAS":            "pg_namespace",
		"DESCRIBE users;":         "information_schema.columns",
		"desc users":              "information_schema.columns",
		"SHOW COLUMNS FROM users": "information_schema.columns",
	}
	for q, want := range cases {
		got, ok := pgTranslateMySQLDialect(q)
		if !ok {
			t.Fatalf("%q seharusnya diterjemahkan", q)
		}
		if !strings.Contains(got, want) {
			t.Fatalf("terjemahan %q = %q, seharusnya memakai %s", q, got, want)
		}
	}

	// Nama tabel masuk sebagai literal SQL — kutip tunggal wajib di-escape.
	got, _ := pgTranslateMySQLDialect("DESCRIBE `we'ird`")
	if !strings.Contains(got, "'we''ird'") {
		t.Fatalf("nama tabel tidak di-escape dengan benar: %s", got)
	}

	// Statement SHOW yang SAH di PostgreSQL tidak boleh ikut diterjemahkan.
	for _, q := range []string{"SHOW search_path", "show timezone;", "SELECT * FROM users ORDER BY id DESC"} {
		if translated, ok := pgTranslateMySQLDialect(q); ok {
			t.Fatalf("%q sah di PostgreSQL, tidak boleh diterjemahkan, got: %s", q, translated)
		}
	}
}

func TestMariaDBVersionedInstallScriptDoesNotCreateEmptyPasswordRootAt127(t *testing.T) {
	script, ok := dbVersionedInstallScript("mysql", "apt", "mariadb-10.11", "")
	if !ok {
		t.Fatalf("dbVersionedInstallScript not ok")
	}
	if strings.Contains(script, "root'@'127.0.0.1'") {
		t.Fatalf("must not bootstrap root@127.0.0.1 anymore: %s", script)
	}
}
