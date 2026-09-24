// Package agentcfg memuat konfigurasi poinhost-agent dari satu file JSON.
//
// Formatnya JSON, bukan YAML, karena file ini DITULIS oleh app desktop lewat
// SFTP dan tidak dimaksudkan untuk diedit tangan — memakai encoding/json
// berarti agent tidak menambah dependensi apa pun, dan binary yang dikirim ke
// server tetap sekecil mungkin.
package agentcfg

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// DefaultListen adalah alamat default HTTP lokal agent.
//
// Port 7898 dan bind 127.0.0.1 — agent TIDAK PERNAH mendengarkan di alamat
// publik. Kalau REST API-nya perlu dijangkau dari luar, jalurnya adalah
// reverse proxy nginx + TLS certbot yang sudah dipunyai poinhost, sehingga
// terminasi TLS, rate limit, dan firewall ditangani komponen yang memang
// dibuat untuk itu — bukan oleh agent.
const DefaultListen = "127.0.0.1:7898"

// DefaultDataDir adalah direktori data agent di server.
const DefaultDataDir = "/var/lib/poinhost-agent"

// DefaultPath adalah lokasi baku file konfigurasi.
const DefaultPath = "/etc/poinhost-agent/config.json"

// Config adalah seluruh isi config.json.
type Config struct {
	// DataDir menampung SQLite, vault, dan known_hosts milik agent.
	DataDir string `json:"dataDir"`

	// Listen wajib loopback — lihat DefaultListen.
	Listen string `json:"listen"`

	// SelfServerID menandai entri server mana yang mewakili mesin tempat
	// agent ini berjalan. Agent tetap menghubunginya lewat SSH ke 127.0.0.1
	// seperti server lain — itu membuat SELURUH modul poinhost langsung
	// bekerja untuk server lokal tanpa satu pun perubahan — dan field ini
	// hanya dipakai untuk menandainya di UI/bot.
	SelfServerID string `json:"selfServerId,omitempty"`

	// SelfServer adalah cara agent mengelola mesin tempat ia berjalan:
	// sebagai server SSH biasa ke 127.0.0.1.
	SelfServer SelfServer `json:"selfServer"`

	// APIToken melindungi seluruh endpoint HTTP kecuali /healthz. Meski
	// sekarang hanya bisa dijangkau dari mesin itu sendiri, token ini ada
	// sejak awal supaya saat nanti endpoint-nya dipublikasikan lewat nginx
	// tidak ada momen "sempat terbuka tanpa autentikasi".
	APIToken string `json:"apiToken"`

	Telegram Telegram `json:"telegram"`
}

// SelfServer mendeskripsikan koneksi SSH agent ke mesinnya sendiri.
//
// Kenapa lewat SSH ke 127.0.0.1 dan bukan "executor lokal": seluruh modul
// poinhost (docker, website, firewall, services) menerima *sshpool.Executor
// yang konkret. Menambahkan jalur eksekusi lokal berarti mengekstrak interface
// dan menyentuh setiap modul — pekerjaan besar dan berisiko, di langkah paling
// awal. Dengan memperlakukan mesin sendiri sebagai server terdaftar biasa,
// SEMUA modul langsung bekerja tanpa satu baris pun berubah. Ongkosnya satu
// keypair khusus yang ditulis saat instalasi.
type SelfServer struct {
	Enabled  bool   `json:"enabled"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	KeyPath  string `json:"keyPath"`
	UseSudo  bool   `json:"useSudo"`
}

// Telegram menyimpan konfigurasi konektor bot.
type Telegram struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"`

	// AllowedUserIDs berisi user_id Telegram, BUKAN chat_id. Perbedaannya
	// penting: di grup, chat_id adalah grupnya, sehingga siapa pun yang
	// ditambahkan ke grup itu ikut bisa menjalankan perintah tanpa pernah
	// disetujui. Agent juga hanya melayani chat privat (lihat bot.go).
	AllowedUserIDs []int64 `json:"allowedUserIds"`
}

// Load membaca dan memvalidasi konfigurasi dari path.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("baca konfigurasi %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse konfigurasi %s: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("konfigurasi %s: %w", path, err)
	}
	return &cfg, nil
}

// Save menulis konfigurasi ke path dengan mode 0600, lewat file sementara
// supaya tidak pernah ada config setengah tertulis kalau proses mati.
//
// Field yang TIDAK dikenal struct ini dipertahankan apa adanya, bukan dibuang.
//
// Ini bukan kehati-hatian teoretis. Config ditulis app desktop tapi juga
// diperbarui agent (mis. allowlist bertambah setelah pairing), sementara versi
// desktop dan versi agent memang dirancang untuk bisa berbeda — itulah gunanya
// tombol Update. Agent lama yang menulis ulang seluruh berkas dari struct-nya
// akan menghapus setiap blok yang baru dikenal desktop, diam-diam, tanpa error.
// Persis itu yang pernah terjadi pada blok selfServer.
func Save(path string, cfg *Config) error {
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return err
	}

	fresh, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	var next map[string]any
	if err := json.Unmarshal(fresh, &next); err != nil {
		return err
	}

	if existing, rerr := os.ReadFile(path); rerr == nil {
		var prev map[string]any
		if json.Unmarshal(existing, &prev) == nil {
			next = mergeKeepingUnknown(prev, next)
		}
	}

	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// mergeKeepingUnknown mengembalikan next, ditambah setiap kunci dari prev yang
// tidak ada di next. Objek bersarang digabung rekursif; nilai skalar dan array
// selalu diambil dari next, karena itulah yang memang sedang diubah pemanggil.
func mergeKeepingUnknown(prev, next map[string]any) map[string]any {
	out := make(map[string]any, len(next)+len(prev))
	for k, v := range next {
		out[k] = v
	}
	for k, pv := range prev {
		nv, ada := out[k]
		if !ada {
			out[k] = pv
			continue
		}
		pm, pok := pv.(map[string]any)
		nm, nok := nv.(map[string]any)
		if pok && nok {
			out[k] = mergeKeepingUnknown(pm, nm)
		}
	}
	return out
}

func (c *Config) applyDefaults() {
	if strings.TrimSpace(c.DataDir) == "" {
		c.DataDir = DefaultDataDir
	}
	if strings.TrimSpace(c.Listen) == "" {
		c.Listen = DefaultListen
	}
	if c.SelfServer.Enabled {
		if strings.TrimSpace(c.SelfServer.Host) == "" {
			c.SelfServer.Host = "127.0.0.1"
		}
		if c.SelfServer.Port == 0 {
			c.SelfServer.Port = 22
		}
		if strings.TrimSpace(c.SelfServer.Name) == "" {
			c.SelfServer.Name = "This server"
		}
	}
}

// Validate memeriksa hal-hal yang membuat agent tidak boleh start.
func (c *Config) Validate() error {
	if err := ValidateListen(c.Listen); err != nil {
		return err
	}
	if c.Telegram.Enabled && strings.TrimSpace(c.Telegram.Token) == "" {
		return errors.New("telegram diaktifkan tapi token kosong")
	}
	return nil
}

// ErrListenNotLoopback dikembalikan kalau alamat listen bukan loopback.
var ErrListenNotLoopback = errors.New("alamat listen agent harus loopback")

// ValidateListen menolak alamat non-loopback.
//
// Ini ditegakkan di KODE, bukan cuma ditulis di dokumen. Agent memegang akses
// setara root ke server tempat ia berjalan sekaligus kredensial SSH ke semua
// server terdaftar; satu salah ketik "0.0.0.0" di config akan mengekspos
// seluruhnya ke internet. Menolak start jauh lebih baik daripada berjalan
// dalam keadaan itu tanpa ada yang menyadari.
func ValidateListen(addr string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return fmt.Errorf("%w: alamat %q tidak berbentuk host:port", ErrListenNotLoopback, addr)
	}
	if port == "" {
		return fmt.Errorf("%w: port kosong pada %q", ErrListenNotLoopback, addr)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("%w: %q bukan alamat IP", ErrListenNotLoopback, host)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("%w: %q bisa dijangkau dari jaringan — pakai 127.0.0.1 dan publikasikan lewat reverse proxy kalau perlu diakses dari luar", ErrListenNotLoopback, host)
	}
	return nil
}

// IsAllowed melaporkan apakah user Telegram boleh memakai bot.
func (t Telegram) IsAllowed(userID int64) bool {
	for _, id := range t.AllowedUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}
