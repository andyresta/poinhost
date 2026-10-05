// Package api menyajikan HTTP poinhost-agent untuk Mobile Control.
//
// Agent tetap bind di loopback saja (lihat agentcfg.ValidateListen). Yang
// menjangkaunya dari luar adalah reverse proxy nginx + TLS certbot di server
// yang sama — komponen yang memang dibuat untuk terminasi TLS, rate limit,
// dan firewall, dan yang sudah dikelola poinhost. Agent tidak perlu tahu apa
// pun soal itu.
//
// Seluruh endpoint di sini BACA saja. Tidak ada satu pun yang mengubah
// server, sesuai tahap pertama Mobile Control — aksi baru dipertimbangkan
// setelah model autentikasinya terbukti di pemakaian nyata.
package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"crypto/subtle"

	"github.com/andyresta/poinhost/internal/agent/backend"
)

// Deps adalah yang dibutuhkan handler.
type Deps struct {
	Version   string
	StartedAt time.Time
	APIToken  string

	Reader  backend.Reader
	Servers ServerStore
}

// ServerStore adalah daftar server yang dikenal agent.
//
// Sejak satu agent hanya mengurus mesinnya sendiri, isinya praktis satu
// entri — tapi tetap berbentuk daftar supaya bentuk responsnya tidak perlu
// berubah kalau suatu saat ada lebih dari satu.
type ServerStore interface {
	List() ([]backend.Entry, error)
}

// Handler membangun mux HTTP agent.
func Handler(d Deps) http.Handler {
	mux := http.NewServeMux()

	// /healthz sengaja TANPA autentikasi: inilah yang dipanggil app desktop
	// untuk membedakan "terpasang tapi mati" dari "jalan", dan nilainya
	// justru pada kesederhanaannya. Isinya tidak memuat apa pun yang
	// rahasia.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":            true,
			"version":       d.Version,
			"uptimeSeconds": int64(time.Since(d.StartedAt).Seconds()),
		})
	})

	mux.Handle("GET /v1/status", d.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		list, err := d.Reader.ListServers()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"version":       d.Version,
			"uptimeSeconds": int64(time.Since(d.StartedAt).Seconds()),
			"serverCount":   len(list),
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

	// /v1/overview adalah endpoint yang dipanggil aplikasi mobile berulang
	// kali. Server DAN metriknya digabung dalam satu respons supaya HP cukup
	// satu permintaan untuk menggambar layarnya — bukan satu panggilan untuk
	// daftar lalu satu lagi per server, yang di jaringan seluler berarti
	// menunggu berkali-kali.
	//
	// Angkanya dibaca dari snapshot Collector, tanpa satu pun round-trip SSH,
	// jadi endpoint ini murah untuk di-poll.
	mux.Handle("GET /v1/overview", d.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		list, err := d.Reader.ListServers()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]map[string]any, 0, len(list))
		for _, s := range list {
			entry := map[string]any{
				"id":     s.ID,
				"name":   s.Name,
				"host":   s.Host,
				"isSelf": s.IsSelf,
			}
			if st, ok := d.Reader.Status(s.ID); ok {
				entry["status"] = statusJSON(st)
			}
			out = append(out, entry)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"version":       d.Version,
			"uptimeSeconds": int64(time.Since(d.StartedAt).Seconds()),
			"servers":       out,
		})
	})))

	mux.Handle("GET /v1/servers/{id}/status", d.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, ok := d.Reader.Server(id); !ok {
			writeErr(w, http.StatusNotFound, "server tidak ditemukan")
			return
		}
		st, ok := d.Reader.Status(id)
		if !ok {
			writeErr(w, http.StatusServiceUnavailable, "metrik belum tersedia")
			return
		}
		writeJSON(w, http.StatusOK, statusJSON(st))
	})))

	return mux
}

// statusJSON memetakan status internal ke bentuk respons.
//
// Dipetakan eksplisit, bukan dengan tag JSON di tipe internalnya, supaya
// mengganti nama field internal tidak diam-diam mengubah kontrak yang sudah
// dipakai aplikasi mobile yang terpasang di HP orang.
func statusJSON(st backend.Status) map[string]any {
	out := map[string]any{
		"connection":    st.Connection,
		"osName":        st.OSName,
		"hostname":      st.Hostname,
		"uptimeSeconds": st.UptimeSeconds,
		"cpuCores":      st.CPUCores,
		"cpuUsedPct":    st.CPUUsedPct,
		"memUsedMB":     st.MemUsedMB,
		"memTotalMB":    st.MemTotalMB,
		"diskUsedGB":    st.DiskUsedGB,
		"diskTotalGB":   st.DiskTotalGB,
		"stale":         st.Stale,
	}
	if !st.CheckedAt.IsZero() {
		out["checkedAt"] = st.CheckedAt.UTC().Format(time.RFC3339)
	}
	if st.Error != "" {
		out["error"] = st.Error
	}
	return out
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
		// Perbandingan waktu-tetap: token ini dipakai lewat jaringan.
		if subtle.ConstantTimeCompare([]byte(got), []byte(d.APIToken)) != 1 {
			writeErr(w, http.StatusUnauthorized, "token tidak cocok")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
