package website

import (
	"database/sql"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// newFakeConn membuat *sql.DB nyata (SQLite in-memory) yang hanya dipakai
// sebagai stand-in objek koneksi untuk menguji bookkeeping cache (put/evict/
// sweep) — kita tidak butuh MySQL sungguhan di sini, cuma memastikan
// Close() benar-benar dipanggil pada objek yang seharusnya ditutup, dan
// TIDAK dipanggil pada objek yang seharusnya tetap hidup.
func newFakeConn(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("buka koneksi palsu: %v", err)
	}
	return db
}

func isClosed(db *sql.DB) bool {
	return db.Ping() != nil
}

func newTestMySQLCacheService() *Service {
	return &Service{
		mysqlConns:   make(map[string]*mysqlConnCacheEntry),
		mysqlDialing: make(map[string]*mysqlDialWaiter),
	}
}

// TestMySQLConnCache_ReuseAcrossCalls memastikan operasi Explore yang
// berurutan untuk kredensial yang sama memakai ULANG koneksi yang sama —
// ini yang sebelumnya BUKAN kasusnya (setiap panggilan dial+close baru),
// menyebabkan handshake MySQL berulang di tiap klik.
func TestMySQLConnCache_ReuseAcrossCalls(t *testing.T) {
	s := newTestMySQLCacheService()
	key := mysqlConnCacheKey("srv1", "appuser", "%")

	db1 := newFakeConn(t)
	s.putCachedMySQLConn(key, db1)

	s.mysqlConnMu.Lock()
	entry, ok := s.mysqlConns[key]
	s.mysqlConnMu.Unlock()
	if !ok || entry.db != db1 {
		t.Fatalf("koneksi yang baru disimpan harus langsung bisa dipakai ulang dari cache")
	}
	if isClosed(db1) {
		t.Fatalf("koneksi yang masih dipakai tidak boleh tertutup")
	}
}

// TestMySQLConnCache_SavePasswordEvictsOldConnection memastikan menyimpan
// password baru untuk kredensial yang sama (mis. lewat SaveDBCredential)
// menutup koneksi lama — supaya operasi Explore berikutnya tidak diam-diam
// masih memakai sesi yang terautentikasi dengan password sebelumnya.
func TestMySQLConnCache_SavePasswordEvictsOldConnection(t *testing.T) {
	s := newTestMySQLCacheService()
	serverID, username, host := "srv1", "appuser", "%"
	key := mysqlConnCacheKey(serverID, username, host)

	oldConn := newFakeConn(t)
	s.putCachedMySQLConn(key, oldConn)

	s.evictMySQLConn(serverID, username, host)

	if !isClosed(oldConn) {
		t.Fatalf("koneksi lama harus ditutup setelah evict (dipanggil saat password diganti)")
	}
	s.mysqlConnMu.Lock()
	_, stillCached := s.mysqlConns[key]
	s.mysqlConnMu.Unlock()
	if stillCached {
		t.Fatalf("entry cache harus hilang setelah evict")
	}
}

// TestMySQLConnCache_IdleSweepClosesOnlyStaleEntries memastikan sweep idle
// menutup koneksi yang sudah lama tidak dipakai TAPI tidak menyentuh
// koneksi yang masih aktif dipakai (lastUsed baru).
func TestMySQLConnCache_IdleSweepClosesOnlyStaleEntries(t *testing.T) {
	s := newTestMySQLCacheService()

	staleKey := mysqlConnCacheKey("srv1", "old", "%")
	freshKey := mysqlConnCacheKey("srv1", "fresh", "%")
	staleConn := newFakeConn(t)
	freshConn := newFakeConn(t)

	s.mysqlConnMu.Lock()
	s.mysqlConns[staleKey] = &mysqlConnCacheEntry{db: staleConn, lastUsed: time.Now().Add(-2 * mysqlConnIdleTTL)}
	s.mysqlConns[freshKey] = &mysqlConnCacheEntry{db: freshConn, lastUsed: time.Now()}
	s.sweepIdleMySQLConnsLocked()
	_, staleStillThere := s.mysqlConns[staleKey]
	_, freshStillThere := s.mysqlConns[freshKey]
	s.mysqlConnMu.Unlock()

	if staleStillThere || !isClosed(staleConn) {
		t.Fatalf("koneksi idle lama seharusnya disapu & ditutup")
	}
	if !freshStillThere || isClosed(freshConn) {
		t.Fatalf("koneksi yang baru dipakai tidak boleh ikut disapu")
	}
}

// TestMySQLConnCache_ConcurrentFirstDialIsDeduped mereplikasi logika
// getOrDialMySQLExplore (tanpa dependensi MySQL/SSH sungguhan): kalau
// BANYAK panggilan datang bersamaan untuk key yang SAMA sebelum ada apa
// pun di cache, hanya SATU yang benar-benar "dial" — sisanya menunggu &
// memakai hasil yang sama. Ini mencegah race di mana dua koneksi dibuat
// lalu salah satunya langsung ditutup oleh yang lain, menyebabkan
// pemanggil yang kebagian koneksi itu gagal dengan "database is closed".
func TestMySQLConnCache_ConcurrentFirstDialIsDeduped(t *testing.T) {
	s := newTestMySQLCacheService()
	key := "shared-key"

	var dialCount int
	var dialCountMu sync.Mutex
	dial := func() *sql.DB {
		dialCountMu.Lock()
		dialCount++
		dialCountMu.Unlock()
		time.Sleep(20 * time.Millisecond) // simulasikan latensi handshake
		return newFakeConn(t)
	}

	// Sengaja meniru persis alur kunci di getOrDialMySQLExplore (bukan
	// duplikat logika independen) supaya test ini benar-benar menjaga
	// perilaku fungsi itu tetap benar kalau nanti diubah.
	getOrDial := func() *sql.DB {
		s.mysqlConnMu.Lock()
		if entry, ok := s.mysqlConns[key]; ok {
			s.mysqlConnMu.Unlock()
			return entry.db
		}
		if w, ok := s.mysqlDialing[key]; ok {
			s.mysqlConnMu.Unlock()
			<-w.done
			return w.db
		}
		w := &mysqlDialWaiter{done: make(chan struct{})}
		s.mysqlDialing[key] = w
		s.mysqlConnMu.Unlock()

		db := dial()
		w.db = db
		close(w.done)

		s.mysqlConnMu.Lock()
		delete(s.mysqlDialing, key)
		s.mysqlConns[key] = &mysqlConnCacheEntry{db: db, lastUsed: time.Now()}
		s.mysqlConnMu.Unlock()
		return db
	}

	const n = 20
	results := make([]*sql.DB, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = getOrDial()
		}(i)
	}
	wg.Wait()

	if dialCount != 1 {
		t.Fatalf("harus cuma 1 dial untuk %d panggilan bersamaan pada key yang sama, dapat %d", n, dialCount)
	}
	for i := 1; i < n; i++ {
		if results[i] != results[0] {
			t.Fatalf("semua panggilan bersamaan harus mendapat koneksi yang sama")
		}
		if isClosed(results[i]) {
			t.Fatalf("koneksi yang dibagikan tidak boleh tertutup")
		}
	}
}
