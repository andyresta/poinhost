// Package pkgfail menerjemahkan kegagalan pemasangan paket di server remote
// menjadi kalimat yang bisa ditindaklanjuti.
//
// Alasannya: instalasi dialirkan baris demi baris ke layar, dan sebab
// sebenarnya — satu baris di antara ratusan baris keluaran dnf/apt — sudah
// menggulung jauh ke atas saat prosesnya berhenti. Yang tersisa di layar
// cuma "proses gagal dengan kode keluar 1", yang tidak menyebut apa pun yang
// bisa diperbaiki.
//
// Pemetaannya sengaja PENDEK. Tiap pola di sini adalah kegagalan yang benar
// benar sering terjadi dan punya satu langkah pemeriksaan yang jelas; yang
// jarang atau ambigu lebih baik dibiarkan apa adanya daripada ditebak.
package pkgfail

import (
	"strconv"
	"strings"
)

// Diagnosa adalah satu penjelasan beserta perintah pemeriksaannya.
type Diagnosa struct {
	// Sebab, dalam satu kalimat, dalam bahasa pemilik server.
	Sebab string

	// Periksa adalah perintah yang menegaskan atau membantah Sebab.
	// Kosong kalau tidak ada satu perintah yang cukup.
	Periksa string
}

type pola struct {
	cocok []string
	hasil Diagnosa
}

// Urutannya menentukan: yang lebih spesifik harus lebih dulu, karena satu
// keluaran gagal sering memicu beberapa pola sekaligus. DNS, misalnya, juga
// memunculkan "Failed to download metadata" — dan yang berguna disebut
// adalah DNS-nya, bukan akibatnya.
var daftar = []pola{
	{
		cocok: []string{
			"could not resolve host",
			"temporary failure resolving",
			"name or service not known",
			"curl error (6)",
		},
		hasil: Diagnosa{
			Sebab:   "server tidak bisa menerjemahkan nama domain (DNS), jadi repositori paket tidak terjangkau",
			Periksa: "cat /etc/resolv.conf && getent hosts deb.debian.org",
		},
	},
	{
		cocok: []string{
			"could not get lock",
			"unable to acquire the dpkg frontend lock",
			"another app is currently holding the yum lock",
			"existing lock",
		},
		hasil: Diagnosa{
			Sebab:   "ada proses lain yang sedang memakai manajer paket",
			Periksa: "ps aux | grep -E 'apt|dnf|yum' | grep -v grep",
		},
	},
	{
		cocok: []string{"no space left on device"},
		hasil: Diagnosa{
			Sebab:   "disk server penuh",
			Periksa: "df -h /",
		},
	},
	{
		cocok: []string{
			"connection timed out",
			"couldn't connect to server",
			"failed to connect to",
			"network is unreachable",
		},
		hasil: Diagnosa{
			Sebab:   "server tidak bisa menjangkau repositori paket lewat jaringan",
			Periksa: "curl -sS -o /dev/null -w '%{http_code}\\n' https://deb.debian.org",
		},
	},
	{
		// Paling umum, jadi paling akhir: ia benar untuk banyak sebab, dan
		// tiap pola di atas lebih berguna kalau yang cocok memang itu.
		cocok: []string{
			"failed to download metadata for repo",
			"404 not found",
			"no more mirrors to try",
		},
		hasil: Diagnosa{
			Sebab:   "repositori paket tidak bisa dibaca",
			Periksa: "dnf repolist || apt-get update",
		},
	},
}

// Telaah mencari sebab kegagalan dari keluaran pemasangan.
//
// Yang diperiksa hanya ekor keluaran: galat yang menghentikan pemasangan
// selalu ada di dekat akhir, sementara di awal sering ada peringatan lama
// yang sudah dilewati dan tidak ada hubungannya.
func Telaah(baris []string) (Diagnosa, bool) {
	const ekor = 60
	if len(baris) > ekor {
		baris = baris[len(baris)-ekor:]
	}
	gabung := strings.ToLower(strings.Join(baris, "\n"))
	for _, p := range daftar {
		for _, c := range p.cocok {
			if strings.Contains(gabung, c) {
				return p.hasil, true
			}
		}
	}
	return Diagnosa{}, false
}

// Pesan menyusun kalimat galat akhir untuk satu pemasangan yang gagal.
//
// Kode keluarnya tetap disebut: ia tidak berguna bagi pemilik server, tapi
// sangat berguna saat ia menempelkan pesan ini ke sebuah laporan.
func Pesan(exitCode int, baris []string) string {
	dasar := "instalasi gagal (kode keluar " + strconv.Itoa(exitCode) + ")"
	d, ok := Telaah(baris)
	if !ok {
		return dasar
	}
	out := dasar + " — " + d.Sebab
	if d.Periksa != "" {
		out += ". Periksa di server: " + d.Periksa
	}
	return out
}
