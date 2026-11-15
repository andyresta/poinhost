package docker

import (
	"os/exec"
	"strings"
	"testing"
)

// Skrip pemasangan untuk keluarga RPM dijalankan di shell SUNGGUHAN, dengan
// curl dan package manager yang dipalsukan.
//
// Mencocokkan teksnya saja tidak cukup di sini: yang diperbaiki justru
// sebuah KEPUTUSAN — repositori mana yang dipakai — dan keputusan itu hidup
// di dalam if/else shell, bukan di Go. Bug aslinya pun bukan salah ketik:
// get.docker.com memilih jalur repo dari ID distro, dan jalur
// download.docker.com/linux/rocky memuat containerd.io beserta seluruh
// plugin TAPI tidak memuat docker-ce — sehingga pemasangan baru gagal
// sesudah dua menit, dengan galat yang tidak menyebut sebabnya.

// stubRPM memalsukan semua yang menyentuh dunia luar.
//
// punyaEngine menentukan jalur mana yang "berisi" docker-ce, persis seperti
// yang dilihat skrip saat membaca indeks paket Docker.
func stubRPM(punyaEngine ...string) string {
	var cocok strings.Builder
	for _, j := range punyaEngine {
		cocok.WriteString("    *linux/" + j + "/*) echo 'docker-ce-29.0.0-1.el8.x86_64.rpm' ;;\n")
	}
	return `
curl() {
  _url=""
  for _a in "$@"; do case "$_a" in https://*) _url="$_a" ;; esac; done
  case "$_url" in
` + cocok.String() + `    *) : ;;
  esac
  return 0
}
dnf() { echo "PM $*"; }
yum() { echo "PM $*"; }
sh() { echo "GETDOCKER $*"; }
systemctl() { :; }
docker() { :; }
sleep() { :; }
rm() { :; }
`
}

func jalankanRPM(t *testing.T, bin, id, ver, stub string) string {
	t.Helper()
	shBin, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash tidak tersedia")
	}
	// ID/VERSION_ID lewat environment: /etc/os-release tidak ada di mesin
	// yang menjalankan tes, dan skripnya memang sudah memaafkan itu.
	pre := "export ID=" + id + " VERSION_ID=" + ver + "\n"
	cmd := exec.Command(shBin, "-c", pre+stub+"\n"+skripRPM(bin))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("skrip gagal: %v\n%s", err, out)
	}
	return string(out)
}

// Inilah kasus yang dilaporkan: Rocky 8.10, jalur rocky tanpa docker-ce.
func TestSkripRPM_RockyJatuhKeJalurCentos(t *testing.T) {
	out := jalankanRPM(t, "dnf", "rocky", "8.10", stubRPM("centos"))

	if !strings.Contains(out, "memakai jalur resmi centos/8") {
		t.Errorf("tidak berpindah ke jalur centos:\n%s", out)
	}
	// VERSION_ID 8.10 harus jadi 8 — centos/8.10 tidak ada.
	if strings.Contains(out, "centos/8.10") {
		t.Errorf("versi minor ikut terbawa ke jalur repo:\n%s", out)
	}
	for _, mau := range []string{"PM makecache", "PM install -y docker-ce docker-ce-cli containerd.io"} {
		if !strings.Contains(out, mau) {
			t.Errorf("tidak memuat %q:\n%s", mau, out)
		}
	}
	// get.docker.com TIDAK dipakai di jalur ini — ia yang memilih repo yang
	// salah, jadi menjalankannya lagi hanya mengulang kegagalan yang sama.
	if strings.Contains(out, "GETDOCKER") {
		t.Errorf("masih menjalankan get.docker.com:\n%s", out)
	}
}

// Kalau Docker melengkapi jalur rocky, kode ini harus KEMBALI memakai
// get.docker.com dengan sendirinya. Itu sebabnya jalurnya diperiksa, bukan
// ditanam sebagai daftar distro.
func TestSkripRPM_JalurDistroSendiriDipakaiKalauLengkap(t *testing.T) {
	out := jalankanRPM(t, "dnf", "rocky", "8.10", stubRPM("rocky", "centos"))
	if !strings.Contains(out, "GETDOCKER") {
		t.Errorf("tidak memakai get.docker.com padahal jalur rocky sudah lengkap:\n%s", out)
	}
	if strings.Contains(out, "jalur resmi centos") {
		t.Errorf("berpindah ke centos padahal tidak perlu:\n%s", out)
	}
}

// CentOS sendiri tidak boleh mendapat pesan "memakai jalur centos" — jalur
// itu memang jalurnya sendiri, dan kalimat itu akan membingungkan.
func TestSkripRPM_CentOSTidakMengumumkanPerpindahan(t *testing.T) {
	out := jalankanRPM(t, "yum", "centos", "9", stubRPM("centos"))
	if strings.Contains(out, "memakai jalur resmi centos") {
		t.Errorf("mengumumkan perpindahan ke jalurnya sendiri:\n%s", out)
	}
	if !strings.Contains(out, "GETDOCKER") {
		t.Errorf("centos tidak memakai get.docker.com:\n%s", out)
	}
}

// Pemeriksaan yang TIDAK BISA menjawab — tidak ada jaringan ke indeks paket,
// proxy memblokir, HTML berubah bentuk — tidak boleh menghentikan
// pemasangan. Yang diketahui cuma "tidak bisa memeriksa", bukan "repo ini
// rusak", dan menolak atas dasar itu menukar satu kegagalan dengan satu
// lagi yang lebih membingungkan.
func TestSkripRPM_PemeriksaanBuntuTetapMencoba(t *testing.T) {
	out := jalankanRPM(t, "dnf", "rocky", "8.10", stubRPM())
	if !strings.Contains(out, "GETDOCKER") {
		t.Errorf("pemeriksaan yang buntu menghentikan pemasangan:\n%s", out)
	}
}

// Server tanpa /etc/os-release tidak boleh menghentikan instalasi di baris
// pembacaannya — di bawah set -e itulah yang dulu akan terjadi.
func TestSkripRPM_TanpaOsReleaseTidakBerhenti(t *testing.T) {
	shBin, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash tidak tersedia")
	}
	cmd := exec.Command(shBin, "-c", "unset ID VERSION_ID\n"+stubRPM("centos")+"\n"+skripRPM("dnf"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("skrip berhenti padahal cuma nama distronya yang hilang: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Selesai") {
		t.Errorf("skrip tidak sampai ke akhir:\n%s", out)
	}
}

func TestSkripRPM_SintaksnyaSah(t *testing.T) {
	shBin, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash tidak tersedia")
	}
	for _, bin := range []string{"dnf", "yum"} {
		script, ok := installDockerScript(bin)
		if !ok {
			t.Fatalf("%s tidak didukung", bin)
		}
		wrapped := "(\n" + script + "\n) 2>&1\necho \"__poinhost_INSTALL_DONE__:$?\""
		if out, err := exec.Command(shBin, "-n", "-c", wrapped).CombinedOutput(); err != nil {
			t.Errorf("%s: sintaks tidak sah: %v\n%s", bin, err, out)
		}
	}
}
