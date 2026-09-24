package dist

import (
	"errors"
	"strings"
	"testing"
)

func TestArchFromUname(t *testing.T) {
	ok := map[string]Arch{
		"x86_64":  AMD64,
		"amd64":   AMD64,
		"aarch64": ARM64,
		"arm64":   ARM64,
	}
	for in, want := range ok {
		got, err := ArchFromUname(in)
		if err != nil || got != want {
			t.Errorf("ArchFromUname(%q) = (%q,%v), mau %q", in, got, err, want)
		}
	}

	// Arsitektur yang tidak didukung harus ditolak dengan pesan yang
	// menyebutkan apa yang ADA — kalau tidak, user cuma melihat tombol mati
	// tanpa tahu sebabnya.
	for _, in := range []string{"armv7l", "riscv64", "i686", ""} {
		_, err := ArchFromUname(in)
		if err == nil {
			t.Errorf("ArchFromUname(%q) berhasil, mau error", in)
		} else if !strings.Contains(err.Error(), "x86_64") {
			t.Errorf("pesan error tidak menyebut arsitektur yang didukung: %v", err)
		}
	}
}

// Build tanpa binary agent (clone bersih yang belum menjalankan `make agent`)
// harus tetap bisa dikompilasi dan berjalan — yang nonaktif hanya tombol
// Pasang. Test ini ikut membuktikan embed direktori bin/ tidak mematahkan
// kompilasi saat isinya hanya README.
func TestBinary_TanpaBundleMengembalikanErrNotBundled(t *testing.T) {
	if Available(AMD64) {
		t.Skip("build ini membawa binary agent, jalur tanpa bundle tidak berlaku")
	}
	_, _, err := Binary(AMD64)
	if !errors.Is(err, ErrNotBundled) {
		t.Fatalf("Binary = %v, mau ErrNotBundled", err)
	}
}

// Kalau binary-nya ada, ia harus benar-benar bisa didekompresi dan checksum-nya
// konsisten — checksum inilah yang diverifikasi ulang di server sebelum
// dipasang.
func TestBinary_ChecksumStabilDanBisaDidekompresi(t *testing.T) {
	if !Available(AMD64) {
		t.Skip("build ini tidak membawa binary agent")
	}
	data, sum1, err := Binary(AMD64)
	if err != nil {
		t.Fatalf("Binary: %v", err)
	}
	if len(data) < 1<<20 {
		t.Errorf("binary hanya %d byte — kemungkinan bukan binary agent yang utuh", len(data))
	}
	// Binary Linux ELF selalu diawali 0x7F 'E' 'L' 'F'.
	if len(data) < 4 || string(data[1:4]) != "ELF" {
		t.Error("hasil dekompresi bukan binary ELF")
	}
	_, sum2, err := Binary(AMD64)
	if err != nil || sum1 != sum2 {
		t.Errorf("checksum tidak stabil antar panggilan: %q vs %q", sum1, sum2)
	}
}

// Versi dibaca dari berkas yang ditulis `make agent` BERSAMA binary-nya,
// bukan dari -ldflags terpisah saat app desktop dikompilasi.
//
// Ini menutup kelas bug yang sempat terjadi: binary agent basi ikut ter-embed
// tanpa satu pun tanda, karena versinya berasal dari sumber yang berbeda
// dengan binary-nya. Kalau keduanya berasal dari satu langkah build, keduanya
// tidak bisa lagi menyimpang.
func TestVersion_IkutBundleBinary(t *testing.T) {
	v := Version()
	if Bundled() && v == "" {
		t.Error("binary agent dibundel tapi versinya kosong — `make agent` tidak menulis version.txt")
	}
	if !Bundled() && v != "" {
		t.Errorf("versi %q dilaporkan padahal binary agent tidak dibundel", v)
	}
	if strings.ContainsAny(v, " \n\r\t") {
		t.Errorf("versi %q memuat spasi/baris baru — akan merusak perbandingan versi", v)
	}
}
