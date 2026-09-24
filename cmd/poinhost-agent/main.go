// Command poinhost-agent adalah service yang dipasang di satu server dari
// menu Agent Control poinhost.
//
// Yang dilakukannya:
//   - memakai ULANG internal/modules/* yang sama dengan app desktop, jadi
//     tidak ada logika pengelolaan server yang ditulis dua kali;
//   - menyajikan HTTP di loopback saja (healthz + kontrol lokal);
//   - menjalankan bot Telegram lewat long-polling, tanpa port masuk.
//
// Server tempat agent berjalan diperlakukan seperti server terdaftar lain:
// koneksi SSH ke 127.0.0.1. Itu membuat SELURUH modul poinhost langsung
// bekerja untuk mesin lokal tanpa satu pun perubahan pada lapisan executor.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/andyresta/poinhost/internal/agent/agentcfg"
	"github.com/andyresta/poinhost/internal/agent/api"
	"github.com/andyresta/poinhost/internal/agent/backend"
	"github.com/andyresta/poinhost/internal/agent/bot"
	"github.com/andyresta/poinhost/internal/agent/telegram"
	"github.com/andyresta/poinhost/internal/core/config"
	"github.com/andyresta/poinhost/internal/core/database"
	"github.com/andyresta/poinhost/internal/core/secrets"
	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/servers"
)

// Version diisi saat build lewat -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	var (
		cfgPath     = flag.String("config", agentcfg.DefaultPath, "path file konfigurasi")
		showVersion = flag.Bool("version", false, "tampilkan versi lalu keluar")
	)
	flag.Parse()

	// --version dipakai app desktop untuk mendeteksi apakah agent terpasang
	// dan versi berapa, jadi keluarannya harus satu baris yang stabil.
	if *showVersion {
		fmt.Println(Version)
		return
	}

	log.SetFlags(log.LstdFlags | log.LUTC)
	if err := run(*cfgPath); err != nil {
		log.Fatalf("poinhost-agent: %v", err)
	}
}

func run(cfgPath string) error {
	acfg, err := agentcfg.Load(cfgPath)
	if err != nil {
		return err
	}

	// Sinyal ditangkap SEBELUM apa pun dibangun, supaya Ctrl-C / systemd stop
	// tetap bekerja meski startup masih berjalan lama (mis. bootstrap SSH).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	stack, err := newStack(acfg, cfgPath)
	if err != nil {
		return err
	}
	defer stack.Close()

	log.Printf("poinhost-agent %s — data %s, listen %s", Version, acfg.DataDir, acfg.Listen)

	pairing := bot.NewPairing()
	allow := backend.NewAllowlist(cfgPath, acfg)
	tstate := &telegramState{enabled: acfg.Telegram.Enabled}

	var wg sync.WaitGroup

	srv := &http.Server{
		Addr: acfg.Listen,
		Handler: api.Handler(api.Deps{
			Version:   Version,
			StartedAt: time.Now(),
			APIToken:  acfg.APIToken,
			DataDir:   acfg.DataDir,
			Pairing:   pairing,
			Allowlist: allow,
			Telegram:  tstate,
			Backend:   stack.backend,
			Servers:   stack.store,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http: %v", err)
			stop()
		}
	}()

	if acfg.Telegram.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := telegram.New(acfg.Telegram.Token)
			b := bot.New(client, stack.backend, allow, pairing)

			if me, err := client.GetMe(ctx); err == nil {
				tstate.set(me.DisplayName(), "")
			}
			if err := b.Run(ctx); err != nil && ctx.Err() == nil {
				// Bot berhenti sendiri hanya untuk kegagalan yang tidak bisa
				// diperbaiki dengan mencoba lagi (token ditolak). Agent TIDAK
				// ikut mati: /healthz harus tetap menjawab supaya Agent
				// Control bisa menampilkan sebabnya, bukan sekadar "mati".
				log.Printf("telegram: konektor berhenti: %v", err)
				tstate.set("", err.Error())
			}
		}()
	} else {
		log.Print("telegram: konektor nonaktif (belum dikonfigurasi)")
	}

	<-ctx.Done()
	log.Print("poinhost-agent: shutdown…")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)

	wg.Wait()
	return nil
}

// stack adalah seluruh dependensi poinhost yang dipakai agent.
type stack struct {
	backend   *backend.Backend
	store     *backend.Store
	collector *servers.Collector
	closeDB   func() error
}

func newStack(acfg *agentcfg.Config, cfgPath string) (*stack, error) {
	cfg, err := config.At(acfg.DataDir)
	if err != nil {
		return nil, err
	}
	if err := cfg.EnsureDirs(); err != nil {
		return nil, fmt.Errorf("siapkan direktori data: %w", err)
	}

	db, err := database.Open(cfg.DBPath())
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	vault, err := secrets.New(cfg.DataDir)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	knownHosts := sshpool.NewKnownHostsStore(cfg.KnownHostsPath())
	pool := sshpool.NewPool(cfg.SSH, knownHosts)
	mutex := sshpool.NewServerMutexRegistry()
	executor := sshpool.NewExecutor(pool, mutex, cfg.Executor)

	serversSvc := servers.NewService(servers.NewRepository(db), pool, executor, vault)
	collector := servers.NewCollector(serversSvc)

	// Bootstrap mendaftarkan semua server tersimpan ke pool. Tanpa ini,
	// status koneksi akan selalu "offline" sampai ada operasi yang kebetulan
	// membuka koneksi.
	if err := serversSvc.Bootstrap(context.Background()); err != nil {
		log.Printf("bootstrap server: %v", err)
	}

	// Mesin sendiri didaftarkan sebagai server SSH biasa ke 127.0.0.1 —
	// lihat agentcfg.SelfServer untuk alasannya.
	if err := ensureSelfServer(serversSvc, acfg, cfgPath); err != nil {
		log.Printf("daftarkan server lokal: %v", err)
	}

	// Collector menjadwalkan metrik untuk SEMUA server aktif (interval idle
	// 90 detik), bukan hanya yang sedang "dilihat" — di agent tidak ada
	// konsep tab, jadi cukup dijalankan dan snapshot-nya selalu hangat.
	// Itulah yang membuat /servers dan /status membalas tanpa SSH round-trip.
	collector.Start()

	return &stack{
		backend:   backend.New(serversSvc, collector, acfg.SelfServerID),
		store:     backend.NewStore(serversSvc, collector, knownHosts, filepath.Join(cfg.DataDir, "keys"), acfg.SelfServerID),
		collector: collector,
		closeDB:   db.Close,
	}, nil
}

func (s *stack) Close() {
	s.collector.Stop()
	_ = s.closeDB()
}

// telegramState menyimpan kondisi konektor untuk dilaporkan /healthz dan
// /v1/status, sehingga Agent Control bisa menampilkan "token ditolak" alih-alih
// hanya "bot tidak membalas".
type telegramState struct {
	mu       sync.RWMutex
	enabled  bool
	username string
	lastErr  string
}

func (t *telegramState) set(username, errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if username != "" {
		t.username = username
	}
	t.lastErr = errMsg
}

func (t *telegramState) Enabled() bool { return t.enabled }

func (t *telegramState) BotUsername() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.username
}

func (t *telegramState) LastError() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastErr
}

// selfServerTag menandai entri server yang mewakili mesin tempat agent
// berjalan, sehingga entri itu bisa ditemukan lagi meski SelfServerID di config
// hilang.
const selfServerTag = "agent-host"

// ensureSelfServer memastikan ada TEPAT SATU entri server yang mewakili mesin
// ini, lalu menyimpan ID-nya kembali ke config.
//
// Pencocokannya tidak hanya lewat SelfServerID: entri yang cocok dicari juga
// lewat tag + host + username. Tanpa itu, sekali saja ID di config hilang atau
// dikosongkan keliru, setiap start berikutnya menambah satu entri baru yang
// semuanya menunjuk 127.0.0.1 — dan daftar server di bot terisi duplikat yang
// tidak bisa dibersihkan dari mana pun.
func ensureSelfServer(svc *servers.Service, acfg *agentcfg.Config, cfgPath string) error {
	self := acfg.SelfServer
	if !self.Enabled {
		return nil
	}

	list, err := svc.List()
	if err != nil {
		return err
	}

	mine := make([]*servers.Server, 0, 2)
	for _, srv := range list {
		if srv.ID == acfg.SelfServerID || isSelfEntry(srv, self) {
			mine = append(mine, srv)
		}
	}

	if len(mine) > 0 {
		keep := pickSelfEntry(mine, acfg.SelfServerID)
		// Duplikat dari versi sebelumnya dibersihkan di sini, bukan dibiarkan
		// menumpuk — ini juga yang memulihkan instalasi yang sudah terlanjur
		// punya beberapa entri.
		for _, srv := range mine {
			if srv.ID == keep.ID {
				continue
			}
			if derr := svc.Delete(srv.ID); derr != nil {
				log.Printf("hapus entri server lokal duplikat %s: %v", srv.ID, derr)
				continue
			}
			log.Printf("entri server lokal duplikat dihapus (%s)", srv.ID)
		}
		if acfg.SelfServerID != keep.ID {
			acfg.SelfServerID = keep.ID
			if serr := agentcfg.Save(cfgPath, acfg); serr != nil {
				return fmt.Errorf("simpan selfServerId: %w", serr)
			}
		}
		return nil
	}

	saved, err := svc.Save(servers.SaveServerRequest{
		Name:     self.Name,
		Host:     self.Host,
		Port:     self.Port,
		Username: self.Username,
		AuthType: "key",
		KeyPath:  self.KeyPath,
		UseSudo:  self.UseSudo,
		Tags:     []string{selfServerTag},
	})
	if err != nil {
		return err
	}

	acfg.SelfServerID = saved.ID
	if err := agentcfg.Save(cfgPath, acfg); err != nil {
		return fmt.Errorf("simpan selfServerId: %w", err)
	}
	log.Printf("server lokal terdaftar sebagai %q (%s)", saved.Name, saved.ID)
	return nil
}

// isSelfEntry mengenali entri yang dibuat agent untuk mesinnya sendiri.
func isSelfEntry(srv *servers.Server, self agentcfg.SelfServer) bool {
	if srv == nil || srv.Host != self.Host || srv.Username != self.Username {
		return false
	}
	for _, tag := range srv.Tags {
		if tag == selfServerTag {
			return true
		}
	}
	return false
}

// pickSelfEntry memilih entri mana yang dipertahankan: yang ID-nya sudah
// tercatat di config kalau ada, kalau tidak yang paling lama dibuat — supaya
// pilihannya tidak berubah-ubah tiap start.
func pickSelfEntry(mine []*servers.Server, preferID string) *servers.Server {
	keep := mine[0]
	for _, srv := range mine {
		if srv.ID == preferID {
			return srv
		}
		if srv.CreatedAt.Before(keep.CreatedAt) {
			keep = srv
		}
	}
	return keep
}
