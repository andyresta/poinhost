package filexfer

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/servers"
)

const (
	// workers menentukan berapa path sumber tingkat atas yang disalurkan
	// bersamaan. Lebih dari sekadar 1 membantu saat memilih banyak folder
	// kecil (latensi per sesi tar jadi tumpang tindih), tapi menaikkannya
	// terus tidak membantu: batasnya bandwidth, bukan jumlah sesi.
	workers = 2

	// scanTimeout membatasi `du -sb` per path. Ini cuma untuk taksiran
	// ukuran, jadi lebih baik menyerah dan jalan tanpa persentase daripada
	// menahan transfer gara-gara satu pohon direktori yang sangat besar.
	scanTimeout = 2 * time.Minute

	// transferTimeout adalah batas atas satu job. Dipasang longgar karena
	// memindahkan ratusan gigabyte memang bisa berjam-jam.
	transferTimeout = 12 * time.Hour

	listDirTimeout = 60 * time.Second
)

// Service mengorkestrasi transfer file antar server.
//
// Satu job aktif pada satu waktu: dua transfer besar yang jalan bersamaan
// hanya membagi bandwidth yang sama sambil membuat kedua progress bar
// melambat, dan membuat laporan "apa yang sedang berjalan" jadi ambigu.
type Service struct {
	servers *servers.Service
	pool    *sshpool.Pool
	sftp    *sshpool.SFTPClient

	emitMu sync.RWMutex
	emit   func(Progress)

	mu        sync.Mutex
	jobs      map[string]*Job
	activeJob string
}

// NewService membuat service transfer file baru.
func NewService(serversSvc *servers.Service, pool *sshpool.Pool, sftpClient *sshpool.SFTPClient) *Service {
	return &Service{
		servers: serversSvc,
		pool:    pool,
		sftp:    sftpClient,
		jobs:    make(map[string]*Job),
	}
}

// SetEmitter memasang callback yang menerima setiap pembaruan progress.
// Dipanggil sekali saat startup oleh app.go untuk menyambungkannya ke event
// Wails.
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

// ListDirectory menampilkan isi satu folder di server, untuk browser di
// panel migrasi.
func (s *Service) ListDirectory(ctx context.Context, serverID, dir string) (*ListDirResponse, error) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return nil, fmt.Errorf("server belum dipilih")
	}
	p, err := NormalizeRemotePath(dir)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, listDirTimeout)
	defer cancel()

	entries, err := s.sftp.ListDirectory(ctx, serverID, p)
	if err != nil {
		return nil, fmt.Errorf("baca folder %s: %w", p, err)
	}

	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, DirEntry{Name: e.Name, Path: e.Path, IsDir: e.IsDir, Size: e.Size})
	}
	// Folder dulu, lalu urut nama — sama seperti file manager, supaya
	// menelusuri pohon direktori terasa sama di kedua fitur.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})

	parent := path.Dir(p)
	if p == "/" {
		parent = ""
	}
	return &ListDirResponse{Path: p, Parent: parent, Entries: out}, nil
}

// Start memvalidasi permintaan lalu menjalankan job transfer di latar
// belakang, mengembalikan progress awalnya.
func (s *Service) Start(req StartRequest) (*Progress, error) {
	srcID := strings.TrimSpace(req.SourceServerID)
	dstID := strings.TrimSpace(req.DestServerID)
	if srcID == "" || dstID == "" {
		return nil, fmt.Errorf("server asal dan server tujuan wajib dipilih")
	}
	if _, err := s.servers.Get(srcID); err != nil {
		return nil, fmt.Errorf("server asal: %w", err)
	}
	if _, err := s.servers.Get(dstID); err != nil {
		return nil, fmt.Errorf("server tujuan: %w", err)
	}

	destPath, err := NormalizeRemotePath(req.DestPath)
	if err != nil {
		return nil, fmt.Errorf("folder tujuan: %w", err)
	}

	srcPaths := make([]string, 0, len(req.SourcePaths))
	for _, raw := range req.SourcePaths {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		p, err := NormalizeRemotePath(raw)
		if err != nil {
			return nil, fmt.Errorf("item sumber: %w", err)
		}
		srcPaths = append(srcPaths, p)
	}
	if len(srcPaths) == 0 {
		return nil, fmt.Errorf("pilih minimal satu file atau folder yang akan dipindahkan")
	}
	srcPaths = PruneRedundantPaths(srcPaths)

	// Menyalin ke dalam dirinya sendiri di server yang sama akan membuat
	// tar membaca apa yang baru saja ditulisnya — pemeriksaan ini menolak
	// itu sebelum ada satu byte pun yang bergerak.
	if srcID == dstID {
		for _, sp := range srcPaths {
			if destPath == sp {
				return nil, fmt.Errorf("folder tujuan sama dengan item sumber: %s", sp)
			}
			if isUnder(destPath, sp) {
				return nil, fmt.Errorf("folder tujuan berada di dalam item sumber: %s", sp)
			}
		}
	}

	s.mu.Lock()
	if j, ok := s.jobs[s.activeJob]; ok && j != nil && !j.IsTerminal() {
		s.mu.Unlock()
		return nil, fmt.Errorf("masih ada transfer berjalan — tunggu selesai atau batalkan dulu")
	}
	s.mu.Unlock()

	parent, parentCancel := context.WithTimeout(context.Background(), transferTimeout)
	job := newJob(parent, req, srcPaths, destPath, s.publish)

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
		return nil, fmt.Errorf("transfer %s tidak ditemukan", jobID)
	}
	snap := job.Snapshot()
	return &snap, nil
}

// Cancel membatalkan job yang sedang berjalan.
func (s *Service) Cancel(jobID string) (*Progress, error) {
	job := s.job(jobID)
	if job == nil {
		return nil, fmt.Errorf("transfer %s tidak ditemukan", jobID)
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

// run menjalankan pipeline satu job: buka koneksi kedua sisi, taksir ukuran,
// lalu salurkan tiap path lewat worker pool.
func (s *Service) run(job *Job) {
	ctx := job.Context()
	defer func() {
		s.mu.Lock()
		if s.activeJob == job.ID {
			s.activeJob = ""
		}
		s.mu.Unlock()
	}()

	// Koneksi dedicated, bukan slot shared: satu transfer bisa berjam-jam
	// dan menjenuhkan kanalnya, sedangkan slot shared dipakai bergantian
	// oleh semua tab & modul yang menunjuk server yang sama — memakainya
	// di sini akan membuat panel lain ikut tersendat selama transfer.
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

	job.SetStatus(StatusScanning, "Menghitung ukuran…")
	var total int64
	for _, p := range job.SourcePaths {
		if ctx.Err() != nil {
			break
		}
		scanCtx, cancel := context.WithTimeout(ctx, scanTimeout)
		total += scanSize(scanCtx, srcClient, p)
		cancel()
	}
	job.totalBytes.Store(total)

	if ctx.Err() != nil {
		job.SetStatus(StatusCanceled, "Dibatalkan")
		return
	}

	job.SetStatus(StatusRunning, "Memindahkan…")

	n := workers
	if n > len(job.SourcePaths) {
		n = len(job.SourcePaths)
	}
	if n < 1 {
		n = 1
	}

	queue := make(chan string)
	go func() {
		defer close(queue)
		for _, p := range job.SourcePaths {
			select {
			case queue <- p:
			case <-ctx.Done():
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range queue {
				job.SetPathStatus(p, PathRunning, "")
				err := runOnePath(ctx, srcClient, dstClient, p, job.DestPath, job.Exclude, job.Compress, job)
				if err != nil {
					msg := err.Error()
					if ctx.Err() != nil {
						msg = "dibatalkan"
					}
					job.SetPathStatus(p, PathFailed, msg)
					continue
				}
				job.SetPathStatus(p, PathDone, "")
			}
		}()
	}
	wg.Wait()

	if ctx.Err() != nil {
		job.SetStatus(StatusCanceled, "Dibatalkan")
		return
	}

	var done, failed int
	for _, r := range job.Snapshot().Paths {
		switch r.Status {
		case PathDone:
			done++
		case PathFailed:
			failed++
		}
	}
	if failed > 0 {
		job.SetStatus(StatusFailed, fmt.Sprintf("%d item berhasil, %d gagal", done, failed))
		return
	}
	job.markBytesComplete()
	job.SetStatus(StatusDone, fmt.Sprintf("%d item selesai dipindahkan", done))
}
