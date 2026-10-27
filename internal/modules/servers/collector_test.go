package servers

import (
	"testing"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
)

// enqueueDue melewati server yang SUDAH diketahui offline supaya tidak
// membuang waktu di timeout. Yang tidak boleh ikut terlewat adalah server
// yang belum pernah diprobe sama sekali: sshpool.Pool melaporkan "offline"
// untuk server yang belum pernah di-dial, dan probe metrik inilah satu-
// satunya yang men-dial di agent.
//
// Tanpa pembedaan itu keduanya saling mengunci — metrik dilewati karena
// offline, status tetap offline karena tidak ada yang menyambung — dan
// agent melaporkan CPU, RAM, serta disk nol berjam-jam tanpa satu pun
// pesan galat.
func TestEnqueueDue_ServerYangBelumPernahTerbacaTetapDiprobeWalauOffline(t *testing.T) {
	c, jobs := collectorUji(t, "s1")

	// Persis keadaan agent yang baru start: connectionLoop sudah menaruh
	// entri berisi status koneksi saja, dan belum ada metrik sama sekali.
	c.cache["s1"] = ServerStatus{
		ID:         "s1",
		Connection: string(sshpool.StatusOffline),
	}
	c.nextDue["s1"] = time.Now().Add(-time.Second)

	c.enqueueDue(jobs)

	select {
	case got := <-jobs:
		if got != "s1" {
			t.Fatalf("mengantre %q, mau s1", got)
		}
	default:
		t.Fatal("server yang belum pernah terbaca tidak diantre — metriknya tidak akan pernah terkumpul")
	}
}

func TestEnqueueDue_ServerOfflineYangPernahTerbacaDilewati(t *testing.T) {
	c, jobs := collectorUji(t, "s1")

	// Sudah pernah terbaca, lalu mesinnya mati. Di sini melewatinya memang
	// benar: koneksinya pernah ada, jadi "offline" adalah hasil percobaan,
	// bukan sekadar belum dicoba.
	c.cache["s1"] = ServerStatus{
		ID:               "s1",
		Connection:       string(sshpool.StatusOffline),
		MetricsCheckedAt: time.Now().Add(-5 * time.Minute),
	}
	c.nextDue["s1"] = time.Now().Add(-time.Second)

	c.enqueueDue(jobs)

	select {
	case got := <-jobs:
		t.Fatalf("server offline yang sudah pernah terbaca ikut diantre (%q)", got)
	default:
	}

	// Dan dijadwalkan ulang, bukan ditinggalkan.
	if due := c.nextDue["s1"]; !due.After(time.Now()) {
		t.Errorf("nextDue = %v, mau di masa depan", due)
	}
}

// collectorUji membuat Collector dengan repo asli berisi satu server aktif.
// Pool dan executor sengaja nil: enqueueDue hanya membaca daftar server dan
// cache, tidak menyentuh SSH sama sekali.
func collectorUji(t *testing.T, serverID string) (*Collector, chan string) {
	t.Helper()
	repo := newTestRepo(t)
	srv := &Server{
		ID:       serverID,
		Name:     "uji",
		Host:     "127.0.0.1",
		Username: "root",
		AuthType: "key",
		IsActive: true,
	}
	if err := repo.Create(srv); err != nil {
		t.Fatalf("Create: %v", err)
	}
	c := NewCollector(&Service{repo: repo})
	return c, make(chan string, 4)
}
