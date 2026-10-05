package agentctl

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/agent/agentcfg"
)

// callAgent memanggil HTTP lokal agent lewat SSH.
//
// Port agent hanya bind 127.0.0.1, jadi satu-satunya yang bisa menjangkaunya
// adalah proses di mesin itu sendiri — `curl` yang dijalankan lewat SSH.
// Bearer token dibaca dari config agent, bukan disimpan di sisi desktop, supaya
// tidak ada dua sumber kebenaran yang bisa berbeda.
func (s *Service) callAgent(ctx context.Context, serverID, method, path string) (string, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return "", err
	}
	acfg, err := s.readConfig(ctx, access)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(acfg.APIToken) == "" {
		return "", fmt.Errorf("config agent belum punya apiToken — pasang ulang agent")
	}

	script := "t=$(echo " + shellQuote(base64.StdEncoding.EncodeToString([]byte(acfg.APIToken))) + " | base64 -d)\n" +
		`curl -fsS --max-time 10 -X ` + method +
		` -H "Authorization: Bearer $t" http://127.0.0.1:` + strconv.Itoa(portOf(acfg.Listen)) + path + "\n"
	res, err := s.run(ctx, access, script, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("agent tidak merespons — pastikan service-nya berjalan: %w", err)
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
