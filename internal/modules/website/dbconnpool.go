package website

import (
	"database/sql"
	"strings"
	"time"
)

// dbConnCacheEntry satu koneksi Explore (MySQL ATAU PostgreSQL) yang
// ditahan hidup, dipakai ulang oleh panggilan-panggilan berikutnya untuk
// kredensial yang sama — lihat getOrDialDBConn.
type dbConnCacheEntry struct {
	db       *sql.DB
	lastUsed time.Time
}

// dbDialWaiter menandai satu proses dial yang sedang berjalan untuk satu
// key tertentu — kalau ada panggilan LAIN untuk key yang SAMA datang
// selagi dial pertama masih berlangsung, panggilan itu menunggu hasil dial
// yang sama alih-alih ikut membuka koneksi kedua (yang kalau dibiarkan,
// salah satunya bakal langsung ditutup lagi oleh yang lain dan menyebabkan
// pemanggil yang "kalah" itu gagal dengan "database is closed").
type dbDialWaiter struct {
	done chan struct{}
	db   *sql.DB
	err  error
}

// dbConnIdleTTL — koneksi yang tidak dipakai selama ini ditutup otomatis
// (disapu lazy setiap ada akses baru) supaya tidak menahan koneksi ke
// server/database yang sudah lama tidak di-browse. 5 menit cukup untuk
// satu sesi klik-klik pindah tabel/halaman, tapi tidak menumpuk koneksi
// selamanya.
const dbConnIdleTTL = 5 * time.Minute

// dbConnCacheKeyPrefix membangun bagian AWAL key cache (engine+server+user),
// dipakai baik untuk membangun key penuh (dbConnCacheKey) maupun untuk
// evictDBConnsWithPrefix (menghapus SEMUA koneksi ter-cache milik satu
// kredensial, lintas discriminator).
func dbConnCacheKeyPrefix(engine, serverID, username string) string {
	return engine + "\x00" + serverID + "\x00" + username + "\x00"
}

// dbConnCacheKey membangun key cache — engine diikutkan supaya kredensial
// MySQL & PostgreSQL untuk server/username yang kebetulan sama tidak
// tertukar; discriminator adalah host (MySQL, grant host mis. "%") atau
// nama database (PostgreSQL, karena satu koneksi Postgres SELALU terikat
// ke satu database — beda dari MySQL yang bisa lintas database dalam satu
// koneksi via USE).
func dbConnCacheKey(engine, serverID, username, discriminator string) string {
	return dbConnCacheKeyPrefix(engine, serverID, username) + discriminator
}

// getOrDialDBConn adalah mekanisme cache-koneksi GENERIK dipakai bersama
// oleh mysqlexplore.go dan pgexplore.go — supaya kedua engine berbagi
// PERSIS logika yang sama (reuse antar panggilan, idle sweep, dedup dial
// bersamaan) alih-alih mengimplementasikannya dua kali secara terpisah dan
// berisiko salah satunya luput dari perbaikan/bug yang sudah ditemukan di
// yang lain. dial dipanggil HANYA saat cache miss (dan HANYA oleh satu
// goroutine per key, lihat dbDialing).
func (s *Service) getOrDialDBConn(key string, dial func() (*sql.DB, error)) (*sql.DB, error) {
	s.dbConnMu.Lock()
	s.sweepIdleDBConnsLocked()
	if entry, ok := s.dbConns[key]; ok {
		entry.lastUsed = time.Now()
		db := entry.db
		s.dbConnMu.Unlock()
		return db, nil
	}
	if w, ok := s.dbDialing[key]; ok {
		s.dbConnMu.Unlock()
		<-w.done
		return w.db, w.err
	}
	w := &dbDialWaiter{done: make(chan struct{})}
	s.dbDialing[key] = w
	s.dbConnMu.Unlock()

	db, err := dial()
	w.db, w.err = db, err
	close(w.done)

	s.dbConnMu.Lock()
	delete(s.dbDialing, key)
	if err == nil {
		s.dbConns[key] = &dbConnCacheEntry{db: db, lastUsed: time.Now()}
	}
	s.dbConnMu.Unlock()

	return db, err
}

// sweepIdleDBConnsLocked menutup & membuang entry yang sudah idle lebih
// dari dbConnIdleTTL. Dipanggil dengan dbConnMu SUDAH terkunci.
func (s *Service) sweepIdleDBConnsLocked() {
	now := time.Now()
	for key, entry := range s.dbConns {
		if now.Sub(entry.lastUsed) > dbConnIdleTTL {
			_ = entry.db.Close()
			delete(s.dbConns, key)
		}
	}
}

// putCachedDBConn menyimpan koneksi langsung ke cache (dipakai oleh
// verifyMySQLCredential/verifyPGCredential — koneksi hasil verifikasi
// password langsung dipakai ulang, tidak ditutup) — menutup entry lama
// untuk key yang sama dulu kalau ada.
func (s *Service) putCachedDBConn(key string, db *sql.DB) {
	s.dbConnMu.Lock()
	defer s.dbConnMu.Unlock()
	if old, ok := s.dbConns[key]; ok {
		_ = old.db.Close()
	}
	s.dbConns[key] = &dbConnCacheEntry{db: db, lastUsed: time.Now()}
}

// evictDBConn menutup & membuang koneksi cache untuk satu kredensial —
// dipanggil saat password kredensial itu berubah (SaveDBCredential) atau
// dilupakan (ForgetDBCredential), supaya operasi Explore berikutnya tidak
// diam-diam masih memakai koneksi dari password lama.
func (s *Service) evictDBConn(key string) {
	s.dbConnMu.Lock()
	defer s.dbConnMu.Unlock()
	if entry, ok := s.dbConns[key]; ok {
		_ = entry.db.Close()
		delete(s.dbConns, key)
	}
}

// evictDBConnsWithPrefix menutup & membuang SEMUA koneksi cache yang key-nya
// diawali prefix tertentu — dipakai PostgreSQL, karena satu role bisa
// punya BANYAK koneksi ter-cache sekaligus (satu per database yang pernah
// di-browse, sebab koneksi Postgres selalu terikat ke satu database) —
// beda dari MySQL yang cukup satu entry per (server,user,host) sehingga
// evictDBConn (exact key) sudah cukup.
func (s *Service) evictDBConnsWithPrefix(prefix string) {
	s.dbConnMu.Lock()
	defer s.dbConnMu.Unlock()
	for key, entry := range s.dbConns {
		if strings.HasPrefix(key, prefix) {
			_ = entry.db.Close()
			delete(s.dbConns, key)
		}
	}
}

// CloseAllDBConns menutup semua koneksi Explore (MySQL & PostgreSQL) yang
// masih tertahan di cache — dipanggil saat aplikasi ditutup (lihat app.go:
// shutdown), sejalan dengan pool.Close() untuk koneksi SSH.
func (s *Service) CloseAllDBConns() {
	s.dbConnMu.Lock()
	defer s.dbConnMu.Unlock()
	for key, entry := range s.dbConns {
		_ = entry.db.Close()
		delete(s.dbConns, key)
	}
}
