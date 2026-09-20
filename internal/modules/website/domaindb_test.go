package website

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// newDomainDBTestService membuat *sql.DB SQLite in-memory dengan tabel
// website_domain_databases (skema disalin dari migrations/001_core.sql,
// tanpa FK ke servers — SQLite tidak menegakkan FK kecuali diminta, jadi
// tidak perlu tabel servers sungguhan untuk tes ini).
func newDomainDBTestService(t *testing.T) *Service {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("buka SQLite in-memory: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE website_domain_databases (
			id            TEXT PRIMARY KEY,
			server_id     TEXT NOT NULL,
			domain        TEXT NOT NULL,
			engine        TEXT NOT NULL DEFAULT 'mysql',
			db_name       TEXT NOT NULL,
			created_at    TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE(server_id, domain, engine, db_name)
		)
	`)
	if err != nil {
		t.Fatalf("buat tabel website_domain_databases: %v", err)
	}
	return &Service{db: db}
}

func TestLinkUnlinkDomainDatabase(t *testing.T) {
	s := newDomainDBTestService(t)

	if err := s.LinkDomainDatabase("srv1", "app.example.com", "mysql", "appdb"); err != nil {
		t.Fatalf("LinkDomainDatabase: %v", err)
	}
	// Menautkan dua kali (mis. user klik ulang) harus idempotent, bukan error.
	if err := s.LinkDomainDatabase("srv1", "app.example.com", "mysql", "appdb"); err != nil {
		t.Fatalf("LinkDomainDatabase (ulang) seharusnya idempotent: %v", err)
	}

	links, err := s.ListDomainDatabases("srv1", "app.example.com")
	if err != nil {
		t.Fatalf("ListDomainDatabases: %v", err)
	}
	if len(links) != 1 || links[0].Database != "appdb" || links[0].Engine != "mysql" {
		t.Fatalf("ListDomainDatabases = %+v, mau 1 entri appdb/mysql", links)
	}

	if err := s.UnlinkDomainDatabase("srv1", "app.example.com", "mysql", "appdb"); err != nil {
		t.Fatalf("UnlinkDomainDatabase: %v", err)
	}
	links, err = s.ListDomainDatabases("srv1", "app.example.com")
	if err != nil {
		t.Fatalf("ListDomainDatabases setelah unlink: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("ListDomainDatabases setelah unlink = %+v, mau kosong", links)
	}
}

// ListServerDatabaseDomains adalah arah kebalikan dari ListDomainDatabases —
// dipakai kolom "Domain" di panel Database supaya terisi juga di modul
// top-level per server (di luar konteks satu domain), bukan cuma satu
// baris ringkasan yang sebelumnya hanya muncul di dalam tab per-domain.
func TestListServerDatabaseDomains(t *testing.T) {
	s := newDomainDBTestService(t)

	// Satu database dipakai DUA domain sekaligus (mis. domain utama + alias).
	if err := s.LinkDomainDatabase("srv1", "app.example.com", "mysql", "appdb"); err != nil {
		t.Fatalf("link 1: %v", err)
	}
	if err := s.LinkDomainDatabase("srv1", "alias.example.com", "mysql", "appdb"); err != nil {
		t.Fatalf("link 2: %v", err)
	}
	if err := s.LinkDomainDatabase("srv1", "blog.example.com", "mysql", "blogdb"); err != nil {
		t.Fatalf("link 3: %v", err)
	}
	// Server lain dan engine lain TIDAK boleh ikut muncul.
	if err := s.LinkDomainDatabase("srv2", "other.example.com", "mysql", "appdb"); err != nil {
		t.Fatalf("link server lain: %v", err)
	}
	if err := s.LinkDomainDatabase("srv1", "pgsite.example.com", "postgresql", "appdb"); err != nil {
		t.Fatalf("link engine lain: %v", err)
	}

	links, err := s.ListServerDatabaseDomains("srv1", "mysql")
	if err != nil {
		t.Fatalf("ListServerDatabaseDomains: %v", err)
	}
	if len(links) != 3 {
		t.Fatalf("ListServerDatabaseDomains = %+v, mau 3 entri (bukan dari server/engine lain)", links)
	}

	byDB := map[string][]string{}
	for _, l := range links {
		if l.ServerID != "srv1" || l.Engine != "mysql" {
			t.Fatalf("entri bocor dari server/engine lain: %+v", l)
		}
		byDB[l.Database] = append(byDB[l.Database], l.Domain)
	}
	if len(byDB["appdb"]) != 2 {
		t.Fatalf("appdb harus tertaut ke 2 domain, dapat %v", byDB["appdb"])
	}
	if len(byDB["blogdb"]) != 1 {
		t.Fatalf("blogdb harus tertaut ke 1 domain, dapat %v", byDB["blogdb"])
	}
}

func TestListServerDatabaseDomains_RejectsUnknownEngine(t *testing.T) {
	s := newDomainDBTestService(t)
	if _, err := s.ListServerDatabaseDomains("srv1", "oracle"); err == nil {
		t.Fatal("engine tidak dikenal seharusnya ditolak")
	}
}

// SetDatabaseDomains adalah REPLACE PENUH, dipakai dialog "Kelola domain"
// multi-select — satu panggilan bisa menambah DAN melepas beberapa tautan
// sekaligus, bukan toggle satu domain per klik.
func TestSetDatabaseDomains_ReplacesFully(t *testing.T) {
	s := newDomainDBTestService(t)

	if err := s.SetDatabaseDomains("srv1", "mysql", "appdb", []string{"a.example.com", "b.example.com"}); err != nil {
		t.Fatalf("SetDatabaseDomains (awal): %v", err)
	}
	links, err := s.ListServerDatabaseDomains("srv1", "mysql")
	if err != nil {
		t.Fatalf("ListServerDatabaseDomains: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("setelah set awal = %+v, mau 2 domain", links)
	}

	// Ganti jadi satu domain lama + satu domain baru — "a.example.com"
	// harus tetap ada, "b.example.com" harus lepas, "c.example.com" baru.
	if err := s.SetDatabaseDomains("srv1", "mysql", "appdb", []string{"a.example.com", "c.example.com"}); err != nil {
		t.Fatalf("SetDatabaseDomains (ganti): %v", err)
	}
	links, err = s.ListServerDatabaseDomains("srv1", "mysql")
	if err != nil {
		t.Fatalf("ListServerDatabaseDomains setelah ganti: %v", err)
	}
	got := map[string]bool{}
	for _, l := range links {
		got[l.Domain] = true
	}
	if len(got) != 2 || !got["a.example.com"] || !got["c.example.com"] || got["b.example.com"] {
		t.Fatalf("hasil setelah ganti = %v, mau tepat {a.example.com, c.example.com}", got)
	}
}

// Daftar kosong berarti lepas SEMUA tautan database ini — dipakai dialog
// "Kelola domain" saat user menghapus centang semua domain.
func TestSetDatabaseDomains_EmptyListClearsAll(t *testing.T) {
	s := newDomainDBTestService(t)
	if err := s.SetDatabaseDomains("srv1", "mysql", "appdb", []string{"a.example.com"}); err != nil {
		t.Fatalf("set awal: %v", err)
	}
	if err := s.SetDatabaseDomains("srv1", "mysql", "appdb", nil); err != nil {
		t.Fatalf("set kosong: %v", err)
	}
	links, err := s.ListServerDatabaseDomains("srv1", "mysql")
	if err != nil {
		t.Fatalf("ListServerDatabaseDomains: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("setelah set kosong = %+v, mau tidak ada tautan", links)
	}
}

// Database lain di server yang sama tidak boleh ikut tersentuh — replace
// hanya berlaku untuk (server, engine, db_name) yang diminta.
func TestSetDatabaseDomains_DoesNotTouchOtherDatabases(t *testing.T) {
	s := newDomainDBTestService(t)
	if err := s.LinkDomainDatabase("srv1", "blog.example.com", "mysql", "blogdb"); err != nil {
		t.Fatalf("link blogdb: %v", err)
	}
	if err := s.SetDatabaseDomains("srv1", "mysql", "appdb", []string{"app.example.com"}); err != nil {
		t.Fatalf("set appdb: %v", err)
	}
	links, err := s.ListServerDatabaseDomains("srv1", "mysql")
	if err != nil {
		t.Fatalf("ListServerDatabaseDomains: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("links = %+v, mau 2 (blogdb tetap ada + appdb baru)", links)
	}
}
