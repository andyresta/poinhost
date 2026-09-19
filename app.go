package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/andyresta/poinhost/internal/core/backup"
	"github.com/andyresta/poinhost/internal/core/config"
	"github.com/andyresta/poinhost/internal/core/database"
	"github.com/andyresta/poinhost/internal/core/secrets"
	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/docker"
	"github.com/andyresta/poinhost/internal/modules/dockerxfer"
	"github.com/andyresta/poinhost/internal/modules/files"
	"github.com/andyresta/poinhost/internal/modules/filexfer"
	"github.com/andyresta/poinhost/internal/modules/firewall"
	"github.com/andyresta/poinhost/internal/modules/servers"
	"github.com/andyresta/poinhost/internal/modules/services"
	"github.com/andyresta/poinhost/internal/modules/terminal"
	"github.com/andyresta/poinhost/internal/modules/website"
	"github.com/andyresta/poinhost/internal/session"
	"github.com/google/uuid"
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

	serversSvc    *servers.Service
	collector     *servers.Collector
	sessionMgr    *session.Manager
	terminals     *session.TerminalRegistry
	terminalSvc   *terminal.Service
	filesSvc      *files.Service
	filexferSvc   *filexfer.Service
	dockerSvc     *docker.Service
	dockerxferSvc *dockerxfer.Service
	servicesSvc   *services.Service
	firewallSvc   *firewall.Service
	websiteSvc    *website.Service
	backupSvc     *backup.Service

	// streamMu/streams melacak stream Docker yang sedang berjalan (logs,
	// stats, instalasi engine) supaya frontend bisa membatalkannya secara
	// eksplisit (mis. tutup modal log) — beda dari Terminal yang sesinya
	// dikelola terminalSvc, stream Docker cuma butuh satu context.CancelFunc
	// per panggilan karena tidak ada input interaktif untuk dikirim balik.
	streamMu sync.Mutex
	streams  map[string]context.CancelFunc
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

	// vault SATU instance dipakai bersama untuk semua rahasia lokal — password
	// SSH server (servers.Repository, lihat migrateLegacyPasswords) maupun
	// kredensial database (website.Service, fitur Explore) — bukan dua vault
	// terpisah untuk hal yang secara prinsip sama (rahasia di mesin user,
	// tidak pernah ditulis ke server target).
	vault := secrets.New(cfg.DataDir)

	serversRepo := servers.NewRepository(db)
	serversSvc := servers.NewService(serversRepo, pool, executor, vault)
	collector := servers.NewCollector(serversSvc)

	sessionMgr := session.NewManager(db)
	terminals := session.NewTerminalRegistry(pool)
	terminalSvc := terminal.NewService(terminals)

	sftpClient := sshpool.NewSFTPClient(pool)
	filesSvc := files.NewService(sftpClient, executor, serversSvc)
	filexferSvc := filexfer.NewService(serversSvc, pool, sftpClient)

	dockerSvc := docker.NewService(serversSvc, executor, mutex)
	dockerxferSvc := dockerxfer.NewService(serversSvc, dockerSvc, pool)
	servicesSvc := services.NewService(serversSvc, executor, mutex)
	firewallSvc := firewall.NewService(serversSvc, executor, mutex)
	websiteSvc := website.NewService(serversSvc, executor, mutex, db, vault, firewallSvc)
	backupSvc := backup.NewService(serversSvc, websiteSvc)

	return &App{
		cfg:           cfg,
		db:            db,
		pool:          pool,
		serversSvc:    serversSvc,
		collector:     collector,
		sessionMgr:    sessionMgr,
		terminals:     terminals,
		terminalSvc:   terminalSvc,
		filesSvc:      filesSvc,
		filexferSvc:   filexferSvc,
		dockerSvc:     dockerSvc,
		dockerxferSvc: dockerxferSvc,
		servicesSvc:   servicesSvc,
		firewallSvc:   firewallSvc,
		websiteSvc:    websiteSvc,
		backupSvc:     backupSvc,
		streams:       make(map[string]context.CancelFunc),
	}
}

// registerStream mendaftarkan context.CancelFunc satu stream Docker (logs,
// stats, atau instalasi engine) supaya bisa dibatalkan lewat StopDockerStream.
func (a *App) registerStream(id string, cancel context.CancelFunc) {
	a.streamMu.Lock()
	a.streams[id] = cancel
	a.streamMu.Unlock()
}

// stopStream membatalkan (jika masih terdaftar) dan melupakan satu stream.
// Aman dipanggil dua kali (idempotent) — baik dari StopDockerStream (user
// menutup modal) maupun dari goroutine stream itu sendiri saat selesai wajar.
func (a *App) stopStream(id string) {
	a.streamMu.Lock()
	cancel, ok := a.streams[id]
	delete(a.streams, id)
	a.streamMu.Unlock()
	if ok {
		cancel()
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

	// Progress transfer file dikirim per job ("xfer:progress:<jobId>"),
	// bukan satu event global, supaya panel migrasi yang sedang menonton
	// satu job tidak perlu menyaring event milik job lain.
	a.filexferSvc.SetEmitter(func(p filexfer.Progress) {
		runtime.EventsEmit(ctx, "xfer:progress:"+p.JobID, p)
	})

	// Sama seperti filexfer: progress migrasi Docker dikirim per job
	// ("dockerxfer:progress:<jobId>"), bukan event global.
	a.dockerxferSvc.SetEmitter(func(p dockerxfer.Progress) {
		runtime.EventsEmit(ctx, "dockerxfer:progress:"+p.JobID, p)
	})
}

// shutdown dipanggil Wails saat aplikasi ditutup — hentikan collector dulu
// (supaya tidak ada goroutine background yang masih mencoba pakai pool/db
// setelah keduanya ditutup), baru tutup koneksi SSH & database.
func (a *App) shutdown(ctx context.Context) {
	a.collector.Stop()
	a.streamMu.Lock()
	for _, cancel := range a.streams {
		cancel()
	}
	a.streamMu.Unlock()
	a.websiteSvc.CloseAllDBConns()
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
	// Tab migrasi tidak menunjuk server manapun, jadi tidak ada langganan
	// status collector yang perlu diturunkan prioritasnya.
	if tab.ServerID != "" {
		a.collector.Unsubscribe(tab.ServerID)
	}
	return nil
}

// OpenMigrationTab membuka tab migrasi baru. Berbeda dari tab server, tab
// ini tidak terikat ke satu server — server asal dan tujuan dipilih di
// dalam panelnya.
func (a *App) OpenMigrationTab(title string) (*session.Tab, error) {
	return a.sessionMgr.OpenMigrationTab(title)
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

// ---------------------------------------------------------------------
// Bindings: Files (lihat internal/modules/files)
// ---------------------------------------------------------------------

// ListFiles menampilkan isi satu direktori remote. asUser kosong berarti
// beroperasi sebagai user SSH yang login (lihat files/access.go untuk
// mekanisme elevasi sudo ke user lain).
func (a *App) ListFiles(serverID, path, asUser string) (*files.ListResult, error) {
	return a.filesSvc.List(a.ctx, serverID, path, asUser)
}

// ListSystemUsers mengembalikan user Linux di server yang bisa dipilih
// sebagai "jalankan sebagai" — dipakai untuk mengisi dropdown elevasi sudo
// di FilesPanel, cuma relevan kalau server.useSudo true.
func (a *App) ListSystemUsers(serverID string) ([]files.SystemUser, error) {
	return a.filesSvc.ListSystemUsers(a.ctx, serverID)
}

// CreateFolder membuat direktori baru.
func (a *App) CreateFolder(req files.MkdirRequest) error {
	return a.filesSvc.Mkdir(a.ctx, req)
}

// CreateFile membuat file kosong baru.
func (a *App) CreateFile(req files.CreateFileRequest) error {
	return a.filesSvc.CreateFile(a.ctx, req)
}

// RenameFile mengganti nama atau memindahkan file/direktori.
func (a *App) RenameFile(req files.RenameRequest) error {
	return a.filesSvc.Rename(a.ctx, req)
}

// DeleteFiles menghapus satu atau lebih file/direktori (rekursif untuk
// direktori berisi).
func (a *App) DeleteFiles(req files.DeleteRequest) error {
	return a.filesSvc.Delete(a.ctx, req)
}

// CompressFiles mengarsipkan file/direktori terpilih jadi satu file zip
// atau tar.gz.
func (a *App) CompressFiles(req files.CompressRequest) error {
	return a.filesSvc.Compress(a.ctx, req)
}

// ExtractArchive mengekstrak arsip zip/tar.gz ke direktori tujuan.
func (a *App) ExtractArchive(req files.ExtractRequest) error {
	return a.filesSvc.Extract(a.ctx, req)
}

// CopyFiles menyalin satu/lebih file/direktori ke lokasi tujuan (sumber
// tetap ada, beda dari RenameFile yang memindahkan).
func (a *App) CopyFiles(req files.CopyRequest) error {
	return a.filesSvc.Copy(a.ctx, req)
}

// SearchFiles mencari file/direktori di bawah satu path yang namanya
// mengandung kata kunci (rekursif, dibatasi 200 hasil).
func (a *App) SearchFiles(req files.SearchRequest) (*files.SearchResult, error) {
	return a.filesSvc.Search(a.ctx, req)
}

// ReadFileContent membaca isi file teks remote untuk dibuka di editor.
func (a *App) ReadFileContent(serverID, path, asUser string) (*files.ReadResult, error) {
	return a.filesSvc.ReadFile(a.ctx, serverID, path, asUser)
}

// WriteFileContent menyimpan hasil edit isi file teks remote.
func (a *App) WriteFileContent(req files.WriteFileRequest) error {
	return a.filesSvc.WriteFile(a.ctx, req)
}

// ChmodFile mengubah permission file/direktori remote.
func (a *App) ChmodFile(req files.ChmodRequest) error {
	return a.filesSvc.Chmod(a.ctx, req)
}

// UploadFilesToServer membuka dialog pilih-file NATIVE OS (boleh pilih
// lebih dari satu sekaligus), lalu meng-upload tiap file yang dipilih ke
// remoteDir. Ini sengaja BUKAN <input type=file>+base64 ala aplikasi web —
// dialog asli lebih natural untuk aplikasi desktop dan menghindari
// menampung seluruh isi file di memori JS.
func (a *App) UploadFilesToServer(serverID, remoteDir, asUser string) error {
	localPaths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Pilih file untuk diupload",
	})
	if err != nil {
		return err
	}
	for _, localPath := range localPaths {
		if err := a.filesSvc.UploadFromLocalPath(a.ctx, serverID, localPath, remoteDir, asUser); err != nil {
			return fmt.Errorf("upload %s: %w", localPath, err)
		}
	}
	return nil
}

// DownloadFileFromServer membuka dialog simpan NATIVE OS untuk memilih
// lokasi tujuan di komputer lokal, lalu men-stream file remote langsung ke
// sana. String path kosong (tanpa error) berarti user membatalkan dialog.
func (a *App) DownloadFileFromServer(serverID, remotePath, asUser string) error {
	localPath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Simpan file",
		DefaultFilename: files.BaseName(remotePath),
	})
	if err != nil {
		return err
	}
	if localPath == "" {
		return nil
	}
	return a.filesSvc.DownloadToLocalPath(a.ctx, serverID, remotePath, localPath, asUser)
}

// ---------------------------------------------------------------------
// Bindings: Docker (lihat internal/modules/docker)
//
// Scope sengaja mengikuti homepoin apa adanya: Containers dan Networks
// adalah satu-satunya submenu Docker yang benar-benar berfungsi di sana
// (routing dicek langsung di homepoin/internal/modules/servers/docker) —
// Images, Volumes, dan Compose di homepoin cuma halaman placeholder
// "coming soon" tanpa backend sama sekali. poinhost meniru itu: Containers
// + Networks dibangun penuh (termasuk inspect/recreate, log & stats
// streaming, exec masuk container, dan wizard install engine), sementara
// tiga submenu lain diberi placeholder yang sama jujurnya di frontend,
// bukan dipalsukan seolah sudah ada.
// ---------------------------------------------------------------------

// dockerStreamEvent payload event baris demi baris (logs & instalasi
// engine) yang dikirim lewat runtime.EventsEmit ke "docker:logs:<id>" atau
// "docker:install:<id>".
type dockerStreamEvent struct {
	Type    string `json:"type"` // line | end | error
	Line    string `json:"line,omitempty"`
	Message string `json:"message,omitempty"`
}

// dockerStatsEvent payload event sampel statistik container yang dikirim
// lewat runtime.EventsEmit ke "docker:stats:<id>".
type dockerStatsEvent struct {
	Type    string                `json:"type"` // stats | end | error
	Stats   *docker.StatsResponse `json:"stats,omitempty"`
	Message string                `json:"message,omitempty"`
}

// ListDockerContainers mengembalikan daftar container Docker di server
// (di-cache 8 detik di dalam docker.Service agar switch tab cepat tidak
// membanjiri server dengan `docker ps` berulang).
func (a *App) ListDockerContainers(serverID string) (*docker.ListResponse, error) {
	return a.dockerSvc.ListContainers(serverID)
}

// StartDockerContainer/StopDockerContainer/RestartDockerContainer/RemoveDockerContainer
// menjalankan aksi pada satu container (mutex per-server mencegah dua aksi
// docker konflik dari dua tab yang menunjuk server yang sama).
func (a *App) StartDockerContainer(serverID, containerID string) error {
	return a.dockerSvc.StartContainer(serverID, containerID)
}

func (a *App) StopDockerContainer(serverID, containerID string) error {
	return a.dockerSvc.StopContainer(serverID, containerID)
}

func (a *App) RestartDockerContainer(serverID, containerID string) error {
	return a.dockerSvc.RestartContainer(serverID, containerID)
}

func (a *App) RemoveDockerContainer(serverID, containerID string) error {
	return a.dockerSvc.RemoveContainer(serverID, containerID)
}

// DockerContainerLogs membaca snapshot tail log container (dipakai saat
// modal log pertama kali dibuka, sebelum stream realtime tersambung).
func (a *App) DockerContainerLogs(req docker.LogsRequest) (*docker.LogsResponse, error) {
	return a.dockerSvc.ContainerLogs(req)
}

// StreamDockerContainerLogs membuka stream `docker logs -f` realtime dan
// mengembalikan streamID — baris berikutnya mengalir lewat event
// "docker:logs:<streamID>" sampai StopDockerStream dipanggil atau proses
// berakhir sendiri (container stop, dsb).
func (a *App) StreamDockerContainerLogs(req docker.LogsRequest) (string, error) {
	streamID := uuid.NewString()
	ctx, cancel := context.WithCancel(a.ctx)
	a.registerStream(streamID, cancel)
	eventName := "docker:logs:" + streamID

	go func() {
		err := a.dockerSvc.StreamContainerLogs(ctx, req, func(line string) error {
			runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "line", Line: line})
			return nil
		})
		a.stopStream(streamID)
		if err != nil && !errors.Is(err, context.Canceled) {
			runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "error", Message: err.Error()})
			return
		}
		runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "end"})
	}()

	return streamID, nil
}

// DockerContainerStats membaca snapshot statistik container sekali (tanpa stream).
func (a *App) DockerContainerStats(serverID, containerID string) (*docker.StatsResponse, error) {
	return a.dockerSvc.ContainerStats(serverID, containerID)
}

// StreamDockerContainerStats membuka stream `docker stats` realtime (~1
// sampel/detik) dan mengembalikan streamID — sampel berikutnya mengalir
// lewat event "docker:stats:<streamID>".
func (a *App) StreamDockerContainerStats(serverID, containerID string) (string, error) {
	streamID := uuid.NewString()
	ctx, cancel := context.WithCancel(a.ctx)
	a.registerStream(streamID, cancel)
	eventName := "docker:stats:" + streamID

	go func() {
		err := a.dockerSvc.StreamContainerStats(ctx, serverID, containerID, func(sample *docker.StatsResponse) error {
			runtime.EventsEmit(a.ctx, eventName, dockerStatsEvent{Type: "stats", Stats: sample})
			return nil
		})
		a.stopStream(streamID)
		if err != nil && !errors.Is(err, context.Canceled) {
			runtime.EventsEmit(a.ctx, eventName, dockerStatsEvent{Type: "error", Message: err.Error()})
			return
		}
		runtime.EventsEmit(a.ctx, eventName, dockerStatsEvent{Type: "end"})
	}()

	return streamID, nil
}

// StopDockerStream membatalkan satu stream Docker yang sedang berjalan
// (logs, stats, atau instalasi engine) — dipanggil saat user menutup modal
// atau berpindah container/tab.
func (a *App) StopDockerStream(streamID string) {
	a.stopStream(streamID)
}

// ListDockerNetworks mengembalikan daftar network Docker di server.
func (a *App) ListDockerNetworks(serverID string) (*docker.NetworkListResponse, error) {
	return a.dockerSvc.ListNetworks(serverID)
}

// CreateDockerNetwork membuat network Docker baru di server.
func (a *App) CreateDockerNetwork(req docker.NetworkCreateRequest) (*docker.NetworkInfo, error) {
	return a.dockerSvc.CreateNetwork(req)
}

// RemoveDockerNetwork menghapus network Docker (bawaan Docker ditolak di service).
func (a *App) RemoveDockerNetwork(serverID, name string) error {
	return a.dockerSvc.RemoveNetwork(serverID, name)
}

// InspectDockerContainer membaca konfigurasi container untuk ditampilkan di
// editor recreate (env/port/volume/memory).
func (a *App) InspectDockerContainer(serverID, containerID string) (*docker.ContainerInspectResponse, error) {
	return a.dockerSvc.InspectContainer(serverID, containerID)
}

// RecreateDockerContainer menerapkan perubahan konfigurasi lewat siklus
// stop → rm → create → start (docker tidak punya "edit container in place").
func (a *App) RecreateDockerContainer(req docker.RecreateContainerRequest) (*docker.RecreateContainerResponse, error) {
	return a.dockerSvc.RecreateContainer(req)
}

// DetectDockerEngine membaca status instalasi Docker di server (dipakai
// wizard install: distro, package manager, sudah terpasang atau belum).
func (a *App) DetectDockerEngine(serverID string) (*docker.EngineStatus, error) {
	return a.dockerSvc.DetectEngineStatus(serverID)
}

// StartDockerEngine mengaktifkan (systemctl start/enable) service Docker
// yang sudah terpasang tapi sedang tidak berjalan.
func (a *App) StartDockerEngine(serverID string) (*docker.EngineStatus, error) {
	return a.dockerSvc.StartEngine(serverID)
}

// StreamDockerEngineInstall menjalankan wizard instalasi Docker Engine
// (get.docker.com per distro) sambil mengalirkan progresnya lewat event
// "docker:install:<streamID>", timeout keseluruhan 30 menit.
func (a *App) StreamDockerEngineInstall(serverID string) (string, error) {
	streamID := uuid.NewString()
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Minute)
	a.registerStream(streamID, cancel)
	eventName := "docker:install:" + streamID

	go func() {
		err := a.dockerSvc.StreamInstallEngine(ctx, serverID, func(line string) error {
			runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "line", Line: line})
			return nil
		})
		a.stopStream(streamID)
		switch {
		case err == nil:
			runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "end"})
		case errors.Is(err, context.Canceled):
			// Dibatalkan user lewat StopDockerStream — tidak perlu event error.
		case errors.Is(err, context.DeadlineExceeded):
			runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "error", Message: "Instalasi melebihi batas waktu (30 menit) — periksa koneksi atau lock paket di server."})
		default:
			runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "error", Message: err.Error()})
		}
	}()

	return streamID, nil
}

// OpenDockerExec membuka sesi PTY "docker exec -it <container> <shell>" di
// dalam tab tertentu, memakai ulang seluruh infrastruktur terminal.Service
// (event "terminal:output:<sessionId>"/"terminal:exit:<sessionId>" yang
// sama dengan Terminal VPS biasa — lihat startup()). Kalau server butuh
// sudo, password dikirim lewat stdin PTY (WriteTerminal) sesaat setelah
// sesi terbuka, sama seperti pola sudo di modul files.
func (a *App) OpenDockerExec(tabID, containerID, shell string) (string, error) {
	tab, err := a.sessionMgr.GetTab(tabID)
	if err != nil {
		return "", err
	}
	execCmd, err := a.dockerSvc.BuildExecCommand(tab.ServerID, containerID, shell)
	if err != nil {
		return "", err
	}
	sessionID, err := a.terminalSvc.OpenCommand(a.ctx, tabID, tab.ServerID, execCmd.Command)
	if err != nil {
		return "", err
	}
	if execCmd.SudoPassword != "" {
		go func() {
			time.Sleep(120 * time.Millisecond)
			_ = a.terminalSvc.Write(sessionID, []byte(execCmd.SudoPassword+"\n"))
		}()
	}
	return sessionID, nil
}

// ---------------------------------------------------------------------
// Bindings: Website (lihat internal/modules/website)
//
// Scope tahap ini: domain/vhost Nginx (list/buat/subdomain/hapus/enable-
// disable), wizard install Nginx, PHP-FPM (versi/switch per domain), dan
// SSL Let's Encrypt via certbot — ini "inti" menu Website homepoin.
// Files/Logs/Proxy/Database/Cron/DNS per-domain + akun SFTP menyusul di
// sesi berikutnya (lihat ARCHITECTURE.md §Website untuk detail scope dan
// diagnosis KENAPA homepoin terasa lambat pindah menu di modul ini —
// bukan soal SSH re-handshake, tapi terlalu banyak round-trip perintah
// berurutan per halaman, yang diperbaiki di sini lewat penggabungan skrip
// + cache pendek per server).
// ---------------------------------------------------------------------

// ListWebsites mengembalikan status Nginx + daftar domain/vhost di server.
func (a *App) ListWebsites(serverID string) (*website.ListResponse, error) {
	return a.websiteSvc.List(serverID)
}

// CreateWebsite adalah alur "Buat Website" gabungan: domain + PHP (opsional)
// + SSL (opsional) dalam SATU submit — beda dari homepoin yang mengharuskan
// tiga kunjungan halaman terpisah (Domains -> PHP -> SSL) untuk hasil yang
// sama.
func (a *App) CreateWebsite(req website.CreateWebsiteRequest) (*website.CreateWebsiteResult, error) {
	return a.websiteSvc.CreateWebsite(req)
}

// CreateWebsiteSubdomain membuat subdomain baru di bawah domain induk.
func (a *App) CreateWebsiteSubdomain(req website.CreateSubdomainRequest) (website.DomainInfo, error) {
	return a.websiteSvc.CreateSubdomain(req)
}

// DeleteWebsite menghapus vhost domain (dan opsional document root-nya).
func (a *App) DeleteWebsite(req website.DeleteDomainRequest) error {
	return a.websiteSvc.Delete(req)
}

// SetWebsiteEnabled mengaktifkan/menonaktifkan satu domain.
func (a *App) SetWebsiteEnabled(req website.SetEnabledRequest) error {
	return a.websiteSvc.SetEnabled(req)
}

// DetectNginxEngine membaca status instalasi Nginx di server (dipakai wizard).
func (a *App) DetectNginxEngine(serverID string) (*website.NginxStatus, error) {
	return a.websiteSvc.GetNginxStatus(serverID)
}

// StartNginxEngine mengaktifkan service Nginx yang sudah terpasang tapi
// sedang tidak berjalan.
func (a *App) StartNginxEngine(serverID string) (*website.NginxStatus, error) {
	return a.websiteSvc.StartNginx(serverID)
}

// StreamWebsiteInstall menjalankan instalasi salah satu komponen hosting
// (kind: "nginx" | "php-repo" | "php" | "certbot"; param = versi PHP kalau
// kind=="php") sambil mengalirkan progresnya lewat event
// "website:install:<streamID>" — satu titik masuk untuk semua jenis
// instalasi hosting, mengikuti pola homepoin yang juga memakai SATU handler
// untuk nginx/php/php-repo/certbot (bukan endpoint terpisah per jenis).
func (a *App) StreamWebsiteInstall(serverID, kind, param string) (string, error) {
	streamID := uuid.NewString()
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Minute)
	a.registerStream(streamID, cancel)
	eventName := "website:install:" + streamID

	go func() {
		err := a.websiteSvc.StreamInstall(ctx, serverID, kind, param, func(line string) error {
			runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "line", Line: line})
			return nil
		})
		a.stopStream(streamID)
		switch {
		case err == nil:
			runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "end"})
		case errors.Is(err, context.Canceled):
			// Dibatalkan user lewat StopDockerStream — tidak perlu event error.
		case errors.Is(err, context.DeadlineExceeded):
			runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "error", Message: "Instalasi melebihi batas waktu (30 menit) — periksa koneksi atau lock paket di server."})
		default:
			runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "error", Message: err.Error()})
		}
	}()

	return streamID, nil
}

// GetWebsitePHPStatus membaca status PHP-FPM di server (+ PHP domain
// tertentu kalau domain diisi, kosongkan untuk status server-wide saja).
func (a *App) GetWebsitePHPStatus(serverID, domain string) (*website.PHPStatus, error) {
	return a.websiteSvc.PHPStatus(serverID, domain)
}

// SetWebsitePHP mengaktifkan satu versi PHP-FPM untuk satu domain.
func (a *App) SetWebsitePHP(req website.PHPSetDomainRequest) error {
	return a.websiteSvc.PHPSetDomain(req)
}

// DisableWebsitePHP mengembalikan domain ke static-only (melepas PHP).
func (a *App) DisableWebsitePHP(serverID, domain string) error {
	return a.websiteSvc.PHPDisableDomain(serverID, domain)
}

// GetWebsiteSSLStatus membaca status SSL untuk satu domain (parent).
func (a *App) GetWebsiteSSLStatus(serverID, domain string) (*website.SSLStatus, error) {
	return a.websiteSvc.SSLStatus(serverID, domain)
}

// IssueWebsiteSSL menerbitkan sertifikat Let's Encrypt baru (belum otomatis
// mengaktifkannya di vhost — lihat EnableWebsiteSSL).
func (a *App) IssueWebsiteSSL(req website.SSLIssueRequest) (*website.SSLStatus, error) {
	return a.websiteSvc.SSLIssue(req)
}

// EnableWebsiteSSL menerapkan sertifikat yang sudah terbit ke vhost domain
// (+ subdomainnya) dan reload Nginx.
func (a *App) EnableWebsiteSSL(serverID, domain string) (*website.SSLStatus, error) {
	return a.websiteSvc.SSLEnable(serverID, domain)
}

// DisableWebsiteSSL melepas SSL dari vhost (sertifikat di server tidak dihapus).
func (a *App) DisableWebsiteSSL(serverID, domain string) (*website.SSLStatus, error) {
	return a.websiteSvc.SSLDisable(serverID, domain)
}

// RenewWebsiteSSL memperbarui sertifikat via certbot renew + reload Nginx.
func (a *App) RenewWebsiteSSL(serverID, domain string) (*website.SSLStatus, error) {
	return a.websiteSvc.SSLRenew(serverID, domain)
}

// EnableWebsiteSSLAutoRenew memasang ulang mekanisme auto-renew (deploy-hook
// reload Nginx + cron jaring pengaman) — dipasang otomatis setiap kali SSL
// diterbitkan/diaktifkan, binding ini untuk sertifikat yang sudah ada
// SEBELUM fitur ini ditambahkan (atau kalau pemasangan otomatisnya gagal).
func (a *App) EnableWebsiteSSLAutoRenew(serverID, domain string) (*website.SSLStatus, error) {
	return a.websiteSvc.EnableSSLAutoRenew(serverID, domain)
}

// GetWebsiteDomainRoot mengembalikan document root satu domain — dipakai
// tab Files untuk mengunci FilesPanel ke root itu.
func (a *App) GetWebsiteDomainRoot(serverID, domain string) (string, error) {
	return a.websiteSvc.DomainRoot(serverID, domain)
}

// GetWebsiteDNSPreview menghitung preview zona DNS (BIND) untuk satu domain
// parent — murni komputasi, tidak ada exec SSH.
func (a *App) GetWebsiteDNSPreview(serverID, domain string) (*website.DNSPreviewResponse, error) {
	return a.websiteSvc.DNSPreview(serverID, domain)
}

// ExportWebsiteDNSZone membuka dialog "Simpan" native lalu menulis teks
// zona DNS ke file lokal yang dipilih — dialog OS asli, bukan trik
// Blob/`<a download>` ala browser (konsisten dengan UploadFilesToServer/
// DownloadFileFromServer di modul Files).
func (a *App) ExportWebsiteDNSZone(serverID, domain string) error {
	preview, err := a.websiteSvc.DNSPreview(serverID, domain)
	if err != nil {
		return err
	}
	localPath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Simpan file zona DNS",
		DefaultFilename: preview.Filename,
	})
	if err != nil {
		return err
	}
	if localPath == "" {
		return nil
	}
	return os.WriteFile(localPath, []byte(preview.ZoneText), 0o644)
}

// GetWebsiteLog membaca snapshot tail log domain (access/error).
func (a *App) GetWebsiteLog(req website.LogReadRequest) (*website.LogReadResponse, error) {
	return a.websiteSvc.ReadDomainLog(req)
}

// StreamWebsiteLog mengalirkan log domain realtime (`tail -f`) lewat event
// "website:logs:<streamID>" — reuse stream registry yang sama dipakai
// Docker/Nginx install.
func (a *App) StreamWebsiteLog(req website.LogReadRequest) (string, error) {
	streamID := uuid.NewString()
	ctx, cancel := context.WithCancel(a.ctx)
	a.registerStream(streamID, cancel)
	eventName := "website:logs:" + streamID

	go func() {
		err := a.websiteSvc.StreamDomainLog(ctx, req, func(line string) error {
			runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "line", Line: line})
			return nil
		})
		a.stopStream(streamID)
		if err != nil && !errors.Is(err, context.Canceled) {
			runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "error", Message: err.Error()})
			return
		}
		runtime.EventsEmit(a.ctx, eventName, dockerStreamEvent{Type: "end"})
	}()

	return streamID, nil
}

// GetWebsiteProxyStatus membaca status reverse-proxy satu domain.
func (a *App) GetWebsiteProxyStatus(serverID, domain string) (*website.ProxyStatus, error) {
	return a.websiteSvc.ProxyStatus(serverID, domain)
}

// SetWebsiteProxyDomain mengaktifkan proxy untuk SELURUH domain ke satu target.
func (a *App) SetWebsiteProxyDomain(req website.ProxySetDomainRequest) error {
	return a.websiteSvc.ProxySetDomain(req)
}

// DisableWebsiteProxyDomain melepas proxy whole-domain (kembali ke static/PHP).
func (a *App) DisableWebsiteProxyDomain(serverID, domain string) error {
	return a.websiteSvc.ProxyDisableDomain(serverID, domain)
}

// SetWebsiteProxyRule menambah/mengganti satu aturan proxy per-path.
func (a *App) SetWebsiteProxyRule(req website.ProxyRuleRequest) error {
	return a.websiteSvc.ProxySetRule(req)
}

// DeleteWebsiteProxyRule menghapus satu aturan proxy per-path.
func (a *App) DeleteWebsiteProxyRule(serverID, domain, path string) error {
	return a.websiteSvc.ProxyDeleteRule(serverID, domain, path)
}

// ListWebsiteSFTPAccounts mengembalikan akun SFTP ter-chroot milik satu domain.
func (a *App) ListWebsiteSFTPAccounts(serverID, domain string) (*website.SFTPListResponse, error) {
	return a.websiteSvc.ListSFTPAccounts(serverID, domain)
}

// CreateWebsiteSFTPAccount membuat akun SFTP ter-chroot baru untuk satu domain.
func (a *App) CreateWebsiteSFTPAccount(req website.SFTPCreateAccountRequest) error {
	return a.websiteSvc.CreateSFTPAccount(req)
}

// DeleteWebsiteSFTPAccount menghapus akun SFTP.
func (a *App) DeleteWebsiteSFTPAccount(serverID, domain, username string) error {
	return a.websiteSvc.DeleteSFTPAccount(serverID, domain, username)
}

// ListWebsiteCronJobs mengembalikan semua job cron milik satu domain.
func (a *App) ListWebsiteCronJobs(serverID, domain string) (*website.CronListResponse, error) {
	return a.websiteSvc.ListCronJobs(serverID, domain)
}

// CreateWebsiteCronJob membuat job cron baru.
func (a *App) CreateWebsiteCronJob(req website.CronJobRequest) (*website.CronJobInfo, error) {
	return a.websiteSvc.CreateCronJob(req)
}

// UpdateWebsiteCronJob memperbarui job cron yang sudah ada.
func (a *App) UpdateWebsiteCronJob(req website.CronJobRequest) (*website.CronJobInfo, error) {
	return a.websiteSvc.UpdateCronJob(req)
}

// ToggleWebsiteCronJob mengaktifkan/menonaktifkan satu job cron.
func (a *App) ToggleWebsiteCronJob(req website.CronToggleRequest) error {
	return a.websiteSvc.ToggleCronJob(req)
}

// DeleteWebsiteCronJob menghapus satu job cron.
func (a *App) DeleteWebsiteCronJob(serverID, domain, jobID string) error {
	return a.websiteSvc.DeleteCronJob(serverID, domain, jobID)
}

// ReadWebsiteCronLog membaca log satu job cron.
func (a *App) ReadWebsiteCronLog(req website.CronLogRequest) (*website.CronLogResponse, error) {
	return a.websiteSvc.ReadCronLog(req)
}

// GetWebsiteDBPrivileges mengembalikan daftar opsi privilege tetap (statis,
// tanpa SSH) untuk dropdown grants MySQL/PostgreSQL.
func (a *App) GetWebsiteDBPrivileges() []website.DBPrivilegeOption {
	return website.DBPrivilegeOptions()
}

// GetWebsiteDBVersions mengembalikan daftar versi PINNED yang bisa dipilih
// eksplisit saat instalasi (statis, tanpa SSH) — di luar ini, opsi "(bawaan
// distro)" (versi kosong ke StreamWebsiteInstall) tetap selalu tersedia.
func (a *App) GetWebsiteDBVersions(engine string) []string {
	return website.DBSupportedVersions(engine)
}

// GetWebsiteDBStatus membaca status instalasi & service satu engine database.
func (a *App) GetWebsiteDBStatus(serverID, engine string) (*website.DBEngineStatus, error) {
	return a.websiteSvc.DBStatus(serverID, engine)
}

// StartWebsiteDB mengaktifkan service database yang sudah terpasang.
func (a *App) StartWebsiteDB(serverID, engine string) (*website.DBEngineStatus, error) {
	return a.websiteSvc.DBStart(serverID, engine)
}

// GetWebsiteDBDockerAccessStatus membaca status akses Docker->database saat
// ini (bind-address/listen_addresses + aturan firewall poinhost) — untuk
// instalasi yang dibuat lewat wizard poinhost ini sudah otomatis aktif,
// binding ini untuk instalasi yang sudah ada SEBELUM fitur ini ditambahkan.
func (a *App) GetWebsiteDBDockerAccessStatus(serverID, engine string) (*website.DBDockerAccessStatus, error) {
	return a.websiteSvc.GetDBDockerAccessStatus(serverID, engine)
}

// EnsureWebsiteDBDockerAccess mengaktifkan akses Docker->database untuk
// instalasi yang sudah ada sebelumnya (lihat dockeraccess.go).
func (a *App) EnsureWebsiteDBDockerAccess(serverID, engine string) (*website.DBDockerAccessStatus, error) {
	return a.websiteSvc.EnsureDBDockerAccess(serverID, engine)
}

// ListWebsiteDatabases mengembalikan daftar database di server.
func (a *App) ListWebsiteDatabases(serverID, engine string) ([]website.DBDatabaseInfo, error) {
	return a.websiteSvc.DBListDatabases(serverID, engine)
}

// CreateWebsiteDatabase membuat database baru.
func (a *App) CreateWebsiteDatabase(req website.DBCreateDatabaseRequest) error {
	return a.websiteSvc.DBCreateDatabase(req)
}

// ListWebsiteDatabaseUsers mengembalikan daftar user database.
func (a *App) ListWebsiteDatabaseUsers(serverID, engine string) ([]website.DBUserInfo, error) {
	return a.websiteSvc.DBListUsers(serverID, engine)
}

// CreateWebsiteDatabaseUser membuat user database baru + grants awal.
func (a *App) CreateWebsiteDatabaseUser(req website.DBCreateUserRequest) error {
	return a.websiteSvc.DBCreateUser(req)
}

// SetWebsiteDatabaseGrants menerapkan ulang grants untuk user yang sudah ada.
func (a *App) SetWebsiteDatabaseGrants(req website.DBGrantsRequest) error {
	return a.websiteSvc.DBSetGrants(req)
}

// DisableWebsiteDBDockerAccess menutup kembali akses Docker->database
// (database balik hanya mendengarkan localhost).
func (a *App) DisableWebsiteDBDockerAccess(serverID, engine string) (*website.DBDockerAccessStatus, error) {
	return a.websiteSvc.DisableDBDockerAccess(serverID, engine)
}

// DropWebsiteDatabase menghapus satu database beserta isinya (permanen).
func (a *App) DropWebsiteDatabase(serverID, engine, name string) error {
	return a.websiteSvc.DBDropDatabase(serverID, engine, name)
}

// DropWebsiteDatabaseUser menghapus satu user database (permanen). Untuk
// MySQL, host kosong berarti semua host user tersebut.
func (a *App) DropWebsiteDatabaseUser(serverID, engine, username, host string) error {
	return a.websiteSvc.DBDropUser(serverID, engine, username, host)
}

// GrantWebsiteDatabaseUser memberi satu user akses ke satu database
// ("assign user" di daftar database).
func (a *App) GrantWebsiteDatabaseUser(req website.DBDatabaseUserRequest) error {
	return a.websiteSvc.DBGrantDatabaseUser(req)
}

// RevokeWebsiteDatabaseUser mencabut akses satu user dari satu database.
func (a *App) RevokeWebsiteDatabaseUser(req website.DBDatabaseUserRequest) error {
	return a.websiteSvc.DBRevokeDatabaseUser(req)
}

// --- MySQL Manager: kredensial lokal (vault) + Explore koneksi driver asli ---

// SaveWebsiteDBCredential menyimpan password satu user database ke vault
// lokal (OS keychain / fallback file terenkripsi) — TIDAK PERNAH ditulis ke
// server target. Dipakai untuk menghubungkan user yang sudah ada di server
// (dibuat di luar poinhost) supaya bisa dipakai fitur Explore.
func (a *App) SaveWebsiteDBCredential(req website.SaveDBCredentialRequest) (*website.DBCredentialInfo, error) {
	return a.websiteSvc.SaveDBCredential(req)
}

// ForgetWebsiteDBCredential menghapus password tersimpan dari vault lokal.
func (a *App) ForgetWebsiteDBCredential(serverID, engine, username, host string) error {
	return a.websiteSvc.ForgetDBCredential(serverID, engine, username, host)
}

// ListWebsiteDBCredentials mengembalikan daftar kredensial yang sudah
// tersimpan untuk satu server (tanpa password-nya).
func (a *App) ListWebsiteDBCredentials(serverID string) ([]website.DBCredentialInfo, error) {
	return a.websiteSvc.ListDBCredentials(serverID)
}

// MySQLExploreListDatabases daftar database yang bisa dilihat kredensial ini.
func (a *App) MySQLExploreListDatabases(req website.MySQLExploreRequest) ([]string, error) {
	return a.websiteSvc.MySQLExploreListDatabases(req)
}

// MySQLExploreListTables daftar tabel dalam satu database.
func (a *App) MySQLExploreListTables(req website.MySQLExploreRequest, database string) ([]website.MySQLTableInfo, error) {
	return a.websiteSvc.MySQLExploreListTables(req, database)
}

// MySQLExploreListColumns daftar kolom satu tabel.
func (a *App) MySQLExploreListColumns(req website.MySQLExploreRequest, database, table string) ([]website.MySQLColumnInfo, error) {
	return a.websiteSvc.MySQLExploreListColumns(req, database, table)
}

// MySQLExploreTableRows membaca satu halaman baris (via koneksi driver asli
// yang ditunnel SSH — bukan exec CLI per halaman, supaya paginasi tabel
// besar tetap cepat).
func (a *App) MySQLExploreTableRows(req website.MySQLTableRowsRequest) (*website.MySQLTableRowsResult, error) {
	return a.websiteSvc.MySQLExploreTableRows(req)
}

// MySQLExploreInsertRow menyisipkan satu baris baru.
func (a *App) MySQLExploreInsertRow(req website.MySQLRowMutateRequest) error {
	return a.websiteSvc.MySQLExploreInsertRow(req)
}

// MySQLExploreUpdateRow memperbarui satu baris yang cocok dengan req.Where.
func (a *App) MySQLExploreUpdateRow(req website.MySQLRowMutateRequest) error {
	return a.websiteSvc.MySQLExploreUpdateRow(req)
}

// MySQLExploreDeleteRow menghapus satu baris yang cocok dengan req.Where.
func (a *App) MySQLExploreDeleteRow(req website.MySQLRowMutateRequest) error {
	return a.websiteSvc.MySQLExploreDeleteRow(req)
}

// MySQLExploreExecuteQuery menjalankan satu statement SQL bebas (kotak query).
func (a *App) MySQLExploreExecuteQuery(req website.MySQLQueryRequest) (*website.MySQLQueryResult, error) {
	return a.websiteSvc.MySQLExploreExecuteQuery(req)
}

// --- PostgreSQL Explore: koneksi driver asli, satu koneksi per database ---
// (beda dari MySQL yang bisa lintas database dalam satu koneksi via USE,
// lihat pgexplore.go)

// PGExploreListDatabases daftar database yang bisa dilihat role ini.
func (a *App) PGExploreListDatabases(req website.PGExploreRequest) ([]string, error) {
	return a.websiteSvc.PGExploreListDatabases(req)
}

// PGExploreListTables daftar tabel dalam satu schema (default "public").
func (a *App) PGExploreListTables(req website.PGTableRequest) ([]website.PGTableInfo, error) {
	return a.websiteSvc.PGExploreListTables(req)
}

// PGExploreListColumns daftar kolom satu tabel.
func (a *App) PGExploreListColumns(req website.PGTableRequest, table string) ([]website.PGColumnInfo, error) {
	return a.websiteSvc.PGExploreListColumns(req, table)
}

// PGExploreTableRows membaca satu halaman baris.
func (a *App) PGExploreTableRows(req website.PGTableRowsRequest) (*website.PGTableRowsResult, error) {
	return a.websiteSvc.PGExploreTableRows(req)
}

// PGExploreInsertRow menyisipkan satu baris baru.
func (a *App) PGExploreInsertRow(req website.PGRowMutateRequest) error {
	return a.websiteSvc.PGExploreInsertRow(req)
}

// PGExploreUpdateRow memperbarui satu baris yang cocok dengan req.Where.
func (a *App) PGExploreUpdateRow(req website.PGRowMutateRequest) error {
	return a.websiteSvc.PGExploreUpdateRow(req)
}

// PGExploreDeleteRow menghapus satu baris yang cocok dengan req.Where.
func (a *App) PGExploreDeleteRow(req website.PGRowMutateRequest) error {
	return a.websiteSvc.PGExploreDeleteRow(req)
}

// PGExploreExecuteQuery menjalankan satu statement SQL bebas (kotak query).
func (a *App) PGExploreExecuteQuery(req website.PGQueryRequest) (*website.PGQueryResult, error) {
	return a.websiteSvc.PGExploreExecuteQuery(req)
}

// --- Tautan domain<->database (kurasi lokal, lihat domaindb.go) ---

// LinkWebsiteDomainDatabase menautkan satu database ke satu domain.
func (a *App) LinkWebsiteDomainDatabase(serverID, domain, engine, dbName string) error {
	return a.websiteSvc.LinkDomainDatabase(serverID, domain, engine, dbName)
}

// UnlinkWebsiteDomainDatabase melepas tautan domain<->database.
func (a *App) UnlinkWebsiteDomainDatabase(serverID, domain, engine, dbName string) error {
	return a.websiteSvc.UnlinkDomainDatabase(serverID, domain, engine, dbName)
}

// ListWebsiteDomainDatabases daftar database yang ditautkan ke satu domain.
func (a *App) ListWebsiteDomainDatabases(serverID, domain string) ([]website.DomainDatabaseLink, error) {
	return a.websiteSvc.ListDomainDatabases(serverID, domain)
}

// --- Backup: export/import data via satu arsip terenkripsi passphrase ---
//
// Mekanisme pindah data antar perangkat TANPA akun/server/layanan pihak
// ketiga — lihat internal/core/backup untuk rasional lengkapnya. Passphrase
// TIDAK PERNAH disimpan di mana pun oleh poinhost; kalau lupa, arsipnya
// tidak bisa didekripsi sama sekali (tidak ada "lupa passphrase" recovery).

// ExportBackup membuka dialog "Simpan" native, membangun arsip (opsional
// menyertakan isi file private key SSH), mengenkripsinya dengan passphrase,
// lalu menulisnya ke lokasi yang dipilih user. Path kosong tanpa error
// berarti user membatalkan dialog.
func (a *App) ExportBackup(passphrase string, includeKeyFiles bool) (string, error) {
	encrypted, err := a.backupSvc.Export(passphrase, includeKeyFiles)
	if err != nil {
		return "", err
	}
	defaultName := "poinhost-backup-" + time.Now().Format("2006-01-02") + ".poinhostbkp"
	localPath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Simpan arsip backup poinhost",
		DefaultFilename: defaultName,
	})
	if err != nil {
		return "", err
	}
	if localPath == "" {
		return "", nil
	}
	if err := os.WriteFile(localPath, encrypted, 0o600); err != nil {
		return "", err
	}
	return localPath, nil
}

// ImportBackup membuka dialog "Buka" native untuk memilih file arsip,
// mendekripsinya dengan passphrase, lalu menuliskan isinya ke database +
// vault lokal perangkat ini (aman dijalankan berulang untuk arsip yang
// sama — lihat servers.Service.UpsertFromBackup).
func (a *App) ImportBackup(passphrase string) (*backup.ImportSummary, error) {
	localPath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Pilih arsip backup poinhost",
	})
	if err != nil {
		return nil, err
	}
	if localPath == "" {
		return nil, nil
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return nil, err
	}
	return a.backupSvc.Import(data, passphrase)
}

// --- Services (systemd) ---

// ListSystemServices mengembalikan seluruh unit systemd di server beserta
// status jalan & autostart-nya.
func (a *App) ListSystemServices(serverID string) (*services.ListResponse, error) {
	return a.servicesSvc.ListServices(serverID)
}

// SystemServiceAction menjalankan satu aksi systemctl pada satu service:
// start | stop | restart (state sekarang) atau enable | disable (autostart
// saat boot). Mengembalikan status terbaru service itu.
func (a *App) SystemServiceAction(serverID, name, action string) (*services.ServiceInfo, error) {
	return a.servicesSvc.ServiceAction(serverID, name, action)
}

// GetSystemService membaca status satu service.
func (a *App) GetSystemService(serverID, name string) (*services.ServiceInfo, error) {
	return a.servicesSvc.GetService(serverID, name)
}

// journalStreamEvent payload event log journald yang dikirim ke
// "services:journal:<streamID>". Entry-nya sudah terstruktur (bukan baris
// teks mentah seperti dockerStreamEvent) supaya UI bisa mewarnai per level
// tanpa menebak-nebak dari isi pesan.
type journalStreamEvent struct {
	Type    string                 `json:"type"` // entry | end | error
	Entry   *services.JournalEntry `json:"entry,omitempty"`
	Message string                 `json:"message,omitempty"`
}

// ReadSystemJournal membaca log journald (snapshot) sesuai filter unit,
// level, dan rentang waktu.
func (a *App) ReadSystemJournal(req services.JournalRequest) (*services.JournalResponse, error) {
	return a.servicesSvc.ReadJournal(req)
}

// StreamSystemJournal mengikuti log journald realtime (`journalctl -f`) lewat
// event "services:journal:<streamID>" — memakai stream registry yang sama
// dengan log Docker/Website, jadi dihentikan dengan StopDockerStream.
func (a *App) StreamSystemJournal(req services.JournalRequest) (string, error) {
	streamID := uuid.NewString()
	ctx, cancel := context.WithCancel(a.ctx)
	a.registerStream(streamID, cancel)
	eventName := "services:journal:" + streamID

	go func() {
		err := a.servicesSvc.StreamJournal(ctx, req, func(entry services.JournalEntry) error {
			runtime.EventsEmit(a.ctx, eventName, journalStreamEvent{Type: "entry", Entry: &entry})
			return nil
		})
		a.stopStream(streamID)
		if err != nil && !errors.Is(err, context.Canceled) {
			runtime.EventsEmit(a.ctx, eventName, journalStreamEvent{Type: "error", Message: err.Error()})
			return
		}
		runtime.EventsEmit(a.ctx, eventName, journalStreamEvent{Type: "end"})
	}()

	return streamID, nil
}

// --- Docker compose ---

// GetComposeStatus membandingkan konfigurasi container yang BERJALAN dengan
// docker-compose.yml yang ada sekarang. Perbandingannya memakai config-hash
// milik compose sendiri, jadi ukurannya sama dengan yang dipakai compose saat
// memutuskan sebuah service perlu dibuat ulang.
func (a *App) GetComposeStatus(serverID, containerID string) (*docker.ComposeInfo, error) {
	return a.dockerSvc.ComposeStatus(serverID, containerID)
}

// ApplyComposeConfig membuat ulang container dari compose file yang ada
// sekarang, sehingga tidak ada setelan yang tertinggal.
func (a *App) ApplyComposeConfig(serverID, containerID string) (*docker.ComposeInfo, error) {
	return a.dockerSvc.ApplyCompose(serverID, containerID)
}

// --- Firewall ---

// ListFirewallRules mengembalikan status firewall server beserta seluruh
// aturannya. Backend ditentukan dari yang SEDANG AKTIF, bukan ditebak dari
// OS — satu server bisa punya ufw, firewalld, dan nftables sekaligus.
// zone hanya dipakai firewalld; kosongkan untuk memakai zone default server.
func (a *App) ListFirewallRules(serverID, zone string) (*firewall.ListResponse, error) {
	return a.firewallSvc.ListRules(serverID, zone)
}

// AddFirewallRule menambah satu aturan ke backend yang aktif.
func (a *App) AddFirewallRule(req firewall.RuleRequest) (*firewall.ListResponse, error) {
	return a.firewallSvc.AddRule(req)
}

// DeleteFirewallRule menghapus satu aturan berdasarkan ID dari ListFirewallRules.
func (a *App) DeleteFirewallRule(serverID, ruleID, zone string) (*firewall.ListResponse, error) {
	return a.firewallSvc.DeleteRule(serverID, ruleID, zone)
}

// AllowDockerDatabaseAccess memasang aturan agar container Docker bisa
// menghubungi MySQL/PostgreSQL di host. Jalan pintas untuk kasus yang tidak
// terlihat jelas: port container yang di-publish tetap terbuka, tapi koneksi
// container KE host kena kebijakan deny firewall.
func (a *App) AllowDockerDatabaseAccess(serverID, zone string) (*firewall.ListResponse, error) {
	return a.firewallSvc.AllowDockerToDatabase(serverID, zone)
}

// ReloadFirewall menerapkan ulang aturan firewall tanpa mematikannya.
// Dipakai terutama sesudah Docker restart, yang menyisipkan ulang aturan
// iptables-nya sendiri dan bisa mengubah urutan relatif terhadap firewall.
func (a *App) ReloadFirewall(serverID string) (*firewall.ListResponse, error) {
	return a.firewallSvc.Reload(serverID)
}

// DetectDockerSubnets membaca subnet bridge Docker yang benar-benar ada di
// server — dipakai supaya aturan firewall disusun dari keadaan nyata, bukan
// dari rentang yang diasumsikan.
func (a *App) DetectDockerSubnets(serverID string) ([]firewall.DockerSubnet, error) {
	return a.firewallSvc.DetectDockerSubnets(serverID)
}

// SetFirewallEnabled menyalakan/mematikan firewall. Saat menyalakan, port SSH
// yang dipakai koneksi ini dibuka lebih dulu di skrip yang sama supaya user
// tidak terkunci dari servernya sendiri.
func (a *App) SetFirewallEnabled(serverID string, enabled bool) (*firewall.ListResponse, error) {
	return a.firewallSvc.SetEnabled(serverID, enabled)
}

// ---------------------------------------------------------------------
// Bindings: Migrasi — transfer file antar server (lihat internal/modules/filexfer)
// ---------------------------------------------------------------------

// XferListDirectory menampilkan isi folder di server, dipakai browser
// sumber & tujuan di panel migrasi.
func (a *App) XferListDirectory(serverID, dir string) (*filexfer.ListDirResponse, error) {
	return a.filexferSvc.ListDirectory(a.ctx, serverID, dir)
}

// XferStart memulai transfer file antar server. Hasilnya hanya progress
// AWAL — kemajuan selanjutnya mengalir lewat event "xfer:progress:<jobId>"
// sampai statusnya terminal.
func (a *App) XferStart(req filexfer.StartRequest) (*filexfer.Progress, error) {
	return a.filexferSvc.Start(req)
}

// XferStatus menarik progress terakhir satu transfer — dipakai saat panel
// dibuka kembali, supaya tidak perlu menunggu event berikutnya.
func (a *App) XferStatus(jobID string) (*filexfer.Progress, error) {
	return a.filexferSvc.Status(jobID)
}

// XferCancel membatalkan transfer yang sedang berjalan.
func (a *App) XferCancel(jobID string) (*filexfer.Progress, error) {
	return a.filexferSvc.Cancel(jobID)
}

// ---------------------------------------------------------------------
// Bindings: Migrasi — migrasi container Docker antar server
// (lihat internal/modules/dockerxfer; daftar container sumber memakai
// ListDockerContainers yang sudah ada, tidak perlu binding baru untuk itu).
// ---------------------------------------------------------------------

// DockerXferStart memulai migrasi satu container Docker ke server lain.
// Hasilnya hanya progress AWAL — kemajuan selanjutnya mengalir lewat event
// "dockerxfer:progress:<jobId>" sampai statusnya terminal.
func (a *App) DockerXferStart(req dockerxfer.StartRequest) (*dockerxfer.Progress, error) {
	return a.dockerxferSvc.Start(req)
}

// DockerXferStatus menarik progress terakhir satu migrasi — dipakai saat
// panel dibuka kembali.
func (a *App) DockerXferStatus(jobID string) (*dockerxfer.Progress, error) {
	return a.dockerxferSvc.Status(jobID)
}

// DockerXferCancel membatalkan migrasi yang sedang berjalan.
func (a *App) DockerXferCancel(jobID string) (*dockerxfer.Progress, error) {
	return a.dockerxferSvc.Cancel(jobID)
}
