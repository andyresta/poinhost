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
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/modules/docker"
	"github.com/andyresta/poinhost/internal/modules/files"
	"github.com/andyresta/poinhost/internal/modules/servers"
	"github.com/andyresta/poinhost/internal/modules/services"
	"github.com/andyresta/poinhost/internal/modules/website"
)

// errBelumKelolaDiri dipakai untuk modul yang butuh koneksi ke mesin ini.
//
// Tanpa pengelolaan-diri agent tidak punya satu pun jalan menyentuh
// mesinnya, dan menyebutnya "gagal" akan mengirim pemiliknya mencari
// masalah jaringan — padahal yang kurang satu tombol di panel Mobile
// Control.
var errBelumKelolaDiri = errors.New("the agent does not manage its own machine yet")

// errAksiTidakDikenal adalah jaring untuk aksi yang lolos penyaringan
// di lapisan API — lihat ContainerAction.
var errAksiTidakDikenal = errors.New("unknown action")

// Backend mengimplementasikan Reader.
type Backend struct {
	svc       *servers.Service
	collector *servers.Collector
	services  *services.Service
	files     *files.Service
	website   *website.Service
	docker    *docker.Service

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
	websiteSvc *website.Service,
	dockerSvc *docker.Service,
	selfServerID string,
) *Backend {
	return &Backend{
		svc:       svc,
		collector: collector,
		services:  servicesSvc,
		files:     filesSvc,
		website:   websiteSvc,
		docker:    dockerSvc,
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

// WriteFile menimpa isi satu berkas di mesin ini.
func (b *Backend) WriteFile(path string, data []byte) error {
	ctx, cancel, err := b.konteksFile()
	if err != nil {
		return err
	}
	defer cancel()
	return b.files.WriteFile(ctx, files.WriteFileRequest{
		ServerID: b.selfID,
		Path:     path,
		Content:  string(data),
	})
}

// RenameFile memindahkan satu berkas atau direktori.
func (b *Backend) RenameFile(oldPath, newPath string) error {
	ctx, cancel, err := b.konteksFile()
	if err != nil {
		return err
	}
	defer cancel()
	return b.files.Rename(ctx, files.RenameRequest{
		ServerID: b.selfID,
		OldPath:  oldPath,
		NewPath:  newPath,
	})
}

// DeleteFiles menghapus berkas atau direktori.
func (b *Backend) DeleteFiles(paths []string) error {
	ctx, cancel, err := b.konteksFile()
	if err != nil {
		return err
	}
	defer cancel()
	return b.files.Delete(ctx, files.DeleteRequest{
		ServerID: b.selfID,
		Paths:    paths,
	})
}

// konteksFile menyiapkan konteks bertenggat untuk satu operasi berkas, dan
// menolak lebih dulu kalau agent belum mengelola mesinnya sendiri.
//
// Tenggatnya lebih longgar daripada operasi baca: menghapus direktori besar
// dan mengunggah berkas lewat SFTP sama-sama bisa makan waktu, dan memutus
// di tengah penulisan justru meninggalkan berkas setengah jadi.
func (b *Backend) konteksFile() (context.Context, context.CancelFunc, error) {
	if b.selfID == "" || b.files == nil {
		return nil, nil, errBelumKelolaDiri
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	return ctx, cancel, nil
}

// Websites membaca seluruh vhost nginx di mesin ini.
func (b *Backend) Websites() (WebsiteListing, error) {
	if b.selfID == "" || b.website == nil {
		return WebsiteListing{}, errBelumKelolaDiri
	}
	res, err := b.website.List(b.selfID)
	if err != nil {
		return WebsiteListing{}, err
	}
	out := WebsiteListing{
		NginxInstalled: res.Nginx.Installed,
		NginxActive:    res.Nginx.Active,
		NginxVersion:   res.Nginx.Version,
	}
	out.Sites = make([]WebsiteInfo, 0, len(res.Domains))
	for _, d := range res.Domains {
		out.Sites = append(out.Sites, WebsiteInfo{
			Domain:      d.Domain,
			Parent:      d.Parent,
			IsSubdomain: d.IsSubdomain,
			Root:        d.Root,
			Enabled:     d.Enabled,
			PHPVersion:  d.PHPVersion,
			PHPEnabled:  d.PHPEnabled,
			ProxyTarget: d.ProxyTarget,
			SSLEnabled:  d.SSLEnabled,
		})
	}
	return out, nil
}

// WebsiteSSL membaca keadaan sertifikat satu domain.
func (b *Backend) WebsiteSSL(domain string) (WebsiteSSL, error) {
	if b.selfID == "" || b.website == nil {
		return WebsiteSSL{}, errBelumKelolaDiri
	}
	st, err := b.website.SSLStatus(b.selfID, domain)
	if err != nil {
		return WebsiteSSL{}, err
	}
	out := WebsiteSSL{
		Domain:           st.Domain,
		SSLEnabled:       st.SSLEnabled,
		AutoRenewEnabled: st.AutoRenewEnabled,
		CertExists:       st.Certificate.Exists,
		NotAfter:         st.Certificate.NotAfter,
		CertDomains:      st.Certificate.Domains,
		MissingSANs:      st.MissingSANs,
	}
	if t, ok := uraiTanggalSertifikat(st.Certificate.NotAfter); ok {
		out.NotAfter = t.UTC().Format(time.RFC3339)
		sisa := int(time.Until(t).Hours() / 24)
		out.ExpiresInDays = &sisa
	}
	return out, nil
}

// uraiTanggalSertifikat membaca keluaran `openssl x509 -enddate`.
//
// Bentuknya "Jan  2 15:04:05 2006 GMT" — format milik openssl, bukan
// standar mana pun. Diuraikan SEKALI di sini supaya tiap klien tidak punya
// kesempatan sendiri untuk salah membacanya; yang gagal diurai diteruskan
// apa adanya, bukan dibuang, karena tanggal yang tidak bisa dihitung tetap
// lebih berguna daripada tidak ada tanggal.
func uraiTanggalSertifikat(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		"Jan _2 15:04:05 2006 MST",
		"Jan _2 15:04:05 2006",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// SetWebsiteEnabled menyalakan atau mematikan satu vhost.
func (b *Backend) SetWebsiteEnabled(domain string, enabled bool) error {
	if b.selfID == "" || b.website == nil {
		return errBelumKelolaDiri
	}
	return b.website.SetEnabled(website.SetEnabledRequest{
		ServerID: b.selfID,
		Domain:   domain,
		Enabled:  enabled,
	})
}
