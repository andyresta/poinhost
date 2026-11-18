package sshpool

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

// Pembungkus hitungannya diuji langsung, tanpa SSH.
//
// Yang menentukan di sini bukan angkanya, melainkan dua hal yang tidak
// terlihat sampai dipakai: bahwa laporan TERAKHIR selalu terkirim, dan
// bahwa laporannya dibatasi jedanya. Tanpa yang pertama, transfer yang
// selesai tepat sesudah satu laporan berhenti di 97% selamanya di layar.
// Tanpa yang kedua, satu berkas 1 GB memicu lebih dari tiga puluh ribu
// event dan antrean UI jadi pekerjaan yang lebih berat daripada
// transfernya sendiri.

type rekaman struct {
	done, total int64
}

func rekam(ke *[]rekaman) Progress {
	return func(done, total int64) { *ke = append(*ke, rekaman{done, total}) }
}

// penghitung dipakai untuk UNDUHAN: ia membungkus tujuannya.
func TestPenghitung_MenghitungSemuaYangDitulis(t *testing.T) {
	var got []rekaman
	var buf bytes.Buffer
	// terakhir dibuat jauh di masa lalu supaya tiap Write melapor, dan yang
	// diuji adalah hitungannya, bukan jedanya.
	p := &penghitung{w: &buf, total: 9, lapor: rekam(&got), terakhir: time.Time{}}

	if _, err := io.Copy(p, strings.NewReader("123456789")); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "123456789" {
		t.Errorf("isi yang ditulis berubah: %q", buf.String())
	}
	if p.done != 9 {
		t.Errorf("done = %d, mau 9", p.done)
	}
	if len(got) == 0 {
		t.Fatal("tidak melapor sama sekali")
	}
	if akhir := got[len(got)-1]; akhir.done != 9 || akhir.total != 9 {
		t.Errorf("laporan terakhir = %+v, mau {9 9}", akhir)
	}
}

// pembaca dipakai untuk UNGGAHAN: ia membungkus SUMBERNYA, bukan tujuannya.
//
// Itu bukan pilihan sembarangan — io.Copy hanya memakai jalur cepat
// File.ReadFrom milik pkg/sftp kalau tujuannya benar-benar *sftp.File.
// Membungkus tujuannya akan menyembunyikan jalur itu dan memperlambat
// unggahan demi sebuah bar.
func TestPembaca_MenghitungSemuaYangDibaca(t *testing.T) {
	var got []rekaman
	p := &pembaca{r: strings.NewReader("abcdefghij"), total: 10, lapor: rekam(&got), terakhir: time.Time{}}

	data, err := io.ReadAll(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abcdefghij" {
		t.Errorf("isi yang dibaca berubah: %q", data)
	}
	if p.done != 10 {
		t.Errorf("done = %d, mau 10", p.done)
	}
	if len(got) == 0 {
		t.Fatal("tidak melapor sama sekali — laporan terakhir hilang, dan barnya akan berhenti sebelum penuh")
	}
	if akhir := got[len(got)-1]; akhir.done != 10 {
		t.Errorf("laporan terakhir = %+v, mau done 10", akhir)
	}
}

// Jeda laporan benar-benar membatasi. Tanpa ini, io.Copy yang memanggil
// Write tiap 32 KiB akan membanjiri UI.
func TestJedaLapor_MembatasiBanyaknyaLaporan(t *testing.T) {
	var got []rekaman
	var buf bytes.Buffer
	// terakhir = sekarang, jadi tidak satu pun Write boleh melapor selama
	// penyalinannya berlangsung — seluruhnya selesai jauh di bawah 150 ms.
	p := &penghitung{w: &buf, total: 1 << 20, lapor: rekam(&got), terakhir: time.Now()}

	if _, err := io.Copy(p, bytes.NewReader(make([]byte, 1<<20))); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("melapor %d kali dalam satu jeda, mau 0", len(got))
	}
	// Hitungannya tetap berjalan walau laporannya ditahan — kalau tidak,
	// laporan terakhir dari pemanggil akan menyebut angka yang salah.
	if p.done != 1<<20 {
		t.Errorf("done = %d, mau %d", p.done, 1<<20)
	}
}

// lapor nil berarti pemanggil memang tidak ingin dilaporkan — jalur itu
// tetap harus menyalin dengan benar, bukan panik.
func TestTanpaPelapor(t *testing.T) {
	var buf bytes.Buffer
	w := &penghitung{w: &buf, total: 3}
	if _, err := io.Copy(w, strings.NewReader("abc")); err != nil {
		t.Fatal(err)
	}
	r := &pembaca{r: strings.NewReader("abc"), total: 3}
	if _, err := io.ReadAll(r); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "abc" || r.done != 3 {
		t.Errorf("salin tanpa pelapor rusak: %q / %d", buf.String(), r.done)
	}
}

// Ukuran yang tidak diketahui dibedakan dari berkas KOSONG.
//
// total 0 berarti berkasnya memang nol byte dan barnya boleh langsung
// penuh; total -1 berarti ukurannya tidak terbaca, dan layar harus
// menampilkan bar tak tentu alih-alih membagi dengan nol.
func TestTotalTidakDiketahuiDiteruskanApaAdanya(t *testing.T) {
	var got []rekaman
	p := &pembaca{r: strings.NewReader("xyz"), total: -1, lapor: rekam(&got), terakhir: time.Time{}}
	if _, err := io.ReadAll(p); err != nil {
		t.Fatal(err)
	}
	for _, r := range got {
		if r.total != -1 {
			t.Errorf("total berubah jadi %d; -1 harus diteruskan apa adanya", r.total)
		}
	}
}
