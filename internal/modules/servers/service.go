package servers

import (
	"context"
	"fmt"

	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/google/uuid"
)

// Service mengorkestrasi CRUD server + daftar/lepas dari sshpool.Pool.
type Service struct {
	repo     *Repository
	pool     *sshpool.Pool
	executor *sshpool.Executor
}

// NewService membuat servers.Service baru.
func NewService(repo *Repository, pool *sshpool.Pool, executor *sshpool.Executor) *Service {
	return &Service{repo: repo, pool: pool, executor: executor}
}

// Bootstrap memuat semua server dari DB dan mendaftarkannya ke pool +
// langsung memanaskan koneksi shared masing-masing (warm-all-at-startup).
// Ini menghilangkan jeda "terasa seperti handshake dari nol" saat pertama
// kali membuka tab ke sebuah server, yang terjadi di homepoin karena
// WarmConnection cuma dipanggil on-demand per server saat diklik.
func (s *Service) Bootstrap(ctx context.Context) error {
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
	password, _ := s.repo.GetPassword(srv.ID)
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
		req.Color = "#6366f1"
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
		if err := s.repo.Create(srv, req.Password); err != nil {
			return nil, err
		}
	} else {
		srv.ID = req.ID
		if err := s.repo.Update(srv, req.Password); err != nil {
			return nil, err
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

// Delete menghapus server dan melepasnya dari pool (menutup semua koneksi).
func (s *Service) Delete(id string) error {
	s.pool.UnregisterServer(id)
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
func (s *Service) TrustHostKey(req SaveServerRequest) error {
	cfg := s.probeConfig(req)

	ctx, cancel := sshpool.NewProbeContext(s.pool.DialTimeout())
	defer cancel()

	return s.pool.TrustHostKey(ctx, cfg)
}

// probeConfig membangun sshpool.ServerConfig dari request form. Kalau field
// password dikosongkan TAPI request punya ID (form edit, bukan tambah baru)
// dan auth type-nya "password", pakai password yang sudah tersimpan —
// frontend tidak pernah menampilkan/mengirim ulang password lama, jadi kosong
// di sini berarti "belum diganti", bukan "hapus password".
func (s *Service) probeConfig(req SaveServerRequest) sshpool.ServerConfig {
	password := req.Password
	if password == "" && req.ID != "" && req.AuthType == "password" {
		password, _ = s.repo.GetPassword(req.ID)
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
