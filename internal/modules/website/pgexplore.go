package website

import (
	"context"
	"database/sql"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Explore PostgreSQL — sama filosofinya dengan mysqlexplore.go (koneksi
// driver asli tunneled SSH, koneksi ditahan hidup lewat dbconnpool.go
// generik, bukan dial+close per panggilan), TAPI dengan satu perbedaan
// mendasar: koneksi PostgreSQL SELALU terikat ke SATU database (tidak ada
// padanan "USE database" MySQL) — pindah database berarti membuka koneksi
// LAIN, bukan menjalankan statement di koneksi yang sama. Makanya key
// cache-nya (server, role, DATABASE) — bukan (server, user, host) seperti
// MySQL — dan satu role bisa punya BANYAK koneksi ter-cache sekaligus
// (satu per database yang pernah di-browse), lihat evictDBConnsWithPrefix
// di dbconnpool.go untuk cara membuang semuanya sekaligus saat password
// berubah.

// pgExplorePort adalah port PostgreSQL bawaan di sisi REMOTE server (bukan
// port lokal poinhost) — dituju lewat 127.0.0.1 dari sudut pandang server
// itu sendiri, sama seperti mysqlExploreAddr di mysqlexplore.go.
const pgExplorePort = 5432

// PGExploreRequest menunjuk satu kredensial tersimpan (server+role) —
// dipakai untuk enumerasi database yang bisa diakses role ini. Role
// PostgreSQL TIDAK terikat host seperti user MySQL (itu urusan
// pg_hba.conf, bukan identitas role), jadi cukup server+username.
type PGExploreRequest struct {
	ServerID string `json:"serverId"`
	Username string `json:"username"`
}

// PGTableRequest menunjuk satu database spesifik untuk role yang sama.
// Schema default "public" kalau dikosongkan.
type PGTableRequest struct {
	PGExploreRequest
	Database string `json:"database"`
	Schema   string `json:"schema,omitempty"`
}

// PGTableInfo satu tabel dalam schema yang di-explore.
type PGTableInfo struct {
	Name       string `json:"name"`
	Schema     string `json:"schema"`
	ApproxRows int64  `json:"approxRows"`
}

// PGColumnInfo satu kolom tabel — field Key disamakan formatnya ("PRI")
// dengan MySQLColumnInfo supaya frontend bisa berbagi logika deteksi
// primary key (lihat primaryKeyCols di MySQLExplorerModal.tsx).
type PGColumnInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Key      string `json:"key,omitempty"`
	Default  string `json:"default,omitempty"`
}

// PGTableRowsRequest membaca satu halaman baris dari satu tabel.
type PGTableRowsRequest struct {
	PGTableRequest
	Table    string `json:"table"`
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
	OrderBy  string `json:"orderBy,omitempty"`
	OrderDir string `json:"orderDir,omitempty"`
	// SkipTotal: lewati SELECT COUNT(*) — lihat penjelasan yang sama di
	// MySQLTableRowsRequest.
	SkipTotal bool `json:"skipTotal,omitempty"`
}

// PGTableRowsResult satu halaman baris. Total bernilai -1 kalau request
// minta SkipTotal.
type PGTableRowsResult struct {
	Columns []string    `json:"columns"`
	Rows    [][]*string `json:"rows"`
	Total   int64       `json:"total"`
}

// PGRowMutateRequest dipakai untuk insert/update/delete satu baris. Where
// WAJIB diisi untuk update/delete.
type PGRowMutateRequest struct {
	PGTableRequest
	Table  string         `json:"table"`
	Values map[string]any `json:"values,omitempty"`
	Where  map[string]any `json:"where,omitempty"`
}

// PGQueryRequest menjalankan SATU statement SQL bebas.
type PGQueryRequest struct {
	PGTableRequest
	SQL string `json:"sql"`
}

// PGQueryResult hasil query bebas.
type PGQueryResult struct {
	Columns      []string    `json:"columns,omitempty"`
	Rows         [][]*string `json:"rows,omitempty"`
	RowsAffected int64       `json:"rowsAffected"`
	IsSelect     bool        `json:"isSelect"`
}

// quotePGIdent membungkus identifier PostgreSQL dengan kutip ganda (beda
// dari backtick MySQL) — validasi karakternya sendiri berbagi sqlIdentRE
// dengan MySQL (lihat mysqlexplore.go), cuma karakter quote-nya beda.
func quotePGIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func pgSchemaOrDefault(schema string) string {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return "public"
	}
	return schema
}

// classifyPGConnError menandai kegagalan autentikasi secara eksplisit,
// sama seperti classifyMySQLConnError.
func classifyPGConnError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "password authentication failed") || strings.Contains(msg, "authentication failed") {
		return errFmt("AUTENTIKASI_GAGAL: password salah atau sudah diganti langsung di server — perbarui lewat \"Kelola kredensial\"")
	}
	return errFmt("koneksi ke PostgreSQL gagal: %v", err)
}

func pgConnCacheKey(serverID, username, database string) string {
	return dbConnCacheKey("postgresql", serverID, username, database)
}

// dialPGExplore SELALU membuka koneksi BARU lewat tunnel SSH (dial+ping
// sekali) — dipakai hanya oleh getOrDialPGExplore (cache miss) dan
// verifyPGCredential. TIDAK mensyaratkan akses root/sudo SSH, sama seperti
// dialMySQLExplore (alasannya identik — lihat komentar di sana).
//
// sslmode=disable SENGAJA dipasang eksplisit — SSH tunnel-nya sendiri
// sudah terenkripsi, dan negosiasi TLS PostgreSQL di atas tunnel yang
// sudah dienkripsi cuma menambah round-trip tanpa manfaat keamanan
// tambahan (sama seperti MySQL tidak mengaktifkan TLS-nya sendiri di sini).
func (s *Service) dialPGExplore(serverID, username, database, password string) (*sql.DB, error) {
	if _, err := s.servers.Get(serverID); err != nil {
		return nil, errFmt("server tidak ditemukan: %v", err)
	}

	connConfig, err := pgx.ParseConfig("sslmode=disable")
	if err != nil {
		return nil, errFmt("konfigurasi koneksi PostgreSQL tidak valid: %v", err)
	}
	connConfig.Host = "127.0.0.1"
	connConfig.Port = pgExplorePort
	connConfig.Database = database
	connConfig.User = username
	connConfig.Password = password
	connConfig.ConnectTimeout = 10 * time.Second
	connConfig.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return s.executor.DialTunnel(ctx, serverID, network, addr)
	}

	db := stdlib.OpenDB(*connConfig)
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	// SENGAJA tanpa batas umur — sama alasannya dengan dialMySQLExplore.
	db.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, classifyPGConnError(err)
	}
	return db, nil
}

// getOrDialPGExplore mengembalikan koneksi cache kalau ada, atau dial baru
// (lalu menyimpannya ke cache) kalau belum. Key-nya per (server, role,
// DATABASE) — beda database berarti koneksi cache yang BERBEDA, meskipun
// role & servernya sama (lihat catatan di kepala file).
func (s *Service) getOrDialPGExplore(serverID, username, database, password string) (*sql.DB, error) {
	key := pgConnCacheKey(serverID, username, database)
	return s.getOrDialDBConn(key, func() (*sql.DB, error) {
		return s.dialPGExplore(serverID, username, database, password)
	})
}

// verifyPGCredential dipakai SaveDBCredential (opsi Verify) — konek ke
// database "postgres" (maintenance DB bawaan, sama seperti default
// runPostgres di database.go) karena belum tentu tahu database mana yang
// akan di-browse nanti. Koneksi yang berhasil langsung disimpan ke cache.
func (s *Service) verifyPGCredential(serverID, username, password string) error {
	db, err := s.dialPGExplore(serverID, username, "postgres", password)
	if err != nil {
		return err
	}
	s.putCachedDBConn(pgConnCacheKey(serverID, username, "postgres"), db)
	return nil
}

// openPGExploreStored membuka (atau memakai ulang dari cache) koneksi
// Explore ke SATU database, memakai password yang SUDAH tersimpan di vault
// lokal.
func (s *Service) openPGExploreStored(req PGTableRequest) (*sql.DB, string, error) {
	username, err := normalizeDBUsername(req.Username)
	if err != nil {
		return nil, "", err
	}
	database, err := normalizeDBName(req.Database)
	if err != nil {
		return nil, "", err
	}
	password, ok, err := s.getDBCredentialPassword(req.ServerID, "postgresql", username, "-")
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", errFmt("KREDENSIAL_BELUM_TERSIMPAN: belum ada password tersimpan untuk role %s — simpan dulu lewat \"Kelola kredensial\"", username)
	}
	db, err := s.getOrDialPGExplore(req.ServerID, username, database, password)
	return db, username, err
}

// PGExploreListDatabases daftar database yang bisa dilihat role ini — konek
// ke database "postgres" (maintenance) untuk membaca pg_database, filter
// yang sama dipakai DBListDatabases di database.go (skip template & DB
// "postgres" itu sendiri dari daftar yang ditawarkan untuk di-browse).
func (s *Service) PGExploreListDatabases(req PGExploreRequest) ([]string, error) {
	username, err := normalizeDBUsername(req.Username)
	if err != nil {
		return nil, err
	}
	password, ok, err := s.getDBCredentialPassword(req.ServerID, "postgresql", username, "-")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errFmt("KREDENSIAL_BELUM_TERSIMPAN: belum ada password tersimpan untuk role %s — simpan dulu lewat \"Kelola kredensial\"", username)
	}
	db, err := s.getOrDialPGExplore(req.ServerID, username, "postgres", password)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`SELECT datname FROM pg_database WHERE datistemplate = false AND datname != 'postgres' ORDER BY datname`)
	if err != nil {
		return nil, errFmt("daftar database: %v", err)
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// PGExploreListTables daftar tabel dalam satu schema (default "public").
// ApproxRows dari pg_class.reltuples (estimasi planner, sama seperti
// TABLE_ROWS MySQL — bukan COUNT(*) yang mahal).
func (s *Service) PGExploreListTables(req PGTableRequest) ([]PGTableInfo, error) {
	schema := pgSchemaOrDefault(req.Schema)
	if err := validateSQLIdent(schema); err != nil {
		return nil, err
	}
	db, _, err := s.openPGExploreStored(req)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT c.relname, COALESCE(c.reltuples, 0)::bigint
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relkind = 'r'
		ORDER BY c.relname`, schema)
	if err != nil {
		return nil, errFmt("daftar tabel: %v", err)
	}
	defer rows.Close()

	out := make([]PGTableInfo, 0)
	for rows.Next() {
		var t PGTableInfo
		if err := rows.Scan(&t.Name, &t.ApproxRows); err != nil {
			return nil, err
		}
		t.Schema = schema
		out = append(out, t)
	}
	return out, rows.Err()
}

// pgPrimaryKeyColumns mengembalikan nama kolom primary key satu tabel.
func pgPrimaryKeyColumns(db *sql.DB, schema, table string) ([]string, error) {
	rows, err := db.Query(`
		SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = $1 AND tc.table_name = $2
		ORDER BY kcu.ordinal_position`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// PGExploreListColumns daftar kolom satu tabel, ditandai primary key-nya.
func (s *Service) PGExploreListColumns(req PGTableRequest, table string) ([]PGColumnInfo, error) {
	schema := pgSchemaOrDefault(req.Schema)
	if err := validateSQLIdent(schema); err != nil {
		return nil, err
	}
	if err := validateSQLIdent(table); err != nil {
		return nil, err
	}
	db, _, err := s.openPGExploreStored(req)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT column_name, data_type, is_nullable, COALESCE(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`, schema, table)
	if err != nil {
		return nil, errFmt("daftar kolom: %v", err)
	}
	defer rows.Close()

	out := make([]PGColumnInfo, 0)
	for rows.Next() {
		var c PGColumnInfo
		var nullable string
		if err := rows.Scan(&c.Name, &c.Type, &nullable, &c.Default); err != nil {
			return nil, err
		}
		c.Nullable = strings.EqualFold(nullable, "YES")
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	pk, err := pgPrimaryKeyColumns(db, schema, table)
	if err != nil {
		return nil, err
	}
	pkSet := make(map[string]bool, len(pk))
	for _, c := range pk {
		pkSet[c] = true
	}
	for i := range out {
		if pkSet[out[i].Name] {
			out[i].Key = "PRI"
		}
	}
	return out, nil
}

// PGExploreTableRows membaca satu halaman baris (paginated).
func (s *Service) PGExploreTableRows(req PGTableRowsRequest) (*PGTableRowsResult, error) {
	schema := pgSchemaOrDefault(req.Schema)
	if err := validateSQLIdent(schema); err != nil {
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

	db, _, err := s.openPGExploreStored(req.PGTableRequest)
	if err != nil {
		return nil, err
	}

	qualified := quotePGIdent(schema) + "." + quotePGIdent(req.Table)

	result := &PGTableRowsResult{Total: -1}
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
		orderClause = " ORDER BY " + quotePGIdent(req.OrderBy) + " " + dir
	}

	rows, err := db.Query("SELECT * FROM "+qualified+orderClause+" LIMIT $1 OFFSET $2", limit, offset)
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

// buildPGSetClause membangun klausa SET "col1" = $N, ... — placeholder
// mulai dari startIdx supaya bisa digabung dengan placeholder WHERE di
// query yang sama (lihat PGExploreUpdateRow).
func buildPGSetClause(values map[string]any, startIdx int) (string, []interface{}, error) {
	cols := make([]string, 0, len(values))
	for k := range values {
		cols = append(cols, k)
	}
	sort.Strings(cols)

	parts := make([]string, 0, len(cols))
	args := make([]interface{}, 0, len(cols))
	idx := startIdx
	for _, col := range cols {
		if err := validateSQLIdent(col); err != nil {
			return "", nil, err
		}
		parts = append(parts, quotePGIdent(col)+" = $"+strconv.Itoa(idx))
		args = append(args, values[col])
		idx++
	}
	return strings.Join(parts, ", "), args, nil
}

// buildPGWhereClause membangun klausa WHERE "col1" = $N AND ... (placeholder
// mulai dari startIdx) — nilai nil berarti "IS NULL" (tidak makan slot
// placeholder).
func buildPGWhereClause(where map[string]any, startIdx int) (string, []interface{}, error) {
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
	idx := startIdx
	for _, col := range keys {
		if err := validateSQLIdent(col); err != nil {
			return "", nil, err
		}
		val := where[col]
		if val == nil {
			parts = append(parts, quotePGIdent(col)+" IS NULL")
		} else {
			parts = append(parts, quotePGIdent(col)+" = $"+strconv.Itoa(idx))
			args = append(args, val)
			idx++
		}
	}
	return strings.Join(parts, " AND "), args, nil
}

// PGExploreInsertRow menyisipkan satu baris baru.
func (s *Service) PGExploreInsertRow(req PGRowMutateRequest) error {
	schema := pgSchemaOrDefault(req.Schema)
	if err := validateSQLIdent(schema); err != nil {
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
	for i, col := range cols {
		if err := validateSQLIdent(col); err != nil {
			return err
		}
		quotedCols = append(quotedCols, quotePGIdent(col))
		placeholders = append(placeholders, "$"+strconv.Itoa(i+1))
		args = append(args, req.Values[col])
	}

	db, _, err := s.openPGExploreStored(req.PGTableRequest)
	if err != nil {
		return err
	}

	q := "INSERT INTO " + quotePGIdent(schema) + "." + quotePGIdent(req.Table) +
		" (" + strings.Join(quotedCols, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	if _, err := db.Exec(q, args...); err != nil {
		return errFmt("sisipkan baris: %v", err)
	}
	return nil
}

// PGExploreUpdateRow memperbarui satu baris yang cocok dengan req.Where.
// PostgreSQL tidak punya "UPDATE ... LIMIT 1" seperti MySQL, jadi dibatasi
// ke satu baris lewat sub-query "WHERE ctid = (SELECT ctid ... LIMIT 1)" —
// ctid adalah pengenal fisik baris bawaan PostgreSQL, cara standar untuk
// "kenai persis satu baris yang cocok" di sana.
func (s *Service) PGExploreUpdateRow(req PGRowMutateRequest) error {
	schema := pgSchemaOrDefault(req.Schema)
	if err := validateSQLIdent(schema); err != nil {
		return err
	}
	if err := validateSQLIdent(req.Table); err != nil {
		return err
	}
	if len(req.Values) == 0 {
		return errFmt("tidak ada kolom untuk diperbarui")
	}

	setClause, setArgs, err := buildPGSetClause(req.Values, 1)
	if err != nil {
		return err
	}
	whereClause, whereArgs, err := buildPGWhereClause(req.Where, len(setArgs)+1)
	if err != nil {
		return err
	}

	db, _, err := s.openPGExploreStored(req.PGTableRequest)
	if err != nil {
		return err
	}

	qualified := quotePGIdent(schema) + "." + quotePGIdent(req.Table)
	q := "UPDATE " + qualified + " SET " + setClause +
		" WHERE ctid = (SELECT ctid FROM " + qualified + " WHERE " + whereClause + " LIMIT 1)"
	res, err := db.Exec(q, append(setArgs, whereArgs...)...)
	if err != nil {
		return errFmt("perbarui baris: %v", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errFmt("tidak ada baris yang cocok — mungkin sudah diubah/dihapus dari tempat lain")
	}
	return nil
}

// PGExploreDeleteRow menghapus satu baris yang cocok dengan req.Where
// (pola ctid sub-query yang sama dengan UpdateRow).
func (s *Service) PGExploreDeleteRow(req PGRowMutateRequest) error {
	schema := pgSchemaOrDefault(req.Schema)
	if err := validateSQLIdent(schema); err != nil {
		return err
	}
	if err := validateSQLIdent(req.Table); err != nil {
		return err
	}
	whereClause, whereArgs, err := buildPGWhereClause(req.Where, 1)
	if err != nil {
		return err
	}

	db, _, err := s.openPGExploreStored(req.PGTableRequest)
	if err != nil {
		return err
	}

	qualified := quotePGIdent(schema) + "." + quotePGIdent(req.Table)
	q := "DELETE FROM " + qualified + " WHERE ctid = (SELECT ctid FROM " + qualified + " WHERE " + whereClause + " LIMIT 1)"
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

// PGExploreExecuteQuery menjalankan satu statement SQL bebas — search_path
// diset ke schema yang dipilih dulu, di koneksi FISIK yang SAMA (lewat
// db.Conn, bukan db.Exec biasa) supaya konsisten dengan query sesudahnya,
// persis alasan yang sama dengan "USE database" MySQL (lihat komentar di
// MySQLExploreExecuteQuery — bug yang sama kalau dua statement ini lewat
// db.Exec/db.Query terpisah bisa jatuh di koneksi fisik berbeda begitu
// pool membesar).
func (s *Service) PGExploreExecuteQuery(req PGQueryRequest) (*PGQueryResult, error) {
	schema := pgSchemaOrDefault(req.Schema)
	if err := validateSQLIdent(schema); err != nil {
		return nil, err
	}
	sqlText := strings.TrimSpace(req.SQL)
	if sqlText == "" {
		return nil, errFmt("query kosong")
	}
	if !singleStatementSQL(sqlText) {
		return nil, errFmt("hanya satu statement per eksekusi (pisahkan jadi beberapa kali jalankan)")
	}

	db, _, err := s.openPGExploreStored(req.PGTableRequest)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, errFmt("ambil koneksi: %v", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SET search_path TO "+quotePGIdent(schema)); err != nil {
		return nil, errFmt("atur schema: %v", err)
	}

	if isSelectLikeSQL(sqlText) {
		rows, err := conn.QueryContext(ctx, sqlText)
		if err != nil {
			return nil, errFmt("query gagal: %v", err)
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return nil, err
		}
		result := &PGQueryResult{Columns: cols, IsSelect: true}
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

	res, err := conn.ExecContext(ctx, sqlText)
	if err != nil {
		return nil, errFmt("eksekusi gagal: %v", err)
	}
	n, _ := res.RowsAffected()
	return &PGQueryResult{RowsAffected: n, IsSelect: false}, nil
}
