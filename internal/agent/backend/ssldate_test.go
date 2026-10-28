package backend

import (
	"testing"
	"time"
)

// `openssl x509 -enddate` mengeluarkan format milik openssl, bukan standar
// mana pun. Diuraikan sekali di agent supaya tiap klien tidak punya
// kesempatan sendiri untuk salah membacanya.
func TestUraiTanggalSertifikat(t *testing.T) {
	kasus := []struct {
		raw  string
		mau  time.Time
		bisa bool
		nama string
	}{
		{
			nama: "keluaran openssl, tanggal dua digit",
			raw:  "Dec 24 09:12:33 2026 GMT",
			mau:  time.Date(2026, time.December, 24, 9, 12, 33, 0, time.UTC),
			bisa: true,
		},
		{
			// Hari satu digit diratakan dengan DUA spasi oleh openssl.
			// Layout "_2" yang menanganinya; "02" akan gagal di sini.
			nama: "keluaran openssl, tanggal satu digit",
			raw:  "Jan  5 00:00:00 2027 GMT",
			mau:  time.Date(2027, time.January, 5, 0, 0, 0, 0, time.UTC),
			bisa: true,
		},
		{nama: "kosong", raw: "", bisa: false},
		{nama: "bukan tanggal", raw: "tidak ada sertifikat", bisa: false},
	}

	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			got, ok := uraiTanggalSertifikat(k.raw)
			if ok != k.bisa {
				t.Fatalf("ok = %v, mau %v", ok, k.bisa)
			}
			if k.bisa && !got.UTC().Equal(k.mau) {
				t.Errorf("waktu = %v, mau %v", got.UTC(), k.mau)
			}
		})
	}
}
