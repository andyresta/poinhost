package servers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/andyresta/poinhost/internal/core/secrets"
	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/google/uuid"
)

// Service mengorkestrasi CRUD server + daftar/lepas dari sshpool.Pool.
type Service struct {
	repo     *Repository
	pool     *sshpool.Pool
	executor *sshpool.Executor
	// vault menyimpan password SSH server (SAMA instance secrets.Vault yang
	// dipakai website.Service untuk kredensial database) — bukan lagi kolom
	// password_enc di SQLite apa adanya, lihat migrateLegacyPasswords.
	vault secrets.Vault
}

// NewService membuat servers.Service baru.
func NewService(repo *Repository, pool *sshpool.Pool, executor *sshpool.Executor, vault secrets.Vault) *Service {
	return &Service{repo: repo, pool: pool, executor: executor, vault: vault}
}

// vaultKeyForServerPassword key deterministik di vault untuk password SSH
// satu server — tidak perlu kolom terpisah, cukup dihitung ulang dari ID.
func vaultKeyForServerPassword(id string) string {
	return "serverpass:" + id
}

// migrateLegacyPasswords memindahkan password SSH yang masih tersimpan apa
// adanya di kolom password_enc (versi poinhost sebelum vault ini ada) ke
// secrets.Vault, lalu mengosongkan kolom itu — SEKALI jalan tiap startup,
// aman diulang (begitu kolom kosong, ListLegacyPasswords tidak
// mengembalikan apa-apa lagi). Best-effort per server: satu server gagal
// dipindah tidak menghalangi server lain ikut termigrasi.
func (s *Service) migrateLegacyPasswords() error {
	legacy, err := s.repo.ListLegacyPasswords()
	if err != nil {
		return fmt.Errorf("baca password lama: %w", err)
	}
	var errs []error
	for id, pw := range legacy {
		if err := s.vault.Set(vaultKeyForServerPassword(id), pw); err != nil {
			errs = append(errs, fmt.Errorf("migrasi password server %s ke vault: %w", id, err))
			continue
		}
		if err := s.repo.ClearLegacyPassword(id); err != nil {
			errs = append(errs, fmt.Errorf("hapus password lama server %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// Bootstrap memuat semua server dari DB dan mendaftarkannya ke pool +
// langsung memanaskan koneksi shared masing-masing (warm-all-at-startup).
// Ini menghilangkan jeda "terasa seperti handshake dari nol" saat pertama
// kali membuka tab ke sebuah server, yang terjadi di homepoin karena
// WarmConnection cuma dipanggil on-demand per server saat diklik.
func (s *Service) Bootstrap(ctx context.Context) error {
	if err := s.migrateLegacyPasswords(); err != nil {
		// Tidak fatal — server yang belum termigrasi tetap bisa dipakai
		// (registerToPool di bawah ini cuma dapat password kosong untuknya
		// sampai migrasi berhasil di startup berikutnya atau usernya
		// menyimpan ulang lewat form edit).
		log.Printf("migrasi password server lama ke vault: %v", err)
	}

	list, err := s.repo.List()
	if err != nil {
		return err
	}
	for _, srv := range list {
		s.registerToPool(srv)
	}
	go s.warmAll(list)
	return nil
}

func (s *Service) warmAll(list []*Server) {
	for _, srv := range list {
		ctx, cancel := context.WithTimeout(context.Background(), s.pool.DialTimeout()+5_000_000_000)
		_ = s.pool.WarmConnection(ctx, srv.ID)
		cancel()
	}
}

func (s *Service) registerToPool(srv *Server) {
	password, _, _ := s.vault.Get(vaultKeyForServerPassword(srv.ID))
	s.pool.RegisterServer(sshpool.ServerConfig{
		ID:       srv.ID,
		Host:     srv.Host,
		Port:     srv.Port,
		Username: srv.Username,
		AuthType: srv.AuthType,
		KeyPath:  srv.KeyPath,
		Password: password,
	})
}

// List mengembalikan semua server.
func (s *Service) List() ([]*Server, error) {
	return s.repo.List()
}

// Get mengembalikan satu server.
func (s *Service) Get(id string) (*Server, error) {
	return s.repo.Get(id)
}

// Save membuat atau memperbarui server, lalu mendaftarkan ulang ke pool.
func (s *Service) Save(req SaveServerRequest) (*Server, error) {
	if req.Name == "" || req.Host == "" || req.Username == "" {
		return nil, fmt.Errorf("name, host, dan username wajib diisi")
	}
	if req.Port == 0 {
		req.Port = 22
	}
	if req.AuthType == "" {
		req.AuthType = "key"
	}
	if req.Color == "" {
		req.Color = "#b85c3a"
	}

	srv := &Server{
		Name:     req.Name,
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		AuthType: req.AuthType,
		KeyPath:  req.KeyPath,
		Tags:     req.Tags,
		Color:    req.Color,
		Notes:    req.Notes,
		UseSudo:  req.UseSudo,
		IsActive: true,
	}

	if req.ID == "" {
		srv.ID = uuid.NewString()
		if err := s.repo.Create(srv); err != nil {
			return nil, err
		}
	} else {
		srv.ID = req.ID
		if err := s.repo.Update(srv); err != nil {
			return nil, err
		}
	}
	// Password kosong berarti "jangan ubah" (form edit tidak pernah
	// menampilkan/mengirim ulang password lama) — vault yang sudah
	// tersimpan dibiarkan apa adanya.
	if req.Password != "" {
		if err := s.vault.Set(vaultKeyForServerPassword(srv.ID), req.Password); err != nil {
			return nil, fmt.Errorf("simpan password ke vault lokal: %w", err)
		}
	}

	saved, err := s.repo.Get(srv.ID)
	if err != nil {
		return nil, err
	}
	s.registerToPool(saved)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.pool.DialTimeout()+5_000_000_000)
		defer cancel()
		_ = s.pool.WarmConnection(ctx, saved.ID)
	}()
	return saved, nil
}

// UpsertFromBackup menulis satu server dari arsip backup (lihat
// internal/core/backup) MEMPERTAHANKAN ID aslinya — beda dari Save (yang
// SELALU generate ID baru kalau req.ID kosong, dan meng-update BUKAN
// insert kalau ID sudah diisi): saat import ke perangkat baru, ID dari
// arsip biasanya belum ada sama sekali di database lokal, jadi perlu
// upsert eksplisit (insert kalau belum ada, update kalau sudah — mis.
// import ulang arsip yang sama, atau restore ke mesin yang sama).
func (s *Service) UpsertFromBackup(srv *Server, rawPassword string) error {
	if srv.ID == "" {
		srv.ID = uuid.NewString()
	}
	if _, err := s.repo.Get(srv.ID); err != nil {
		if err := s.repo.Create(srv); err != nil {
			return err
		}
	} else {
		if err := s.repo.Update(srv); err != nil {
			return err
		}
	}
	if rawPassword != "" {
		if err := s.vault.Set(vaultKeyForServerPassword(srv.ID), rawPassword); err != nil {
			return fmt.Errorf("simpan password ke vault lokal: %w", err)
		}
	}
	saved, err := s.repo.Get(srv.ID)
	if err != nil {
		return err
	}
	s.registerToPool(saved)
	// Koneksi dihangatkan seperti di Save. Tanpa ini server yang baru masuk
	// terdaftar di pool tapi belum pernah disambungkan, sehingga statusnya
	// "offline" sampai polling metrik pertama menyentuhnya — untuk server idle
	// itu bisa sampai satu setengah menit. Di agent, jeda itu tampil di bot
	// sebagai server yang baru ditautkan tapi "not connected", tanpa sebab yang
	// bisa dilihat siapa pun.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.pool.DialTimeout()+5*time.Second)
		defer cancel()
		_ = s.pool.WarmConnection(ctx, saved.ID)
	}()
	return nil
}

// SudoPassword mengembalikan password SSH tersimpan untuk server — dipakai
// files.Service saat butuh elevasi sudo ke user lain (`sudo -S`) pada server
// yang auth-nya password (bukan key). Server yang login dengan SSH key
// tidak punya password untuk dipipe ke sudo -S; jalur itu bergantung pada
// NOPASSWD di sudoers (lihat files/access.go).
func (s *Service) SudoPassword(id string) (string, error) {
	password, _, err := s.vault.Get(vaultKeyForServerPassword(id))
	return password, err
}

// Delete menghapus server, melepasnya dari pool (menutup semua koneksi), dan
// membuang password tersimpannya dari vault (best-effort — server sudah
// terhapus dari SQL apa pun hasilnya).
func (s *Service) Delete(id string) error {
	s.pool.UnregisterServer(id)
	_ = s.vault.Delete(vaultKeyForServerPassword(id))
	return s.repo.Delete(id)
}

// TestConnection menguji koneksi SSH ke konfigurasi yang diberikan TANPA
// menyimpan atau mendaftarkannya ke pool — dipakai tombol "Tes Koneksi" di
// form tambah/edit server, SEBELUM server itu disimpan.
func (s *Service) TestConnection(req SaveServerRequest) (*ConnectionTestResult, error) {
	cfg := s.probeConfig(req)

	ctx, cancel := sshpool.NewProbeContext(s.pool.DialTimeout())
	defer cancel()

	res, err := s.pool.ProbeConnection(ctx, cfg)
	if err != nil {
		return &ConnectionTestResult{Status: "error", Message: err.Error()}, nil
	}
	return &ConnectionTestResult{
		Status:         res.Status,
		Latency:        res.Latency,
		Fingerprint:    res.Fingerprint,
		OldFingerprint: res.OldFingerprint,
	}, nil
}

// TrustHostKey menyimpan fingerprint host key baru yang ditampilkan ke user
// (mis. host key baru dari VPS yang barusan di-rebuild, atau server yang
// belum pernah disambungkan sama sekali). Sengaja menerima SaveServerRequest
// penuh (bukan hanya ID) — supaya bisa dipanggil SEBELUM server disimpan,
// persis di tengah alur tambah server saat probe pertama kali menemukan host
// key baru. Kalau req.ID sudah ada, itu jalur "trust ulang" untuk server yang
// sudah tersimpan (mis. setelah VPS di-rebuild).
//
// `fingerprint` WAJIB: ini fingerprint yang barusan dikembalikan
// TestConnection dan benar-benar dibaca user di layar. Yang disimpan hanya
// host key yang cocok dengan itu — lihat sshpool.Pool.TrustHostKey untuk
// alasan lengkapnya.
func (s *Service) TrustHostKey(req SaveServerRequest, fingerprint string) error {
	cfg := s.probeConfig(req)

	ctx, cancel := sshpool.NewProbeContext(s.pool.DialTimeout())
	defer cancel()

	return s.pool.TrustHostKey(ctx, cfg, fingerprint)
}

// probeConfig membangun sshpool.ServerConfig dari request form. Kalau field
// password dikosongkan TAPI request punya ID (form edit, bukan tambah baru)
// dan auth type-nya "password", pakai password yang sudah tersimpan —
// frontend tidak pernah menampilkan/mengirim ulang password lama, jadi kosong
// di sini berarti "belum diganti", bukan "hapus password".
func (s *Service) probeConfig(req SaveServerRequest) sshpool.ServerConfig {
	password := req.Password
	if password == "" && req.ID != "" && req.AuthType == "password" {
		password, _, _ = s.vault.Get(vaultKeyForServerPassword(req.ID))
	}

	id := req.ID
	if id == "" {
		id = "probe-" + uuid.NewString()
	}

	port := req.Port
	if port == 0 {
		port = 22
	}

	return sshpool.ServerConfig{
		ID:       id,
		Host:     req.Host,
		Port:     port,
		Username: req.Username,
		AuthType: req.AuthType,
		KeyPath:  req.KeyPath,
		Password: password,
	}
}
