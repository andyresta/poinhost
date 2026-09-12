// Package database menangani koneksi SQLite lokal (~/.poinhost/poinhost.db)
// dan migrasi schema. SQLite dipilih (bukan file JSON/config) karena tetap
// dibutuhkan untuk data yang genuinely relasional & queryable: daftar server,
// koneksi database yang dikelola, dsb — sama seperti homepoin.
package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Open membuka koneksi SQLite dengan mode WAL dan busy timeout.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("buka sqlite: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("set pragma %q: %w", p, err)
		}
	}

	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)

	return db, nil
}

// WithBusyRetry menjalankan fungsi dengan retry otomatis jika SQLITE_BUSY.
func WithBusyRetry(fn func() error) error {
	var last error
	for i := 0; i < 5; i++ {
		last = fn()
		if last == nil {
			return nil
		}
		if !isBusy(last) {
			return last
		}
		time.Sleep(time.Duration(100*(i+1)) * time.Millisecond)
	}
	return last
}

func isBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsAny(msg, "database is locked", "SQLITE_BUSY")
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) && indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
