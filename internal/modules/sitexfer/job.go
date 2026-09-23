package sitexfer

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const emitEvery = 200 * time.Millisecond

// Job state satu migrasi website.
type Job struct {
	ID  string
	Req StartRequest

	status  atomic.Value // string
	message atomic.Value // string

	totalBytes atomic.Int64
	doneBytes  atomic.Int64

	startedAt  time.Time
	finishedAt atomic.Value // *time.Time

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	items  map[string]*ItemResult
	order  []string
	report []string

	emitMu   sync.Mutex
	emit     func(Progress)
	lastEmit time.Time
}

func newJob(parent context.Context, req StartRequest, emit func(Progress)) *Job {
	ctx, cancel := context.WithCancel(parent)
	j := &Job{
		ID: uuid.NewString(), Req: req, startedAt: time.Now().UTC(),
		ctx: ctx, cancel: cancel, items: map[string]*ItemResult{}, emit: emit,
	}
	j.status.Store(StatusQueued)
	j.message.Store("")
	return j
}

func (j *Job) Context() context.Context { return j.ctx }

func (j *Job) Cancel() {
	if j.cancel != nil {
		j.cancel()
	}
}

func (j *Job) IsTerminal() bool {
	st, _ := j.status.Load().(string)
	return st == StatusDone || st == StatusPartial || st == StatusFailed || st == StatusCanceled
}

func (j *Job) SetStatus(status, message string) {
	j.status.Store(status)
	j.message.Store(message)
	if status == StatusDone || status == StatusPartial || status == StatusFailed || status == StatusCanceled {
		now := time.Now().UTC()
		j.finishedAt.Store(&now)
	}
	j.publish(true)
}

// AddItem mendaftarkan satu langkah (urutan pendaftaran = urutan tampil).
func (j *Job) AddItem(key, kind, label string) {
	j.mu.Lock()
	if _, ok := j.items[key]; !ok {
		j.items[key] = &ItemResult{Key: key, Kind: kind, Label: label, Status: ItemPending}
		j.order = append(j.order, key)
	}
	j.mu.Unlock()
}

func (j *Job) update(key string, fn func(*ItemResult)) {
	j.mu.Lock()
	if r, ok := j.items[key]; ok {
		fn(r)
	}
	j.mu.Unlock()
	j.publish(true)
}

func (j *Job) SetItem(key, status, errMsg string) {
	j.update(key, func(r *ItemResult) {
		r.Status = status
		if errMsg != "" {
			r.Error = errMsg
		}
	})
}

func (j *Job) SetWarning(key, w string) { j.update(key, func(r *ItemResult) { r.Warning = w }) }
func (j *Job) SetDetail(key, d string)  { j.update(key, func(r *ItemResult) { r.Detail = d }) }

func (j *Job) ItemStatus(key string) string {
	j.mu.Lock()
	defer j.mu.Unlock()
	if r, ok := j.items[key]; ok {
		return r.Status
	}
	return ""
}

func (j *Job) AddReport(line string) {
	j.mu.Lock()
	j.report = append(j.report, line)
	j.mu.Unlock()
}

func (j *Job) AddTotalBytes(n int64) {
	if n > 0 {
		j.totalBytes.Add(n)
	}
}

func (j *Job) AddDoneBytes(n int64) {
	if n > 0 {
		j.doneBytes.Add(n)
		j.publish(false)
	}
}

// HasFailures true kalau ada item yang gagal.
func (j *Job) HasFailures() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, r := range j.items {
		if r.Status == ItemFailed {
			return true
		}
	}
	return false
}

func (j *Job) Snapshot() Progress {
	j.mu.Lock()
	items := make([]ItemResult, 0, len(j.order))
	for _, k := range j.order {
		items = append(items, *j.items[k])
	}
	report := append([]string{}, j.report...)
	j.mu.Unlock()

	total, done := j.totalBytes.Load(), j.doneBytes.Load()
	percent := 0.0
	if total > 0 {
		percent = float64(done) / float64(total) * 100
		if percent > 100 {
			percent = 100
		}
	}
	status, _ := j.status.Load().(string)
	msg, _ := j.message.Load().(string)
	var fin *time.Time
	if v, ok := j.finishedAt.Load().(*time.Time); ok {
		fin = v
	}
	return Progress{
		JobID: j.ID, Status: status, SourceServerID: j.Req.SourceServerID, DestServerID: j.Req.DestServerID,
		Domains: j.Req.Domains, TotalBytes: total, DoneBytes: done, Percent: percent,
		Items: items, Report: report, Message: msg, StartedAt: j.startedAt, FinishedAt: fin,
	}
}

func (j *Job) publish(force bool) {
	if j.emit == nil {
		return
	}
	j.emitMu.Lock()
	if !force && time.Since(j.lastEmit) < emitEvery {
		j.emitMu.Unlock()
		return
	}
	j.lastEmit = time.Now()
	j.emitMu.Unlock()
	j.emit(j.Snapshot())
}
