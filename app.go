package main

import (
	"context"
	"encoding/base64"
	"log"

	"github.com/andyresta/poinhost/internal/core/config"
	"github.com/andyresta/poinhost/internal/core/database"
	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/servers"
	"github.com/andyresta/poinhost/internal/modules/terminal"
	"github.com/andyresta/poinhost/internal/session"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"database/sql"
)

// App adalah struct utama yang di-bind ke frontend Wails. Semua method
// exported di sini otomatis jadi fungsi yang bisa dipanggil langsung dari
// TypeScript lewat binding auto-generate Wails (lihat frontend/wailsjs).
type App struct {
	ctx context.Context

	cfg  *config.Config
	db   *sql.DB
	pool *sshpool.Pool

	serversSvc  *servers.Service
	collector   *servers.Collector
	sessionMgr  *session.Manager
	terminals   *session.TerminalRegistry
	terminalSvc *terminal.Service
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
	mutex := sshpool.NewServerMutexRegistry()
	executor := sshpool.NewExecutor(pool, mutex, cfg.Executor)

	serversRepo := servers.NewRepository(db)
	serversSvc := servers.NewService(serversRepo, pool, executor)
	collector := servers.NewCollector(serversSvc)

	sessionMgr := session.NewManager(db)
	terminals := session.NewTerminalRegistry(pool)
	terminalSvc := terminal.NewService(terminals)

	return &App{
		cfg:         cfg,
		db:          db,
		pool:        pool,
		serversSvc:  serversSvc,
		collector:   collector,
		sessionMgr:  sessionMgr,
		terminals:   terminals,
		terminalSvc: terminalSvc,
	}
}

// startup dipanggil Wails saat aplikasi siap; context di sini dipakai untuk
// semua pemanggilan runtime Wails (event emit, dialog, dll).
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	if err := a.serversSvc.Bootstrap(ctx); err != nil {
		log.Printf("bootstrap servers: %v", err)
	}

	tabs, err := a.sessionMgr.LoadPersisted()
	if err != nil {
		log.Printf("load tab layout: %v", err)
	}
	// Tab yang direstore dari sesi sebelumnya dianggap "sedang diperhatikan"
	// lagi begitu app dibuka — subscribe status collector-nya supaya
	// server-server itu langsung dicek pada interval cepat (bukan menunggu
	// user klik ulang satu-satu untuk menaikkan prioritasnya).
	for _, t := range tabs {
		a.collector.Subscribe(t.ServerID)
	}

	a.collector.SetEmitter(func(st servers.ServerStatus) {
		runtime.EventsEmit(ctx, "server:status", st)
	})
	a.collector.Start()

	// Output PTY di-base64-kan dulu sebelum lewat event Wails (yang membawa
	// payload sebagai JSON) — byte mentah dari remote shell tidak dijamin
	// UTF-8 valid (mis. karakter multi-byte terpotong tepat di batas satu
	// pembacaan), dan base64 menghindari itu tanpa perlu peduli soal encoding
	// sama sekali. Event per-sesi ("terminal:output:<id>") supaya tiap
	// TerminalPanel di frontend cuma dengar output miliknya sendiri.
	a.terminalSvc.SetEmitters(
		func(sessionID string, data []byte) {
			runtime.EventsEmit(ctx, "terminal:output:"+sessionID, base64.StdEncoding.EncodeToString(data))
		},
		func(sessionID string, reason string) {
			runtime.EventsEmit(ctx, "terminal:exit:"+sessionID, reason)
		},
	)
}

// shutdown dipanggil Wails saat aplikasi ditutup — hentikan collector dulu
// (supaya tidak ada goroutine background yang masih mencoba pakai pool/db
// setelah keduanya ditutup), baru tutup koneksi SSH & database.
func (a *App) shutdown(ctx context.Context) {
	a.collector.Stop()
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
	if err := a.serversSvc.Delete(id); err != nil {
		return err
	}
	a.collector.Forget(id)
	return nil
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
// Bindings: Server status (lihat internal/modules/servers/collector.go)
// ---------------------------------------------------------------------

// ListServerStatuses mengembalikan snapshot status terakhir SEMUA server —
// dipanggil sekali saat frontend mount untuk mengisi state awal; update
// selanjutnya datang lewat event "server:status" (lihat startup()), bukan
// polling berulang dari frontend.
func (a *App) ListServerStatuses() []servers.ServerStatus {
	return a.collector.Snapshot()
}

// RefreshServerStatus memaksa pengambilan metrik satu server SEKARANG,
// dipakai tombol "Refresh" manual di panel Overview.
func (a *App) RefreshServerStatus(serverID string) (servers.ServerStatus, error) {
	return a.collector.RefreshNow(a.ctx, serverID)
}

// ---------------------------------------------------------------------
// Bindings: Tabs (multi-tab session)
// ---------------------------------------------------------------------

// OpenServerTab membuka tab baru menunjuk ke satu server, dan menaikkan
// prioritas status collector untuk server itu (dicek lebih sering selama
// ada tab yang membukanya — lihat Collector.Subscribe).
func (a *App) OpenServerTab(serverID, title string) (*session.Tab, error) {
	tab, err := a.sessionMgr.OpenTab(serverID, title)
	if err != nil {
		return nil, err
	}
	a.collector.Subscribe(serverID)
	return tab, nil
}

// CloseServerTab menutup tab: semua sesi terminal PTY milik tab ini ditutup
// dulu (terminalSvc — shell + koneksi dedicated-nya), lalu jaring pengaman
// session.TerminalRegistry (menutup koneksi dedicated APA PUN yang tersisa
// untuk tab ini walau bukan lewat terminalSvc), baru tab-nya sendiri
// dihapus — dan prioritas status collector untuk server itu diturunkan.
func (a *App) CloseServerTab(tabID string) error {
	tab, err := a.sessionMgr.GetTab(tabID)
	if err != nil {
		return err
	}
	a.terminalSvc.CloseAllForTab(tabID)
	a.terminals.CloseAllForTab(tabID)
	if err := a.sessionMgr.CloseTab(tabID); err != nil {
		return err
	}
	a.collector.Unsubscribe(tab.ServerID)
	return nil
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

// ---------------------------------------------------------------------
// Bindings: Terminal (PTY, lihat internal/modules/terminal)
// ---------------------------------------------------------------------

// OpenTerminal membuka sesi shell interaktif baru untuk tab tertentu.
// Output-nya TIDAK dikembalikan lewat return value — mengalir terus lewat
// event "terminal:output:<sessionId>" (lihat startup()) sampai sesi ditutup.
func (a *App) OpenTerminal(tabID string) (string, error) {
	tab, err := a.sessionMgr.GetTab(tabID)
	if err != nil {
		return "", err
	}
	return a.terminalSvc.Open(a.ctx, tabID, tab.ServerID)
}

// WriteTerminal mengirim keystroke dari xterm.js ke shell remote.
func (a *App) WriteTerminal(sessionID, data string) error {
	return a.terminalSvc.Write(sessionID, []byte(data))
}

// ResizeTerminal memberi tahu ukuran PTY baru (dipanggil FitAddon frontend).
func (a *App) ResizeTerminal(sessionID string, cols, rows int) error {
	return a.terminalSvc.Resize(sessionID, cols, rows)
}

// CloseTerminal menutup satu sesi terminal secara eksplisit (mis. user
// menutup panel terminal tanpa menutup keseluruhan tab — belum ada di UI
// v1, tapi binding-nya sudah tersedia untuk itu).
func (a *App) CloseTerminal(sessionID string) {
	a.terminalSvc.Close(sessionID)
}
