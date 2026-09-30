package docker

import (
	"strings"
	"testing"
)

// Pemeriksaan nama harus menanyakan CONTAINER saja.
//
// `docker inspect` tanpa subperintah adalah inspector serba guna: ia
// mencocokkan container, image, volume, dan network. Migrasi Docker
// memindahkan image yang namanya sama dengan containernya (devpoin-app dan
// devpoin-app:latest), jadi dengan perintah lama, menghapus containernya di
// server tujuan tidak mengubah apa pun — yang ditemukan adalah image-nya,
// dan migrasi tetap ditolak dengan alasan "nama sudah dipakai".
func TestContainerNameExistsCmd_AsksForContainerOnly(t *testing.T) {
	cmd := containerNameExistsCmd("devpoin-app")

	if !strings.Contains(cmd, "docker container inspect") {
		t.Errorf("tidak membatasi ke container: %s", cmd)
	}
	// Penjagaan atas regresi: `docker inspect` polos akan mencocokkan image
	// lagi. Yang boleh muncul hanya bentuk `docker container inspect`.
	if strings.Contains(strings.ReplaceAll(cmd, "docker container inspect", ""), "docker inspect") {
		t.Errorf("masih memakai docker inspect serba guna: %s", cmd)
	}
	if !strings.Contains(cmd, "'devpoin-app'") {
		t.Errorf("nama tidak dikutip aman: %s", cmd)
	}
	// Jawabannya dibaca dari stdout, jadi kedua cabang harus mencetak
	// sesuatu — bukan hanya bergantung exit code.
	if !strings.Contains(cmd, "echo yes") || !strings.Contains(cmd, "echo no") {
		t.Errorf("tidak mencetak jawaban ya/tidak: %s", cmd)
	}
}

func TestContainerNameExistsCmd_QuotesAwkwardNames(t *testing.T) {
	cmd := containerNameExistsCmd("it's-app")
	// Paket ini mengutip dengan gaya '"'"' — tutup kutip, kutip literal,
	// buka lagi. Sama amannya dengan '\'' , hanya beda idiom.
	if !strings.Contains(cmd, `'it'"'"'s-app'`) {
		t.Errorf("kutip tunggal tidak di-escape: %s", cmd)
	}
}
