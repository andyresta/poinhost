package website

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Kredensial database (password user MySQL/role PostgreSQL) disimpan LOKAL
// di mesin user lewat secrets.Vault (OS keychain, fallback file AES-256-GCM)
// — TIDAK PERNAH ditulis ke server target, beda dari homepoin yang menaruh
// file JSON terenkripsi DI server yang dikelola (lihat catatan di
// database.go & vault.go untuk alasannya). Tabel website_db_credentials di
// SQLite lokal cuma menyimpan METADATA (server/engine/username/host mana
// yang sudah tersimpan) supaya UI bisa menampilkan daftarnya — isi
// passwordnya sendiri ada di vault.
//
// Kolom "host" berarti beda hal per engine: untuk MySQL itu grant host asli
// (mis. "%", "localhost") — bagian dari identitas user MySQL. Untuk
// PostgreSQL, role TIDAK terikat host (itu urusan pg_hba.conf, bukan
// identitas role), jadi kolom ini selalu "-" (lihat dbCredentialHost).

// DBCredentialInfo satu kredensial database yang sudah tersimpan di vault
// lokal (dipakai untuk fitur Explore) — TIDAK PERNAH membawa password.
type DBCredentialInfo struct {
	ServerID   string `json:"serverId"`
	Engine     string `json:"engine"`
	Username   string `json:"username"`
	Host       string `json:"host"`
	VerifiedAt string `json:"verifiedAt,omitempty"`
	UpdatedAt  string `json:"updatedAt"`
}

// SaveDBCredentialRequest menyimpan/memperbarui password satu user database
// di vault lokal — dipanggil sesudah membuat user baru (opsional, lewat
// centang "simpan untuk explore nanti"), atau manual untuk menghubungkan
// user yang sudah ada di server (dibuat di luar poinhost) supaya bisa
// di-explore juga.
type SaveDBCredentialRequest struct {
	ServerID string `json:"serverId"`
	Engine   string `json:"engine"`
	Username string `json:"username"`
	Host     string `json:"host,omitempty"` // MySQL saja, default "%" — diabaikan untuk PostgreSQL
	Password string `json:"password"`
	// Verify: kalau true, coba benar-benar konek dulu sebelum menyimpan —
	// supaya vault tidak pernah menyimpan password yang salah/basi. Untuk
	// PostgreSQL, verifikasi konek ke database "postgres" (maintenance DB
	// bawaan, sama seperti default runPostgres di database.go).
	Verify bool `json:"verify"`
}

// dbCredentialHost menormalkan kolom "host" sesuai engine — MySQL benar-benar
// punya konsep grant host (bagian dari identitas user), PostgreSQL TIDAK
// (role tidak terikat host sama sekali, itu urusan pg_hba.conf) — jadi host
// yang diberikan untuk PostgreSQL SELALU diabaikan, dipaksa "-".
func dbCredentialHost(engine, host string) string {
	if engine != "mysql" {
		return "-"
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "%"
	}
	return host
}

// vaultKeyForDBCredential membangun key deterministik di vault untuk satu
// kredensial — tidak perlu kolom terpisah, cukup dihitung ulang dari
// (server, engine, username, host) setiap kali.
func vaultKeyForDBCredential(serverID, engine, username, host string) string {
	return fmt.Sprintf("dbcred:%s:%s:%s:%s", serverID, engine, username, host)
}

// SaveDBCredential menyimpan password user database ke vault lokal + catat
// metadatanya di SQLite. Kalau req.Verify true (dan engine mysql), coba
// konek dulu — gagal auth berarti TIDAK disimpan, supaya vault tidak pernah
// menyimpan kredensial yang salah.
func (s *Service) SaveDBCredential(req SaveDBCredentialRequest) (*DBCredentialInfo, error) {
	engine, err := normalizeDBEngine(req.Engine)
	if err != nil {
		return nil, err
	}
	username, err := normalizeDBUsername(req.Username)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Password) == "" {
		return nil, errFmt("password wajib diisi")
	}
	host := dbCredentialHost(engine, req.Host)

	// Buang dulu koneksi Explore yang mungkin masih hidup di cache untuk
	// kredensial ini (lihat dbconnpool.go) — kalau tidak, operasi Explore
	// berikutnya bisa diam-diam tetap memakai koneksi lama yang sudah
	// terautentikasi dengan password SEBELUMNYA, bukan yang baru saja
	// disimpan di sini. PostgreSQL bisa punya BANYAK koneksi ter-cache
	// sekaligus (satu per database yang pernah di-browse dengan role ini),
	// jadi dibuang semuanya lewat prefix, bukan satu key spesifik.
	switch engine {
	case "mysql":
		s.evictDBConn(mysqlConnCacheKey(req.ServerID, username, host))
	case "postgresql":
		s.evictDBConnsWithPrefix(dbConnCacheKeyPrefix("postgresql", req.ServerID, username))
	}

	verifiedAt := ""
	if req.Verify {
		var verifyErr error
		switch engine {
		case "mysql":
			verifyErr = s.verifyMySQLCredential(req.ServerID, username, host, req.Password)
		case "postgresql":
			verifyErr = s.verifyPGCredential(req.ServerID, username, req.Password)
		}
		if verifyErr != nil {
			return nil, verifyErr
		}
		verifiedAt = time.Now().UTC().Format(time.RFC3339)
	}

	key := vaultKeyForDBCredential(req.ServerID, engine, username, host)
	if err := s.vault.Set(key, req.Password); err != nil {
		return nil, errFmt("simpan password ke vault lokal: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`
		INSERT INTO website_db_credentials (id, server_id, engine, username, host, verified_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(server_id, engine, username, host) DO UPDATE SET
			verified_at = excluded.verified_at,
			updated_at = excluded.updated_at
	`, uuid.NewString(), req.ServerID, engine, username, host, nullIfEmpty(verifiedAt), now, now)
	if err != nil {
		return nil, errFmt("simpan metadata kredensial: %v", err)
	}

	return &DBCredentialInfo{
		ServerID: req.ServerID, Engine: engine, Username: username, Host: host,
		VerifiedAt: verifiedAt, UpdatedAt: now,
	}, nil
}

// ForgetDBCredential menghapus password dari vault + metadatanya.
func (s *Service) ForgetDBCredential(serverID, engine, username, host string) error {
	engine, err := normalizeDBEngine(engine)
	if err != nil {
		return err
	}
	host = dbCredentialHost(engine, host)
	key := vaultKeyForDBCredential(serverID, engine, username, host)
	if err := s.vault.Delete(key); err != nil {
		return errFmt("hapus password dari vault lokal: %v", err)
	}
	_, err = s.db.Exec(`DELETE FROM website_db_credentials WHERE server_id = ? AND engine = ? AND username = ? AND host = ?`,
		serverID, engine, username, host)
	if err != nil {
		return errFmt("hapus metadata kredensial: %v", err)
	}
	switch engine {
	case "mysql":
		s.evictDBConn(mysqlConnCacheKey(serverID, username, host))
	case "postgresql":
		s.evictDBConnsWithPrefix(dbConnCacheKeyPrefix("postgresql", serverID, username))
	}
	return nil
}

// ListDBCredentials mengembalikan daftar kredensial yang sudah tersimpan
// untuk satu server (tanpa password-nya).
func (s *Service) ListDBCredentials(serverID string) ([]DBCredentialInfo, error) {
	rows, err := s.db.Query(`
		SELECT engine, username, host, COALESCE(verified_at, ''), updated_at
		FROM website_db_credentials WHERE server_id = ? ORDER BY username ASC, host ASC
	`, serverID)
	if err != nil {
		return nil, errFmt("baca daftar kredensial: %v", err)
	}
	defer rows.Close()

	out := make([]DBCredentialInfo, 0)
	for rows.Next() {
		var c DBCredentialInfo
		if err := rows.Scan(&c.Engine, &c.Username, &c.Host, &c.VerifiedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.ServerID = serverID
		out = append(out, c)
	}
	return out, rows.Err()
}

// getDBCredentialPassword mengambil password dari vault lokal (dipakai
// internal oleh mysqlexplore.go) — TIDAK PERNAH diekspos ke frontend.
func (s *Service) getDBCredentialPassword(serverID, engine, username, host string) (string, bool, error) {
	key := vaultKeyForDBCredential(serverID, engine, username, host)
	return s.vault.Get(key)
}

// ListAllDBCredentials mengembalikan daftar kredensial tersimpan di SEMUA
// server (bukan server_id, dipakai backup.Service untuk export — lihat
// internal/core/backup).
func (s *Service) ListAllDBCredentials() ([]DBCredentialInfo, error) {
	rows, err := s.db.Query(`
		SELECT server_id, engine, username, host, COALESCE(verified_at, ''), updated_at
		FROM website_db_credentials ORDER BY server_id ASC, username ASC
	`)
	if err != nil {
		return nil, errFmt("baca daftar kredensial: %v", err)
	}
	defer rows.Close()

	out := make([]DBCredentialInfo, 0)
	for rows.Next() {
		var c DBCredentialInfo
		if err := rows.Scan(&c.ServerID, &c.Engine, &c.Username, &c.Host, &c.VerifiedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ExportCredentialPassword mengambil password dari vault lokal untuk
// dipakai backup.Service saat menyusun arsip export — TIDAK dipakai jalur
// lain mana pun (tidak diekspos sebagai binding Wails biasa), supaya
// password mentah tidak "bocor" lewat API umum di luar alur backup.
func (s *Service) ExportCredentialPassword(serverID, engine, username, host string) (string, bool, error) {
	return s.getDBCredentialPassword(serverID, engine, username, host)
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
