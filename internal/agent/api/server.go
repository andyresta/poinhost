// Package api menyajikan HTTP lokal poinhost-agent.
//
// Seluruhnya di loopback (lihat agentcfg.ValidateListen). Ini jalur yang
// dipakai app desktop lewat SSH — `curl` di server target — untuk memeriksa
// kesehatan agent, membuat kode pairing, dan mengelola allowlist tanpa perlu
// mengedit config.json dengan tangan.
//
// Kelak endpoint yang sama menjadi dasar REST API untuk aplikasi mobile,
// dipublikasikan lewat nginx + certbot; karena itu autentikasi bearer token
// sudah dipasang SEKARANG, bukan nanti — supaya tidak pernah ada versi yang
// sempat terbuka tanpa autentikasi.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/agent/backend"
	"github.com/andyresta/poinhost/internal/agent/bot"
	"github.com/andyresta/poinhost/internal/agent/transfer"
)

// Deps adalah yang dibutuhkan handler.
type Deps struct {
	Version   string
	StartedAt time.Time
	APIToken  string

	// DataDir membatasi berkas mana yang boleh dibaca endpoint impor.
	DataDir string

	Pairing   Pairing
	Allowlist Allowlist
	Telegram  TelegramState
	Backend   bot.Backend
	Servers   ServerStore
}

// ServerStore adalah penautan server dari app desktop.
type ServerStore interface {
	List() ([]backend.Entry, error)
	Import(b *transfer.Bundle) (int, error)
	Remove(id string) error
}

// Pairing adalah bagian bot.Pairing yang dipakai API.
type Pairing interface {
	Issue() (string, time.Time, error)
	Pending() int
}

// Allowlist adalah bagian allowlist yang dipakai API.
type Allowlist interface {
	IDs() []int64
	Revoke(userID int64) error
	Count() int
}

// TelegramState melaporkan kondisi konektor Telegram.
type TelegramState interface {
	Enabled() bool
	BotUsername() string
	LastError() string
}

// Handler membangun mux HTTP agent.
func Handler(d Deps) http.Handler {
	mux := http.NewServeMux()

	// /healthz sengaja TANPA autentikasi: inilah yang dipanggil app desktop
	// untuk membedakan "terpasang tapi mati" dari "jalan", dan nilainya justru
	// pada kesederhanaannya. Isinya tidak memuat apa pun yang rahasia.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":         "ok",
			"version":        d.Version,
			"uptimeSeconds":  int64(time.Since(d.StartedAt).Seconds()),
			"telegram":       d.Telegram.Enabled(),
			"telegramBot":    d.Telegram.BotUsername(),
			"allowedUsers":   d.Allowlist.Count(),
			"pendingPairing": d.Pairing.Pending(),
		})
	})

	mux.Handle("GET /v1/status", d.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		servers, err := d.Backend.ListServers()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"version":       d.Version,
			"uptimeSeconds": int64(time.Since(d.StartedAt).Seconds()),
			"serverCount":   len(servers),
			"telegram": map[string]any{
				"enabled":      d.Telegram.Enabled(),
				"botUsername":  d.Telegram.BotUsername(),
				"lastError":    d.Telegram.LastError(),
				"allowedUsers": d.Allowlist.IDs(),
			},
		})
	})))

	mux.Handle("POST /v1/pairing", d.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !d.Telegram.Enabled() {
			writeErr(w, http.StatusConflict, "konektor telegram belum aktif")
			return
		}
		code, exp, err := d.Pairing.Issue()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"code":      bot.FormatCode(code),
			"expiresAt": exp.UTC().Format(time.RFC3339),
			"ttlSecond": int(time.Until(exp).Seconds()),
		})
	})))

	mux.Handle("GET /v1/servers", d.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		list, err := d.Servers.List()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if list == nil {
			list = []backend.Entry{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"servers": list})
	})))

	// Paket server dikirim lewat SFTP sebagai berkas terenkripsi, dan hanya
	// passphrase-nya yang lewat endpoint ini. Kredensial karena itu tidak
	// pernah ada dalam bentuk terbuka di disk server, dan tidak pernah jadi
	// bagian dari command line yang terlihat di `ps`.
	mux.Handle("POST /v1/servers/import", d.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path       string `json:"path"`
			Passphrase string `json:"passphrase"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req) != nil {
			writeErr(w, http.StatusBadRequest, "permintaan tidak bisa dibaca")
			return
		}
		if req.Path == "" || req.Passphrase == "" {
			writeErr(w, http.StatusBadRequest, "path dan passphrase wajib diisi")
			return
		}
		// Tanpa batas ini, endpoint yang seharusnya mengimpor server berubah
		// jadi alat membaca berkas mana pun di server — cukup arahkan ke berkas
		// lain dan baca pesan errornya. Impor hanya boleh menyentuh berkas di
		// dalam direktori data agent.
		if !withinDir(d.DataDir, req.Path) {
			writeErr(w, http.StatusBadRequest, "paket server harus berada di direktori data agent")
			return
		}

		data, err := os.ReadFile(req.Path)
		// Berkas dihapus apa pun hasilnya: ia memuat kredensial terenkripsi dan
		// tidak ada alasan membiarkannya tertinggal di /tmp.
		defer func() { _ = os.Remove(req.Path) }()
		if err != nil {
			writeErr(w, http.StatusBadRequest, "paket server tidak ditemukan")
			return
		}

		bundle, err := transfer.Open(data, req.Passphrase)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		n, err := d.Servers.Import(bundle)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"imported": n})
	})))

	mux.Handle("DELETE /v1/servers/{id}", d.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := d.Servers.Remove(r.PathValue("id")); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"removed": true})
	})))

	mux.Handle("DELETE /v1/allowed/{id}", d.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "user id tidak valid")
			return
		}
		if err := d.Allowlist.Revoke(id); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"allowedUsers": d.Allowlist.IDs()})
	})))

	return mux
}

// auth menuntut bearer token pada semua endpoint selain /healthz.
func (d Deps) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.APIToken == "" {
			// Tanpa token, endpoint ini ditutup sepenuhnya alih-alih dibuka
			// bebas — kegagalan konfigurasi tidak boleh berubah jadi akses
			// tanpa autentikasi.
			writeErr(w, http.StatusServiceUnavailable, "apiToken belum diset di config agent")
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		// Perbandingan waktu-tetap: token ini kelak dipakai lewat jaringan.
		if subtle.ConstantTimeCompare([]byte(got), []byte(d.APIToken)) != 1 {
			writeErr(w, http.StatusUnauthorized, "token tidak cocok")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withinDir melaporkan apakah path berada di dalam dir, setelah kedua sisi
// dibersihkan — supaya "..\/..\/etc/shadow" tidak lolos.
func withinDir(dir, path string) bool {
	if dir == "" {
		return false
	}
	absDir, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
