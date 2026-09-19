package dockerxfer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/docker"
	"github.com/andyresta/poinhost/internal/modules/servers"
)

const (
	// scanTimeout/transferTimeout: nilai sama dengan filexfer, alasan sama
	// — taksiran ukuran boleh gagal dan tetap jalan, sedangkan satu job
	// migrasi (data container bisa besar) butuh jendela waktu yang longgar.
	scanTimeout     = 2 * time.Minute
	transferTimeout = 12 * time.Hour
)

// Service mengorkestrasi migrasi satu container Docker dari satu server ke
// server lain: inspect (docker.Service) → preflight → salin tiap mount →
// salin image → buat+nyalakan container di tujuan (docker.Service) →
// verifikasi ringan.
//
// Satu job aktif pada satu waktu, sama seperti filexfer.Service dan alasan
// yang sama: ini aplikasi desktop satu operator, dua migrasi besar berjalan
// bersamaan cuma berebut bandwidth yang sama sambil membuat "apa yang
// sedang terjadi" ambigu di UI. Ini keputusan desain yang disengaja, bukan
// keterbatasan yang tidak disadari (beda dari homepoin, yang memakai kunci
// job tunggal yang sama tapi tidak pernah menyebutnya sebagai pilihan).
type Service struct {
	servers *servers.Service
	docker  *docker.Service
	pool    *sshpool.Pool

	emitMu sync.RWMutex
	emit   func(Progress)

	mu        sync.Mutex
	jobs      map[string]*Job
	activeJob string
}

// NewService membuat service migrasi Docker baru.
func NewService(serversSvc *servers.Service, dockerSvc *docker.Service, pool *sshpool.Pool) *Service {
	return &Service{
		servers: serversSvc,
		docker:  dockerSvc,
		pool:    pool,
		jobs:    make(map[string]*Job),
	}
}

// SetEmitter memasang callback yang menerima setiap pembaruan progress —
// dipanggil sekali saat startup oleh app.go untuk menyambungkannya ke event
// Wails "dockerxfer:progress:<jobId>".
func (s *Service) SetEmitter(emit func(Progress)) {
	s.emitMu.Lock()
	s.emit = emit
	s.emitMu.Unlock()
}

func (s *Service) publish(p Progress) {
	s.emitMu.RLock()
	emit := s.emit
	s.emitMu.RUnlock()
	if emit != nil {
		emit(p)
	}
}

// Start memvalidasi permintaan lalu menjalankan job migrasi di latar
// belakang, mengembalikan progress awalnya.
func (s *Service) Start(req StartRequest) (*Progress, error) {
	srcID := strings.TrimSpace(req.SourceServerID)
	dstID := strings.TrimSpace(req.DestServerID)
	cid := strings.TrimSpace(req.ContainerID)
	if srcID == "" || dstID == "" {
		return nil, fmt.Errorf("server asal dan server tujuan wajib dipilih")
	}
	if cid == "" {
		return nil, fmt.Errorf("pilih container yang akan dimigrasikan")
	}
	if _, err := s.servers.Get(srcID); err != nil {
		return nil, fmt.Errorf("server asal: %w", err)
	}
	if _, err := s.servers.Get(dstID); err != nil {
		return nil, fmt.Errorf("server tujuan: %w", err)
	}

	s.mu.Lock()
	if j, ok := s.jobs[s.activeJob]; ok && j != nil && !j.IsTerminal() {
		s.mu.Unlock()
		return nil, fmt.Errorf("masih ada migrasi berjalan — tunggu selesai atau batalkan dulu")
	}
	s.mu.Unlock()

	req.SourceServerID, req.DestServerID, req.ContainerID = srcID, dstID, cid

	parent, parentCancel := context.WithTimeout(context.Background(), transferTimeout)
	job := newJob(parent, req, s.publish)

	s.mu.Lock()
	s.jobs[job.ID] = job
	s.activeJob = job.ID
	s.mu.Unlock()

	go func() {
		defer parentCancel()
		s.run(job)
	}()

	snap := job.Snapshot()
	return &snap, nil
}

// Status mengembalikan progress terakhir satu job.
func (s *Service) Status(jobID string) (*Progress, error) {
	job := s.job(jobID)
	if job == nil {
		return nil, fmt.Errorf("migrasi %s tidak ditemukan", jobID)
	}
	snap := job.Snapshot()
	return &snap, nil
}

// Cancel membatalkan job yang sedang berjalan.
func (s *Service) Cancel(jobID string) (*Progress, error) {
	job := s.job(jobID)
	if job == nil {
		return nil, fmt.Errorf("migrasi %s tidak ditemukan", jobID)
	}
	if !job.IsTerminal() {
		job.Cancel()
		job.SetStatus(StatusCanceled, "Dibatalkan")
	}
	snap := job.Snapshot()
	return &snap, nil
}

func (s *Service) job(jobID string) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobs[strings.TrimSpace(jobID)]
}

// run menjalankan seluruh pipeline satu job migrasi.
func (s *Service) run(job *Job) {
	ctx := job.Context()
	defer func() {
		s.mu.Lock()
		if s.activeJob == job.ID {
			s.activeJob = ""
		}
		s.mu.Unlock()
	}()

	job.SetStatus(StatusInspecting, "Membaca konfigurasi container…")
	bp, err := s.docker.InspectMigrationBlueprint(job.SourceServerID, job.ContainerID)
	if err != nil {
		job.SetStatus(StatusFailed, "Gagal membaca konfigurasi container: "+err.Error())
		return
	}
	job.SetBlueprint(bp)

	if err := s.preflight(job, bp); err != nil {
		job.SetStatus(StatusFailed, err.Error())
		return
	}

	// Container asal dihentikan untuk konsistensi data selama disalin (file
	// yang masih ditulis aplikasi yang berjalan bisa ter-tar dalam kondisi
	// setengah jadi). SENGAJA tidak dinyalakan lagi otomatis sesudahnya —
	// beda dari kesan "hanya menyalin, container asal tidak terganggu": ada
	// dua salinan container yang menunjuk data yang sama, membiarkan
	// keduanya menyala akan membuat keduanya menulis ke "sumber kebenaran"
	// yang berbeda dan datanya diam-diam bercabang. Pengguna bisa
	// menyalakan lagi container asal secara manual dari panel Docker kalau
	// sudah menilai risikonya, tapi default yang aman adalah tidak dobel.
	_ = s.docker.StopContainer(job.SourceServerID, job.ContainerID)

	if ctx.Err() != nil {
		job.SetStatus(StatusCanceled, "Dibatalkan")
		return
	}

	srcHandle, srcClient, err := s.pool.OpenDedicated(ctx, job.SourceServerID)
	if err != nil {
		job.SetStatus(StatusFailed, "Gagal terhubung ke server asal: "+err.Error())
		return
	}
	defer s.pool.CloseDedicated(job.SourceServerID, srcHandle)

	dstHandle, dstClient, err := s.pool.OpenDedicated(ctx, job.DestServerID)
	if err != nil {
		job.SetStatus(StatusFailed, "Gagal terhubung ke server tujuan: "+err.Error())
		return
	}
	defer s.pool.CloseDedicated(job.DestServerID, dstHandle)

	job.SetStatus(StatusRunning, "Menghitung ukuran…")
	for _, m := range bp.Mounts {
		if ctx.Err() != nil {
			break
		}
		scanCtx, cancel := context.WithTimeout(ctx, scanTimeout)
		job.AddTotalBytes(scanMountSize(scanCtx, srcClient, s.docker, job.SourceServerID, m))
		cancel()
	}
	if ctx.Err() != nil {
		job.SetStatus(StatusCanceled, "Dibatalkan")
		return
	}

	job.SetStatus(StatusRunning, "Menyalin data…")

	failed := false
	for _, m := range bp.Mounts {
		if ctx.Err() != nil {
			job.SetStatus(StatusCanceled, "Dibatalkan")
			return
		}
		key := mountLabel(m)
		if mountHasExistingData(ctx, dstClient, s.docker, job.DestServerID, m) {
			job.SetItemWarning(key, "Tujuan sudah berisi data — akan digabung, bukan ditimpa bersih")
		}
		job.SetItemStatus(key, ItemRunning, "")
		if err := runOneMount(ctx, srcClient, dstClient, s.docker, job.SourceServerID, job.DestServerID, m, job.Compress, job); err != nil {
			msg := err.Error()
			if ctx.Err() != nil {
				msg = "dibatalkan"
			}
			job.SetItemStatus(key, ItemFailed, msg)
			failed = true
			break
		}
		job.SetItemStatus(key, ItemDone, "")
	}

	if !failed && ctx.Err() == nil {
		job.SetItemStatus(imageItemLabel, ItemRunning, "")
		skipped, err := runImage(ctx, srcClient, dstClient, s.docker, job.SourceServerID, job.DestServerID, bp.Image, job.Compress, job)
		switch {
		case err != nil:
			msg := err.Error()
			if ctx.Err() != nil {
				msg = "dibatalkan"
			}
			job.SetItemStatus(imageItemLabel, ItemFailed, msg)
			failed = true
		case skipped:
			job.SetItemStatus(imageItemLabel, ItemSkipped, "")
		default:
			job.SetItemStatus(imageItemLabel, ItemDone, "")
		}
	}

	if ctx.Err() != nil {
		job.SetStatus(StatusCanceled, "Dibatalkan")
		return
	}
	if failed {
		job.SetStatus(StatusFailed, "Sebagian item gagal disalin — lihat rincian per item")
		return
	}
	job.markBytesComplete()

	job.SetStatus(StatusRunning, "Membuat container di server tujuan…")
	if _, err := s.docker.CreateFromBlueprint(job.DestServerID, *bp); err != nil {
		job.SetStatus(StatusFailed, "Data sudah tersalin, tapi gagal membuat container di tujuan: "+err.Error())
		return
	}

	job.SetStatus(StatusVerifying, "Memverifikasi hasil…")
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer verifyCancel()

	running, err := s.docker.ContainerIsRunning(job.DestServerID, bp.Name)
	if err == nil {
		job.SetContainerRunning(running)
	}
	for _, m := range bp.Mounts {
		verifyMount(verifyCtx, srcClient, dstClient, s.docker, job.SourceServerID, job.DestServerID, m, job, mountLabel(m))
	}

	if err == nil && !running {
		job.SetStatus(StatusFailed, "Container dibuat tapi tidak berjalan di tujuan — cek log container")
		return
	}
	job.SetStatus(StatusDone, fmt.Sprintf("Migrasi selesai: %d item disalin", len(bp.Mounts)+1))
}

// preflight memeriksa prasyarat SEBELUM data mulai dipindah: Docker sehat di
// KEDUA sisi (bukan cuma tujuan seperti homepoin — sumber yang Docker-nya
// mati akan gagal di detik pertama transfer, lebih baik ketahuan di sini
// dengan pesan yang jelas) dan nama container belum dipakai di tujuan.
func (s *Service) preflight(job *Job, bp *docker.MigrationBlueprint) error {
	srcStatus, err := s.docker.DetectEngineStatus(job.SourceServerID)
	if err != nil {
		return fmt.Errorf("cek Docker di server asal: %w", err)
	}
	if !srcStatus.Active {
		return fmt.Errorf("Docker tidak aktif di server asal")
	}
	dstStatus, err := s.docker.DetectEngineStatus(job.DestServerID)
	if err != nil {
		return fmt.Errorf("cek Docker di server tujuan: %w", err)
	}
	if !dstStatus.Active {
		return fmt.Errorf("Docker tidak aktif di server tujuan")
	}

	exists, err := s.docker.ContainerNameExists(job.DestServerID, bp.Name)
	if err != nil {
		return fmt.Errorf("cek nama container di server tujuan: %w", err)
	}
	if exists {
		return fmt.Errorf("nama container %q sudah dipakai di server tujuan — hapus/ganti nama dulu sebelum migrasi", bp.Name)
	}
	return nil
}
