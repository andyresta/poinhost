package website

import (
	"context"
	"database/sql"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// Explore MySQL — beda dari database.go (yang cuma provisioning: buat
// database/user/grants lewat exec CLI sekali jalan) — ini benar-benar
// membuka koneksi protokol MySQL asli (go-sql-driver/mysql) supaya
// pagination/browse tabel besar tidak lambat (per arahan eksplisit: "kalau
// ada potensi lambat pagination mending pakai driver langsung saja"),
// mirip pendekatan homepoin, TAPI tanpa exec CLI fallback rumit dan tanpa
// perlu registry dialer global bernama per-server — cfg.DialFunc (via
// mysql.NewConnector, bukan sql.Open+DSN string) sudah cukup untuk menahan
// closure tunnel per koneksi.
//
// Tunnel-nya sendiri lewat sshpool.Executor.DialTunnel — memakai SLOT
// SHARED yang sama dipakai exec modul lain (Files/Docker/Nginx/dst), jadi
// tidak pernah memicu handshake SSH baru; dan alamat yang dituju di sisi
// REMOTE selalu 127.0.0.1:3306 (localhost dari sudut pandang server itu
// sendiri), sama seperti runMySQL di database.go.

const mysqlExploreAddr = "127.0.0.1:3306"

// MySQLExploreRequest menunjuk satu kredensial tersimpan (server+user+host)
// yang dipakai untuk membuka koneksi Explore.
type MySQLExploreRequest struct {
	ServerID string `json:"serverId"`
	Username string `json:"username"`
	Host     string `json:"host,omitempty"`
}

// MySQLTableInfo satu tabel dalam database yang di-explore.
type MySQLTableInfo struct {
	Name       string `json:"name"`
	ApproxRows int64  `json:"approxRows"`
	Engine     string `json:"engine,omitempty"`
}

// MySQLColumnInfo satu kolom tabel.
type MySQLColumnInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Key      string `json:"key,omitempty"` // PRI/UNI/MUL/""
	Default  string `json:"default,omitempty"`
	Extra    string `json:"extra,omitempty"`
}

// MySQLTableRowsRequest membaca satu halaman baris dari satu tabel.
type MySQLTableRowsRequest struct {
	MySQLExploreRequest
	Database string `json:"database"`
	Table    string `json:"table"`
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
	OrderBy  string `json:"orderBy,omitempty"`
	OrderDir string `json:"orderDir,omitempty"` // "asc" | "desc"
	// SkipTotal: lewati SELECT COUNT(*) (mahal untuk tabel besar — full
	// scan index klaster InnoDB) kalau frontend sudah punya total dari
	// panggilan sebelumnya dan tidak butuh hitung ulang, mis. sekadar
	// pindah halaman di tabel yang sama. Total di hasil bernilai -1 kalau
	// ini true (sentinel "tidak dihitung ulang, pakai nilai lama").
	SkipTotal bool `json:"skipTotal,omitempty"`
}

// MySQLTableRowsResult satu halaman baris — nilai NULL asli direpresentasi
// sebagai pointer nil (bukan string "NULL"), supaya UI bisa membedakannya
// dari string kosong. Total bernilai -1 kalau request minta SkipTotal.
type MySQLTableRowsResult struct {
	Columns []string    `json:"columns"`
	Rows    [][]*string `json:"rows"`
	Total   int64       `json:"total"`
}

// MySQLRowMutateRequest dipakai untuk insert/update/delete satu baris.
// Where WAJIB diisi untuk update/delete (mencegah operasi menimpa/menghapus
// seluruh tabel akibat lupa kondisi) — biasanya berisi primary key baris
// yang sedang diedit di grid.
type MySQLRowMutateRequest struct {
	MySQLExploreRequest
	Database string         `json:"database"`
	Table    string         `json:"table"`
	Values   map[string]any `json:"values,omitempty"`
	Where    map[string]any `json:"where,omitempty"`
}

// MySQLQueryRequest menjalankan SATU statement SQL bebas (kotak query).
// Limit/Offset dipakai untuk paginasi hasil SELECT — lihat
// paginateSelectSQL untuk kenapa paginasinya dilakukan di SISI SERVER.
type MySQLQueryRequest struct {
	MySQLExploreRequest
	Database string `json:"database"`
	SQL      string `json:"sql"`
	Limit    int    `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

// MySQLQueryResult hasil query bebas — Columns/Rows kosong untuk statement
// non-SELECT (INSERT/UPDATE/DELETE/dst), yang mengisi RowsAffected saja.
type MySQLQueryResult struct {
	Columns      []string    `json:"columns,omitempty"`
	Rows         [][]*string `json:"rows,omitempty"`
	RowsAffected int64       `json:"rowsAffected"`
	IsSelect     bool        `json:"isSelect"`
	// Paginated: hasil ini dipotong server (lihat paginateSelectSQL).
	// HasMore: masih ada baris sesudah halaman ini — dideteksi dengan
	// mengambil satu baris LEBIH dari yang ditampilkan, jadi tidak perlu
	// COUNT(*) yang mahal untuk sekadar tahu tombol "berikutnya" perlu
	// dinyalakan atau tidak.
	Paginated bool `json:"paginated"`
	HasMore   bool `json:"hasMore"`
	Offset    int  `json:"offset"`
}

var sqlIdentRE = regexp.MustCompile(`^[A-Za-z0-9_$]{1,64}$`)

func validateSQLIdent(name string) error {
	if !sqlIdentRE.MatchString(name) {
		return errFmt("nama tidak valid: %q (huruf/angka/_ saja)", name)
	}
	return nil
}

func quoteMySQLIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func singleStatementSQL(q string) bool {
	trimmed := strings.TrimRight(strings.TrimSpace(q), "; \t\n")
	return trimmed != "" && !strings.Contains(trimmed, ";")
}

// paginateSelectSQL membungkus query SELECT user jadi subquery ber-LIMIT
// supaya SERVER yang memotong hasilnya, bukan aplikasi ini yang menarik
// seluruh tabel lewat tunnel SSH lalu membuang 99%-nya. Untuk tabel jutaan
// baris, bedanya bukan "agak lebih cepat" tapi "jalan" vs "menggantung dan
// memakan memori".
//
// limit+1 baris diminta dengan sengaja: kalau baris ekstra itu ikut
// terbawa, berarti masih ada halaman berikutnya — tahu itu TANPA
// menjalankan COUNT(*) terpisah yang harus memindai seluruh hasil.
//
// Hanya untuk query yang benar-benar diawali SELECT/WITH: SHOW/DESCRIBE
// tidak sah dijadikan subquery di MySQL, dan hasilnya memang kecil.
func paginateSelectSQL(q string, limit, offset int) (string, bool) {
	if limit <= 0 {
		return q, false
	}
	upper := strings.ToUpper(strings.TrimSpace(q))
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return q, false
	}
	inner := strings.TrimRight(strings.TrimSpace(q), "; \t\n")
	return "SELECT * FROM (" + inner + ") AS poinhost_page LIMIT " + strconv.Itoa(limit+1) + " OFFSET " + strconv.Itoa(offset), true
}

func isSelectLikeSQL(q string) bool {
	upper := strings.ToUpper(strings.TrimSpace(q))
	for _, kw := range []string{"SELECT", "SHOW", "DESC", "DESCRIBE", "EXPLAIN"} {
		if strings.HasPrefix(upper, kw) {
			return true
		}
	}
	return false
}

// classifyMySQLConnError menandai kegagalan autentikasi secara eksplisit
// (bukan sekadar teks error driver) supaya frontend bisa mendeteksinya dan
// menawarkan "perbarui password" — inilah solusi untuk masalah password
// basi yang di homepoin tidak ditangani sama sekali (lihat ARCHITECTURE.md).
func classifyMySQLConnError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "access denied") {
		return errFmt("AUTENTIKASI_GAGAL: password salah atau sudah diganti langsung di server — perbarui lewat \"Kelola kredensial\"")
	}
	return errFmt("koneksi ke MySQL gagal: %v", err)
}

// mysqlConnCacheKey membangun key cache generik (lihat dbconnpool.go) untuk
// satu kredensial MySQL.
func mysqlConnCacheKey(serverID, username, host string) string {
	return dbConnCacheKey("mysql", serverID, username, host)
}

// dialMySQLExplore SELALU membuka koneksi BARU lewat tunnel SSH (dial+ping
// sekali) — dipakai hanya oleh getOrDialMySQLExplore (cache miss) dan
// verifyMySQLCredential (test koneksi eksplisit sebelum simpan password).
// TIDAK mensyaratkan akses root/sudo SSH sama sekali (beda dari operasi
// provisioning di database.go): browsing sebagai user MySQL biasa tidak
// pernah butuh privilege root di sisi SSH, cuma butuh koneksi SSH yang
// hidup untuk di-tunnel — makanya di sini SENGAJA tidak memanggil
// resolveAccess (yang menolak user SSH non-root/non-sudo), supaya server
// dengan SSH user terbatas (praktik yang lebih aman) tetap bisa dipakai
// Explore.
func (s *Service) dialMySQLExplore(serverID, username, host, password string) (*sql.DB, error) {
	if _, err := s.servers.Get(serverID); err != nil {
		return nil, errFmt("server tidak ditemukan: %v", err)
	}

	cfg := mysqldriver.NewConfig()
	cfg.User = username
	cfg.Passwd = password
	cfg.Net = "tcp"
	cfg.Addr = mysqlExploreAddr
	cfg.AllowNativePasswords = true
	cfg.Timeout = 10 * time.Second
	cfg.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return s.executor.DialTunnel(ctx, serverID, network, addr)
	}

	connector, err := mysqldriver.NewConnector(cfg)
	if err != nil {
		return nil, errFmt("konfigurasi koneksi MySQL tidak valid: %v", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	// SENGAJA tanpa batas umur (0 = tidak pernah dipaksa re-connect) — koneksi
	// ini ditahan hidup lewat cache di Service (lihat getOrDialMySQLExplore),
	// jadi tidak ada alasan memaksa handshake ulang secara berkala selama
	// masih dipakai aktif; MySQL server sendiri yang akan menutup kalau
	// benar-benar idle terlalu lama (wait_timeout), dan sweep idle 5 menit
	// di atas sudah membuang koneksi yang memang sudah tidak dipakai.
	db.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, classifyMySQLConnError(err)
	}
	return db, nil
}

// getOrDialMySQLExplore mengembalikan koneksi cache kalau ada, atau dial
// baru (lalu menyimpannya ke cache) kalau belum — inilah satu-satunya jalur
// yang dipakai semua operasi Explore biasa (List*/TableRows/Insert/Update/
// Delete/ExecuteQuery), sehingga handshake MySQL cuma terjadi SEKALI per
// (server, user, host) yang sedang aktif di-browse, bukan sekali per
// panggilan. Mekanisme cache & dedup dial bersamaan-nya generik, lihat
// dbconnpool.go.
func (s *Service) getOrDialMySQLExplore(serverID, username, host, password string) (*sql.DB, error) {
	key := mysqlConnCacheKey(serverID, username, host)
	return s.getOrDialDBConn(key, func() (*sql.DB, error) {
		return s.dialMySQLExplore(serverID, username, host, password)
	})
}

// verifyMySQLCredential dipakai SaveDBCredential (opsi Verify) — memastikan
// password benar SEBELUM disimpan ke vault. Koneksi yang berhasil langsung
// disimpan ke cache (bukan ditutup) supaya Explore pertama setelah
// menyimpan kredensial terasa instan, tidak perlu dial ulang.
func (s *Service) verifyMySQLCredential(serverID, username, host, password string) error {
	db, err := s.dialMySQLExplore(serverID, username, host, password)
	if err != nil {
		return err
	}
	s.putCachedDBConn(mysqlConnCacheKey(serverID, username, host), db)
	return nil
}

// openMySQLExploreStored membuka (atau memakai ulang dari cache) koneksi
// Explore memakai password yang SUDAH tersimpan di vault lokal — jalur
// normal semua endpoint Explore.
func (s *Service) openMySQLExploreStored(req MySQLExploreRequest) (*sql.DB, string, string, error) {
	username, err := normalizeDBUsername(req.Username)
	if err != nil {
		return nil, "", "", err
	}
	host := dbCredentialHost("mysql", req.Host)
	password, ok, err := s.getDBCredentialPassword(req.ServerID, "mysql", username, host)
	if err != nil {
		return nil, "", "", err
	}
	if !ok {
		return nil, "", "", errFmt("KREDENSIAL_BELUM_TERSIMPAN: belum ada password tersimpan untuk %s@%s — simpan dulu lewat \"Kelola kredensial\"", username, host)
	}
	db, err := s.getOrDialMySQLExplore(req.ServerID, username, host, password)
	return db, username, host, err
}

// MySQLExploreListDatabases daftar database yang bisa dilihat user ini
// (otomatis terbatas sesuai grants MySQL-nya sendiri — SHOW DATABASES cuma
// menampilkan yang user berhak akses).
func (s *Service) MySQLExploreListDatabases(req MySQLExploreRequest) ([]string, error) {
	db, _, _, err := s.openMySQLExploreStored(req)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query("SHOW DATABASES")
	if err != nil {
		return nil, errFmt("SHOW DATABASES: %v", err)
	}
	defer rows.Close()

	skip := map[string]bool{"information_schema": true, "mysql": true, "performance_schema": true, "sys": true}
	out := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if skip[name] {
			continue
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// MySQLExploreListTables daftar tabel dalam satu database.
func (s *Service) MySQLExploreListTables(req MySQLExploreRequest, database string) ([]MySQLTableInfo, error) {
	if err := validateSQLIdent(database); err != nil {
		return nil, err
	}
	db, _, _, err := s.openMySQLExploreStored(req)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT TABLE_NAME, IFNULL(TABLE_ROWS, 0), IFNULL(ENGINE, '')
		FROM information_schema.tables WHERE table_schema = ? ORDER BY TABLE_NAME`, database)
	if err != nil {
		return nil, errFmt("daftar tabel: %v", err)
	}
	defer rows.Close()

	out := make([]MySQLTableInfo, 0)
	for rows.Next() {
		var t MySQLTableInfo
		if err := rows.Scan(&t.Name, &t.ApproxRows, &t.Engine); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MySQLExploreListColumns daftar kolom satu tabel.
func (s *Service) MySQLExploreListColumns(req MySQLExploreRequest, database, table string) ([]MySQLColumnInfo, error) {
	if err := validateSQLIdent(database); err != nil {
		return nil, err
	}
	if err := validateSQLIdent(table); err != nil {
		return nil, err
	}
	db, _, _, err := s.openMySQLExploreStored(req)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, IFNULL(COLUMN_DEFAULT, ''), EXTRA
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ? ORDER BY ORDINAL_POSITION`, database, table)
	if err != nil {
		return nil, errFmt("daftar kolom: %v", err)
	}
	defer rows.Close()

	out := make([]MySQLColumnInfo, 0)
	for rows.Next() {
		var c MySQLColumnInfo
		var nullable string
		if err := rows.Scan(&c.Name, &c.Type, &nullable, &c.Key, &c.Default, &c.Extra); err != nil {
			return nil, err
		}
		c.Nullable = strings.EqualFold(nullable, "YES")
		out = append(out, c)
	}
	return out, rows.Err()
}

// MySQLExploreTableRows membaca satu halaman baris (paginated via koneksi
// driver asli, bukan re-exec CLI per halaman — inilah yang bikin geser
// halaman tabel besar tetap cepat).
func (s *Service) MySQLExploreTableRows(req MySQLTableRowsRequest) (*MySQLTableRowsResult, error) {
	if err := validateSQLIdent(req.Database); err != nil {
		return nil, err
	}
	if err := validateSQLIdent(req.Table); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	db, _, _, err := s.openMySQLExploreStored(req.MySQLExploreRequest)
	if err != nil {
		return nil, err
	}

	qualified := quoteMySQLIdent(req.Database) + "." + quoteMySQLIdent(req.Table)

	result := &MySQLTableRowsResult{Total: -1}
	if !req.SkipTotal {
		if err := db.QueryRow("SELECT COUNT(*) FROM " + qualified).Scan(&result.Total); err != nil {
			return nil, errFmt("hitung total baris: %v", err)
		}
	}

	orderClause := ""
	if req.OrderBy != "" {
		if err := validateSQLIdent(req.OrderBy); err != nil {
			return nil, err
		}
		dir := "ASC"
		if strings.EqualFold(req.OrderDir, "desc") {
			dir = "DESC"
		}
		orderClause = " ORDER BY " + quoteMySQLIdent(req.OrderBy) + " " + dir
	}

	rows, err := db.Query("SELECT * FROM "+qualified+orderClause+" LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, errFmt("baca baris: %v", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result.Columns = cols

	for rows.Next() {
		raw := make([]sql.RawBytes, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make([]*string, len(cols))
		for i, b := range raw {
			if b != nil {
				v := string(b)
				row[i] = &v
			}
		}
		result.Rows = append(result.Rows, row)
	}
	return result, rows.Err()
}

// buildMySQLWhereClause membangun klausa WHERE dari map kolom->nilai
// (urutan kolom diurutkan supaya query yang dihasilkan deterministik/mudah
// dibaca saat debug) — nilai nil berarti "IS NULL".
func buildMySQLWhereClause(where map[string]any) (string, []interface{}, error) {
	if len(where) == 0 {
		return "", nil, errFmt("kondisi WHERE wajib diisi (mencegah operasi menimpa seluruh tabel)")
	}
	keys := make([]string, 0, len(where))
	for k := range where {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	args := make([]interface{}, 0, len(keys))
	for _, col := range keys {
		if err := validateSQLIdent(col); err != nil {
			return "", nil, err
		}
		val := where[col]
		if val == nil {
			parts = append(parts, quoteMySQLIdent(col)+" IS NULL")
		} else {
			parts = append(parts, quoteMySQLIdent(col)+" = ?")
			args = append(args, val)
		}
	}
	return strings.Join(parts, " AND "), args, nil
}

// MySQLExploreInsertRow menyisipkan satu baris baru.
func (s *Service) MySQLExploreInsertRow(req MySQLRowMutateRequest) error {
	if err := validateSQLIdent(req.Database); err != nil {
		return err
	}
	if err := validateSQLIdent(req.Table); err != nil {
		return err
	}
	if len(req.Values) == 0 {
		return errFmt("tidak ada kolom untuk disisipkan")
	}

	cols := make([]string, 0, len(req.Values))
	for k := range req.Values {
		cols = append(cols, k)
	}
	sort.Strings(cols)

	quotedCols := make([]string, 0, len(cols))
	placeholders := make([]string, 0, len(cols))
	args := make([]interface{}, 0, len(cols))
	for _, col := range cols {
		if err := validateSQLIdent(col); err != nil {
			return err
		}
		quotedCols = append(quotedCols, quoteMySQLIdent(col))
		placeholders = append(placeholders, "?")
		args = append(args, req.Values[col])
	}

	db, _, _, err := s.openMySQLExploreStored(req.MySQLExploreRequest)
	if err != nil {
		return err
	}

	q := "INSERT INTO " + quoteMySQLIdent(req.Database) + "." + quoteMySQLIdent(req.Table) +
		" (" + strings.Join(quotedCols, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	if _, err := db.Exec(q, args...); err != nil {
		return errFmt("sisipkan baris: %v", err)
	}
	return nil
}

// MySQLExploreUpdateRow memperbarui satu baris yang cocok dengan req.Where.
func (s *Service) MySQLExploreUpdateRow(req MySQLRowMutateRequest) error {
	if err := validateSQLIdent(req.Database); err != nil {
		return err
	}
	if err := validateSQLIdent(req.Table); err != nil {
		return err
	}
	if len(req.Values) == 0 {
		return errFmt("tidak ada kolom untuk diperbarui")
	}

	setCols := make([]string, 0, len(req.Values))
	for k := range req.Values {
		setCols = append(setCols, k)
	}
	sort.Strings(setCols)

	setParts := make([]string, 0, len(setCols))
	args := make([]interface{}, 0, len(setCols))
	for _, col := range setCols {
		if err := validateSQLIdent(col); err != nil {
			return err
		}
		setParts = append(setParts, quoteMySQLIdent(col)+" = ?")
		args = append(args, req.Values[col])
	}

	whereClause, whereArgs, err := buildMySQLWhereClause(req.Where)
	if err != nil {
		return err
	}
	args = append(args, whereArgs...)

	db, _, _, err := s.openMySQLExploreStored(req.MySQLExploreRequest)
	if err != nil {
		return err
	}

	q := "UPDATE " + quoteMySQLIdent(req.Database) + "." + quoteMySQLIdent(req.Table) +
		" SET " + strings.Join(setParts, ", ") + " WHERE " + whereClause + " LIMIT 1"
	res, err := db.Exec(q, args...)
	if err != nil {
		return errFmt("perbarui baris: %v", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errFmt("tidak ada baris yang cocok — mungkin sudah diubah/dihapus dari tempat lain")
	}
	return nil
}

// MySQLExploreDeleteRow menghapus satu baris yang cocok dengan req.Where.
func (s *Service) MySQLExploreDeleteRow(req MySQLRowMutateRequest) error {
	if err := validateSQLIdent(req.Database); err != nil {
		return err
	}
	if err := validateSQLIdent(req.Table); err != nil {
		return err
	}
	whereClause, whereArgs, err := buildMySQLWhereClause(req.Where)
	if err != nil {
		return err
	}

	db, _, _, err := s.openMySQLExploreStored(req.MySQLExploreRequest)
	if err != nil {
		return err
	}

	q := "DELETE FROM " + quoteMySQLIdent(req.Database) + "." + quoteMySQLIdent(req.Table) +
		" WHERE " + whereClause + " LIMIT 1"
	res, err := db.Exec(q, whereArgs...)
	if err != nil {
		return errFmt("hapus baris: %v", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errFmt("tidak ada baris yang cocok — mungkin sudah dihapus dari tempat lain")
	}
	return nil
}

// MySQLExploreExecuteQuery menjalankan satu statement SQL bebas (kotak
// query) — dibatasi SATU statement per eksekusi (guard sederhana, tidak
// mengizinkan multi-statement lewat ";").
func (s *Service) MySQLExploreExecuteQuery(req MySQLQueryRequest) (*MySQLQueryResult, error) {
	if err := validateSQLIdent(req.Database); err != nil {
		return nil, err
	}
	sqlText := strings.TrimSpace(req.SQL)
	if sqlText == "" {
		return nil, errFmt("query kosong")
	}
	if !singleStatementSQL(sqlText) {
		return nil, errFmt("hanya satu statement per eksekusi (pisahkan jadi beberapa kali jalankan)")
	}

	db, _, _, err := s.openMySQLExploreStored(req.MySQLExploreRequest)
	if err != nil {
		return nil, err
	}

	// Ambil SATU koneksi fisik dari pool (bukan db.Exec/db.Query langsung)
	// dan pakai itu juga untuk query sesudahnya — "USE" bersifat per-koneksi,
	// kalau lewat db.Exec+db.Query biasa, database/sql bebas memberikan DUA
	// koneksi fisik berbeda dari pool (MaxOpenConns=4 di sini) sehingga USE
	// di koneksi pertama tidak berlaku sama sekali untuk query di koneksi
	// kedua — db.Conn menjamin keduanya jalan di koneksi fisik yang sama.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, errFmt("ambil koneksi: %v", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "USE "+quoteMySQLIdent(req.Database)); err != nil {
		return nil, errFmt("pilih database: %v", err)
	}

	if isSelectLikeSQL(sqlText) {
		runSQL, paginated := paginateSelectSQL(sqlText, req.Limit, req.Offset)
		rows, err := conn.QueryContext(ctx, runSQL)
		if err != nil {
			return nil, errFmt("query gagal: %v", err)
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return nil, err
		}
		result := &MySQLQueryResult{Columns: cols, IsSelect: true, Paginated: paginated, Offset: req.Offset}
		for rows.Next() {
			// Baris ke-(limit+1) cuma penanda "masih ada lagi" — jangan
			// ikut dikirim ke UI, halamannya tetap sebesar limit.
			if paginated && len(result.Rows) >= req.Limit {
				result.HasMore = true
				break
			}
			raw := make([]sql.RawBytes, len(cols))
			ptrs := make([]interface{}, len(cols))
			for i := range raw {
				ptrs[i] = &raw[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return nil, err
			}
			row := make([]*string, len(cols))
			for i, b := range raw {
				if b != nil {
					v := string(b)
					row[i] = &v
				}
			}
			result.Rows = append(result.Rows, row)
		}
		return result, rows.Err()
	}

	res, err := conn.ExecContext(ctx, sqlText)
	if err != nil {
		return nil, errFmt("eksekusi gagal: %v", err)
	}
	n, _ := res.RowsAffected()
	return &MySQLQueryResult{RowsAffected: n, IsSelect: false}, nil
}
