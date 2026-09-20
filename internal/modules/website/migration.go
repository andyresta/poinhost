package website

import (
	"strconv"
	"strings"
)

// File ini adalah seam khusus modul migrasi database antar server
// (internal/modules/dbxfer): satu-satunya titik di luar paket ini yang
// boleh menyentuh resolveAccess/wrap/runMySQL/runPostgres milik modul
// Database — supaya dump/restore SELALU memakai konteks akses yang sama
// (root lewat unix socket MySQL, `sudo -u postgres` PostgreSQL) dengan
// SETIAP operasi database lain di poinhost (lihat header database.go).
//
// Beda mendasar dari homepoin: homepoin menjalankan dump/restore sebagai
// USER APLIKASI tertentu (perlu resolve kredensial per username+host,
// menulis file .cnf/.pgpass sementara di kedua server, dan men-strip
// DEFINER=user@host dari hasil dump karena restore sebagai user biasa
// butuh privilege SUPER/SET_USER_ID). Di sini dump DAN restore berjalan
// sebagai root/postgres — privilege itu sudah ada dengan sendirinya, jadi
// tidak perlu kredensial APA PUN dan tidak perlu strip DEFINER sama sekali.

// WrapCommand membungkus skrip database-adjacent sembarang (dump/gzip/
// restore yang dijalankan di sesi SSH khusus untuk streaming, bukan lewat
// executor bersama) lewat lapisan sudo yang sama dipakai seluruh modul ini.
func (s *Service) WrapCommand(serverID, script string) (string, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return "", err
	}
	return access.wrap(script), nil
}

// DBDatabaseExists memeriksa apakah database sudah ada di server — dipakai
// migrasi untuk peringatan non-blocking sebelum restore menimpa isi yang
// sudah ada (beda dari homepoin yang diam-diam menimpa tanpa memberi tahu).
func (s *Service) DBDatabaseExists(serverID, engine, name string) (bool, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return false, err
	}
	dbName, err := normalizeDBName(name)
	if err != nil {
		return false, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return false, err
	}
	if engine == "mysql" {
		res, err := s.runMySQL(access, "SHOW DATABASES LIKE '"+mysqlEscape(dbName)+"'")
		if err != nil {
			return false, err
		}
		return strings.TrimSpace(res) != "", nil
	}
	res, err := s.runPostgres(access, "", "SELECT 1 FROM pg_database WHERE datname = '"+pgEscape(dbName)+"'")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(res) == "1", nil
}

// DBTableNames mengembalikan semua nama tabel di satu database — SEMUA
// jenis relasi biasa (termasuk VIEW), bukan cuma BASE TABLE. homepoin
// membatasi verifikasinya ke BASE TABLE saja, sehingga database yang
// isinya cuma VIEW selalu dilaporkan "0 tabel" alias gagal walau restore-
// nya sendiri berhasil sempurna — daftar di sini dipakai baik untuk
// menyelesaikan pilihan "semua tabel" di request migrasi maupun untuk
// verifikasi pasca-restore, jadi kedua sisi konsisten.
func (s *Service) DBTableNames(serverID, engine, database string) ([]string, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return nil, err
	}
	dbName, err := normalizeDBName(database)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0)
	if engine == "mysql" {
		res, err := s.runMySQL(access, "SELECT TABLE_NAME FROM information_schema.tables WHERE table_schema = '"+mysqlEscape(dbName)+"'")
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(res, "\n") {
			name := strings.TrimSpace(line)
			if name != "" {
				out = append(out, name)
			}
		}
		return out, nil
	}
	// Semua schema non-sistem, bukan cuma "public" — pg_dump tanpa -n
	// mendumpkan SEMUA schema, jadi verifikasi yang membatasi diri ke
	// public akan buta terhadap tabel di schema custom (gap nyata lain
	// yang ditemukan di homepoin).
	res, err := s.runPostgres(access, dbName,
		"SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace "+
			"WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(res, "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			out = append(out, name)
		}
	}
	return out, nil
}

// DBTableRowCounts menghitung jumlah baris SEBENARNYA (SELECT COUNT(*),
// bukan taksiran information_schema/pg_class yang bisa selisih jauh untuk
// InnoDB) untuk tiap tabel yang diminta, satu-satu. Dipakai verifikasi
// migrasi untuk membandingkan sumber vs tujuan secara akurat — keputusan
// yang dikunci eksplisit bareng user, sadar konsekuensinya: untuk tabel
// yang sangat besar ini butuh scan penuh di kedua sisi.
func (s *Service) DBTableRowCounts(serverID, engine, database string, tables []string) (map[string]int64, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return nil, err
	}
	dbName, err := normalizeDBName(database)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(tables))
	for _, raw := range tables {
		table, err := normalizeDBName(raw)
		if err != nil {
			return nil, err
		}
		var res string
		if engine == "mysql" {
			res, err = s.runMySQL(access, "SELECT COUNT(*) FROM `"+dbName+"`.`"+table+"`")
		} else {
			res, err = s.runPostgres(access, dbName, `SELECT COUNT(*) FROM "`+table+`"`)
		}
		if err != nil {
			return nil, err
		}
		n, convErr := strconv.ParseInt(strings.TrimSpace(res), 10, 64)
		if convErr != nil {
			return nil, errFmt("hasil COUNT(*) tabel %q tidak terbaca: %q", table, res)
		}
		out[table] = n
	}
	return out, nil
}

// DBEstimateSize menaksir ukuran mentah database (atau subset tabelnya)
// lewat statistik katalog engine — SENGAJA taksiran (data_length+
// index_length di MySQL, pg_total_relation_size di PostgreSQL), BUKAN
// dipakai untuk verifikasi (lihat DBTableRowCounts untuk itu), cuma untuk
// mengisi persentase progress bar sebelum dump benar-benar berjalan.
// Best-effort: gagal berarti 0, migrasi tetap jalan tanpa persentase yang
// akurat untuk item itu — sama seperti scanSize di filexfer.
func (s *Service) DBEstimateSize(serverID, engine, database string, tables []string) int64 {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return 0
	}
	dbName, err := normalizeDBName(database)
	if err != nil {
		return 0
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return 0
	}
	if engine == "mysql" {
		sql := "SELECT COALESCE(SUM(data_length+index_length),0) FROM information_schema.tables WHERE table_schema = '" + mysqlEscape(dbName) + "'"
		if len(tables) > 0 {
			quoted := make([]string, 0, len(tables))
			for _, t := range tables {
				if t, err := normalizeDBName(t); err == nil {
					quoted = append(quoted, "'"+mysqlEscape(t)+"'")
				}
			}
			sql += " AND table_name IN (" + strings.Join(quoted, ",") + ")"
		}
		res, err := s.runMySQL(access, sql)
		if err != nil {
			return 0
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(res), 10, 64)
		return n
	}
	if len(tables) == 0 {
		res, err := s.runPostgres(access, "", "SELECT pg_database_size('"+pgEscape(dbName)+"')")
		if err != nil {
			return 0
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(res), 10, 64)
		return n
	}
	var total int64
	for _, t := range tables {
		table, err := normalizeDBName(t)
		if err != nil {
			continue
		}
		res, err := s.runPostgres(access, dbName, `SELECT pg_total_relation_size('"`+table+`"')`)
		if err != nil {
			continue
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(res), 10, 64)
		total += n
	}
	return total
}

// DBEngineBinary mengembalikan nama biner dump/restore untuk satu engine —
// dipakai dbxfer menyusun perintah shell tanpa perlu tahu detail nama biner
// per engine (mysqldump/mysql vs pg_dump/psql).
func DBEngineBinary(engine string) (dump, restore string, ok bool) {
	switch engine {
	case "mysql":
		return "mysqldump", "mysql", true
	case "postgresql":
		return "pg_dump", "psql", true
	default:
		return "", "", false
	}
}
