package dbxfer

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// emitEvery membatasi laju event progress — alasan sama seperti
// filexfer/dockerxfer: satu callback emitter langsung ke event Wails,
// bukan fan-out WebSocket seperti homepoin, karena hanya ada satu frontend
// yang mendengarkan di aplikasi desktop ini.
const emitEvery = 200 * time.Millisecond

// Job menyimpan state satu migrasi database (satu atau lebih database
// sekaligus, jalan lewat worker pool — beda dari dockerxfer yang satu
// container per job, migrasi database di homepoin memang mendukung banyak
// database sekaligus dan itu fitur nyata yang berguna, jadi dipertahankan).
type Job struct {
	ID             string
	SourceServerID string
	DestServerID   string
	Engine         string

	status  atomic.Value // string
	message atomic.Value // string

	totalBytes atomic.Int64
	doneBytes  atomic.Int64

	startedAt  time.Time
	finishedAt atomic.Value // *time.Time

	ctx    context.Context
	cancel context.CancelFunc

	mu           sync.Mutex
	items        map[string]*ItemResult
	itemOrder    []string
	currentItems map[string]struct{}

	emitMu   sync.Mutex
	emit     func(Progress)
	lastEmit time.Time
}

func newJob(parent context.Context, req StartRequest, items []ItemResult, emit func(Progress)) *Job {
	ctx, cancel := context.WithCancel(parent)
	j := &Job{
		ID:             uuid.NewString(),
		SourceServerID: req.SourceServerID,
		DestServerID:   req.DestServerID,
		Engine:         req.Engine,
		startedAt:      time.Now().UTC(),
		ctx:            ctx,
		cancel:         cancel,
		items:          make(map[string]*ItemResult, len(items)),
		currentItems:   make(map[string]struct{}),
		emit:           emit,
	}
	for i := range items {
		it := items[i]
		j.items[it.Database] = &it
		j.itemOrder = append(j.itemOrder, it.Database)
	}
	j.status.Store(StatusQueued)
	j.message.Store("")
	return j
}

// Context mengembalikan context job, dibatalkan saat Cancel dipanggil.
func (j *Job) Context() context.Context { return j.ctx }

// Cancel meminta pembatalan job.
func (j *Job) Cancel() {
	if j.cancel != nil {
		j.cancel()
	}
}

// IsTerminal melaporkan apakah job sudah selesai (sukses, sebagian, gagal, batal).
func (j *Job) IsTerminal() bool {
	st, _ := j.status.Load().(string)
	return st == StatusDone || st == StatusPartial || st == StatusFailed || st == StatusCanceled
}

// SetStatus mengganti status dan pesan job, lalu mengirim progress segera.
func (j *Job) SetStatus(status, message string) {
	j.status.Store(status)
	j.message.Store(message)
	if j.IsTerminal() {
		now := time.Now().UTC()
		j.finishedAt.Store(&now)
	}
	j.publish(true)
}

// AddTotalBytes menambah taksiran ukuran total job.
func (j *Job) AddTotalBytes(n int64) {
	if n > 0 {
		j.totalBytes.Add(n)
	}
}

// AddDoneBytes menambah hitungan byte terkirim; dipanggil dari
// countingReader untuk setiap potongan yang lewat pipe.
func (j *Job) AddDoneBytes(n int64) {
	if n > 0 {
		j.doneBytes.Add(n)
		j.publish(false)
	}
}

// markBytesComplete menyamakan doneBytes dengan totalBytes saat job selesai.
func (j *Job) markBytesComplete() {
	j.doneBytes.Store(j.totalBytes.Load())
}

// SetItemStatus memperbarui status satu item (key: nama database sumber).
func (j *Job) SetItemStatus(db, status, errMsg string) {
	j.mu.Lock()
	if r, ok := j.items[db]; ok {
		r.Status = status
		if errMsg != "" {
			r.Error = errMsg
		}
	}
	if status == ItemRunning {
		j.currentItems[db] = struct{}{}
	} else {
		delete(j.currentItems, db)
	}
	j.mu.Unlock()
	j.publish(true)
}

// SetItemWarning mencatat catatan non-fatal pada satu item (mis. database
// tujuan sudah ada) — tidak mengubah Status.
func (j *Job) SetItemWarning(db, warning string) {
	j.mu.Lock()
	if r, ok := j.items[db]; ok {
		r.Warning = warning
	}
	j.mu.Unlock()
	j.publish(true)
}

// SetItemTableCount mencatat jumlah tabel nyata yang akan disalin (setelah
// AllTables diselesaikan ke daftar konkret).
func (j *Job) SetItemTableCount(db string, n int) {
	j.mu.Lock()
	if r, ok := j.items[db]; ok {
		r.TableCount = n
	}
	j.mu.Unlock()
}

// SetItemVerify mencatat hasil verifikasi satu item.
func (j *Job) SetItemVerify(db, verify, detail string) {
	j.mu.Lock()
	if r, ok := j.items[db]; ok {
		r.Verify = verify
		r.VerifyDetail = detail
	}
	j.mu.Unlock()
	j.publish(true)
}

// Snapshot mengembalikan salinan progress job saat ini.
func (j *Job) Snapshot() Progress {
	status, _ := j.status.Load().(string)
	msg, _ := j.message.Load().(string)
	total := j.totalBytes.Load()
	done := j.doneBytes.Load()

	var percent float64
	if total > 0 {
		percent = float64(done) / float64(total) * 100
		if percent > 100 {
			percent = 100
		}
	}

	j.mu.Lock()
	items := make([]ItemResult, 0, len(j.itemOrder))
	for _, key := range j.itemOrder {
		if r, ok := j.items[key]; ok {
			items = append(items, *r)
		}
	}
	current := make([]string, 0, len(j.currentItems))
	for key := range j.currentItems {
		current = append(current, key)
	}
	j.mu.Unlock()

	var finishedAt *time.Time
	if v := j.finishedAt.Load(); v != nil {
		finishedAt, _ = v.(*time.Time)
	}

	return Progress{
		JobID:          j.ID,
		Status:         status,
		SourceServerID: j.SourceServerID,
		DestServerID:   j.DestServerID,
		Engine:         j.Engine,
		TotalBytes:     total,
		DoneBytes:      done,
		Percent:        percent,
		CurrentItems:   current,
		Items:          items,
		Message:        msg,
		StartedAt:      j.startedAt,
		FinishedAt:     finishedAt,
	}
}

// publish mengirim snapshot ke emitter, dibatasi lajunya kecuali force.
func (j *Job) publish(force bool) {
	j.emitMu.Lock()
	if j.emit == nil {
		j.emitMu.Unlock()
		return
	}
	now := time.Now()
	if !force && now.Sub(j.lastEmit) < emitEvery {
		j.emitMu.Unlock()
		return
	}
	j.lastEmit = now
	emit := j.emit
	j.emitMu.Unlock()

	emit(j.Snapshot())
}
