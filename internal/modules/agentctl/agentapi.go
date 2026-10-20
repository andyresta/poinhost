package agentctl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/agent/agentcfg"
)

// AdminSocket adalah unix socket permukaan admin agent.
const AdminSocket = DataDir + "/admin.sock"

// errAgentTooOld adalah pesan untuk agent yang belum punya socket admin.
//
// Dipakai sebagai penanda di stderr sekaligus sebagai pesan ke user, supaya
// yang dibaca layar persis yang dimaksud — bukan terjemahan curl atas
// keadaan yang tidak dipahaminya.
const errAgentTooOld = "Agent di server ini belum mendukung persetujuan perangkat — tekan Update agent dulu."

// callAdmin memanggil permukaan admin agent lewat SSH.
//
// Lewat unix socket, bukan port TCP agent: socket itu ada di dalam direktori
// data bermode 0700 milik root, sehingga vhost nginx yang menerbitkan agent ke
// internet tidak punya jalan menyentuh persetujuan perangkat sama sekali.
// Autentikasinya adalah izin berkas itu — app desktop sudah naik ke root di
// sesi SSH-nya, dan tidak ada rahasia tambahan yang perlu disimpan di sini.
func (s *Service) callAdmin(ctx context.Context, serverID, method, path string) (string, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return "", err
	}
	script := "curl -fsS --max-time 10 -X " + method +
		" --unix-socket " + shellQuote(AdminSocket) +
		" http://localhost" + path + "\n"
	res, err := s.run(ctx, access, script, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("agent tidak merespons — pastikan service-nya berjalan dan versinya sudah mendukung persetujuan perangkat: %w", err)
	}
	return strings.TrimSpace(res.Stdout), nil
}

func (s *Service) readConfig(ctx context.Context, access *agentAccess) (*agentcfg.Config, error) {
	res, err := s.run(ctx, access, "base64 -w0 < "+ConfigPath+" 2>/dev/null || base64 < "+ConfigPath+" | tr -d '\\n'", 20*time.Second)
	if err != nil {
		return nil, err
	}
	cfg := decodeConfig(strings.TrimSpace(res.Stdout))
	if cfg == nil {
		return nil, fmt.Errorf("tidak bisa membaca %s — agent belum terpasang?", ConfigPath)
	}
	return cfg, nil
}

func (s *Service) writeConfig(ctx context.Context, serverID string, access *agentAccess, cfg *agentcfg.Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := s.uploadTemp(ctx, serverID, tmpConfigPath, append(b, '\n')); err != nil {
		return err
	}
	script := "set -e\ninstall -m 0600 -o root -g root " + tmpConfigPath + " " + ConfigPath + "\nrm -f " + tmpConfigPath + "\n"
	_, err = s.run(ctx, access, script, 30*time.Second)
	return err
}

func trimForError(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
