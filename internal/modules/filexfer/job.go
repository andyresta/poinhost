package filexfer

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Job menyimpan state satu transfer file antar server.
//
// Progress-nya dikirim lewat satu callback emitter (dipasang Service, yang
// menyambungkannya ke event Wails) — bukan lewat kumpulan channel listener
// seperti versi WebSocket di homepoin. Di aplikasi desktop hanya ada satu
// frontend yang mendengarkan, jadi fan-out tidak diperlukan.
type Job struct {
	ID             string
	SourceServerID string
	DestServerID   string
	SourcePaths    []string
	DestPath       string
	Exclude        []string
	Compress       bool

	status  atomic.Value // string
	message atomic.Value // string

	totalBytes atomic.Int64
	doneBytes  atomic.Int64

	startedAt  time.Time
	finishedAt atomic.Value // *time.Time

	ctx    context.Context
	cancel context.CancelFunc

	mu           sync.Mutex
	pathResults  map[string]*PathResult
	currentPaths map[string]struct{}

	emitMu   sync.Mutex
	emit     func(Progress)
	lastEmit time.Time
}

// emitEvery membatasi laju event progress. Tanpa throttle, setiap potongan
// yang terbaca dari pipe akan memicu satu event — puluhan ribu event per
// detik untuk transfer cepat, yang justru membuat UI tersendat.
const emitEvery = 200 * time.Millisecond

// newJob membuat job baru beserta context yang bisa dibatalkan.
func newJob(parent context.Context, req StartRequest, srcPaths []string, destPath string, emit func(Progress)) *Job {
	ctx, cancel := context.WithCancel(parent)
	results := make(map[string]*PathResult, len(srcPaths))
	for _, p := range srcPaths {
		results[p] = &PathResult{Path: p, Status: PathPending}
	}
	j := &Job{
		ID:             uuid.NewString(),
		SourceServerID: req.SourceServerID,
		DestServerID:   req.DestServerID,
		SourcePaths:    srcPaths,
		DestPath:       destPath,
		Exclude:        append([]string(nil), req.Exclude...),
		Compress:       req.Compress,
		startedAt:      time.Now().UTC(),
		ctx:            ctx,
		cancel:         cancel,
		pathResults:    results,
		currentPaths:   make(map[string]struct{}),
		emit:           emit,
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

// IsTerminal melaporkan apakah job sudah selesai (sukses, gagal, batal).
func (j *Job) IsTerminal() bool {
	st, _ := j.status.Load().(string)
	return st == StatusDone || st == StatusFailed || st == StatusCanceled
}

// SetStatus mengganti status dan pesan job, lalu mengirim progress segera.
func (j *Job) SetStatus(status, message string) {
	j.status.Store(status)
	j.message.Store(message)
	if status == StatusDone || status == StatusFailed || status == StatusCanceled {
		now := time.Now().UTC()
		j.finishedAt.Store(&now)
	}
	j.publish(true)
}

// AddDoneBytes menambah hitungan byte terkirim; dipanggil dari
// countingReader untuk setiap potongan yang lewat pipe.
func (j *Job) AddDoneBytes(n int64) {
	if n > 0 {
		j.doneBytes.Add(n)
		j.publish(false)
	}
}

// markBytesComplete menyamakan doneBytes dengan totalBytes saat job benar
// benar tuntas. totalBytes adalah ukuran MENTAH hasil `du -sb`, sedangkan
// doneBytes menghitung byte stream tar yang sudah dikompresi — dua angka
// yang tidak sepadan, jadi persentase tidak akan pernah sampai 100 sendiri
// kalau datanya kompresibel. Disamakan di akhir supaya bar progress
// mencerminkan "selesai", bukan berhenti di angka rasio kompresi.
func (j *Job) markBytesComplete() {
	j.doneBytes.Store(j.totalBytes.Load())
}

// SetPathStatus memperbarui status satu path sumber tingkat atas.
func (j *Job) SetPathStatus(p, status, errMsg string) {
	j.mu.Lock()
	if r, ok := j.pathResults[p]; ok {
		r.Status = status
		if errMsg != "" {
			r.Error = errMsg
		}
	}
	if status == PathRunning {
		j.currentPaths[p] = struct{}{}
	} else {
		delete(j.currentPaths, p)
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
	paths := make([]PathResult, 0, len(j.SourcePaths))
	for _, p := range j.SourcePaths {
		if r, ok := j.pathResults[p]; ok {
			paths = append(paths, *r)
		}
	}
	current := make([]string, 0, len(j.currentPaths))
	for p := range j.currentPaths {
		current = append(current, p)
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
		SourcePaths:    append([]string(nil), j.SourcePaths...),
		DestPath:       j.DestPath,
		Compress:       j.Compress,
		TotalBytes:     total,
		DoneBytes:      done,
		Percent:        percent,
		CurrentPaths:   current,
		Paths:          paths,
		Message:        msg,
		StartedAt:      j.startedAt,
		FinishedAt:     finishedAt,
	}
}

// publish mengirim snapshot ke emitter. force melewati throttle — dipakai
// untuk perubahan yang tidak boleh telat terlihat (ganti status, satu path
// selesai), sementara pertambahan byte biasa tetap dibatasi lajunya.
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
