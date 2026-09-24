package database

import (
	"database/sql"
	"path/filepath"
	"sort"
	"testing"
	"testing/fstest"
)

func openTempDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "uji.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// Migrate tidak lagi menerima embed.FS dari pemanggil: migrasinya menempel di
// package ini. Yang diuji di sini adalah konsekuensinya — binary mana pun
// (app desktop maupun agent) cukup memanggil Migrate(db) dan mendapat schema
// yang sama, tanpa menyalin folder migrations.
func TestMigrate_MemakaiMigrasiBawaanPackage(t *testing.T) {
	db := openTempDB(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	tabel := daftarTabel(t, db)
	// Tabel inti dari 001_core.sql — kalau embed-nya kosong atau path-nya
	// salah, Migrate tetap "sukses" tanpa membuat apa pun, jadi keberadaan
	// tabel inilah buktinya.
	for _, mau := range []string{"servers", "app_settings", "ui_tabs"} {
		if !punya(tabel, mau) {
			t.Errorf("tabel %q tidak ada setelah Migrate; yang ada: %v", mau, tabel)
		}
	}
}

// Migrasi dijalankan tiap startup, jadi menjalankannya dua kali harus aman.
func TestMigrate_AmanDijalankanBerulang(t *testing.T) {
	db := openTempDB(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate pertama: %v", err)
	}
	sebelum := daftarTabel(t, db)

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate kedua: %v", err)
	}
	sesudah := daftarTabel(t, db)

	if len(sebelum) != len(sesudah) {
		t.Errorf("jumlah tabel berubah setelah migrasi diulang: %v -> %v", sebelum, sesudah)
	}
}

// MigrateFS tetap ada supaya test bisa menjalankan schema sendiri tanpa
// menyentuh migrasi sungguhan.
func TestMigrateFS_MenerimaFSDariPemanggil(t *testing.T) {
	db := openTempDB(t)

	fsys := fstest.MapFS{
		"migrations/001_uji.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE IF NOT EXISTS cuma_untuk_test (id INTEGER PRIMARY KEY);`),
		},
	}
	if err := MigrateFS(db, fsys); err != nil {
		t.Fatalf("MigrateFS: %v", err)
	}
	if !punya(daftarTabel(t, db), "cuma_untuk_test") {
		t.Error("tabel dari FS pemanggil tidak dibuat")
	}
}

// File non-.sql diabaikan, bukan dieksekusi dan bikin error.
func TestMigrateFS_MelewatiBerkasBukanSQL(t *testing.T) {
	db := openTempDB(t)

	fsys := fstest.MapFS{
		"migrations/README.md":   &fstest.MapFile{Data: []byte("bukan sql")},
		"migrations/001_ok.sql":  &fstest.MapFile{Data: []byte(`CREATE TABLE IF NOT EXISTS ok (id INTEGER);`)},
		"migrations/catatan.txt": &fstest.MapFile{Data: []byte("juga bukan sql")},
	}
	if err := MigrateFS(db, fsys); err != nil {
		t.Fatalf("MigrateFS: %v", err)
	}
	if !punya(daftarTabel(t, db), "ok") {
		t.Error("migrasi .sql tidak dijalankan")
	}
}

func daftarTabel(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	sort.Strings(out)
	return out
}

func punya(xs []string, mau string) bool {
	for _, x := range xs {
		if x == mau {
			return true
		}
	}
	return false
}
