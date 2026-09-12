package servers

import (
	"context"
	"time"
)

// ServerStatus adalah snapshot kesehatan satu server untuk UI.
//
// Field `Connection` SELALU tersedia (dibaca dari sshpool.Pool, gratis, tidak
// pernah gagal/timeout) — status "online/offline/reconnecting" ini tidak
// bergantung pada berhasil-tidaknya pengambilan metrik. Field metrik
// (OS/CPU/RAM/disk/uptime) HANYA terisi kalau percobaan metrik terakhir
// berhasil; kalau gagal, field ini tetap menyimpan nilai TERAKHIR yang
// pernah berhasil didapat (lihat Collector) supaya UI tidak "kosong"
// hanya karena satu tick metrik kebetulan gagal/timeout.
type ServerStatus struct {
	ID         string `json:"id"`
	Connection string `json:"connection"` // online | offline | reconnecting

	OSName        string  `json:"osName,omitempty"`
	Hostname      string  `json:"hostname,omitempty"`
	UptimeSeconds int64   `json:"uptimeSeconds,omitempty"`
	CPUCores      int     `json:"cpuCores,omitempty"`
	CPUUsedPct    float64 `json:"cpuUsedPct,omitempty"`
	MemUsedMB     int64   `json:"memUsedMb,omitempty"`
	MemTotalMB    int64   `json:"memTotalMb,omitempty"`
	DiskUsedGB    float64 `json:"diskUsedGb,omitempty"`
	DiskTotalGB   float64 `json:"diskTotalGb,omitempty"`

	MetricsCheckedAt time.Time `json:"metricsCheckedAt,omitempty"`
	MetricsStale     bool      `json:"metricsStale"`
	MetricsError     string    `json:"metricsError,omitempty"`
}

// ConnectionStatus mengembalikan status koneksi server LANGSUNG dari
// sshpool.Pool — tidak ada SSH round-trip di sini, murni baca state yang
// sudah dijaga hidup oleh keepalive pool (lihat internal/core/sshpool).
// Ini yang membuat status koneksi "gratis" ditampilkan untuk berapa pun
// banyak server tanpa menambah beban.
func (s *Service) ConnectionStatus(serverID string) string {
	return string(s.pool.Status(serverID))
}

// fetchMetrics menjalankan metricsScript pada slot dedicated SlotMetrics.
// Dipanggil oleh Collector, BUKAN langsung oleh binding UI — supaya semua
// pemanggilan metrik lewat satu jalur yang sama (bounded worker pool +
// staggering), lihat collector.go.
func (s *Service) fetchMetrics(ctx context.Context, serverID string, timeout time.Duration) (*ServerStatus, error) {
	res, err := s.executor.ExecMetrics(ctx, serverID, timeout, metricsScript)
	if err != nil {
		return nil, err
	}
	st := &ServerStatus{ID: serverID, MetricsCheckedAt: time.Now()}
	applyMetricsOutput(st, res.Stdout)
	return st, nil
}
