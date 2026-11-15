package website

import (
	"os/exec"
	"strings"
	"testing"
)

// Skrip pemasangan certbot untuk keluarga RPM dijalankan di shell SUNGGUHAN.
//
// Yang diperbaiki di sini adalah sebuah KEPUTUSAN — mengaktifkan EPEL atau
// tidak — dan keputusan itu hidup di if/else shell, bukan di Go. Mencocokkan
// teks skripnya saja tidak akan menangkap kesalahan logikanya.
//
// Bug aslinya: di Rocky, AlmaLinux, CentOS, dan RHEL, certbot ada di EPEL,
// bukan di BaseOS/AppStream. `dnf install -y certbot` polos berhenti dengan
// "No match for argument: certbot" — galat yang terdengar seperti salah
// ketik nama paket padahal yang kurang adalah satu repositori.

// stubCertbot memalsukan package manager-nya.
//
// infoOK menentukan apakah `dnf -q info certbot` berhasil, yaitu apakah
// certbot terjangkau dari repositori yang sekarang aktif. punyaBin
// menentukan apakah certbot sudah terpasang SEBELUM skrip jalan.
//
// Stub-nya mencatat pemasangan: sesudah `install -y certbot` berhasil,
// `command -v certbot` ikut berhasil. Tanpa itu stub-nya memodelkan server
// yang tidak mungkin ada — paket terpasang tapi binarinya tidak muncul —
// dan setiap kasus uji gagal di baris verifikasi terakhir skripnya.
func stubCertbot(pm string, infoOK, punyaBin bool) string {
	info := "return 1"
	if infoOK {
		info = "return 0"
	}
	awal := "0"
	if punyaBin {
		awal = "1"
	}
	return `
_ada_certbot=` + awal + `
` + pm + `() {
  if [ "$1" = "-q" ] && [ "$2" = "info" ]; then ` + info + `; fi
  if [ "$1" = "install" ]; then
    for _a in "$@"; do case "$_a" in *certbot*) _ada_certbot=1 ;; esac; done
  fi
  echo "PM $*"
  return 0
}
command() {
  if [ "$1" = "-v" ] && [ "$2" = "certbot" ]; then
    if [ "$_ada_certbot" = 1 ]; then echo /usr/bin/certbot; return 0; fi
    return 1
  fi
  builtin command "$@"
}
`
}

func jalankanCertbot(t *testing.T, pm, id, ver, stub string) string {
	t.Helper()
	sh, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash tidak tersedia")
	}
	pre := "export ID=" + id + " VERSION_ID=" + ver + "\n"
	out, err := exec.Command(sh, "-c", pre+stub+"\n"+skripCertbotRPM(pm)).CombinedOutput()
	if err != nil {
		t.Fatalf("skrip gagal: %v\n%s", err, out)
	}
	return string(out)
}

// Inilah kasus yang dilaporkan: Rocky 8.10, certbot tidak ada di repo aktif.
func TestSkripCertbotRPM_RockyMengaktifkanEPEL(t *testing.T) {
	out := jalankanCertbot(t, "dnf", "rocky", "8.10", stubCertbot("dnf", false, false))

	if !strings.Contains(out, "mengaktifkan EPEL") {
		t.Errorf("tidak mengaktifkan EPEL:\n%s", out)
	}
	for _, mau := range []string{"PM install -y epel-release", "PM makecache", "PM install -y certbot"} {
		if !strings.Contains(out, mau) {
			t.Errorf("tidak memuat %q:\n%s", mau, out)
		}
	}
	// epel-release HARUS dipasang sebelum certbot, atau pemasangannya
	// mengulangi kegagalan yang sama persis.
	if strings.Index(out, "install -y epel-release") > strings.Index(out, "install -y certbot") {
		t.Errorf("certbot dipasang sebelum EPEL aktif:\n%s", out)
	}
}

// Fedora TIDAK boleh mendapat EPEL: certbot ada di repo utamanya, dan EPEL
// memang tidak ada untuk Fedora. Memasangnya di sana berarti memaksa sebuah
// paket yang tidak akan pernah ditemukan.
func TestSkripCertbotRPM_FedoraTidakMenyentuhEPEL(t *testing.T) {
	out := jalankanCertbot(t, "dnf", "fedora", "43", stubCertbot("dnf", true, false))
	if strings.Contains(out, "EPEL") || strings.Contains(out, "epel-release") {
		t.Errorf("menyentuh EPEL padahal certbot sudah terjangkau:\n%s", out)
	}
	if !strings.Contains(out, "PM install -y certbot") {
		t.Errorf("certbot tidak jadi dipasang:\n%s", out)
	}
}

// Server yang EPEL-nya SUDAH aktif dari sebelumnya juga tidak perlu
// menyentuhnya lagi — pemeriksaannya bertanya "certbot terjangkau?", bukan
// "distro ini keluarga RHEL?".
func TestSkripCertbotRPM_EPELSudahAktif(t *testing.T) {
	out := jalankanCertbot(t, "dnf", "rocky", "8.10", stubCertbot("dnf", true, false))
	if strings.Contains(out, "mengaktifkan EPEL") {
		t.Errorf("mengaktifkan EPEL yang sudah aktif:\n%s", out)
	}
	if !strings.Contains(out, "sudah terjangkau") {
		t.Errorf("tidak melaporkan bahwa certbot sudah terjangkau:\n%s", out)
	}
}

// certbot yang sudah TERPASANG tidak memicu apa pun, walau `info` gagal.
func TestSkripCertbotRPM_SudahTerpasang(t *testing.T) {
	out := jalankanCertbot(t, "dnf", "rocky", "8.10", stubCertbot("dnf", false, true))
	if strings.Contains(out, "mengaktifkan EPEL") {
		t.Errorf("mengaktifkan EPEL padahal certbot sudah terpasang:\n%s", out)
	}
}

// Nama paket pengaktif EPEL berbeda per distro, jadi ketiga jalur resminya
// harus benar-benar dicoba berurutan — bukan berhenti di yang pertama gagal.
func TestSkripCertbotRPM_TigaJalurEPELDicobaBerurutan(t *testing.T) {
	sh, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash tidak tersedia")
	}
	// Hanya rpm langsung dari Fedora Project yang berhasil — keadaan RHEL,
	// yang memang tidak membawa epel-release sendiri.
	stub := `
_ada_certbot=0
dnf() {
  if [ "$1" = "-q" ] && [ "$2" = "info" ]; then return 1; fi
  for _a in "$@"; do
    case "$_a" in
      epel-release|oracle-epel-release-el8) echo "PM GAGAL $_a"; return 1 ;;
      certbot) _ada_certbot=1 ;;
    esac
  done
  echo "PM $*"
  return 0
}
command() {
  if [ "$1" = "-v" ] && [ "$2" = "certbot" ]; then
    if [ "$_ada_certbot" = 1 ]; then echo /usr/bin/certbot; return 0; fi
    return 1
  fi
  builtin command "$@"
}
`
	pre := "export ID=rhel VERSION_ID=8.10\n"
	out, err := exec.Command(sh, "-c", pre+stub+"\n"+skripCertbotRPM("dnf")).CombinedOutput()
	if err != nil {
		t.Fatalf("skrip gagal padahal jalur ketiga berhasil: %v\n%s", err, out)
	}
	teks := string(out)
	for _, mau := range []string{
		"PM GAGAL epel-release",
		"PM GAGAL oracle-epel-release-el8",
		"epel-release-latest-8.noarch.rpm",
		"PM install -y certbot",
	} {
		if !strings.Contains(teks, mau) {
			t.Errorf("tidak memuat %q:\n%s", mau, teks)
		}
	}
	// Versi minor tidak boleh terbawa: epel-release-latest-8.10 tidak ada.
	if strings.Contains(teks, "latest-8.10") {
		t.Errorf("versi minor terbawa ke nama rpm EPEL:\n%s", teks)
	}
}

func TestSkripCertbotRPM_SintaksnyaSah(t *testing.T) {
	sh, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash tidak tersedia")
	}
	for _, pm := range []string{"dnf", "yum"} {
		script, ok := installCertbotScript(pm)
		if !ok {
			t.Fatalf("%s tidak didukung", pm)
		}
		wrapped := "(\n" + script + "\n) 2>&1\necho \"__poinhost_INSTALL_DONE__:$?\""
		if out, err := exec.Command(sh, "-n", "-c", wrapped).CombinedOutput(); err != nil {
			t.Errorf("%s: sintaks tidak sah: %v\n%s", pm, err, out)
		}
	}
}
