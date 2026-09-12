package website

import (
	"context"
	"strings"
	"time"
)

// LogReadRequest permintaan baca/stream log satu domain.
type LogReadRequest struct {
	ServerID string `json:"serverId"`
	Domain   string `json:"domain"`
	LogType  string `json:"logType"` // "access" | "error"
	Lines    int    `json:"lines"`
}

// LogReadResponse isi log domain (snapshot).
type LogReadResponse struct {
	Domain  string `json:"domain"`
	LogType string `json:"logType"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Exists  bool   `json:"exists"`
}

// domainLogPath path log Nginx untuk satu domain — sesuai `access_log`/
// `error_log` yang ditulis buildVhostConfig di vhost.go, jadi selalu
// konsisten dengan vhost yang sesungguhnya (bukan konvensi terpisah).
func domainLogPath(domain, logType string) (string, error) {
	switch logType {
	case "access":
		return "/var/log/nginx/" + domain + ".access.log", nil
	case "error":
		return "/var/log/nginx/" + domain + ".error.log", nil
	default:
		return "", errFmt("jenis log tidak dikenal: %s (pakai access atau error)", logType)
	}
}

func clampLogLines(n int) int {
	if n <= 0 {
		return 200
	}
	if n > 2000 {
		return 2000
	}
	return n
}

// ReadDomainLog membaca snapshot tail log domain (dipakai saat panel log
// pertama kali dibuka, sebelum stream realtime tersambung).
func (s *Service) ReadDomainLog(req LogReadRequest) (*LogReadResponse, error) {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return nil, err
	}
	path, err := domainLogPath(domain, req.LogType)
	if err != nil {
		return nil, err
	}
	if _, err := s.getDomain(req.ServerID, domain); err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return nil, err
	}

	lines := clampLogLines(req.Lines)
	script := `if [ ! -f ` + shellQuote(path) + ` ]; then echo "__poinhost_NOT_FOUND__"; exit 0; fi
tail -n ` + itoa(lines) + ` ` + shellQuote(path) + ` 2>/dev/null || true`

	res, err := s.run(access, script, 15*time.Second)
	if err != nil {
		return nil, err
	}
	if strings.Contains(res.Stdout, "__poinhost_NOT_FOUND__") {
		return &LogReadResponse{Domain: domain, LogType: req.LogType, Path: path, Exists: false}, nil
	}
	return &LogReadResponse{Domain: domain, LogType: req.LogType, Path: path, Content: res.Stdout, Exists: true}, nil
}

// StreamDomainLog mengalirkan log domain realtime (`tail -f`) lewat koneksi
// dedicated — konsisten dengan pola StreamContainerLogs di modul docker,
// tidak ikut mengantre di belakang operasi lain ke server yang sama.
func (s *Service) StreamDomainLog(ctx context.Context, req LogReadRequest, onLine func(line string) error) error {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return err
	}
	path, err := domainLogPath(domain, req.LogType)
	if err != nil {
		return err
	}
	if _, err := s.getDomain(req.ServerID, domain); err != nil {
		return err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}

	lines := clampLogLines(req.Lines)
	cmd := "tail -n " + itoa(lines) + " -f " + shellQuote(path) + " 2>/dev/null"
	return s.executor.ExecStreamDedicated(ctx, access.serverID, access.wrap(cmd), onLine)
}
