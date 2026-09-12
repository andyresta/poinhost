package website

import (
	"database/sql"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// newFakeConn membuat *sql.DB nyata (SQLite in-memory) yang hanya dipakai
// sebagai stand-in objek koneksi untuk menguji bookkeeping cache generik di
// dbconnpool.go (dipakai bersama MySQL & PostgreSQL Explore) — kita tidak
// butuh MySQL/PostgreSQL sungguhan di sini, cuma memastikan Close() benar-
// benar dipanggil pada objek yang seharusnya ditutup, dan TIDAK dipanggil
// pada objek yang seharusnya tetap hidup.
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

func newTestDBCacheService() *Service {
	return &Service{
		dbConns:   make(map[string]*dbConnCacheEntry),
		dbDialing: make(map[string]*dbDialWaiter),
	}
}

// TestDBConnCache_ReuseAcrossCalls memastikan operasi Explore yang berurutan
// untuk kredensial yang sama memakai ULANG koneksi yang sama — sebelumnya
// BUKAN kasusnya (setiap panggilan dial+close baru), menyebabkan handshake
// berulang di tiap klik.
func TestDBConnCache_ReuseAcrossCalls(t *testing.T) {
	s := newTestDBCacheService()
	key := dbConnCacheKey("mysql", "srv1", "appuser", "%")

	db1 := newFakeConn(t)
	s.putCachedDBConn(key, db1)

	s.dbConnMu.Lock()
	entry, ok := s.dbConns[key]
	s.dbConnMu.Unlock()
	if !ok || entry.db != db1 {
		t.Fatalf("koneksi yang baru disimpan harus langsung bisa dipakai ulang dari cache")
	}
	if isClosed(db1) {
		t.Fatalf("koneksi yang masih dipakai tidak boleh tertutup")
	}
}

// TestDBConnCache_SavePasswordEvictsOldConnection memastikan menyimpan
// password baru untuk kredensial yang sama (mis. lewat SaveDBCredential)
// menutup koneksi lama — supaya operasi Explore berikutnya tidak diam-diam
// masih memakai sesi yang terautentikasi dengan password sebelumnya.
func TestDBConnCache_SavePasswordEvictsOldConnection(t *testing.T) {
	s := newTestDBCacheService()
	key := dbConnCacheKey("mysql", "srv1", "appuser", "%")

	oldConn := newFakeConn(t)
	s.putCachedDBConn(key, oldConn)

	s.evictDBConn(key)

	if !isClosed(oldConn) {
		t.Fatalf("koneksi lama harus ditutup setelah evict (dipanggil saat password diganti)")
	}
	s.dbConnMu.Lock()
	_, stillCached := s.dbConns[key]
	s.dbConnMu.Unlock()
	if stillCached {
		t.Fatalf("entry cache harus hilang setelah evict")
	}
}

// TestDBConnCache_EvictWithPrefixClosesAllDatabasesForRole memastikan
// evictDBConnsWithPrefix (dipakai PostgreSQL — satu role bisa punya BANYAK
// koneksi ter-cache sekaligus, satu per database) menutup SEMUA koneksi
// milik role itu, TIDAK menyentuh koneksi role/server lain.
func TestDBConnCache_EvictWithPrefixClosesAllDatabasesForRole(t *testing.T) {
	s := newTestDBCacheService()
	appDB1 := dbConnCacheKey("postgresql", "srv1", "appuser", "app1")
	appDB2 := dbConnCacheKey("postgresql", "srv1", "appuser", "app2")
	otherRole := dbConnCacheKey("postgresql", "srv1", "otheruser", "app1")

	conn1, conn2, conn3 := newFakeConn(t), newFakeConn(t), newFakeConn(t)
	s.putCachedDBConn(appDB1, conn1)
	s.putCachedDBConn(appDB2, conn2)
	s.putCachedDBConn(otherRole, conn3)

	s.evictDBConnsWithPrefix(dbConnCacheKeyPrefix("postgresql", "srv1", "appuser"))

	if !isClosed(conn1) || !isClosed(conn2) {
		t.Fatalf("semua koneksi milik role yang di-evict harus tertutup")
	}
	if isClosed(conn3) {
		t.Fatalf("koneksi role lain tidak boleh ikut tertutup")
	}
	s.dbConnMu.Lock()
	_, stillHasOther := s.dbConns[otherRole]
	s.dbConnMu.Unlock()
	if !stillHasOther {
		t.Fatalf("entry role lain harus tetap ada di cache")
	}
}

// TestDBConnCache_IdleSweepClosesOnlyStaleEntries memastikan sweep idle
// menutup koneksi yang sudah lama tidak dipakai TAPI tidak menyentuh
// koneksi yang masih aktif dipakai (lastUsed baru).
func TestDBConnCache_IdleSweepClosesOnlyStaleEntries(t *testing.T) {
	s := newTestDBCacheService()

	staleKey := dbConnCacheKey("mysql", "srv1", "old", "%")
	freshKey := dbConnCacheKey("mysql", "srv1", "fresh", "%")
	staleConn := newFakeConn(t)
	freshConn := newFakeConn(t)

	s.dbConnMu.Lock()
	s.dbConns[staleKey] = &dbConnCacheEntry{db: staleConn, lastUsed: time.Now().Add(-2 * dbConnIdleTTL)}
	s.dbConns[freshKey] = &dbConnCacheEntry{db: freshConn, lastUsed: time.Now()}
	s.sweepIdleDBConnsLocked()
	_, staleStillThere := s.dbConns[staleKey]
	_, freshStillThere := s.dbConns[freshKey]
	s.dbConnMu.Unlock()

	if staleStillThere || !isClosed(staleConn) {
		t.Fatalf("koneksi idle lama seharusnya disapu & ditutup")
	}
	if !freshStillThere || isClosed(freshConn) {
		t.Fatalf("koneksi yang baru dipakai tidak boleh ikut disapu")
	}
}

// TestDBConnCache_ConcurrentFirstDialIsDeduped memanggil getOrDialDBConn
// (fungsi ASLI yang dipakai getOrDialMySQLExplore & getOrDialPGExplore, bukan
// tiruan logikanya) — kalau BANYAK panggilan datang bersamaan untuk key yang
// SAMA sebelum ada apa pun di cache, hanya SATU yang benar-benar "dial",
// sisanya menunggu & memakai hasil yang sama. Ini mencegah race di mana dua
// koneksi dibuat lalu salah satunya langsung ditutup oleh yang lain,
// menyebabkan pemanggil yang kebagian koneksi itu gagal dengan
// "database is closed".
func TestDBConnCache_ConcurrentFirstDialIsDeduped(t *testing.T) {
	s := newTestDBCacheService()
	key := "shared-key"

	var dialCount int32
	var dialCountMu sync.Mutex
	dial := func() (*sql.DB, error) {
		dialCountMu.Lock()
		dialCount++
		dialCountMu.Unlock()
		time.Sleep(20 * time.Millisecond) // simulasikan latensi handshake
		return newFakeConn(t), nil
	}

	const n = 20
	results := make([]*sql.DB, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db, err := s.getOrDialDBConn(key, dial)
			if err != nil {
				t.Errorf("getOrDialDBConn: %v", err)
			}
			results[i] = db
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
