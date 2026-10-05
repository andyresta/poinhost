// Package api menyajikan HTTP poinhost-agent untuk Mobile Control.
//
// Agent tetap bind di loopback saja (lihat agentcfg.ValidateListen). Yang
// menjangkaunya dari luar adalah reverse proxy nginx + TLS certbot di server
// yang sama — komponen yang memang dibuat untuk terminasi TLS, rate limit,
// dan firewall, dan yang sudah dikelola poinhost. Agent tidak perlu tahu apa
// pun soal itu.
//
// Seluruh endpoint data di sini BACA saja. Tidak ada satu pun yang mengubah
// server, sesuai tahap pertama Mobile Control.
//
// Bentuknya TUNGGAL, bukan daftar. Satu agent hanya mengurus mesinnya
// sendiri, jadi endpoint daftar server hanya akan mengembalikan satu baris
// yang sama selamanya — bentuk jamak yang menjanjikan sesuatu yang tidak
// pernah ada, dan membuat aplikasi di HP menulis loop untuk koleksi
// beranggota satu.
//
// Pendaftaran dan persetujuan ada di dua permukaan BERBEDA. Yang ini, yang
// dipublikasikan ke internet, hanya bisa mendaftar dan mengantre. Yang
// menyetujui ada di socket admin yang cuma bisa disentuh root di mesin itu
// (lihat admin.go), jadi tidak ada urutan permintaan dari internet yang bisa
// menyetujui dirinya sendiri.
package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"crypto/subtle"

	"github.com/andyresta/poinhost/internal/agent/backend"
	"github.com/andyresta/poinhost/internal/agent/devices"
)

// Deps adalah yang dibutuhkan handler.
type Deps struct {
	Version   string
	StartedAt time.Time

	// EnrollToken menjaga pendaftaran perangkat. Ini BUKAN kredensial akses:
	// yang dibuka dengannya hanya antrean persetujuan.
	EnrollToken string

	Reader  backend.Reader
	Devices DeviceStore
}

// DeviceStore adalah bagian devices.Store yang disentuh permukaan publik.
//
// Sempit dengan sengaja: dari sini perangkat hanya bisa DIBUAT dan DIPERIKSA.
// Menyetujui tidak ada di interface ini sama sekali, jadi tidak mungkin
// seseorang menambahkan handler publik yang menyetujui tanpa lebih dulu
// melebarkan kontraknya di sini.
type DeviceStore interface {
	Register(name, platform string) (devices.Device, string, error)
	Authenticate(token string) (devices.Device, bool)
}

// Handler membangun mux HTTP publik agent.
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

	// POST /v1/devices menukar token pendaftaran dengan token milik satu
	// perangkat. Perangkatnya langsung berstatus menunggu — token yang
	// dikembalikan belum membuka apa pun.
	//
	// Tokennya dikembalikan SEKARANG, bukan nanti setelah disetujui, supaya
	// HP tidak perlu menyimpan token pendaftaran dan menukarnya lagi
	// belakangan. HP cukup menyimpan satu token lalu memanggil /v1/server
	// seperti biasa: selama masih 403, tampilkan layar menunggu. Tidak ada
	// handshake yang perlu ditulis di sisi HP.
	mux.HandleFunc("POST /v1/devices", func(w http.ResponseWriter, r *http.Request) {
		if d.EnrollToken == "" {
			writeErr(w, http.StatusServiceUnavailable, "pendaftaran belum dibuka — buat token di app poinhost")
			return
		}
		if !tokenCocok(r, d.EnrollToken) {
			writeErr(w, http.StatusUnauthorized, "token pendaftaran tidak cocok")
			return
		}
		if d.Devices == nil {
			writeErr(w, http.StatusServiceUnavailable, "daftar perangkat tidak tersedia")
			return
		}

		var body struct {
			Name     string `json:"name"`
			Platform string `json:"platform"`
		}
		// Body yang tidak terbaca BUKAN alasan menolak: namanya hanya label
		// supaya pemilik mengenali HP-nya di daftar, dan menolak pendaftaran
		// karena label tidak terkirim cuma membuat HP mentok tanpa jalan
		// keluar yang bisa dipahami.
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body)

		dev, token, err := d.Devices.Register(body.Name, body.Platform)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"deviceId": dev.ID,
			"name":     dev.Name,
			"token":    token,
			"status":   dev.Status,
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
	mux.Handle("GET /v1/server", d.deviceAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// deviceAuth menuntut token milik perangkat yang SUDAH disetujui.
//
// Tiga jawaban yang sengaja dibedakan, karena ketiganya menuntut tindakan
// berbeda dari pemilik HP: 401 berarti daftar ulang, 403 menunggu berarti
// buka app desktop dan setujui, 403 dicabut berarti aksesnya memang diambil.
// Satu kode untuk ketiganya akan membuat layar HP cuma bisa bilang "gagal".
func (d Deps) deviceAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Devices == nil {
			writeErr(w, http.StatusServiceUnavailable, "daftar perangkat tidak tersedia")
			return
		}
		dev, ok := d.Devices.Authenticate(bearer(r))
		if !ok {
			writeErr(w, http.StatusUnauthorized, "token perangkat tidak dikenal")
			return
		}
		switch dev.Status {
		case devices.StatusApproved:
			next.ServeHTTP(w, r)
		case devices.StatusPending:
			writeStatusErr(w, http.StatusForbidden, "menunggu persetujuan di app poinhost", dev.Status)
		default:
			writeStatusErr(w, http.StatusForbidden, "akses perangkat ini sudah dicabut", dev.Status)
		}
	})
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

func bearer(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

// tokenCocok membandingkan dalam waktu tetap: token ini dipakai lewat jaringan.
func tokenCocok(r *http.Request, want string) bool {
	return subtle.ConstantTimeCompare([]byte(bearer(r)), []byte(want)) == 1
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeStatusErr(w http.ResponseWriter, code int, msg, status string) {
	writeJSON(w, code, map[string]string{"error": msg, "status": status})
}
