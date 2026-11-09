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
	"errors"
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
// DNSMati dipakai DUA kali: oleh Telaah, sesudah pemasangan gagal, dan oleh
// pemeriksaan awal sebelum pemasangan dimulai. Kalimatnya satu supaya user
// tidak membaca dua penjelasan berbeda untuk keadaan yang sama.
var DNSMati = Diagnosa{
	Sebab:   "server tidak bisa menerjemahkan nama domain (DNS), jadi repositori paket tidak terjangkau",
	Periksa: "cat /etc/resolv.conf && getent hosts deb.debian.org",
}

var daftar = []pola{
	{
		cocok: []string{
			"could not resolve host",
			"temporary failure resolving",
			"name or service not known",
			"curl error (6)",
		},
		hasil: DNSMati,
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

// SkripPeriksaDNS memeriksa server masih bisa menerjemahkan nama publik.
//
// Dijalankan SEBELUM pemasangan, bukan hanya ditebak sesudahnya: dnf dan apt
// butuh puluhan detik sampai menyerah, dan selama itu layar penuh keluaran
// yang mengarahkan perhatian ke repositori padahal resolvernya yang mati.
//
// Lulus kalau SATU saja nama berhasil diterjemahkan. Memakai satu nama saja
// akan menuduh DNS padahal yang diblokir cuma host itu — dan menolak
// memasang karena tuduhan yang salah jauh lebih merugikan daripada
// membiarkan dnf mencoba.
//
// getent bagian dari glibc dan ada di Debian maupun RHEL. Kalau toh tidak
// ada, pemeriksaannya LULUS: menolak memasang karena alat pemeriksanya
// hilang akan menghalangi pemasangan yang sebenarnya sehat.
// Kode keluar skrip pemeriksaan. Dibedakan karena yang harus dilakukan
// pemilik server juga berbeda: resolver yang mati diperbaiki di
// /etc/resolv.conf, sedangkan satu host repo yang tidak terjawab sementara
// resolvernya sehat menunjuk ke penyaringan atau konfigurasi repo.
const (
	periksaOK        = 0
	periksaDNSMati   = 1
	periksaRepoBuntu = 2
)

// skripPeriksa membangun pemeriksaan untuk satu host repo tertentu.
//
// DUA tahap, dan itu intinya. Versi pertama hanya mencoba beberapa nama dan
// lulus kalau SATU saja berhasil — sehingga server yang resolvernya hidup
// tapi tidak bisa menerjemahkan mirror distronya sendiri lolos, lalu gagal
// di dnf beberapa puluh detik kemudian. Persis yang terjadi di Rocky.
//
// Tahap pertama tetap memakai beberapa nama netral: satu nama saja akan
// menuduh DNS mati padahal yang diblokir cuma host itu. Tahap kedua memakai
// nama yang BENAR-BENAR dibutuhkan instalasi ini.
func skripPeriksa(hostRepo string) string {
	return `command -v getent >/dev/null 2>&1 || exit ` + itoa(periksaOK) + `
_umum=0
for h in deb.debian.org archive.ubuntu.com mirrors.fedoraproject.org; do
  getent hosts "$h" >/dev/null 2>&1 && { _umum=1; break; }
done
[ "$_umum" = 1 ] || exit ` + itoa(periksaDNSMati) + `
_repo=` + shellQuote(hostRepo) + `
[ -n "$_repo" ] || exit ` + itoa(periksaOK) + `
getent hosts "$_repo" >/dev/null 2>&1 || exit ` + itoa(periksaRepoBuntu) + `
exit ` + itoa(periksaOK)
}

// HostRepo adalah nama host yang BENAR-BENAR dibutuhkan pemasangan di
// distro ini. Kosong berarti tidak diketahui — dan tahap kedua dilewati,
// bukan ditebak.
func HostRepo(pm, distroID string) string {
	switch distroID {
	case "rocky":
		return "mirrors.rockylinux.org"
	case "almalinux":
		return "mirrors.almalinux.org"
	case "fedora", "centos", "rhel", "ol", "oracle":
		return "mirrors.fedoraproject.org"
	case "debian":
		return "deb.debian.org"
	case "ubuntu":
		return "archive.ubuntu.com"
	case "alpine":
		return "dl-cdn.alpinelinux.org"
	}
	// Distro tidak dikenal: tebak dari package managernya, dan kalau itu pun
	// tidak menunjuk ke satu tempat yang pasti, lewati saja.
	switch pm {
	case "apk":
		return "dl-cdn.alpinelinux.org"
	}
	return ""
}

// RepoBuntu dipakai saat resolver SEHAT tapi mirror distronya sendiri tidak
// terjawab. Sengaja berbeda dari DNSMati: menyuruh orang memeriksa
// /etc/resolv.conf padahal resolvernya baik-baik saja membuang waktunya di
// tempat yang salah.
func RepoBuntu(host string) Diagnosa {
	return Diagnosa{
		Sebab: "DNS server berjalan, tapi " + host + " — mirror yang dibutuhkan " +
			"distro ini — tidak bisa diterjemahkan",
		Periksa: "getent hosts " + host + " ; grep -rn mirror /etc/yum.repos.d/ /etc/apt/sources.list* 2>/dev/null | head",
	}
}

// Periksa menjalankan pemeriksaan lewat runner pemanggilnya, lalu
// menerjemahkan kode keluarnya.
//
// Runner mengembalikan kode keluar dan galat transport. Galat transport
// (SSH putus, timeout) TIDAK menghalangi pemasangan: yang diketahui cuma
// "pemeriksaan tidak selesai", bukan "DNS rusak", dan menolak atas dasar itu
// menukar satu kegagalan yang jelas dengan satu lagi yang membingungkan.
func Periksa(hostRepo string, jalankan func(script string) (int, error)) error {
	kode, err := jalankan(skripPeriksa(hostRepo))
	if err != nil {
		return nil
	}
	switch kode {
	case periksaDNSMati:
		return errors.New(PesanAwal(DNSMati))
	case periksaRepoBuntu:
		return errors.New(PesanAwal(RepoBuntu(hostRepo)))
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func itoa(n int) string { return strconv.Itoa(n) }

// PesanAwal menyusun galat untuk pemeriksaan yang gagal SEBELUM pemasangan
// dimulai. Tanpa kode keluar, karena memang belum ada yang dijalankan.
func PesanAwal(d Diagnosa) string {
	out := d.Sebab
	if out != "" {
		out = strings.ToUpper(out[:1]) + out[1:]
	}
	if d.Periksa != "" {
		out += ". Periksa di server: " + d.Periksa
	}
	return out
}
