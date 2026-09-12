package servers

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
)

// Tuning default Collector. Sengaja konstanta (bukan config.Config) di
// skeleton ini — gampang dipromosikan ke field config kalau nanti perlu
// disetel per instalasi, tapi belum ada kebutuhan itu selama masih tahap
// awal. Lihat ARCHITECTURE.md untuk penjelasan tiap angka.
const (
	DefaultMetricsWorkers = 6
	DefaultActiveInterval = 15 * time.Second // server dengan tab terbuka
	DefaultIdleInterval   = 90 * time.Second // server tanpa tab terbuka
	DefaultConnectionPoll = 4 * time.Second  // baca status pool (gratis, tanpa SSH)
	DefaultSchedulerTick  = 5 * time.Second  // granularitas pengecekan jadwal metrik
	DefaultMetricsTimeout = 8 * time.Second
)

// Collector menjaga snapshot status tiap server & menjadwalkan pengambilan
// metrik secara efisien untuk populasi server yang bisa jadi banyak.
//
// Dua loop independen (lihat Start):
//  1. connectionLoop  — murah, sering: baca sshpool.Pool.Status() (tanpa SSH
//     sama sekali) untuk SEMUA server, emit HANYA kalau berubah.
//  2. metricsScheduler — lebih berat (SSH round-trip), pakai worker pool
//     terbatas + jadwal per-server yang di-stagger (acak) supaya tidak ada
//     "gelombang" cek serentak, dan interval berbeda tergantung apakah
//     server itu sedang "diperhatikan" (ada tab terbuka) atau tidak.
type Collector struct {
	svc *Service

	workers        int
	activeInterval time.Duration
	idleInterval   time.Duration
	metricsTimeout time.Duration

	mu       sync.RWMutex
	cache    map[string]ServerStatus
	subs     map[string]int // serverID -> jumlah tab yang sedang membukanya
	nextDue  map[string]time.Time
	inFlight map[string]bool
	emit     func(ServerStatus)

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewCollector membuat Collector dengan tuning default.
func NewCollector(svc *Service) *Collector {
	return &Collector{
		svc:            svc,
		workers:        DefaultMetricsWorkers,
		activeInterval: DefaultActiveInterval,
		idleInterval:   DefaultIdleInterval,
		metricsTimeout: DefaultMetricsTimeout,
		cache:          make(map[string]ServerStatus),
		subs:           make(map[string]int),
		nextDue:        make(map[string]time.Time),
		inFlight:       make(map[string]bool),
		stopCh:         make(chan struct{}),
	}
}

// SetEmitter mendaftarkan callback yang dipanggil tiap status berubah.
// app.go menghubungkan ini ke runtime.EventsEmit setelah context Wails siap,
// supaya package ini sendiri tidak perlu tahu apa-apa soal Wails.
func (c *Collector) SetEmitter(fn func(ServerStatus)) {
	c.mu.Lock()
	c.emit = fn
	c.mu.Unlock()
}

// Subscribe menandai server sedang "diperhatikan" (ada tab terbuka) — dicek
// metrik pada activeInterval yang lebih cepat, dan langsung dijadwalkan
// "jatuh tempo sekarang" supaya user yang baru buka tab tidak menunggu lama
// untuk lihat angka pertama.
func (c *Collector) Subscribe(serverID string) {
	c.mu.Lock()
	c.subs[serverID]++
	c.nextDue[serverID] = time.Now()
	c.mu.Unlock()
}

// Unsubscribe menurunkan prioritas server (kembali ke idleInterval) begitu
// tab terakhir yang membukanya ditutup.
func (c *Collector) Unsubscribe(serverID string) {
	c.mu.Lock()
	if c.subs[serverID] > 0 {
		c.subs[serverID]--
	}
	c.mu.Unlock()
}

// Snapshot mengembalikan status semua server yang pernah dicek (dipakai
// untuk isi awal store frontend saat app dibuka).
func (c *Collector) Snapshot() []ServerStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]ServerStatus, 0, len(c.cache))
	for _, st := range c.cache {
		out = append(out, st)
	}
	return out
}

// Forget membuang semua state (cache, jadwal, subscribe count) milik satu
// server — dipanggil saat server itu dihapus, supaya tidak ada "status
// hantu" untuk ID yang sudah tidak ada nyangkut selamanya di memori
// Collector (servers.List() otomatis berhenti mengembalikannya, tapi tanpa
// ini map internal Collector tidak pernah tahu untuk membersihkannya).
func (c *Collector) Forget(serverID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, serverID)
	delete(c.subs, serverID)
	delete(c.nextDue, serverID)
	delete(c.inFlight, serverID)
}

// RefreshNow memaksa pengambilan metrik SEKARANG untuk satu server, di luar
// jadwal — dipakai tombol "Refresh" manual di UI supaya terasa instan,
// tidak menunggu tick berikutnya.
func (c *Collector) RefreshNow(ctx context.Context, serverID string) (ServerStatus, error) {
	st, err := c.probeOne(ctx, serverID)
	if err != nil {
		return c.getOrEmpty(serverID), err
	}
	return st, nil
}

// Start menjalankan connectionLoop + worker pool metrik + scheduler-nya.
func (c *Collector) Start() {
	jobs := make(chan string, c.workers*2)
	for w := 0; w < c.workers; w++ {
		c.wg.Add(1)
		go c.metricsWorker(jobs)
	}
	c.wg.Add(1)
	go c.connectionLoop()
	c.wg.Add(1)
	go c.metricsSchedulerLoop(jobs)
}

// Stop menghentikan semua loop & worker dengan rapi.
func (c *Collector) Stop() {
	close(c.stopCh)
	c.wg.Wait()
}

// connectionLoop membaca status koneksi (gratis, dari sshpool.Pool) untuk
// SEMUA server setiap DefaultConnectionPoll, dan HANYA emit kalau nilainya
// berubah dari cache — supaya frontend tidak dibanjiri event identik.
func (c *Collector) connectionLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(DefaultConnectionPoll)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.pollConnections()
		}
	}
}

func (c *Collector) pollConnections() {
	list, err := c.svc.List()
	if err != nil {
		return
	}
	for _, srv := range list {
		conn := c.svc.ConnectionStatus(srv.ID)

		c.mu.Lock()
		cur, ok := c.cache[srv.ID]
		if !ok {
			cur = ServerStatus{ID: srv.ID}
		}
		changed := !ok || cur.Connection != conn
		cur.Connection = conn
		c.cache[srv.ID] = cur
		emitFn := c.emit
		c.mu.Unlock()

		if changed && emitFn != nil {
			emitFn(cur)
		}
	}
}

// metricsSchedulerLoop menentukan server mana yang "jatuh tempo" dicek
// metriknya, dengan stagger di percobaan pertama dan interval yang beda
// tergantung status subscribe — lalu mengantre ke worker pool (tidak
// pernah blocking: kalau antrean penuh, server itu dicoba lagi tick
// berikutnya, bukan menumpuk).
func (c *Collector) metricsSchedulerLoop(jobs chan<- string) {
	defer c.wg.Done()
	defer close(jobs)
	ticker := time.NewTicker(DefaultSchedulerTick)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.enqueueDue(jobs)
		}
	}
}

func (c *Collector) enqueueDue(jobs chan<- string) {
	list, err := c.svc.List()
	if err != nil {
		return
	}
	now := time.Now()

	for _, srv := range list {
		if !srv.IsActive {
			continue
		}

		c.mu.Lock()
		if c.inFlight[srv.ID] {
			c.mu.Unlock()
			continue
		}
		due, seen := c.nextDue[srv.ID]
		if !seen {
			// Kunjungan pertama: sebar jadwal cek pertama secara acak di
			// dalam satu window interval, supaya N server baru terdaftar
			// tidak semuanya "jatuh tempo" di detik yang sama.
			interval := c.intervalFor(srv.ID)
			jitter := time.Duration(rand.Int63n(int64(interval)))
			c.nextDue[srv.ID] = now.Add(jitter)
			c.mu.Unlock()
			continue
		}
		if now.Before(due) {
			c.mu.Unlock()
			continue
		}
		// Server sudah diketahui offline (dari connectionLoop, gratis) ->
		// jangan buang waktu ~1-2 detik mencoba metrik yang pasti timeout;
		// cukup jadwalkan ulang beberapa saat lagi untuk dicek kembali.
		if cur, ok := c.cache[srv.ID]; ok && cur.Connection == string(sshpool.StatusOffline) {
			c.nextDue[srv.ID] = now.Add(c.intervalFor(srv.ID))
			c.mu.Unlock()
			continue
		}
		c.inFlight[srv.ID] = true
		c.mu.Unlock()

		select {
		case jobs <- srv.ID:
		default:
			// Worker pool sedang penuh — lepas inFlight supaya dicoba lagi
			// tick berikutnya, bukan menunggu selamanya di sini.
			c.mu.Lock()
			c.inFlight[srv.ID] = false
			c.mu.Unlock()
		}
	}
}

// intervalFor mengasumsikan c.mu SUDAH dipegang pemanggil (read atau write)
// — sengaja tidak lock sendiri di sini karena semua pemanggilnya memanggil
// ini dari dalam critical section yang lain. sync.RWMutex Go tidak
// reentrant, jadi locking di sini akan deadlock kalau pemanggilnya sedang
// memegang write lock (Lock()).
func (c *Collector) intervalFor(serverID string) time.Duration {
	if c.subs[serverID] > 0 {
		return c.activeInterval
	}
	return c.idleInterval
}

func (c *Collector) metricsWorker(jobs <-chan string) {
	defer c.wg.Done()
	for serverID := range jobs {
		ctx, cancel := context.WithTimeout(context.Background(), c.metricsTimeout)
		_, _ = c.probeOne(ctx, serverID)
		cancel()

		c.mu.Lock()
		c.inFlight[serverID] = false
		c.nextDue[serverID] = time.Now().Add(c.intervalFor(serverID))
		c.mu.Unlock()
	}
}

// probeOne mengambil metrik satu server, memperbarui cache (mempertahankan
// nilai metrik terakhir yang berhasil kalau percobaan ini gagal — lihat
// komentar ServerStatus), lalu emit hasilnya.
func (c *Collector) probeOne(ctx context.Context, serverID string) (ServerStatus, error) {
	fresh, err := c.svc.fetchMetrics(ctx, serverID, c.metricsTimeout)

	c.mu.Lock()
	cur, ok := c.cache[serverID]
	if !ok {
		cur = ServerStatus{ID: serverID, Connection: c.svc.ConnectionStatus(serverID)}
	}
	if err != nil {
		cur.MetricsStale = true
		cur.MetricsError = err.Error()
	} else {
		id := cur.ID
		connection := c.svc.ConnectionStatus(serverID)
		cur = *fresh
		cur.ID = id
		cur.Connection = connection
		cur.MetricsStale = false
		cur.MetricsError = ""
	}
	c.cache[serverID] = cur
	emitFn := c.emit
	c.mu.Unlock()

	if emitFn != nil {
		emitFn(cur)
	}
	return cur, err
}

func (c *Collector) getOrEmpty(serverID string) ServerStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if st, ok := c.cache[serverID]; ok {
		return st
	}
	return ServerStatus{ID: serverID, Connection: c.svc.ConnectionStatus(serverID)}
}
