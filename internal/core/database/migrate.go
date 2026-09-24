package database

import (
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// Migrate menjalankan migrasi bawaan package ini. Ini yang dipakai jalur
// normal — app desktop maupun agent — supaya keduanya tidak pernah bisa
// menjalankan schema yang berbeda.
func Migrate(db *sql.DB) error {
	return MigrateFS(db, embeddedMigrations)
}

// MigrateFS menjalankan semua file migrasi SQL idempotent dari fs.FS mana pun.
// Setiap file harus aman diulang (CREATE IF NOT EXISTS, INSERT OR IGNORE, dll).
// Dipisah dari Migrate supaya test bisa menyuntikkan kumpulan migrasi sendiri.
func MigrateFS(db *sql.DB, migrationsFS fs.FS) error {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("baca folder migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		content, err := fs.ReadFile(migrationsFS, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("baca migrasi %s: %w", name, err)
		}
		if err := execMigration(db, name, string(content)); err != nil {
			return err
		}
	}

	return nil
}

func execMigration(db *sql.DB, name, sqlContent string) error {
	return WithBusyRetry(func() error {
		if _, err := db.Exec(sqlContent); err != nil {
			return fmt.Errorf("jalankan migrasi %s: %w", name, err)
		}
		return nil
	})
}
