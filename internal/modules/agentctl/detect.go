package agentctl

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/agent/agentcfg"
	"github.com/andyresta/poinhost/internal/agent/dist"
)

// Tiga kemungkinan keadaan agent di satu server. Sengaja tiga, bukan dua:
// "terpasang tapi mati" adalah keadaan yang paling sering bikin bingung, dan
// menyembunyikannya di balik "belum terpasang" membuat user memasang ulang di
// atas instalasi yang sudah ada.
const (
	StateNotInstalled = "not-installed"
	StateStopped      = "stopped"
	StateRunning      = "running"
)

// Status adalah laporan lengkap Mobile Control untuk satu server.
type Status struct {
	State string `json:"state"`

	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
	Active    bool   `json:"active"`
	Enabled   bool   `json:"enabled"`
	Healthy   bool   `json:"healthy"`

	Listen string `json:"listen,omitempty"`
	// SelfManaged: agent punya entri server untuk mesinnya sendiri.
	SelfManaged   bool  `json:"selfManaged"`
	UptimeSeconds int64 `json:"uptimeSeconds,omitempty"`
	ServerCount   int   `json:"serverCount,omitempty"`

	// APIConfigured: config agent sudah punya apiToken, jadi API Mobile
	// Control bisa dipakai. Tanpa token, seluruh endpoint selain /healthz
	// menolak — kegagalan konfigurasi tidak pernah berubah jadi akses bebas.
	APIConfigured bool `json:"apiConfigured"`

	// Arch dan dukungannya menentukan apakah tombol Pasang boleh aktif.
	Arch          string `json:"arch,omitempty"`
	ArchSupported bool   `json:"archSupported"`
	HasSystemd    bool   `json:"hasSystemd"`
	CanElevate    bool   `json:"canElevate"`

	// BundledVersion adalah versi agent yang dibawa build desktop ini.
	BundledVersion  string `json:"bundledVersion,omitempty"`
	BundleAvailable bool   `json:"bundleAvailable"`

	// UpdateAvailable hanya true kalau versi yang dibundel terbukti lebih baru
	// dari yang terpasang — bukan sekadar berbeda. Perbedaan saja akan ikut
	// menawarkan PENURUNAN versi sebagai peningkatan saat app desktop yang
	// dipakai lebih tua dari agent di server.
	UpdateAvailable bool `json:"updateAvailable"`

	// Problem menerangkan kenapa aksi tertentu tidak bisa dilakukan, dalam
	// bahasa yang bisa ditindaklanjuti user.
	Problem string `json:"problem,omitempty"`
}


// detectScript mengumpulkan SEMUA fakta dalam satu round-trip SSH: keadaan
// yang dilaporkan harus menggambarkan satu momen, bukan lima momen berbeda.
const detectScript = `
bin=/usr/local/bin/poinhost-agent
echo "arch=$(uname -m 2>/dev/null)"
if command -v systemctl >/dev/null 2>&1; then echo "systemd=1"; else echo "systemd=0"; fi
if [ -x "$bin" ]; then
  echo "installed=1"
  echo "version=$("$bin" --version 2>/dev/null | head -n1)"
else
  echo "installed=0"
fi
echo "active=$(systemctl is-active poinhost-agent 2>/dev/null)"
echo "enabled=$(systemctl is-enabled poinhost-agent 2>/dev/null)"
if [ -r /etc/poinhost-agent/config.json ]; then
  echo "config_b64=$(base64 -w0 < /etc/poinhost-agent/config.json 2>/dev/null || base64 < /etc/poinhost-agent/config.json | tr -d '\n')"
fi
`

// Detect memeriksa keadaan agent di satu server.
func (s *Service) Detect(ctx context.Context, serverID string) (*Status, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	st := &Status{
		State:           StateNotInstalled,
		CanElevate:      access.isRoot(),
		BundledVersion:  dist.Version(),
		BundleAvailable: dist.Bundled(),
	}

	res, err := s.run(ctx, access, detectScript, 25*time.Second)
	if err != nil {
		return nil, err
	}
	kv := parseKV(res.Stdout)

	st.Arch = kv["arch"]
	if _, aerr := dist.ArchFromUname(st.Arch); aerr == nil {
		st.ArchSupported = true
	}
	st.HasSystemd = kv["systemd"] == "1"
	st.Installed = kv["installed"] == "1"
	st.Version = kv["version"]
	st.Active = kv["active"] == "active"
	st.Enabled = kv["enabled"] == "enabled"

	acfg := decodeConfig(kv["config_b64"])
	if acfg != nil {
		st.Listen = acfg.Listen
		st.SelfManaged = acfg.SelfServer.Enabled
		st.APIConfigured = strings.TrimSpace(acfg.APIToken) != ""
	}

	switch {
	case !st.Installed:
		st.State = StateNotInstalled
	case st.Active:
		st.State = StateRunning
	default:
		st.State = StateStopped
	}

	// Health check dijalankan DI SERVER lewat SSH, bukan lewat tunnel:
	// portnya hanya bind ke 127.0.0.1, jadi satu-satunya yang bisa
	// menjangkaunya adalah proses di mesin itu sendiri.
	if st.State == StateRunning && acfg != nil {
		if h := s.health(ctx, access, acfg.Listen); h != nil {
			st.Healthy = true
			st.UptimeSeconds = h.UptimeSeconds
		}
	}

	st.UpdateAvailable = st.Installed && st.BundleAvailable &&
		decideVersionAction(st.Version, st.BundledVersion) == versionUpdate

	st.Problem = describeProblem(st)
	return st, nil
}

type healthResponse struct {
	// Agent Mobile Control menjawab {"ok":true}. Field status dipertahankan
	// karena agent versi lama — yang masih terpasang di server sampai
	// seseorang menekan Update — menjawab {"status":"ok"}, dan app desktop
	// yang baru harus tetap bisa membedakan agent lama yang SEHAT dari agent
	// yang benar-benar mati.
	OK            bool   `json:"ok"`
	Status        string `json:"status"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptimeSeconds"`
}

func (s *Service) health(ctx context.Context, access *agentAccess, listen string) *healthResponse {
	port := portOf(listen)
	script := "curl -fsS --max-time 4 http://127.0.0.1:" + strconv.Itoa(port) + "/healthz 2>/dev/null || true"
	res, err := s.run(ctx, access, script, 15*time.Second)
	if err != nil || res == nil {
		return nil
	}
	return parseHealth(res.Stdout)
}

// parseHealth memutuskan apakah jawaban /healthz berarti agent SEHAT.
func parseHealth(raw string) *healthResponse {
	var h healthResponse
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &h) != nil {
		return nil
	}
	if !h.OK && h.Status != "ok" {
		return nil
	}
	return &h
}

func decodeConfig(b64 string) *agentcfg.Config {
	if b64 == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil
	}
	var cfg agentcfg.Config
	if json.Unmarshal(raw, &cfg) != nil {
		return nil
	}
	// Config lengkap dikembalikan, token API dan semuanya — tapi Status yang
	// dibentuk Detect hanya membawa APIConfigured. Tokennya baru menyeberang
	// ke UI kalau user memintanya lewat APIToken(), supaya ia tidak ikut
	// terbawa di setiap refresh panel.
	return &cfg
}

func portOf(listen string) int {
	if _, p, ok := strings.Cut(listen, ":"); ok {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			return n
		}
	}
	return DefaultPort
}

func describeProblem(st *Status) string {
	switch {
	case !st.HasSystemd:
		return "Server ini tidak memakai systemd, sementara agent dipasang sebagai unit systemd."
	case st.Arch != "" && !st.ArchSupported:
		return "Arsitektur " + st.Arch + " belum didukung — agent tersedia untuk x86_64 dan aarch64."
	case !st.CanElevate:
		return "Butuh akses root: login sebagai root, atau aktifkan sudo untuk server ini di pengaturan server."
	case !st.BundleAvailable && !st.Installed:
		return "Build poinhost ini tidak membawa binary agent, jadi pemasangan tidak tersedia."
	}
	return ""
}

// CanInstall melaporkan apakah pemasangan mungkin dilakukan sekarang.
func (st *Status) CanInstall() bool {
	return st.HasSystemd && st.ArchSupported && st.CanElevate && st.BundleAvailable
}
