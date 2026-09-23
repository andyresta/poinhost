package dockerxfer

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andyresta/poinhost/internal/modules/docker"
	"github.com/google/uuid"
)

// emitEvery membatasi laju event progress — sama seperti filexfer, dan
// dengan alasan yang sama: satu callback emitter langsung ke event Wails,
// bukan fan-out ala WebSocket homepoin, karena hanya ada satu frontend yang
// mendengarkan di aplikasi desktop ini.
const emitEvery = 200 * time.Millisecond

// Job menyimpan state satu migrasi container Docker antar server.
//
// Item transfer (mount + satu entri sintetis image) DISATUKAN dalam satu
// daftar dengan progress byte gabungan, bukan dua progress bar terpisah
// "Stage 1 volume / Stage 2 image" seperti homepoin — pola yang sama dengan
// filexfer (satu daftar path, satu bar). Konsekuensinya: byte image dan
// byte mount digabung jadi satu persentase, tapi status per-item (mount
// mana yang gagal, apakah image di-skip karena sudah ada) tetap terlihat
// per baris, sama presisinya dengan dua bar terpisah tanpa kerumitan UI
// tambahan.
type Job struct {
	ID             string
	SourceServerID string
	DestServerID   string
	ContainerID    string
	Compress       bool
	CopyWorkdir    bool

	// Blueprint diisi setelah tahap inspect (lihat SetBlueprint) — kosong
	// sebelum itu, karena daftar mount container tidak diketahui sebelum
	// di-inspect.
	Blueprint *docker.MigrationBlueprint

	status  atomic.Value // string
	message atomic.Value // string

	totalBytes atomic.Int64
	doneBytes  atomic.Int64

	containerRunning atomic.Value // *bool

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

func newJob(parent context.Context, req StartRequest, emit func(Progress)) *Job {
	ctx, cancel := context.WithCancel(parent)
	j := &Job{
		ID:             uuid.NewString(),
		SourceServerID: req.SourceServerID,
		DestServerID:   req.DestServerID,
		ContainerID:    req.ContainerID,
		Compress:       req.Compress,
		CopyWorkdir:    req.CopyWorkdir,
		startedAt:      time.Now().UTC(),
		ctx:            ctx,
		cancel:         cancel,
		items:          make(map[string]*ItemResult),
		currentItems:   make(map[string]struct{}),
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

// SetBlueprint mencatat hasil inspect dan menyiapkan daftar item (satu per
// mount, plus satu entri sintetis untuk image) — dipanggil sekali di awal
// run(), sebelum transfer per-item dimulai.
func (j *Job) SetBlueprint(bp *docker.MigrationBlueprint) {
	j.SetPlan(bp, transferPlan{Mounts: bp.Mounts})
}

// SetPlan seperti SetBlueprint, tapi daftar item mengikuti rencana transfer:
// direktori proyek compose (kalau ikut disalin) PERTAMA, lalu mount yang
// tidak tercakup di dalamnya, lalu image.
func (j *Job) SetPlan(bp *docker.MigrationBlueprint, plan transferPlan) {
	j.Blueprint = bp

	j.mu.Lock()
	defer j.mu.Unlock()
	if plan.Workdir != "" {
		label := workdirLabel(plan.Workdir)
		j.items[label] = &ItemResult{Label: label, Status: ItemPending, Warning: coveredNote(plan.Covered)}
		j.itemOrder = append(j.itemOrder, label)
	}
	for _, m := range plan.Mounts {
		label := mountLabel(m)
		j.items[label] = &ItemResult{Label: label, Status: ItemPending}
		j.itemOrder = append(j.itemOrder, label)
	}
	imgLabel := "image:" + bp.Image
	j.items[imageItemLabel] = &ItemResult{Label: imgLabel, Status: ItemPending}
	j.itemOrder = append(j.itemOrder, imageItemLabel)
}

func mountLabel(m docker.MigrationMount) string {
	if m.Named() {
		return "volume:" + m.HostSide
	}
	return "bind:" + m.HostSide
}

// AddTotalBytes menambah taksiran ukuran total job (dipanggil per item saat
// scan awal, sebelum transfer item itu dimulai).
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

// markBytesComplete menyamakan doneBytes dengan totalBytes saat job selesai
// — alasan sama seperti filexfer: totalBytes taksiran mentah (du -sb),
// doneBytes byte stream (bisa terkompresi), dua angka yang tidak akan
// pernah pas sendiri kalau datanya kompresibel.
func (j *Job) markBytesComplete() {
	j.doneBytes.Store(j.totalBytes.Load())
}

// SetItemStatus memperbarui status satu item (key: label mount internal
// atau imageItemLabel).
func (j *Job) SetItemStatus(key, status, errMsg string) {
	j.mu.Lock()
	if r, ok := j.items[key]; ok {
		r.Status = status
		if errMsg != "" {
			r.Error = errMsg
		}
	}
	if status == ItemRunning {
		j.currentItems[key] = struct{}{}
	} else {
		delete(j.currentItems, key)
	}
	j.mu.Unlock()
	j.publish(true)
}

// SetItemWarning mencatat catatan non-fatal pada satu item (mis. tujuan
// sudah berisi data sebelum transfer dimulai) — tidak mengubah Status.
func (j *Job) SetItemWarning(key, warning string) {
	j.mu.Lock()
	if r, ok := j.items[key]; ok {
		r.Warning = warning
	}
	j.mu.Unlock()
	j.publish(true)
}

// SetItemVerify mencatat hasil verifikasi ringan satu item.
func (j *Job) SetItemVerify(key, verify, detail string) {
	j.mu.Lock()
	if r, ok := j.items[key]; ok {
		r.Verify = verify
		r.VerifyDetail = detail
	}
	j.mu.Unlock()
	j.publish(true)
}

// SetContainerRunning mencatat hasil verifikasi status container tujuan.
func (j *Job) SetContainerRunning(running bool) {
	j.containerRunning.Store(&running)
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
		if r, ok := j.items[key]; ok {
			current = append(current, r.Label)
		}
	}
	j.mu.Unlock()

	var finishedAt *time.Time
	if v := j.finishedAt.Load(); v != nil {
		finishedAt, _ = v.(*time.Time)
	}
	var running *bool
	if v := j.containerRunning.Load(); v != nil {
		running, _ = v.(*bool)
	}

	containerName := ""
	if j.Blueprint != nil {
		containerName = j.Blueprint.Name
	}

	return Progress{
		JobID:            j.ID,
		Status:           status,
		SourceServerID:   j.SourceServerID,
		DestServerID:     j.DestServerID,
		ContainerID:      j.ContainerID,
		ContainerName:    containerName,
		Compress:         j.Compress,
		CopyWorkdir:      j.CopyWorkdir,
		TotalBytes:       total,
		DoneBytes:        done,
		Percent:          percent,
		CurrentItems:     current,
		Items:            items,
		ContainerRunning: running,
		Message:          msg,
		StartedAt:        j.startedAt,
		FinishedAt:       finishedAt,
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
