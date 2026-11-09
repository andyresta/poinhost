package pkgfail

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// Keluaran asli dnf di Rocky 8 saat resolvernya mati. Inilah yang memicu
// seluruh paket ini: yang tersisa di layar user cuma "kode keluar 1",
// sementara sebabnya ada di baris-baris ini dan sudah menggulung ke atas.
var rockyDNSMati = []string{
	"Rocky Linux 8 - AppStream       0.0  B/s |   0  B     00:00",
	"Errors during downloading metadata for repository 'appstream':",
	"  - Curl error (6): Couldn't resolve host name for https://mirrors.rockylinux.org/mirrorlist?arch=x86_64&repo=AppStream-8 [Could not resolve host: mirrors.rockylinux.org]",
	"Error: Failed to download metadata for repo 'appstream': Cannot prepare internal mirrorlist: Curl error (6)",
}

func TestTelaah_DNSDidahulukanDariAkibatnya(t *testing.T) {
	d, ok := Telaah(rockyDNSMati)
	if !ok {
		t.Fatal("tidak mengenali kegagalan DNS")
	}
	// Keluaran yang sama JUGA memuat "Failed to download metadata for repo".
	// Yang berguna disebut adalah DNS-nya, bukan akibatnya — kalau urutan
	// polanya terbalik, user disuruh memeriksa repo padahal resolvernya yang
	// mati.
	if !strings.Contains(d.Sebab, "DNS") {
		t.Errorf("sebab = %q, mau menyebut DNS", d.Sebab)
	}
	if d.Periksa == "" {
		t.Error("tidak ada perintah pemeriksaan")
	}
}

func TestTelaah(t *testing.T) {
	kasus := []struct {
		nama  string
		baris []string
		mau   string
	}{
		{
			nama:  "apt, resolver mati",
			baris: []string{"Err:1 http://deb.debian.org/debian bookworm InRelease", "  Temporary failure resolving 'deb.debian.org'"},
			mau:   "DNS",
		},
		{
			nama:  "apt, lock dipegang proses lain",
			baris: []string{"E: Could not get lock /var/lib/dpkg/lock-frontend"},
			mau:   "manajer paket",
		},
		{
			nama:  "disk penuh",
			baris: []string{"dpkg: unrecoverable fatal error: No space left on device"},
			mau:   "penuh",
		},
		{
			nama:  "jaringan buntu, bukan DNS",
			baris: []string{"Could not connect to archive.ubuntu.com:80 (1.2.3.4), connection timed out"},
			mau:   "jaringan",
		},
		{
			nama:  "repo rusak tanpa sebab yang lebih spesifik",
			baris: []string{"Error: Failed to download metadata for repo 'epel'"},
			mau:   "repositori",
		},
	}

	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			d, ok := Telaah(k.baris)
			if !ok {
				t.Fatal("tidak dikenali")
			}
			if !strings.Contains(d.Sebab, k.mau) {
				t.Errorf("sebab = %q, mau memuat %q", d.Sebab, k.mau)
			}
		})
	}
}

// Yang tidak dikenali dibiarkan apa adanya. Menebak sebab yang salah lebih
// buruk daripada tidak menyebut sebab sama sekali: user akan memeriksa hal
// yang keliru lebih dulu.
func TestTelaah_YangTidakDikenaliTidakDitebak(t *testing.T) {
	if _, ok := Telaah([]string{"Preparing to unpack .../nginx_1.22.1-9_amd64.deb ..."}); ok {
		t.Error("keluaran normal dianggap kegagalan")
	}
	if got := Pesan(1, []string{"sesuatu yang tidak dikenal"}); got != "instalasi gagal (kode keluar 1)" {
		t.Errorf("pesan = %q", got)
	}
}

// Galat yang menghentikan pemasangan selalu ada di dekat akhir; peringatan
// lama di awal sering sudah dilewati dan tidak ada hubungannya.
func TestTelaah_HanyaEkorKeluaran(t *testing.T) {
	baris := []string{"E: Could not get lock /var/lib/dpkg/lock-frontend"}
	for i := 0; i < 100; i++ {
		baris = append(baris, "Unpacking paket-"+strconv.Itoa(i))
	}
	if _, ok := Telaah(baris); ok {
		t.Error("galat lama di awal keluaran masih dianggap sebab kegagalannya")
	}
}

func TestPesan_MenyebutKodeDanLangkahnya(t *testing.T) {
	got := Pesan(1, rockyDNSMati)
	for _, mau := range []string{"kode keluar 1", "DNS", "Periksa di server:"} {
		if !strings.Contains(got, mau) {
			t.Errorf("pesan tidak memuat %q:\n%s", mau, got)
		}
	}
}

// Kalimatnya satu, dipakai sebelum maupun sesudah pemasangan. Kalau
// DNSMati lepas dari daftar polanya, user membaca dua penjelasan berbeda
// untuk keadaan yang persis sama.
func TestDNSMati_SatuKalimatUntukKeduaJalur(t *testing.T) {
	sesudah, ok := Telaah(rockyDNSMati)
	if !ok {
		t.Fatal("Telaah tidak mengenali DNS mati")
	}
	if sesudah != DNSMati {
		t.Errorf("diagnosa dari Telaah berbeda dari DNSMati:\n%+v\n%+v", sesudah, DNSMati)
	}

	awal := PesanAwal(DNSMati)
	if !strings.Contains(awal, "DNS") || !strings.Contains(awal, "Periksa di server:") {
		t.Errorf("pesan awal = %q", awal)
	}
	// Berdiri sendiri sebagai kalimat, jadi huruf pertamanya besar — beda
	// dari Pesan(), yang menyisipkannya di tengah kalimat.
	if awal[0] < 'A' || awal[0] > 'Z' {
		t.Errorf("pesan awal tidak diawali huruf besar: %q", awal)
	}
}

// Pemeriksaannya DUA tahap, dan tahap keduanya yang menutup lubang nyata:
// versi pertama lulus begitu satu nama berhasil, sehingga server yang
// resolvernya hidup tapi tidak bisa menerjemahkan mirror distronya sendiri
// lolos — lalu gagal di dnf beberapa puluh detik kemudian.
func TestSkripPeriksa_MemeriksaMirrorDistroJuga(t *testing.T) {
	skrip := skripPeriksa("mirrors.rockylinux.org")

	if !strings.Contains(skrip, "mirrors.rockylinux.org") {
		t.Error("host repo tidak ikut diperiksa")
	}
	// Tahap pertama TIDAK boleh memuat host distronya, atau ia akan lulus
	// lewat jalur umum dan tahap kedua jadi tidak pernah menentukan.
	tahap1 := skrip[:strings.Index(skrip, "_repo=")]
	if strings.Contains(tahap1, "rockylinux") {
		t.Error("host distro ikut di tahap umum — tahap kedua jadi percuma")
	}
	// getent yang hilang membuat pemeriksaan LULUS: menolak memasang karena
	// alat pemeriksanya tidak ada akan menghalangi pemasangan yang sehat.
	if !strings.HasPrefix(skrip, "command -v getent >/dev/null 2>&1 || exit 0") {
		t.Error("getent yang hilang tidak membuat pemeriksaan lulus")
	}
}

func TestSkripPeriksa_TanpaHostRepoHanyaTahapUmum(t *testing.T) {
	skrip := skripPeriksa("")
	if !strings.Contains(skrip, `_repo=''`) {
		t.Errorf("host kosong tidak dilewati dengan benar:\n%s", skrip)
	}
}

func TestHostRepo(t *testing.T) {
	kasus := map[string]string{
		"rocky":     "mirrors.rockylinux.org",
		"almalinux": "mirrors.almalinux.org",
		"debian":    "deb.debian.org",
		"ubuntu":    "archive.ubuntu.com",
		"alpine":    "dl-cdn.alpinelinux.org",
	}
	for id, mau := range kasus {
		if got := HostRepo("", id); got != mau {
			t.Errorf("HostRepo(%q) = %q, mau %q", id, got, mau)
		}
	}
	// Distro tak dikenal: tahap kedua DILEWATI, bukan ditebak. Menebak
	// host yang salah akan menolak pemasangan yang sebenarnya sehat.
	if got := HostRepo("", "distro-aneh"); got != "" {
		t.Errorf("distro tak dikenal menebak host %q", got)
	}
}

// Dua kegagalan, dua kalimat: resolver mati diperbaiki di /etc/resolv.conf,
// sedangkan satu mirror yang tidak terjawab sementara resolvernya sehat
// menunjuk ke tempat yang sama sekali lain.
func TestPeriksa_MembedakanDuaKegagalan(t *testing.T) {
	jalan := func(kode int) func(string) (int, error) {
		return func(string) (int, error) { return kode, nil }
	}

	if err := Periksa("mirrors.rockylinux.org", jalan(periksaOK)); err != nil {
		t.Errorf("lulus malah dianggap gagal: %v", err)
	}

	err := Periksa("mirrors.rockylinux.org", jalan(periksaDNSMati))
	if err == nil || !strings.Contains(err.Error(), "resolv.conf") {
		t.Errorf("DNS mati = %v", err)
	}

	err = Periksa("mirrors.rockylinux.org", jalan(periksaRepoBuntu))
	if err == nil || !strings.Contains(err.Error(), "mirrors.rockylinux.org") {
		t.Errorf("repo buntu = %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "resolv.conf") {
		t.Error("repo buntu menyuruh memeriksa resolv.conf padahal resolvernya sehat")
	}

	// Pemeriksaan yang GAGAL DIJALANKAN tidak menghalangi pemasangan.
	gagal := func(string) (int, error) { return 0, errors.New("ssh putus") }
	if err := Periksa("x", gagal); err != nil {
		t.Errorf("pemeriksaan yang tidak selesai menghalangi pemasangan: %v", err)
	}
}
