package website

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/servers"
)

// statusCacheTTL adalah masa berlaku cache status Nginx & daftar domain per
// server. Homepoin mendeteksi ulang status Nginx (2 perintah SSH) DAN
// mem-parsing ulang seluruh vhost setiap kali domains/php/ssl butuh salah
// satunya, walau dalam satu kunjungan halaman logis semuanya sudah tahu
// jawabannya — cache pendek ini menghapus pengulangan itu tanpa membuat
// datanya basi terlalu lama (5 detik cukup untuk satu sesi klik-klik
// berpindah tab domain/php/ssl, tapi tetap terasa "hidup" kalau ada
// perubahan nyata di server).
const statusCacheTTL = 5 * time.Second

type nginxStatusCacheEntry struct {
	status    *NginxStatus
	expiresAt time.Time
}

type domainListCacheEntry struct {
	domains   []DomainInfo
	expiresAt time.Time
}

// Service mengorkestrasi manajemen Website (domain/vhost Nginx + PHP-FPM +
// SSL Let's Encrypt) di server remote via SSH. Semua perintah dijalankan
// lewat sshpool.Executor pada slot shared yang SAMA dipakai modul lain —
// pindah dari tab Website ke Files/Docker/Terminal (atau sebaliknya) tidak
// pernah memicu dial SSH baru (lihat ARCHITECTURE.md §3).
type Service struct {
	servers  *servers.Service
	executor *sshpool.Executor
	mutex    *sshpool.ServerMutexRegistry

	cacheMu     sync.Mutex
	nginxCache  map[string]nginxStatusCacheEntry
	domainCache map[string]domainListCacheEntry
}

// NewService membuat service modul website baru.
func NewService(serversSvc *servers.Service, executor *sshpool.Executor, mutex *sshpool.ServerMutexRegistry) *Service {
	return &Service{
		servers:     serversSvc,
		executor:    executor,
		mutex:       mutex,
		nginxCache:  make(map[string]nginxStatusCacheEntry),
		domainCache: make(map[string]domainListCacheEntry),
	}
}

// run menjalankan satu perintah/skrip hosting dengan context & timeout
// sendiri (destructive=true — tanpa retry, supaya hang sudo/nginx tidak
// dikalikan RetryMax milik Executor).
func (s *Service) run(access *websiteAccess, script string, timeout time.Duration) (*sshpool.ExecResult, error) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	res, err := s.executor.Exec(ctx, access.serverID, timeout, access.wrap(script), true)
	if err != nil {
		return res, mapWebsiteError(err)
	}
	return res, nil
}

func (s *Service) invalidateNginxCache(serverID string) {
	s.cacheMu.Lock()
	delete(s.nginxCache, serverID)
	s.cacheMu.Unlock()
}

func (s *Service) invalidateDomainCache(serverID string) {
	s.cacheMu.Lock()
	delete(s.domainCache, serverID)
	s.cacheMu.Unlock()
}

// StreamInstall menginstal salah satu komponen hosting (nginx, repo PHP,
// satu versi PHP-FPM, atau certbot) sambil mengalirkan progres ke onLine —
// satu titik masuk untuk semua jenis instalasi hosting, mengikuti pola
// homepoin yang juga memakai SATU handler WS untuk nginx/php/php-repo/
// certbot (bukan endpoint terpisah per jenis).
//
// kind: "nginx" | "php-repo" | "php" (param = versi, mis. "8.3") | "certbot"
func (s *Service) StreamInstall(ctx context.Context, serverID, kind, param string, onLine func(string) error) error {
	if onLine == nil {
		onLine = func(string) error { return nil }
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}

	_ = onLine("Mendeteksi sistem operasi server...")
	distro, err := s.detectDistro(access)
	if err != nil {
		return err
	}
	_ = onLine("Distro terdeteksi: " + distro.Name + " (paket: " + distro.PackageManager + ")")

	var script string
	var ok bool
	switch kind {
	case "nginx":
		script, ok = installNginxScript(distro.PackageManager)
	case "php-repo":
		script, ok = phpRepoScript(distro.PackageManager, distro.ID)
	case "php":
		script, ok = phpInstallVersionScript(distro.PackageManager, param)
	case "certbot":
		script, ok = installCertbotScript(distro.PackageManager)
	default:
		return errFmt("jenis instalasi %q tidak dikenal", kind)
	}
	if !ok {
		return errFmt("distro tidak didukung untuk instalasi otomatis (package manager: %s)", distro.PackageManager)
	}

	if !s.mutex.TryLock(serverID) {
		_ = onLine("Server sedang dipakai operasi lain, menunggu antrian...")
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				if s.mutex.TryLock(serverID) {
					goto locked
				}
			}
		}
	}
locked:
	defer s.mutex.Unlock(serverID)

	_ = onLine("Memulai instalasi...")
	if err := s.runInstallStream(ctx, access, script, onLine); err != nil {
		return err
	}
	switch kind {
	case "nginx":
		s.invalidateNginxCache(serverID)
	case "php":
		// tidak ada cache PHP khusus — status di-fetch ulang oleh frontend
	}
	_ = onLine("Instalasi selesai.")
	return nil
}

func (s *Service) runInstallStream(ctx context.Context, access *websiteAccess, script string, onLine func(string) error) error {
	const sentinel = "__poinhost_INSTALL_DONE__:"
	wrapped := "(\n" + script + "\n) 2>&1\necho \"" + sentinel + "$?\""
	exitCode := -1
	streamErr := s.executor.ExecStreamPTY(ctx, access.serverID, access.wrap(wrapped), func(line string) error {
		if strings.HasPrefix(line, sentinel) {
			codeStr := strings.TrimSpace(strings.TrimPrefix(line, sentinel))
			if n, err := parseExitCode(codeStr); err == nil {
				exitCode = n
			}
			return nil
		}
		return onLine(line)
	})
	if streamErr != nil {
		return streamErr
	}
	if exitCode != 0 {
		return errFmt("proses gagal dengan kode keluar %d", exitCode)
	}
	return nil
}
