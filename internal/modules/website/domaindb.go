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

// SetDatabaseDomains menyamakan SEMUA domain yang menautkan satu database
// dengan `domains` yang diberikan — replace PENUH (hapus dulu semua tautan
// lama untuk database ini, lalu tulis ulang yang baru), bukan toggle satu
// domain per panggilan seperti Link/UnlinkDomainDatabase. Dipakai dialog
// "Kelola domain" yang mendukung multi-select, supaya satu database bisa
// ditautkan ke BEBERAPA domain/subdomain sekaligus (mis. domain utama dan
// alias-nya) dalam satu kali simpan, bukan satu per satu.
func (s *Service) SetDatabaseDomains(serverID, engine, dbName string, domains []string) error {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return err
	}
	dbName, err = normalizeDBName(dbName)
	if err != nil {
		return err
	}

	clean := make([]string, 0, len(domains))
	seen := make(map[string]bool, len(domains))
	for _, d := range domains {
		d = strings.TrimSpace(d)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		clean = append(clean, d)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return errFmt("mulai transaksi tautan domain-database: %v", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		DELETE FROM website_domain_databases WHERE server_id = ? AND engine = ? AND db_name = ?
	`, serverID, engine, dbName); err != nil {
		return errFmt("hapus tautan lama: %v", err)
	}
	for _, d := range clean {
		if _, err := tx.Exec(`
			INSERT INTO website_domain_databases (id, server_id, domain, engine, db_name)
			VALUES (?, ?, ?, ?, ?)
		`, uuid.NewString(), serverID, d, engine, dbName); err != nil {
			return errFmt("simpan tautan domain %q: %v", d, err)
		}
	}
	return tx.Commit()
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

// ListServerDatabaseDomains adalah arah KEBALIKAN dari ListDomainDatabases:
// bukan "database mana yang dipakai satu domain", tapi "domain mana yang
// memakai tiap database" di satu server+engine — dipakai panel Database
// TOP-LEVEL (tanpa domain tertentu) supaya kolom "Domain" di daftar
// database bisa terisi juga di luar konteks tab per-domain Website, bukan
// cuma satu baris ringkasan yang hanya muncul kalau panel dibuka dari
// dalam satu domain (lihat DatabaseManagerPanel.tsx).
func (s *Service) ListServerDatabaseDomains(serverID, engine string) ([]DomainDatabaseLink, error) {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT domain, db_name FROM website_domain_databases
		WHERE server_id = ? AND engine = ? ORDER BY db_name ASC, domain ASC
	`, serverID, engine)
	if err != nil {
		return nil, errFmt("baca tautan domain-database: %v", err)
	}
	defer rows.Close()

	out := make([]DomainDatabaseLink, 0)
	for rows.Next() {
		var l DomainDatabaseLink
		if err := rows.Scan(&l.Domain, &l.Database); err != nil {
			return nil, err
		}
		l.ServerID = serverID
		l.Engine = engine
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
