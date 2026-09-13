package servers

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

const testSchema = `
CREATE TABLE servers (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    host          TEXT NOT NULL,
    port          INTEGER NOT NULL DEFAULT 22,
    username      TEXT NOT NULL,
    auth_type     TEXT NOT NULL DEFAULT 'key',
    key_path      TEXT,
    password_enc  TEXT,
    tags          TEXT NOT NULL DEFAULT '[]',
    color         TEXT NOT NULL DEFAULT '#6366f1',
    notes         TEXT NOT NULL DEFAULT '',
    is_active     INTEGER NOT NULL DEFAULT 1,
    use_sudo      INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
`

func newTestRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("buka db uji: %v", err)
	}
	if _, err := db.Exec(testSchema); err != nil {
		t.Fatalf("buat skema uji: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(db)
}

func TestRepositoryCreateUpdateNeverTouchPasswordColumn(t *testing.T) {
	repo := newTestRepo(t)
	srv := &Server{ID: "s1", Name: "srv1", Host: "1.2.3.4", Port: 22, Username: "root", AuthType: "password"}
	if err := repo.Create(srv); err != nil {
		t.Fatalf("Create: %v", err)
	}

	legacy, err := repo.ListLegacyPasswords()
	if err != nil {
		t.Fatalf("ListLegacyPasswords: %v", err)
	}
	if len(legacy) != 0 {
		t.Fatalf("Create seharusnya tidak pernah menulis password_enc, got %v", legacy)
	}

	srv.Name = "srv1-renamed"
	if err := repo.Update(srv); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := repo.Get("s1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "srv1-renamed" {
		t.Fatalf("Update tidak menyimpan perubahan metadata, got name=%q", got.Name)
	}
}

func TestListAndClearLegacyPasswords(t *testing.T) {
	repo := newTestRepo(t)
	if err := repo.Create(&Server{ID: "s1", Name: "a", Host: "h", Username: "u", AuthType: "password"}); err != nil {
		t.Fatalf("Create s1: %v", err)
	}
	if err := repo.Create(&Server{ID: "s2", Name: "b", Host: "h", Username: "u", AuthType: "key"}); err != nil {
		t.Fatalf("Create s2: %v", err)
	}
	// Simula password lama yang ditulis versi poinhost SEBELUM vault ini ada
	// — langsung ke kolomnya, karena Create/Update sekarang sengaja tidak
	// pernah menulisi kolom ini lagi.
	if _, err := repo.db.Exec(`UPDATE servers SET password_enc = ? WHERE id = ?`, "old-plaintext-pw", "s1"); err != nil {
		t.Fatalf("seed password lama: %v", err)
	}

	legacy, err := repo.ListLegacyPasswords()
	if err != nil {
		t.Fatalf("ListLegacyPasswords: %v", err)
	}
	if len(legacy) != 1 || legacy["s1"] != "old-plaintext-pw" {
		t.Fatalf("ListLegacyPasswords = %v, want cuma s1", legacy)
	}

	if err := repo.ClearLegacyPassword("s1"); err != nil {
		t.Fatalf("ClearLegacyPassword: %v", err)
	}
	legacy, err = repo.ListLegacyPasswords()
	if err != nil {
		t.Fatalf("ListLegacyPasswords sesudah clear: %v", err)
	}
	if len(legacy) != 0 {
		t.Fatalf("password_enc s1 seharusnya kosong sesudah ClearLegacyPassword, got %v", legacy)
	}
}
