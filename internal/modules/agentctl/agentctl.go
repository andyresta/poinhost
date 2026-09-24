// Package agentctl mengelola poinhost-agent di server remote dari app desktop:
// deteksi, pasang, konfigurasi, update, dan copot — semuanya lewat SSH/SFTP.
//
// Arah kendalinya satu arah dan disengaja: app desktop yang MENYURUH server,
// bukan agent yang melapor keluar. Dengan begitu operasi paling rawan (menimpa
// binary agent lalu me-restart service-nya) tidak pernah dijalankan oleh proses
// yang sedang menimpa dirinya sendiri.
package agentctl

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/servers"
)

// Letak berkas agent di server. Tetap, supaya deteksi dan pencopotan tidak
// perlu menebak apa pun.
const (
	BinaryPath   = "/usr/local/bin/poinhost-agent"
	PrevPath     = "/usr/local/bin/poinhost-agent.prev"
	ConfigDir    = "/etc/poinhost-agent"
	ConfigPath   = "/etc/poinhost-agent/config.json"
	DataDir      = "/var/lib/poinhost-agent"
	KeyPath      = "/var/lib/poinhost-agent/id_ed25519"
	UnitName     = "poinhost-agent.service"
	UnitPath     = "/etc/systemd/system/poinhost-agent.service"
	DefaultPort  = 7898
	authKeyLabel = "poinhost-agent"
)

// Service mengorkestrasi operasi Agent Control.
type Service struct {
	servers  *servers.Service
	executor *sshpool.Executor
	sftp     *sshpool.SFTPClient

	// knownHosts dipakai untuk menyalin host key yang SUDAH disetujui user ke
	// agent — agent tidak punya manusia yang bisa menyetujui dialog fingerprint.
	knownHosts *sshpool.KnownHostsStore

	emitMu sync.RWMutex
	emit   func(Progress)
}

// NewService membuat service modul agentctl.
func NewService(serversSvc *servers.Service, executor *sshpool.Executor, sftp *sshpool.SFTPClient, knownHosts *sshpool.KnownHostsStore) *Service {
	return &Service{servers: serversSvc, executor: executor, sftp: sftp, knownHosts: knownHosts}
}

// agentAccess adalah konteks eksekusi di satu server. Polanya sama dengan
// modul docker/services/firewall: sudo murni untuk naik ke root kalau user SSH
// bukan root.
type agentAccess struct {
	serverID     string
	sshUser      string
	sshPort      int
	useSudo      bool
	sudoPassword string
}

func (s *Service) resolveAccess(serverID string) (*agentAccess, error) {
	srv, err := s.servers.Get(serverID)
	if err != nil {
		return nil, err
	}
	a := &agentAccess{
		serverID: serverID,
		sshUser:  srv.Username,
		sshPort:  srv.Port,
		useSudo:  srv.UseSudo,
	}
	if a.sshPort <= 0 {
		a.sshPort = 22
	}
	if srv.UseSudo && srv.Username != "root" && srv.AuthType == "password" {
		pw, _ := s.servers.SudoPassword(serverID)
		a.sudoPassword = pw
	}
	return a, nil
}

func (a *agentAccess) wrap(script string) string {
	inner := "bash -c " + shellQuote(script)
	if a.sshUser == "root" || !a.useSudo {
		return inner
	}
	if a.sudoPassword != "" {
		return "echo " + shellQuote(a.sudoPassword) + " | sudo -S -p '' " + inner
	}
	return "sudo -n " + inner
}

// isRoot melaporkan apakah perintah akan berjalan sebagai root.
func (a *agentAccess) isRoot() bool { return a.sshUser == "root" || a.useSudo }

func (s *Service) run(ctx context.Context, a *agentAccess, script string, timeout time.Duration) (*sshpool.ExecResult, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	// destructive=true → tanpa retry otomatis. Perintah di modul ini memasang,
	// mengganti, dan menghapus berkas; mengulanginya diam-diam saat timeout
	// bisa menjalankan setengah instalasi dua kali.
	res, err := s.executor.Exec(ctx, a.serverID, timeout, a.wrap(script), true)
	if err != nil {
		return res, mapAgentError(err, res)
	}
	return res, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// mapAgentError menerjemahkan kegagalan umum jadi pesan yang bisa
// ditindaklanjuti, bukan melempar stderr mentah ke UI.
func mapAgentError(err error, res *sshpool.ExecResult) error {
	if err == nil {
		return nil
	}
	detail := ""
	if res != nil {
		detail = strings.TrimSpace(sshpool.FailureDetail(res))
	}
	msg := strings.ToLower(err.Error() + " " + detail)
	switch {
	case strings.Contains(msg, "a password is required"),
		strings.Contains(msg, "a terminal is required"),
		strings.Contains(msg, "incorrect password attempt"):
		return fmt.Errorf("sudo membutuhkan password — pakai auth password SSH atau atur NOPASSWD di sudoers")
	case strings.Contains(msg, "permission denied"), strings.Contains(msg, "access denied"):
		return fmt.Errorf("tidak punya izin — Agent Control butuh root atau sudo di server ini")
	case strings.Contains(msg, "deadline exceeded"), strings.Contains(msg, "timeout"):
		return fmt.Errorf("timeout menjalankan perintah di server — cek koneksi atau beban server")
	}
	if detail != "" {
		return fmt.Errorf("%s", detail)
	}
	return err
}

// parseKV membaca keluaran skrip berbentuk "kunci=nilai" per baris.
//
// Formatnya sengaja sesederhana ini, bukan JSON yang disusun shell: skrip
// deteksi berjalan di server mana pun dengan util seadanya, dan menyusun JSON
// dengan echo adalah cara paling gampang menghasilkan keluaran rusak saat ada
// nilai yang memuat kutip.
func parseKV(out string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		m[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return m
}
