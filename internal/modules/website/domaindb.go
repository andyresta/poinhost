package website

import (
	"strings"

	"github.com/google/uuid"
)

// Tautan domain<->database — METADATA KURASI LOKAL SAJA (tabel
// website_domain_databases di SQLite poinhost sendiri). MySQL/PostgreSQL
// tidak mengenal scoping per domain, jadi ini TIDAK mengubah grants/akses
// apa pun di server — cuma supaya tab Database di suatu domain bisa
// menampilkan "database yang dipakai situs ini", beda dari homepoin yang
// field Domain-nya di tab Database sekadar hiasan UI tak terpakai backend.

// DomainDatabaseLink satu tautan domain->database.
type DomainDatabaseLink struct {
	ServerID string `json:"serverId"`
	Domain   string `json:"domain"`
	Engine   string `json:"engine"`
	Database string `json:"database"`
}

// LinkDomainDatabase menautkan satu database ke satu domain (murni kurasi).
func (s *Service) LinkDomainDatabase(serverID, domain, engine, dbName string) error {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return err
	}
	dbName, err = normalizeDBName(dbName)
	if err != nil {
		return err
	}
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return errFmt("domain wajib diisi")
	}
	_, err = s.db.Exec(`
		INSERT INTO website_domain_databases (id, server_id, domain, engine, db_name)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(server_id, domain, engine, db_name) DO NOTHING
	`, uuid.NewString(), serverID, domain, engine, dbName)
	if err != nil {
		return errFmt("simpan tautan domain-database: %v", err)
	}
	return nil
}

// UnlinkDomainDatabase melepas tautan (tidak menghapus database-nya).
func (s *Service) UnlinkDomainDatabase(serverID, domain, engine, dbName string) error {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		DELETE FROM website_domain_databases WHERE server_id = ? AND domain = ? AND engine = ? AND db_name = ?
	`, serverID, domain, engine, dbName)
	if err != nil {
		return errFmt("hapus tautan domain-database: %v", err)
	}
	return nil
}

// ListAllDomainDatabases mengembalikan SEMUA tautan domain<->database di
// SEMUA server (dipakai backup.Service untuk export — lihat
// internal/core/backup).
func (s *Service) ListAllDomainDatabases() ([]DomainDatabaseLink, error) {
	rows, err := s.db.Query(`
		SELECT server_id, domain, engine, db_name FROM website_domain_databases
		ORDER BY server_id ASC, domain ASC
	`)
	if err != nil {
		return nil, errFmt("baca semua tautan domain-database: %v", err)
	}
	defer rows.Close()

	out := make([]DomainDatabaseLink, 0)
	for rows.Next() {
		var l DomainDatabaseLink
		if err := rows.Scan(&l.ServerID, &l.Domain, &l.Engine, &l.Database); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ListDomainDatabases daftar database yang ditautkan ke satu domain.
func (s *Service) ListDomainDatabases(serverID, domain string) ([]DomainDatabaseLink, error) {
	rows, err := s.db.Query(`
		SELECT engine, db_name FROM website_domain_databases
		WHERE server_id = ? AND domain = ? ORDER BY engine ASC, db_name ASC
	`, serverID, domain)
	if err != nil {
		return nil, errFmt("baca tautan domain-database: %v", err)
	}
	defer rows.Close()

	out := make([]DomainDatabaseLink, 0)
	for rows.Next() {
		var l DomainDatabaseLink
		if err := rows.Scan(&l.Engine, &l.Database); err != nil {
			return nil, err
		}
		l.ServerID = serverID
		l.Domain = domain
		out = append(out, l)
	}
	return out, rows.Err()
}
