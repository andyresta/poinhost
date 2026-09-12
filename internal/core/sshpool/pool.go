// Package sshpool mengelola koneksi SSH per server untuk seluruh aplikasi.
//
// Perbedaan utama dari pool SSH homepoin: slot "terminal" tunggal per server
// (yang di homepoin membuat sesi terminal baru selalu menggantikan sesi lama)
// digantikan oleh peta koneksi "dedicated" ber-handle, sehingga BANYAK tab
// yang membuka terminal (atau log stream) ke server yang SAMA masing-masing
// mendapat koneksi TCP+SSH sendiri yang hidup independen — persis kebutuhan
// skema multi-tab. Operasi ringan (files, docker, dll) tetap lewat slot
// "shared" yang di-multiplex (1 sampai beberapa koneksi dipakai bersama
// lintas modul & lintas tab untuk server yang sama), supaya pindah tab tidak
// memicu dial SSH baru selama koneksi shared masih hidup.
package sshpool

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/andyresta/poinhost/internal/core/config"
	"golang.org/x/crypto/ssh"
)

// SlotType menentukan jenis slot koneksi di pool untuk operasi berumur pendek.
type SlotType int

const (
	// SlotMetrics adalah slot dedicated untuk pengumpulan metrik.
	SlotMetrics SlotType = iota
	// SlotShared adalah slot bersama untuk operasi modul lainnya (files,
	// docker, dbmanager, dll) — di-multiplex, dipakai bersama semua tab
	// yang menunjuk ke server yang sama.
	SlotShared
)

// ServerConfig menyimpan konfigurasi koneksi SSH satu server.
type ServerConfig struct {
	ID       string
	Host     string
	Port     int
	Username string
	AuthType string
	KeyPath  string
	Password string
}

// PoolStatus menyimpan status koneksi pool per server.
type PoolStatus string

const (
	StatusOnline       PoolStatus = "online"
	StatusOffline      PoolStatus = "offline"
	StatusReconnecting PoolStatus = "reconnecting"
)

// Pool mengelola koneksi SSH per server dengan keepalive dan health check.
type Pool struct {
	cfg         config.SSHConfig
	knownHosts  *KnownHostsStore
	mu          sync.RWMutex
	servers     map[string]ServerConfig
	serverPools map[string]*serverPool
	status      map[string]PoolStatus
}

type serverPool struct {
	metrics *pooledConn
	shared  []*pooledConn

	// dedicated menyimpan koneksi SSH berumur panjang di luar mux shared,
	// satu per handle. Dipakai untuk terminal PTY & log stream — BANYAK
	// tab boleh punya handle dedicated masing-masing ke server yang sama
	// secara bersamaan, tidak saling menggantikan.
	dedicated map[string]*ssh.Client

	sharedMu    sync.Mutex
	dedicatedMu sync.Mutex
	mu          sync.Mutex
}

type pooledConn struct {
	client   *ssh.Client
	inUse    bool
	lastUsed time.Time
}

// NewPool membuat pool SSH baru (lazy connect).
func NewPool(cfg config.SSHConfig, knownHosts *KnownHostsStore) *Pool {
	return &Pool{
		cfg:         cfg,
		knownHosts:  knownHosts,
		servers:     make(map[string]ServerConfig),
		serverPools: make(map[string]*serverPool),
		status:      make(map[string]PoolStatus),
	}
}

// RegisterServer mendaftarkan konfigurasi server ke pool.
func (p *Pool) RegisterServer(cfg ServerConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.servers[cfg.ID] = cfg
	if _, ok := p.serverPools[cfg.ID]; !ok {
		p.serverPools[cfg.ID] = &serverPool{dedicated: make(map[string]*ssh.Client)}
		p.status[cfg.ID] = StatusOffline
	}
}

// UnregisterServer menghapus server dari pool dan menutup koneksi.
func (p *Pool) UnregisterServer(serverID string) {
	p.mu.Lock()
	sp, ok := p.serverPools[serverID]
	delete(p.servers, serverID)
	delete(p.serverPools, serverID)
	delete(p.status, serverID)
	p.mu.Unlock()
	if ok {
		sp.closeAll()
	}
}

// RegisteredServerIDs mengembalikan semua ID server yang terdaftar di pool —
// dipakai untuk warm-all-at-startup (bukan lazy-connect-on-first-click
// seperti homepoin, yang menyebabkan klik pertama ke server terasa lambat).
func (p *Pool) RegisteredServerIDs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ids := make([]string, 0, len(p.servers))
	for id := range p.servers {
		ids = append(ids, id)
	}
	return ids
}

// Status mengembalikan status koneksi server saat ini.
func (p *Pool) Status(serverID string) PoolStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if s, ok := p.status[serverID]; ok {
		return s
	}
	return StatusOffline
}

// DialTimeout mengembalikan batas waktu dial TCP yang dikonfigurasi.
func (p *Pool) DialTimeout() time.Duration {
	return p.cfg.DialTimeout
}

// Acquire mengambil koneksi shared/metrics dari pool; menunggu hingga
// timeout jika penuh.
func (p *Pool) Acquire(ctx context.Context, serverID string, slot SlotType) (*ssh.Client, error) {
	deadline := time.Now().Add(p.cfg.PoolWaitTimeout)
	for {
		conn, err := p.tryAcquire(ctx, serverID, slot)
		if err == nil {
			return conn, nil
		}
		if err != ErrPoolExhausted {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrPoolExhausted
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (p *Pool) tryAcquire(ctx context.Context, serverID string, slot SlotType) (*ssh.Client, error) {
	p.mu.RLock()
	cfg, ok := p.servers[serverID]
	sp, spOK := p.serverPools[serverID]
	p.mu.RUnlock()
	if !ok || !spOK {
		return nil, fmt.Errorf("server %s tidak terdaftar di pool", serverID)
	}

	sp.mu.Lock()
	defer sp.mu.Unlock()

	switch slot {
	case SlotMetrics:
		if sp.metrics != nil && sp.metrics.inUse {
			return nil, ErrPoolExhausted
		}
		if sp.metrics == nil || !p.connUsable(ctx, sp.metrics) {
			client, err := p.dialWithRetry(ctx, cfg)
			if err != nil {
				return nil, err
			}
			sp.metrics = &pooledConn{client: client}
		}
		sp.metrics.inUse = true
		sp.metrics.lastUsed = time.Now()
		return sp.metrics.client, nil

	default:
		sp.sharedMu.Lock()
		defer sp.sharedMu.Unlock()

		maxMux := p.cfg.SharedMuxConns
		if maxMux < 1 {
			maxMux = 1
		}

		// Multiplex: beberapa koneksi TCP dipakai bergantian oleh semua
		// tab & modul yang menunjuk server yang sama (tanpa flag inUse
		// per-session — SSH channel multiplex di atas 1 TCP connection).
		for _, c := range sp.shared {
			if c != nil && p.connUsable(ctx, c) {
				c.lastUsed = time.Now()
				return c.client, nil
			}
		}

		if len(sp.shared) < maxMux {
			client, err := p.dialWithRetry(ctx, cfg)
			if err != nil {
				return nil, err
			}
			pc := &pooledConn{client: client, lastUsed: time.Now()}
			sp.shared = append(sp.shared, pc)
			return client, nil
		}

		for i, c := range sp.shared {
			if c != nil && c.client != nil {
				_ = c.client.Close()
			}
			client, err := p.dialWithRetry(ctx, cfg)
			if err != nil {
				return nil, err
			}
			sp.shared[i] = &pooledConn{client: client, lastUsed: time.Now()}
			return client, nil
		}
		return nil, ErrPoolExhausted
	}
}

// WarmConnection membuka slot shared agar koneksi SSH siap dipakai. Dipanggil
// untuk SEMUA server terdaftar saat aplikasi start (lihat cmd bootstrap),
// bukan hanya saat server pertama kali diklik — inilah salah satu perbaikan
// performa utama dibanding homepoin.
func (p *Pool) WarmConnection(ctx context.Context, serverID string) error {
	conn, err := p.Acquire(ctx, serverID, SlotShared)
	if err != nil {
		return err
	}
	p.Release(serverID, SlotShared, conn)
	return nil
}

// OpenDedicated membuka koneksi SSH baru yang TIDAK di-multiplex dan TIDAK
// menghitung kuota shared — dipakai untuk sesi berumur panjang seperti
// terminal PTY per tab atau `tail -f` log. Mengembalikan handle unik untuk
// menutup koneksi ini nanti via CloseDedicated. Boleh dipanggil berkali-kali
// untuk server yang sama secara bersamaan (mis. 3 tab terminal ke server X).
func (p *Pool) OpenDedicated(ctx context.Context, serverID string) (handle string, client *ssh.Client, err error) {
	p.mu.RLock()
	cfg, ok := p.servers[serverID]
	sp, spOK := p.serverPools[serverID]
	p.mu.RUnlock()
	if !ok || !spOK {
		return "", nil, fmt.Errorf("server %s tidak terdaftar di pool", serverID)
	}

	client, err = p.dial(ctx, cfg)
	if err != nil {
		return "", nil, err
	}

	handle = newHandle()
	sp.dedicatedMu.Lock()
	sp.dedicated[handle] = client
	sp.dedicatedMu.Unlock()

	p.setStatus(cfg.ID, StatusOnline)
	return handle, client, nil
}

// CloseDedicated menutup koneksi dedicated tertentu berdasarkan handle-nya.
func (p *Pool) CloseDedicated(serverID, handle string) {
	p.mu.RLock()
	sp, ok := p.serverPools[serverID]
	p.mu.RUnlock()
	if !ok {
		return
	}
	sp.dedicatedMu.Lock()
	client, exists := sp.dedicated[handle]
	delete(sp.dedicated, handle)
	sp.dedicatedMu.Unlock()
	if exists && client != nil {
		_ = client.Close()
	}
}

// DedicatedCount mengembalikan jumlah koneksi dedicated aktif untuk server
// (berguna untuk menampilkan "N sesi terminal aktif" di UI).
func (p *Pool) DedicatedCount(serverID string) int {
	p.mu.RLock()
	sp, ok := p.serverPools[serverID]
	p.mu.RUnlock()
	if !ok {
		return 0
	}
	sp.dedicatedMu.Lock()
	defer sp.dedicatedMu.Unlock()
	return len(sp.dedicated)
}

// Release mengembalikan koneksi shared/metrics ke pool setelah selesai dipakai.
func (p *Pool) Release(serverID string, slot SlotType, client *ssh.Client) {
	p.mu.RLock()
	sp, ok := p.serverPools[serverID]
	p.mu.RUnlock()
	if !ok {
		return
	}

	sp.mu.Lock()
	defer sp.mu.Unlock()

	switch slot {
	case SlotMetrics:
		if sp.metrics != nil && sp.metrics.client == client {
			sp.metrics.inUse = false
		}
	default:
		sp.sharedMu.Lock()
		for _, c := range sp.shared {
			if c != nil && c.client == client {
				c.lastUsed = time.Now()
				break
			}
		}
		sp.sharedMu.Unlock()
	}
}

// Close menutup semua koneksi di pool (dipanggil saat aplikasi shutdown).
func (p *Pool) Close() {
	p.mu.Lock()
	pools := make([]*serverPool, 0, len(p.serverPools))
	for _, sp := range p.serverPools {
		pools = append(pools, sp)
	}
	p.serverPools = make(map[string]*serverPool)
	p.servers = make(map[string]ServerConfig)
	p.status = make(map[string]PoolStatus)
	p.mu.Unlock()

	for _, sp := range pools {
		sp.closeAll()
	}
}

func (p *Pool) dialWithRetry(ctx context.Context, cfg ServerConfig) (*ssh.Client, error) {
	p.setStatus(cfg.ID, StatusReconnecting)
	attempt := 0
	for {
		client, err := p.dial(ctx, cfg)
		if err == nil {
			p.setStatus(cfg.ID, StatusOnline)
			go p.startKeepalive(cfg.ID, client)
			return client, nil
		}
		if hm, ok := IsHostKeyMismatch(err); ok {
			p.setStatus(cfg.ID, StatusOffline)
			return nil, hm
		}
		backoff := p.reconnectBackoff(attempt)
		attempt++
		select {
		case <-ctx.Done():
			p.setStatus(cfg.ID, StatusOffline)
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}
}

func (p *Pool) dial(ctx context.Context, cfg ServerConfig) (*ssh.Client, error) {
	authMethods, err := buildAuthMethods(cfg)
	if err != nil {
		return nil, err
	}

	addr := formatAddr(cfg.Host, cfg.Port)
	conn, err := dialTCP(ctx, p.cfg.DialTimeout, addr)
	if err != nil {
		return nil, err
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: p.knownHosts.Callback(),
	}
	return newSSHClient(ctx, conn, addr, sshCfg)
}

func (p *Pool) connUsable(ctx context.Context, pc *pooledConn) bool {
	if pc == nil || pc.client == nil {
		return false
	}
	if p.cfg.HealthCheckSkipAfterUse > 0 && !pc.lastUsed.IsZero() {
		if time.Since(pc.lastUsed) < p.cfg.HealthCheckSkipAfterUse {
			return true
		}
	}
	return p.isHealthy(ctx, pc.client)
}

func (p *Pool) isHealthy(ctx context.Context, client *ssh.Client) bool {
	if client == nil {
		return false
	}
	hctx, cancel := context.WithTimeout(ctx, p.cfg.HealthCheckTimeout)
	defer cancel()

	sess, err := client.NewSession()
	if err != nil {
		return false
	}
	defer sess.Close()

	done := make(chan error, 1)
	go func() { done <- sess.Run("echo poinhost-ping") }()
	select {
	case <-hctx.Done():
		return false
	case err := <-done:
		return err == nil
	}
}

func (p *Pool) startKeepalive(serverID string, client *ssh.Client) {
	ticker := time.NewTicker(p.cfg.KeepaliveInterval)
	defer ticker.Stop()
	for range ticker.C {
		_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
		if err != nil {
			p.invalidate(serverID, client)
			return
		}
	}
}

func (p *Pool) invalidate(serverID string, client *ssh.Client) {
	p.mu.RLock()
	sp, ok := p.serverPools[serverID]
	p.mu.RUnlock()
	if !ok {
		return
	}
	sp.removeConn(client)
	p.setStatus(serverID, StatusOffline)
}

func (p *Pool) setStatus(serverID string, status PoolStatus) {
	p.mu.Lock()
	p.status[serverID] = status
	p.mu.Unlock()
}

func (p *Pool) reconnectBackoff(attempt int) time.Duration {
	if attempt < len(p.cfg.ReconnectBackoff) {
		return p.cfg.ReconnectBackoff[attempt]
	}
	if len(p.cfg.ReconnectBackoff) > 0 {
		return p.cfg.ReconnectBackoff[len(p.cfg.ReconnectBackoff)-1]
	}
	return 30 * time.Second
}

func buildAuthMethods(cfg ServerConfig) ([]ssh.AuthMethod, error) {
	switch cfg.AuthType {
	case "password":
		return []ssh.AuthMethod{ssh.Password(cfg.Password)}, nil
	default:
		keyPath := cfg.KeyPath
		if keyPath == "" {
			home, _ := os.UserHomeDir()
			keyPath = home + "/.ssh/id_ed25519"
		}
		key, err := os.ReadFile(keyPath)
		if err != nil {
			home, _ := os.UserHomeDir()
			key, err = os.ReadFile(home + "/.ssh/id_rsa")
			if err != nil {
				return nil, fmt.Errorf("baca kunci SSH: %w", err)
			}
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse kunci SSH: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}
}

func (sp *serverPool) closeAll() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if sp.metrics != nil && sp.metrics.client != nil {
		_ = sp.metrics.client.Close()
	}
	for _, c := range sp.shared {
		if c != nil && c.client != nil {
			_ = c.client.Close()
		}
	}
	sp.dedicatedMu.Lock()
	for _, c := range sp.dedicated {
		if c != nil {
			_ = c.Close()
		}
	}
	sp.dedicated = make(map[string]*ssh.Client)
	sp.dedicatedMu.Unlock()
	sp.metrics = nil
	sp.shared = nil
}

func (sp *serverPool) removeConn(client *ssh.Client) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if sp.metrics != nil && sp.metrics.client == client {
		_ = client.Close()
		sp.metrics = nil
	}
	sp.sharedMu.Lock()
	filtered := sp.shared[:0]
	for _, c := range sp.shared {
		if c.client == client {
			_ = client.Close()
			continue
		}
		filtered = append(filtered, c)
	}
	sp.shared = filtered
	sp.sharedMu.Unlock()
}

func newHandle() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
