// Package backend menyambungkan API agent ke modul poinhost yang sudah ada.
//
// Tidak ada logika baru di sini — semuanya delegasi ke servers.Service dan
// servers.Collector. Itu memang tujuannya: agent memakai ULANG jalur yang
// sama dengan app desktop, sehingga "status server" tidak pernah punya dua
// implementasi yang lama-lama berbeda jawabannya.
package backend

import "github.com/andyresta/poinhost/internal/modules/servers"

// Backend mengimplementasikan Reader.
type Backend struct {
	svc       *servers.Service
	collector *servers.Collector

	// selfID adalah server yang mewakili mesin tempat agent berjalan, supaya
	// bisa ditandai di daftar. Kosong berarti agent belum mendaftarkan
	// dirinya sendiri.
	selfID string
}

// New membuat backend.
func New(svc *servers.Service, collector *servers.Collector, selfServerID string) *Backend {
	return &Backend{svc: svc, collector: collector, selfID: selfServerID}
}

// Self mengembalikan mesin tempat agent berjalan.
//
// Hanya entri itu, tidak pernah yang lain: server yang dulu ditautkan ke
// agent versi lama tetap ada di database-nya sampai agent dicopot, dan
// melaporkannya lewat API berarti satu agent kembali bicara atas nama mesin
// orang lain — persis yang dibuang dari fitur ini.
func (b *Backend) Self() (ServerInfo, bool) {
	if b.selfID == "" {
		return ServerInfo{}, false
	}
	s, err := b.svc.Get(b.selfID)
	if err != nil || s == nil || !s.IsActive {
		return ServerInfo{}, false
	}
	return b.toInfo(s), true
}

// Status membaca snapshot Collector — tanpa SSH round-trip sama sekali,
// karena Collector sudah menjadwalkan metrik untuk server aktif (lihat
// collector.enqueueDue). Inilah yang membuat /v1/server membalas seketika.
func (b *Backend) Status(id string) (Status, bool) {
	for _, st := range b.collector.Snapshot() {
		if st.ID == id {
			return toStatus(st), true
		}
	}
	return Status{}, false
}

func (b *Backend) toInfo(s *servers.Server) ServerInfo {
	return ServerInfo{ID: s.ID, Name: s.Name, Host: s.Host}
}

func toStatus(st servers.ServerStatus) Status {
	return Status{
		Connection:    st.Connection,
		OSName:        st.OSName,
		Hostname:      st.Hostname,
		UptimeSeconds: st.UptimeSeconds,
		CPUCores:      st.CPUCores,
		CPUUsedPct:    st.CPUUsedPct,
		MemUsedMB:     st.MemUsedMB,
		MemTotalMB:    st.MemTotalMB,
		DiskUsedGB:    st.DiskUsedGB,
		DiskTotalGB:   st.DiskTotalGB,
		CheckedAt:     st.MetricsCheckedAt,
		Stale:         st.MetricsStale,
		Error:         st.MetricsError,
	}
}
