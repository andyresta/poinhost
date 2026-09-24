package sshpool

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andyresta/poinhost/internal/core/config"
	"golang.org/x/crypto/ssh"
)

// fakeSSHServer adalah server SSH minimal di 127.0.0.1 yang dipakai untuk
// menguji jalur trust host key SUNGGUHAN (handshake beneran), bukan tiruan
// callback — karena yang mau dibuktikan justru urutan kejadian di dalam
// handshake: host key diserahkan sebelum autentikasi.
type fakeSSHServer struct {
	addr     net.Addr
	hostKey  ssh.Signer
	listener net.Listener

	mu           sync.Mutex
	passwordSeen bool // true kalau klien sempat mencoba auth password
	keySeen      bool // true kalau klien sempat mencoba auth public key
}

func newFakeSSHServer(t *testing.T) *fakeSSHServer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("bikin host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer host key: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	s := &fakeSSHServer{addr: ln.Addr(), hostKey: signer, listener: ln}
	t.Cleanup(func() { _ = ln.Close() })

	go s.serve()
	return s
}

func (s *fakeSSHServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeSSHServer) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
			s.mu.Lock()
			s.passwordSeen = true
			s.mu.Unlock()
			return nil, errors.New("ditolak")
		},
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			s.mu.Lock()
			s.keySeen = true
			s.mu.Unlock()
			return nil, errors.New("ditolak")
		},
	}
	cfg.AddHostKey(s.hostKey)

	sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for ch := range chans {
		_ = ch.Reject(ssh.Prohibited, "tidak dipakai di test")
	}
	_ = sc.Close()
}

func (s *fakeSSHServer) sawAnyAuthAttempt() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.passwordSeen || s.keySeen
}

func (s *fakeSSHServer) fingerprint() string {
	return FingerprintSHA256(s.hostKey.PublicKey())
}

func (s *fakeSSHServer) serverConfig() ServerConfig {
	host, portStr, _ := net.SplitHostPort(s.addr.String())
	port, _ := strconv.Atoi(portStr)
	return ServerConfig{
		ID:       "uji",
		Host:     host,
		Port:     port,
		Username: "siapa-saja",
		AuthType: "password",
		Password: "rahasia-yang-tidak-boleh-bocor",
	}
}

func newTestPool(t *testing.T) (*Pool, string) {
	t.Helper()
	khPath := filepath.Join(t.TempDir(), "known_hosts")
	pool := NewPool(config.SSHConfig{DialTimeout: 5 * time.Second}, NewKnownHostsStore(khPath))
	return pool, khPath
}

func trustCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// Jalur normal: fingerprint yang disetujui user sama dengan yang dikirim
// server, jadi host key tersimpan.
func TestTrustHostKey_MenyimpanSaatFingerprintCocok(t *testing.T) {
	srv := newFakeSSHServer(t)
	pool, khPath := newTestPool(t)

	if err := pool.TrustHostKey(trustCtx(t), srv.serverConfig(), srv.fingerprint()); err != nil {
		t.Fatalf("TrustHostKey gagal padahal fingerprint cocok: %v", err)
	}

	data, err := os.ReadFile(khPath)
	if err != nil {
		t.Fatalf("known_hosts tidak ditulis: %v", err)
	}
	if !strings.Contains(string(data), "ssh-ed25519") {
		t.Errorf("isi known_hosts tidak memuat host key: %q", data)
	}
}

// Inti perbaikannya: host key yang TIDAK cocok dengan yang disetujui user
// harus ditolak dan tidak boleh menyentuh known_hosts sama sekali. Sebelum
// ini, apa pun yang dikirim koneksi kedua langsung disimpan.
func TestTrustHostKey_MenolakFingerprintYangBerbeda(t *testing.T) {
	srv := newFakeSSHServer(t)
	pool, khPath := newTestPool(t)

	disetujuiUser := "SHA256:" + strings.Repeat("A", 43)

	err := pool.TrustHostKey(trustCtx(t), srv.serverConfig(), disetujuiUser)
	if err == nil {
		t.Fatal("TrustHostKey berhasil padahal host key berbeda dari yang disetujui")
	}

	hm, ok := IsHostKeyMismatch(err)
	if !ok {
		t.Fatalf("error = %v, mau *HostKeyMismatch", err)
	}
	if hm.OldFingerprint != disetujuiUser {
		t.Errorf("OldFingerprint = %q, mau fingerprint yang disetujui user", hm.OldFingerprint)
	}
	if hm.NewFingerprint != srv.fingerprint() {
		t.Errorf("NewFingerprint = %q, mau %q", hm.NewFingerprint, srv.fingerprint())
	}

	if _, err := os.Stat(khPath); !os.IsNotExist(err) {
		t.Error("known_hosts ditulis padahal fingerprint tidak cocok")
	}
}

// Entri known_hosts yang sudah ada tidak boleh hilang gara-gara percobaan
// trust yang ditolak — Trust() menghapus entri lama sebelum menulis yang
// baru, jadi kalau penolakannya terjadi terlambat, pin lama ikut terhapus.
func TestTrustHostKey_TidakMerusakEntriLamaSaatDitolak(t *testing.T) {
	srv := newFakeSSHServer(t)
	pool, khPath := newTestPool(t)

	if err := pool.TrustHostKey(trustCtx(t), srv.serverConfig(), srv.fingerprint()); err != nil {
		t.Fatalf("persiapan: %v", err)
	}
	sebelum, err := os.ReadFile(khPath)
	if err != nil {
		t.Fatalf("persiapan baca known_hosts: %v", err)
	}

	if err := pool.TrustHostKey(trustCtx(t), srv.serverConfig(), "SHA256:"+strings.Repeat("B", 43)); err == nil {
		t.Fatal("trust dengan fingerprint asal berhasil")
	}

	sesudah, err := os.ReadFile(khPath)
	if err != nil {
		t.Fatalf("baca known_hosts: %v", err)
	}
	if string(sebelum) != string(sesudah) {
		t.Errorf("known_hosts berubah setelah trust ditolak:\nsebelum: %q\nsesudah: %q", sebelum, sesudah)
	}
}

// Tanpa fingerprint tidak ada yang bisa dibandingkan, jadi operasinya ditolak
// di awal alih-alih diam-diam mempercayai apa pun.
func TestTrustHostKey_MenolakFingerprintKosong(t *testing.T) {
	srv := newFakeSSHServer(t)
	pool, khPath := newTestPool(t)

	err := pool.TrustHostKey(trustCtx(t), srv.serverConfig(), "")
	if !errors.Is(err, ErrFingerprintRequired) {
		t.Fatalf("error = %v, mau ErrFingerprintRequired", err)
	}
	if _, err := os.Stat(khPath); !os.IsNotExist(err) {
		t.Error("known_hosts ditulis padahal fingerprint kosong")
	}
}

// Mengambil host key tidak boleh membocorkan kredensial: identitas server
// justru BELUM terverifikasi saat itu. Server palsu di sini mencatat setiap
// percobaan auth; tidak boleh ada satu pun.
func TestTrustHostKey_TidakMengirimKredensial(t *testing.T) {
	srv := newFakeSSHServer(t)
	pool, _ := newTestPool(t)

	// Dijalankan pada kedua hasil (cocok dan tidak cocok) karena keduanya
	// sama-sama menyelesaikan key exchange lebih dulu.
	if err := pool.TrustHostKey(trustCtx(t), srv.serverConfig(), srv.fingerprint()); err != nil {
		t.Fatalf("TrustHostKey: %v", err)
	}
	if srv.sawAnyAuthAttempt() {
		t.Error("klien mencoba autentikasi saat mengambil host key")
	}

	_ = pool.TrustHostKey(trustCtx(t), srv.serverConfig(), "SHA256:"+strings.Repeat("C", 43))
	if srv.sawAnyAuthAttempt() {
		t.Error("klien mencoba autentikasi pada jalur fingerprint tidak cocok")
	}
}
