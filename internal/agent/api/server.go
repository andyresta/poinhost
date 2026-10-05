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
//
// Bentuknya TUNGGAL, bukan daftar. Satu agent hanya mengurus mesinnya
// sendiri, jadi endpoint daftar server dan endpoint "status server ke-N"
// hanya akan mengembalikan satu baris yang sama selamanya — bentuk jamak
// yang menjanjikan sesuatu yang tidak pernah ada, dan membuat aplikasi di HP
// menulis loop untuk koleksi beranggota satu.
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

	Reader backend.Reader
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

	// /v1/server adalah SATU-SATUNYA endpoint data, dan inilah yang dipanggil
	// aplikasi mobile berulang kali. Identitas mesin, metriknya, dan versi
	// agent digabung dalam satu respons supaya HP cukup satu permintaan untuk
	// menggambar layarnya — di jaringan seluler, dua permintaan berarti dua
	// kali menunggu.
	//
	// Angkanya dibaca dari snapshot Collector, tanpa satu pun round-trip SSH,
	// jadi endpoint ini murah untuk di-poll.
	mux.Handle("GET /v1/server", d.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := d.Reader.Self()
		if !ok {
			// Agent terpasang tanpa kunci pengelolaan-diri, atau kuncinya
			// rusak. Dibedakan dari "metrik belum ada" karena perbaikannya
			// berbeda: yang ini ditekan dari panel Mobile Control.
			writeErr(w, http.StatusServiceUnavailable, "agent belum mengelola mesinnya sendiri")
			return
		}
		out := map[string]any{
			"version":       d.Version,
			"uptimeSeconds": int64(time.Since(d.StartedAt).Seconds()),
			"id":            info.ID,
			"name":          info.Name,
			"host":          info.Host,
		}
		// Metrik DIHILANGKAN kalau belum pernah terkumpul, bukan dikirim
		// sebagai nol — layar yang menampilkan CPU 0% dan disk 0 GB terlihat
		// seperti server sehat yang menganggur.
		if st, ok := d.Reader.Status(info.ID); ok {
			out["status"] = statusJSON(st)
		}
		writeJSON(w, http.StatusOK, out)
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
