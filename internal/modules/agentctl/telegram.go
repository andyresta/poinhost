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

// TelegramRequest adalah isi form Bot Telegram di Agent Control.
type TelegramRequest struct {
	ServerID string `json:"serverId"`
	Enabled  bool   `json:"enabled"`

	// Token kosong berarti "jangan ubah" — layar tidak pernah menampilkan
	// token yang tersimpan, jadi kosong di sini berarti belum diganti, bukan
	// perintah menghapus. Pola yang sama dipakai form password server.
	Token string `json:"token"`
}

// TelegramTestResult adalah hasil verifikasi token.
type TelegramTestResult struct {
	OK          bool   `json:"ok"`
	BotUsername string `json:"botUsername,omitempty"`
	Message     string `json:"message,omitempty"`
}

// ConfigureTelegram menulis ulang blok telegram di config agent lalu me-restart
// service supaya konfigurasinya terpakai.
func (s *Service) ConfigureTelegram(ctx context.Context, req TelegramRequest) (*Status, error) {
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return nil, err
	}

	acfg, err := s.readConfig(ctx, access)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.Token) != "" {
		acfg.Telegram.Token = strings.TrimSpace(req.Token)
	}
	acfg.Telegram.Enabled = req.Enabled
	if acfg.Telegram.AllowedUserIDs == nil {
		acfg.Telegram.AllowedUserIDs = []int64{}
	}
	if acfg.Telegram.Enabled && strings.TrimSpace(acfg.Telegram.Token) == "" {
		return nil, fmt.Errorf("token bot belum diisi")
	}

	if err := s.writeConfig(ctx, req.ServerID, access, acfg); err != nil {
		return nil, err
	}
	if _, err := s.run(ctx, access, "systemctl restart "+UnitName, 45*time.Second); err != nil {
		return nil, err
	}
	return s.waitHealthy(ctx, req.ServerID, 60)
}

// TestTelegramToken memverifikasi token DARI SERVER, bukan dari mesin desktop.
//
// Itu penting: pemeriksaan ini sekaligus membuktikan server bisa menjangkau
// api.telegram.org. Kalau diverifikasi dari laptop, yang terbukti cuma laptop
// punya internet — sementara yang nanti melakukan long-poll adalah servernya.
func (s *Service) TestTelegramToken(ctx context.Context, serverID, token string) (*TelegramTestResult, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	token = strings.TrimSpace(token)
	if token == "" {
		acfg, cerr := s.readConfig(ctx, access)
		if cerr != nil {
			return nil, cerr
		}
		token = strings.TrimSpace(acfg.Telegram.Token)
	}
	if token == "" {
		return &TelegramTestResult{Message: "token bot belum diisi"}, nil
	}

	// Token dikirim lewat variabel yang diisi dari base64, bukan ditulis
	// langsung di command line — apa pun yang ada di command line terlihat di
	// `ps` selama perintah berjalan.
	script := "t=$(echo " + shellQuote(base64.StdEncoding.EncodeToString([]byte(token))) + " | base64 -d)\n" +
		`curl -fsS --max-time 10 "https://api.telegram.org/bot$t/getMe" 2>/dev/null || true` + "\n"
	res, err := s.run(ctx, access, script, 30*time.Second)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
		Description string `json:"description"`
	}
	body := strings.TrimSpace(res.Stdout)
	if body == "" {
		return &TelegramTestResult{Message: "server tidak bisa menghubungi api.telegram.org — cek koneksi keluar atau firewall server"}, nil
	}
	if json.Unmarshal([]byte(body), &parsed) != nil {
		return &TelegramTestResult{Message: "balasan Telegram tidak dikenali"}, nil
	}
	if !parsed.OK {
		msg := parsed.Description
		if msg == "" {
			msg = "token ditolak Telegram"
		}
		return &TelegramTestResult{Message: msg}, nil
	}
	return &TelegramTestResult{OK: true, BotUsername: "@" + parsed.Result.Username}, nil
}

// PairingCode adalah kode sekali-pakai untuk menambahkan user Telegram.
type PairingCode struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
	TTLSecond int    `json:"ttlSecond"`
}

// CreatePairingCode meminta agent membuat kode pairing.
//
// Kode dipakai supaya user tidak perlu mencari tahu user ID Telegram-nya
// sendiri lewat bot pihak ketiga — dan yang masuk allowlist dijamin benar-benar
// pemegang akun itu, bukan angka yang salah ketik.
func (s *Service) CreatePairingCode(ctx context.Context, serverID string) (*PairingCode, error) {
	out, err := s.callAgent(ctx, serverID, "POST", "/v1/pairing")
	if err != nil {
		return nil, err
	}
	var pc PairingCode
	if json.Unmarshal([]byte(out), &pc) != nil || pc.Code == "" {
		return nil, fmt.Errorf("agent tidak mengembalikan kode pairing: %s", trimForError(out))
	}
	return &pc, nil
}

// RevokeTelegramUser menghapus satu user dari allowlist agent.
func (s *Service) RevokeTelegramUser(ctx context.Context, serverID string, userID int64) (*Status, error) {
	if _, err := s.callAgent(ctx, serverID, "DELETE", "/v1/allowed/"+strconv.FormatInt(userID, 10)); err != nil {
		return nil, err
	}
	return s.Detect(ctx, serverID)
}

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
