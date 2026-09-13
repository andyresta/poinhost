package servers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Repository menangani CRUD tabel `servers` di SQLite lokal.
type Repository struct {
	db *sql.DB
}

// NewRepository membuat repository baru.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// List mengembalikan semua server terdaftar.
func (r *Repository) List() ([]*Server, error) {
	rows, err := r.db.Query(`
		SELECT id, name, host, port, username, auth_type, key_path, tags,
		       color, notes, is_active, use_sudo, created_at, updated_at
		FROM servers ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Server
	for rows.Next() {
		s, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Get mengembalikan satu server berdasarkan ID.
func (r *Repository) Get(id string) (*Server, error) {
	row := r.db.QueryRow(`
		SELECT id, name, host, port, username, auth_type, key_path, tags,
		       color, notes, is_active, use_sudo, created_at, updated_at
		FROM servers WHERE id = ?`, id)
	return scanServer(row)
}

// Create menyimpan server baru. Password SSH TIDAK lewat sini — itu
// tanggung jawab servers.Service lewat secrets.Vault yang sama dipakai
// kredensial database (lihat migrateLegacyPasswords di service.go).
func (r *Repository) Create(s *Server) error {
	tagsJSON, err := json.Marshal(s.Tags)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`
		INSERT INTO servers (id, name, host, port, username, auth_type, key_path,
		                      tags, color, notes, is_active, use_sudo)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`,
		s.ID, s.Name, s.Host, s.Port, s.Username, s.AuthType, s.KeyPath,
		string(tagsJSON), s.Color, s.Notes, boolToInt(s.UseSudo))
	if err != nil {
		return fmt.Errorf("simpan server: %w", err)
	}
	return nil
}

// Update memperbarui server yang sudah ada (metadata saja — password SSH
// ada di vault, lihat Create).
func (r *Repository) Update(s *Server) error {
	tagsJSON, err := json.Marshal(s.Tags)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`
		UPDATE servers SET name=?, host=?, port=?, username=?, auth_type=?,
		       key_path=?, tags=?, color=?, notes=?, use_sudo=?,
		       updated_at=datetime('now')
		WHERE id=?`,
		s.Name, s.Host, s.Port, s.Username, s.AuthType, s.KeyPath,
		string(tagsJSON), s.Color, s.Notes, boolToInt(s.UseSudo), s.ID)
	if err != nil {
		return fmt.Errorf("update server: %w", err)
	}
	return nil
}

// ListLegacyPasswords mengembalikan password SSH yang masih tersimpan apa
// adanya di kolom password_enc (versi poinhost sebelum dipindah ke
// secrets.Vault) — dipakai SEKALI oleh migrateLegacyPasswords saat startup,
// bukan jalur normal baca password (itu sudah lewat vault).
func (r *Repository) ListLegacyPasswords() (map[string]string, error) {
	rows, err := r.db.Query(`SELECT id, password_enc FROM servers WHERE password_enc IS NOT NULL AND password_enc <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var id, pw string
		if err := rows.Scan(&id, &pw); err != nil {
			return nil, err
		}
		out[id] = pw
	}
	return out, rows.Err()
}

// ClearLegacyPassword mengosongkan password_enc sesudah nilainya berhasil
// dipindah ke vault — supaya tidak ada lagi plaintext yang tertinggal di
// SQLite (lihat migrateLegacyPasswords).
func (r *Repository) ClearLegacyPassword(id string) error {
	_, err := r.db.Exec(`UPDATE servers SET password_enc = NULL WHERE id = ?`, id)
	return err
}

// Delete menghapus server (cascade ke ui_tabs & tabel terkait lain).
func (r *Repository) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM servers WHERE id = ?`, id)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanServer(row rowScanner) (*Server, error) {
	var s Server
	var keyPath sql.NullString
	var tagsJSON string
	var isActive, useSudo int
	var createdAt, updatedAt string

	err := row.Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.Username, &s.AuthType,
		&keyPath, &tagsJSON, &s.Color, &s.Notes, &isActive, &useSudo, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	s.KeyPath = keyPath.String
	s.IsActive = isActive == 1
	s.UseSudo = useSudo == 1
	_ = json.Unmarshal([]byte(tagsJSON), &s.Tags)
	s.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	s.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return &s, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
