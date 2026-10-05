// Package backend menyambungkan bot.Backend ke modul poinhost yang sudah ada.
//
// Tidak ada logika baru di sini — semuanya delegasi ke servers.Service dan
// servers.Collector. Itu memang tujuannya: agent memakai ULANG jalur yang
// sama dengan app desktop, sehingga "status server" tidak pernah punya dua
// implementasi yang lama-lama berbeda jawabannya.
package backend

import (
	"context"

	"github.com/andyresta/poinhost/internal/modules/servers"
)

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

// ListServers mengembalikan server aktif, diurutkan seperti yang disimpan.
func (b *Backend) ListServers() ([]ServerInfo, error) {
	list, err := b.svc.List()
	if err != nil {
		return nil, err
	}
	out := make([]ServerInfo, 0, len(list))
	for _, s := range list {
		if !s.IsActive {
			continue
		}
		out = append(out, b.toInfo(s))
	}
	return out, nil
}

// Server mencari satu server berdasarkan ID.
func (b *Backend) Server(id string) (ServerInfo, bool) {
	s, err := b.svc.Get(id)
	if err != nil || s == nil {
		return ServerInfo{}, false
	}
	return b.toInfo(s), true
}

// Status membaca snapshot Collector — tanpa SSH round-trip sama sekali,
// karena Collector sudah menjadwalkan metrik untuk SEMUA server aktif
// (lihat collector.enqueueDue). Inilah yang membuat /servers dan /status
// membalas seketika.
func (b *Backend) Status(id string) (Status, bool) {
	for _, st := range b.collector.Snapshot() {
		if st.ID == id {
			return toStatus(st), true
		}
	}
	return Status{}, false
}

// Refresh memaksa pengambilan metrik baru untuk satu server (tombol Refresh).
func (b *Backend) Refresh(ctx context.Context, id string) (Status, error) {
	st, err := b.collector.RefreshNow(ctx, id)
	if err != nil {
		return Status{}, err
	}
	return toStatus(st), nil
}

func (b *Backend) toInfo(s *servers.Server) ServerInfo {
	return ServerInfo{
		ID:     s.ID,
		Name:   s.Name,
		Host:   s.Host,
		IsSelf: b.selfID != "" && s.ID == b.selfID,
	}
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
