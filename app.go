package main

import (
	"context"
	"log"

	"github.com/andyresta/poinhost/internal/core/config"
	"github.com/andyresta/poinhost/internal/core/database"
	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/servers"
	"github.com/andyresta/poinhost/internal/session"

	"database/sql"
)

// App adalah struct utama yang di-bind ke frontend Wails. Semua method
// exported di sini otomatis jadi fungsi yang bisa dipanggil langsung dari
// TypeScript lewat binding auto-generate Wails (lihat frontend/wailsjs).
type App struct {
	ctx context.Context

	cfg *config.Config
	db  *sql.DB
	pool *sshpool.Pool

	serversSvc *servers.Service
	sessionMgr *session.Manager
	terminals  *session.TerminalRegistry
}

// NewApp membuat instance App baru. Semua wiring dependency (config, db,
// ssh pool, service tiap modul) terjadi di sini — satu tempat, mirip
// router.Deps di homepoin tapi untuk binding Wails, bukan HTTP routes.
func NewApp() *App {
	cfg, err := config.Default()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := cfg.EnsureDirs(); err != nil {
		log.Fatalf("ensure dirs: %v", err)
	}

	db, err := database.Open(cfg.DBPath())
	if err != nil {
		log.Fatalf("buka database: %v", err)
	}
	if err := database.Migrate(db, migrationsFS); err != nil {
		log.Fatalf("migrasi database: %v", err)
	}

	knownHosts := sshpool.NewKnownHostsStore(cfg.KnownHostsPath())
	pool := sshpool.NewPool(cfg.SSH, knownHosts)

	serversRepo := servers.NewRepository(db)
	serversSvc := servers.NewService(serversRepo, pool)

	sessionMgr := session.NewManager(db)
	terminals := session.NewTerminalRegistry(pool)

	return &App{
		cfg:        cfg,
		db:         db,
		pool:       pool,
		serversSvc: serversSvc,
		sessionMgr: sessionMgr,
		terminals:  terminals,
	}
}

// startup dipanggil Wails saat aplikasi siap; context di sini dipakai untuk
// semua pemanggilan runtime Wails (event emit, dialog, dll).
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	if err := a.serversSvc.Bootstrap(ctx); err != nil {
		log.Printf("bootstrap servers: %v", err)
	}
	if _, err := a.sessionMgr.LoadPersisted(); err != nil {
		log.Printf("load tab layout: %v", err)
	}
}

// shutdown dipanggil Wails saat aplikasi ditutup — tutup semua koneksi SSH
// & database dengan rapi (setara graceful shutdown homepoin, tanpa perlu
// timeout HTTP server karena tidak ada server HTTP di sini).
func (a *App) shutdown(ctx context.Context) {
	a.pool.Close()
	_ = a.db.Close()
}

// ---------------------------------------------------------------------
// Bindings: Servers
// ---------------------------------------------------------------------

// ListServers mengembalikan semua server terdaftar.
func (a *App) ListServers() ([]*servers.Server, error) {
	return a.serversSvc.List()
}

// SaveServer membuat atau memperbarui server (req.ID kosong = create).
func (a *App) SaveServer(req servers.SaveServerRequest) (*servers.Server, error) {
	return a.serversSvc.Save(req)
}

// DeleteServer menghapus server (dan seluruh tab yang menunjuk ke sana).
func (a *App) DeleteServer(id string) error {
	return a.serversSvc.Delete(id)
}

// TestServerConnection menguji kredensial SSH sebelum server disimpan.
func (a *App) TestServerConnection(req servers.SaveServerRequest) (*servers.ConnectionTestResult, error) {
	return a.serversSvc.TestConnection(req)
}

// TrustServerHostKey menyimpan fingerprint host key baru. Menerima payload
// form yang sama dengan SaveServer/TestServerConnection (bukan cuma ID),
// supaya bisa dipanggil di tengah alur TAMBAH server (sebelum ID ada) saat
// probe pertama kali menemukan host key baru — bukan hanya untuk server
// yang sudah tersimpan.
func (a *App) TrustServerHostKey(req servers.SaveServerRequest) error {
	return a.serversSvc.TrustHostKey(req)
}

// ---------------------------------------------------------------------
// Bindings: Tabs (multi-tab session)
// ---------------------------------------------------------------------

// OpenServerTab membuka tab baru menunjuk ke satu server.
func (a *App) OpenServerTab(serverID, title string) (*session.Tab, error) {
	return a.sessionMgr.OpenTab(serverID, title)
}

// CloseServerTab menutup tab: semua sesi terminal dedicated milik tab ini
// ditutup lebih dulu, baru tab-nya sendiri dihapus dari registry.
func (a *App) CloseServerTab(tabID string) error {
	a.terminals.CloseAllForTab(tabID)
	return a.sessionMgr.CloseTab(tabID)
}

// ListServerTabs mengembalikan semua tab yang sedang terbuka, terurut.
func (a *App) ListServerTabs() []*session.Tab {
	return a.sessionMgr.ListTabs()
}

// SetTabActiveModule mengganti modul aktif DI DALAM satu tab (mis. pindah
// dari "files" ke "docker") tanpa membuka/menutup tab atau koneksi apapun.
func (a *App) SetTabActiveModule(tabID, module string) error {
	return a.sessionMgr.SetActiveModule(tabID, module)
}

// ReorderServerTabs menyimpan urutan baru tab bar setelah drag & drop.
func (a *App) ReorderServerTabs(tabIDs []string) error {
	return a.sessionMgr.Reorder(tabIDs)
}
