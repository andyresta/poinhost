package dbxfer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/servers"
	"github.com/andyresta/poinhost/internal/modules/website"
)

const (
	// workers menentukan berapa database berjalan bersamaan dalam satu job
	// — beda dari dockerxfer (1 container per job): migrasi database di
	// homepoin memang mendukung banyak database sekaligus lewat worker
	// pool, dan itu keputusan yang dikunci ulang secara sadar di sini
	// (bukan sekadar meniru), karena migrasi beberapa database aplikasi
	// dalam satu kali jalan adalah kasus nyata yang berguna.
	workers = 2

	transferTimeout = 12 * time.Hour
)

// Service mengorkestrasi migrasi database (MySQL/PostgreSQL) dari satu
// server ke server lain: preflight -> resolusi tabel & peringatan kolisi
// -> dump/restore lewat pipe per database (worker pool) -> verifikasi
// COUNT(*) nyata sumber vs tujuan.
//
// Satu job aktif pada satu waktu (activeJob), sama seperti filexfer dan
// dockerxfer, dengan alasan yang sama: aplikasi desktop satu operator,
// beberapa migrasi besar bersamaan cuma berebut bandwidth yang sama sambil
// membuat "apa yang sedang terjadi" ambigu di UI. Guard ini milik dbxfer
// SENDIRI, tidak dibagi dengan filexfer/dockerxfer — konsisten dengan
// homepoin yang juga punya guard terpisah per fitur migrasi, bukan satu
// kunci global lintas fitur.
type Service struct {
	servers *servers.Service
	website *website.Service
	pool    *sshpool.Pool

	emitMu sync.RWMutex
	emit   func(Progress)

	mu        sync.Mutex
	jobs      map[string]*Job
	activeJob string
}

// NewService membuat service migrasi database baru.
func NewService(serversSvc *servers.Service, websiteSvc *website.Service, pool *sshpool.Pool) *Service {
	return &Service{
		servers: serversSvc,
		website: websiteSvc,
		pool:    pool,
		jobs:    make(map[string]*Job),
	}
}

// SetEmitter memasang callback yang menerima setiap pembaruan progress.
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

// ListTables mengembalikan semua tabel satu database, untuk picker
// frontend memilih subset tabel (bukan seluruh database).
func (s *Service) ListTables(serverID, engine, database string) (*TableListResponse, error) {
	tables, err := s.website.DBTableNames(serverID, engine, database)
	if err != nil {
		return nil, err
	}
	return &TableListResponse{Tables: tables}, nil
}

// Start memvalidasi permintaan lalu menjalankan job migrasi di latar
// belakang, mengembalikan progress awalnya.
func (s *Service) Start(req StartRequest) (*Progress, error) {
	srcID := strings.TrimSpace(req.SourceServerID)
	dstID := strings.TrimSpace(req.DestServerID)
	engine := strings.TrimSpace(req.Engine)
	if srcID == "" || dstID == "" {
		return nil, fmt.Errorf("server asal dan server tujuan wajib dipilih")
	}
	if _, _, ok := website.DBEngineBinary(engine); !ok {
		return nil, fmt.Errorf("engine database tidak dikenal: %s (pakai mysql atau postgresql)", engine)
	}
	if _, err := s.servers.Get(srcID); err != nil {
		return nil, fmt.Errorf("server asal: %w", err)
	}
	if _, err := s.servers.Get(dstID); err != nil {
		return nil, fmt.Errorf("server tujuan: %w", err)
	}
	if len(req.Items) == 0 {
		return nil, fmt.Errorf("pilih minimal satu database untuk dimigrasikan")
	}

	items := make([]ItemResult, 0, len(req.Items))
	tablesByDB := make(map[string][]string, len(req.Items))
	seen := make(map[string]bool, len(req.Items))
	for _, it := range req.Items {
		db := strings.TrimSpace(it.Database)
		if db == "" {
			return nil, fmt.Errorf("nama database sumber wajib diisi")
		}
		if seen[db] {
			return nil, fmt.Errorf("database %q dipilih dua kali", db)
		}
		seen[db] = true
		if !it.AllTables && len(it.Tables) == 0 {
			return nil, fmt.Errorf("database %q: pilih semua tabel atau sebutkan tabelnya", db)
		}
		destDB := strings.TrimSpace(it.DestDatabase)
		if destDB == "" {
			destDB = db
		}
		if destDB != db && len(req.Items) > 1 {
			return nil, fmt.Errorf("ganti nama database tujuan hanya didukung saat memilih tepat satu database")
		}
		items = append(items, ItemResult{Database: db, DestDatabase: destDB, Status: ItemPending})
		if !it.AllTables {
			tablesByDB[db] = it.Tables
		}
	}

	s.mu.Lock()
	if j, ok := s.jobs[s.activeJob]; ok && j != nil && !j.IsTerminal() {
		s.mu.Unlock()
		return nil, fmt.Errorf("masih ada migrasi database berjalan — tunggu selesai atau batalkan dulu")
	}
	s.mu.Unlock()

	req.SourceServerID, req.DestServerID, req.Engine = srcID, dstID, engine

	parent, parentCancel := context.WithTimeout(context.Background(), transferTimeout)
	job := newJob(parent, req, items, s.publish)

	s.mu.Lock()
	s.jobs[job.ID] = job
	s.activeJob = job.ID
	s.mu.Unlock()

	go func() {
		defer parentCancel()
		s.run(job, tablesByDB)
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
//
// explicitTables: item yang TIDAK memilih AllTables, dengan daftar tabel
// eksplisit dari request. Item yang tidak muncul di map ini berarti
// AllTables — daftar nyatanya diselesaikan di sini lewat DBTableNames.
func (s *Service) run(job *Job, explicitTables map[string][]string) {
	ctx := job.Context()
	defer func() {
		s.mu.Lock()
		if s.activeJob == job.ID {
			s.activeJob = ""
		}
		s.mu.Unlock()
	}()

	job.SetStatus(StatusInspecting, "Memeriksa server & database…")

	if err := s.preflight(job); err != nil {
		job.SetStatus(StatusFailed, err.Error())
		return
	}

	dbs := job.Snapshot().Items
	resolvedTables := make(map[string][]string, len(dbs))
	failed := make(map[string]bool, len(dbs))

	for _, it := range dbs {
		if ctx.Err() != nil {
			job.SetStatus(StatusCanceled, "Dibatalkan")
			return
		}
		tables := explicitTables[it.Database]
		if tables == nil {
			all, err := s.website.DBTableNames(job.SourceServerID, job.Engine, it.Database)
			if err != nil {
				job.SetItemStatus(it.Database, ItemFailed, "gagal membaca daftar tabel: "+err.Error())
				failed[it.Database] = true
				continue
			}
			tables = all
		}
		resolvedTables[it.Database] = tables
		job.SetItemTableCount(it.Database, len(tables))
		job.AddTotalBytes(s.website.DBEstimateSize(job.SourceServerID, job.Engine, it.Database, tables))

		exists, err := s.website.DBDatabaseExists(job.DestServerID, job.Engine, it.DestDatabase)
		if err == nil && exists {
			job.SetItemWarning(it.Database, "Database tujuan sudah ada — tabel bernama sama akan ditimpa, tabel lain di tujuan tidak disentuh")
		}
	}

	if len(failed) == len(dbs) {
		job.SetStatus(StatusFailed, "Semua database gagal diperiksa sebelum migrasi")
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

	gtidPurgedSupported := false
	if job.Engine == "mysql" {
		gtidPurgedSupported = mysqldumpSupportsGtidPurged(ctx, srcClient)
	}

	job.SetStatus(StatusRunning, "Menyalin database…")

	n := workers
	if n > len(dbs) {
		n = len(dbs)
	}
	if n < 1 {
		n = 1
	}

	type queued struct {
		Database, DestDatabase string
	}
	queue := make(chan queued)
	go func() {
		defer close(queue)
		for _, it := range dbs {
			if failed[it.Database] {
				continue
			}
			select {
			case queue <- queued{it.Database, it.DestDatabase}:
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
			for q := range queue {
				job.SetItemStatus(q.Database, ItemRunning, "")

				createErr := s.website.DBCreateDatabase(website.DBCreateDatabaseRequest{
					ServerID: job.DestServerID, Engine: job.Engine, Name: q.DestDatabase,
				})
				if createErr != nil && !strings.Contains(strings.ToLower(createErr.Error()), "already exists") &&
					!strings.Contains(strings.ToLower(createErr.Error()), "sudah ada") {
					job.SetItemStatus(q.Database, ItemFailed, "gagal menyiapkan database tujuan: "+createErr.Error())
					continue
				}

				tables := resolvedTables[q.Database]
				itemCtx, itemCancel := context.WithTimeout(ctx, transferTimeout)
				err := runOneItem(itemCtx, srcClient, dstClient, s.website, job.SourceServerID, job.DestServerID, job.Engine, q.Database, q.DestDatabase, tables, gtidPurgedSupported, job)
				itemCancel()
				if err != nil {
					msg := err.Error()
					if ctx.Err() != nil {
						msg = "dibatalkan"
					}
					job.SetItemStatus(q.Database, ItemFailed, msg)
					continue
				}
				job.SetItemStatus(q.Database, ItemDone, "")
			}
		}()
	}
	wg.Wait()

	if ctx.Err() != nil {
		job.SetStatus(StatusCanceled, "Dibatalkan")
		return
	}
	job.markBytesComplete()

	job.SetStatus(StatusVerifying, "Memverifikasi hasil…")

	var done, failedCount int
	for _, it := range job.Snapshot().Items {
		switch it.Status {
		case ItemDone:
			done++
			verify, detail := verifyDatabase(s.website, job.SourceServerID, job.DestServerID, job.Engine, it.Database, it.DestDatabase, resolvedTables[it.Database])
			job.SetItemVerify(it.Database, verify, detail)
		case ItemFailed:
			failedCount++
		}
	}

	switch {
	case done == 0:
		job.SetStatus(StatusFailed, fmt.Sprintf("%d database gagal", failedCount))
	case failedCount == 0:
		job.SetStatus(StatusDone, fmt.Sprintf("%d database berhasil dimigrasikan", done))
	default:
		job.SetStatus(StatusPartial, fmt.Sprintf("%d database berhasil, %d gagal", done, failedCount))
	}
}

// preflight memeriksa Docker... eh, database sehat di KEDUA sisi — homepoin
// tidak melakukan pengecekan ini sama sekali untuk dbxfer (baru ketahuan
// mati saat dump/restore sungguhan gagal di tengah jalan).
func (s *Service) preflight(job *Job) error {
	srcStatus, err := s.website.DBStatus(job.SourceServerID, job.Engine)
	if err != nil {
		return fmt.Errorf("cek database di server asal: %w", err)
	}
	if !srcStatus.Active {
		return fmt.Errorf("database %s tidak aktif di server asal", job.Engine)
	}
	dstStatus, err := s.website.DBStatus(job.DestServerID, job.Engine)
	if err != nil {
		return fmt.Errorf("cek database di server tujuan: %w", err)
	}
	if !dstStatus.Active {
		return fmt.Errorf("database %s tidak aktif di server tujuan", job.Engine)
	}
	return nil
}
