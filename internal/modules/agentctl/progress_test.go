package agentctl

import (
	"bytes"
	"io"
	"testing"
)

// Progress unggahan dihitung dari byte yang benar-benar terkirim, dan hanya
// dilaporkan saat angkanya BERUBAH: unggahan beberapa megabyte memanggil Read
// ribuan kali, dan mengirim event tiap panggilan akan membanjiri jembatan IPC
// tanpa menambah informasi apa pun.
func TestProgressReader_MelaporkanSekaliPerPersen(t *testing.T) {
	const size = 100000
	var laporan []int
	r := newProgressReader(bytes.NewReader(make([]byte, size)), size, 10, 70, func(p int) {
		laporan = append(laporan, p)
	})

	if _, err := io.Copy(io.Discard, r); err != nil {
		t.Fatalf("Copy: %v", err)
	}

	if len(laporan) == 0 {
		t.Fatal("tidak ada laporan progress sama sekali")
	}
	// Rentangnya 10..70, jadi paling banyak 61 nilai berbeda.
	if len(laporan) > 61 {
		t.Errorf("%d laporan untuk rentang 60 persen — ada nilai berulang", len(laporan))
	}
	if laporan[0] < 10 {
		t.Errorf("laporan pertama %d, mau >= 10 (batas bawah rentang)", laporan[0])
	}
	if got := laporan[len(laporan)-1]; got != 70 {
		t.Errorf("laporan terakhir %d, mau 70 (batas atas rentang)", got)
	}
	for i := 1; i < len(laporan); i++ {
		if laporan[i] <= laporan[i-1] {
			t.Fatalf("progress tidak naik monoton: %v", laporan)
		}
	}
}

// Ukuran nol tidak boleh membuat pembagian dengan nol.
func TestProgressReader_UkuranNolAman(t *testing.T) {
	var dipanggil bool
	r := newProgressReader(bytes.NewReader(nil), 0, 0, 100, func(int) { dipanggil = true })
	if _, err := io.Copy(io.Discard, r); err != nil {
		t.Fatal(err)
	}
	if dipanggil {
		t.Error("melaporkan progress untuk unggahan kosong")
	}
}

// Persentase tidak boleh melewati batas fase berikutnya meski waktunya lewat.
func TestMin1(t *testing.T) {
	for in, want := range map[float64]float64{0: 0, 0.5: 0.5, 1: 1, 2.5: 1} {
		if got := min1(in); got != want {
			t.Errorf("min1(%v) = %v, mau %v", in, got, want)
		}
	}
}
