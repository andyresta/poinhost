package database

import (
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// Migrate menjalankan semua file migrasi SQL idempotent setiap startup.
// Setiap file harus aman diulang (CREATE IF NOT EXISTS, INSERT OR IGNORE, dll).
func Migrate(db *sql.DB, migrationsFS fs.FS) error {
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
