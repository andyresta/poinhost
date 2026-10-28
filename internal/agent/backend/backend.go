// Package backend menyambungkan API agent ke modul poinhost yang sudah ada.
//
// Tidak ada logika baru di sini — semuanya delegasi ke servers.Service dan
// servers.Collector. Itu memang tujuannya: agent memakai ULANG jalur yang
// sama dengan app desktop, sehingga "status server" tidak pernah punya dua
// implementasi yang lama-lama berbeda jawabannya.
package backend

import (
	"context"
	"errors"
	"time"

	"github.com/andyresta/poinhost/internal/modules/files"
	"github.com/andyresta/poinhost/internal/modules/servers"
	"github.com/andyresta/poinhost/internal/modules/services"
)

// errBelumKelolaDiri dipakai untuk modul yang butuh koneksi ke mesin ini.
//
// Tanpa pengelolaan-diri agent tidak punya satu pun jalan menyentuh
// mesinnya, dan menyebutnya "gagal" akan mengirim pemiliknya mencari
// masalah jaringan — padahal yang kurang satu tombol di panel Mobile
// Control.
var errBelumKelolaDiri = errors.New("agent belum mengelola mesinnya sendiri")

// Backend mengimplementasikan Reader.
type Backend struct {
	svc       *servers.Service
	collector *servers.Collector
	services  *services.Service
	files     *files.Service

	// selfID adalah server yang mewakili mesin tempat agent berjalan, supaya
	// bisa ditandai di daftar. Kosong berarti agent belum mendaftarkan
	// dirinya sendiri.
	selfID string
}

// New membuat backend.
func New(
	svc *servers.Service,
	collector *servers.Collector,
	servicesSvc *services.Service,
	filesSvc *files.Service,
	selfServerID string,
) *Backend {
	return &Backend{
		svc:       svc,
		collector: collector,
		services:  servicesSvc,
		files:     filesSvc,
		selfID:    selfServerID,
	}
}

// Services membaca unit systemd di mesin ini.
//
// Selalu selfID, tidak pernah id dari permintaan: itu yang membuat satu
// agent mustahil dipakai mengintip — atau mengubah — mesin lain yang
// kebetulan masih tersimpan di database-nya dari versi lama.
func (b *Backend) Services() ([]ServiceInfo, bool, error) {
	if b.selfID == "" || b.services == nil {
		return nil, false, errBelumKelolaDiri
	}
	res, err := b.services.ListServices(b.selfID)
	if err != nil {
		return nil, false, err
	}
	out := make([]ServiceInfo, 0, len(res.Services))
	for _, s := range res.Services {
		out = append(out, toServiceInfo(s))
	}
	return out, res.SystemdAvailable, nil
}

// ServiceAction menjalankan aksi systemctl di mesin ini.
//
// Validasi nama unit dan daftar aksi yang sah ada di modul services, yang
// sama persis dengan yang dipakai app desktop — bukan disalin ke sini.
// Satu-satunya tempat yang boleh tahu cara menyusun perintah systemctl
// adalah modul itu.
func (b *Backend) ServiceAction(name, action string) (ServiceInfo, error) {
	if b.selfID == "" || b.services == nil {
		return ServiceInfo{}, errBelumKelolaDiri
	}
	info, err := b.services.ServiceAction(b.selfID, name, action)
	if err != nil {
		return ServiceInfo{}, err
	}
	return toServiceInfo(*info), nil
}

func toServiceInfo(s services.ServiceInfo) ServiceInfo {
	return ServiceInfo{
		Name:          s.Name,
		Description:   s.Description,
		ActiveState:   s.ActiveState,
		SubState:      s.SubState,
		UnitFileState: s.UnitFileState,
		Running:       s.Running,
		Enabled:       s.Enabled,
		CanEnable:     s.CanEnable,
	}
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

// Files membaca isi satu direktori di mesin ini.
//
// asUser dikosongkan: agent berjalan sebagai root, dan tidak ada user lain
// untuk dipilih dari HP. Mengirimkannya dari jaringan justru akan menambah
// satu nilai lagi yang harus divalidasi tanpa menambah satu pun kemampuan.
func (b *Backend) Files(path string) (FileListing, error) {
	if b.selfID == "" || b.files == nil {
		return FileListing{}, errBelumKelolaDiri
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := b.files.List(ctx, b.selfID, path, "")
	if err != nil {
		return FileListing{}, err
	}
	out := FileListing{Path: res.Path, Parent: res.Parent}
	out.Entries = make([]FileEntry, 0, len(res.Entries))
	for _, e := range res.Entries {
		out.Entries = append(out.Entries, FileEntry{
			Name:    e.Name,
			Path:    e.Path,
			Size:    e.Size,
			IsDir:   e.IsDir,
			Mode:    e.Mode,
			ModTime: e.ModTime,
		})
	}
	return out, nil
}

// FileContent membaca isi satu file teks.
//
// Batas ukurannya ditegakkan modul Files, bukan di sini — satu tempat saja
// yang boleh memutuskan berapa besar yang masih masuk akal dibaca.
func (b *Backend) FileContent(path string) (FileText, error) {
	if b.selfID == "" || b.files == nil {
		return FileText{}, errBelumKelolaDiri
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := b.files.ReadFile(ctx, b.selfID, path, "")
	if err != nil {
		return FileText{}, err
	}
	return FileText{Path: res.Path, Content: res.Content, Size: res.Size}, nil
}
