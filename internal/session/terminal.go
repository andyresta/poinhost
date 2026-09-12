package session

import (
	"context"
	"sync"

	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

// TerminalRegistry mengikat sesi terminal PTY (koneksi SSH dedicated, lihat
// sshpool.OpenDedicated) ke tab pemiliknya. Satu tab boleh punya lebih dari
// satu sesi terminal sekaligus (mis. split pane), dan BANYAK tab yang
// menunjuk server yang sama masing-masing punya sesi independen — tidak
// saling menggantikan seperti slot terminal tunggal di homepoin.
type TerminalRegistry struct {
	pool *sshpool.Pool

	mu     sync.Mutex
	byTab  map[string]map[string]string // tabID -> sessionID -> pool handle
	server map[string]string            // sessionID -> serverID (untuk CloseDedicated)
}

// NewTerminalRegistry membuat registry terminal baru.
func NewTerminalRegistry(pool *sshpool.Pool) *TerminalRegistry {
	return &TerminalRegistry{
		pool:   pool,
		byTab:  make(map[string]map[string]string),
		server: make(map[string]string),
	}
}

// Open membuka sesi terminal PTY baru untuk tab tertentu. Mengembalikan
// sessionID (dipakai frontend untuk mengirim keystroke/resize ke sesi yang
// benar lewat event Wails) dan *ssh.Client mentah untuk dibungkus PTY oleh
// pemanggil (handler terminal, belum di-porting di skeleton ini).
func (r *TerminalRegistry) Open(ctx context.Context, tabID, serverID string) (sessionID string, client *ssh.Client, err error) {
	handle, client, err := r.pool.OpenDedicated(ctx, serverID)
	if err != nil {
		return "", nil, err
	}

	sessionID = uuid.NewString()

	r.mu.Lock()
	if r.byTab[tabID] == nil {
		r.byTab[tabID] = make(map[string]string)
	}
	r.byTab[tabID][sessionID] = handle
	r.server[sessionID] = serverID
	r.mu.Unlock()

	return sessionID, client, nil
}

// Close menutup satu sesi terminal spesifik.
func (r *TerminalRegistry) Close(tabID, sessionID string) {
	r.mu.Lock()
	handle, ok := r.byTab[tabID][sessionID]
	if ok {
		delete(r.byTab[tabID], sessionID)
	}
	serverID := r.server[sessionID]
	delete(r.server, sessionID)
	r.mu.Unlock()

	if ok {
		r.pool.CloseDedicated(serverID, handle)
	}
}

// CloseAllForTab menutup semua sesi terminal milik satu tab — WAJIB dipanggil
// sebelum Manager.CloseTab, supaya tidak ada koneksi SSH dedicated yang
// menggantung tanpa tab pemilik.
func (r *TerminalRegistry) CloseAllForTab(tabID string) {
	r.mu.Lock()
	sessions := r.byTab[tabID]
	delete(r.byTab, tabID)
	type closeReq struct{ serverID, handle string }
	var toClose []closeReq
	for sessionID, handle := range sessions {
		toClose = append(toClose, closeReq{r.server[sessionID], handle})
		delete(r.server, sessionID)
	}
	r.mu.Unlock()

	for _, c := range toClose {
		r.pool.CloseDedicated(c.serverID, c.handle)
	}
}

// CountForTab mengembalikan jumlah sesi terminal aktif di satu tab.
func (r *TerminalRegistry) CountForTab(tabID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byTab[tabID])
}
