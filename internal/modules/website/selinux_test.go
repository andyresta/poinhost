package website

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Helper SELinux dijalankan di shell SUNGGUHAN dengan SELinux yang
// dipalsukan — mesin yang menjalankan tes ini tidak punya SELinux sama
// sekali, dan itu justru salah satu keadaan yang wajib diuji.
//
// Mencocokkan teks skripnya saja tidak cukup: yang ditulis di sini adalah
// keputusan — enforcing atau tidak, boolean sudah menyala atau belum,
// perintahnya terpasang atau tidak — dan keputusan itu hidup di dalam shell.

// stubSELinux memalsukan apa yang dilihat helper tentang server.
//
// enforce: isi /sys/fs/selinux/enforce, atau "" kalau berkasnya tidak ada
// sama sekali (server tanpa SELinux, mis. Debian).
// boolOn: jawaban getsebool. adaAlat: apakah policycoreutils terpasang.
func stubSELinux(enforce string, boolOn, adaAlat bool) string {
	var b strings.Builder

	// /sys/fs/selinux/enforce tidak ada di mesin yang menjalankan tes ini,
	// jadi yang dipalsukan adalah jawabannya, bukan berkasnya.
	b.WriteString(`
_ENFORCE=` + shellQuote(enforce) + `
poinhost_selinux_enforcing() {
  [ -n "$_ENFORCE" ] || return 1
  [ "$_ENFORCE" = "1" ] || return 1
  return 0
}
`)

	if !adaAlat {
		b.WriteString(`
command() {
  if [ "$1" = "-v" ]; then
    case "$2" in
      getsebool|setsebool|restorecon|logger) return 1 ;;
    esac
  fi
  builtin command "$@"
}
`)
		return b.String()
	}

	on := "off"
	if boolOn {
		on = "on"
	}
	// Panggilannya DICATAT ke variabel, bukan dicetak: helper menjalankan
	// setsebool dan restorecon dengan `>/dev/null 2>&1`, jadi apa pun yang
	// mereka cetak tidak akan pernah terlihat — dan tes yang memeriksa
	// cetakan itu akan selalu lulus tanpa menguji apa-apa.
	b.WriteString(`
getsebool() { echo "httpd_can_network_connect --> ` + on + `"; }
setsebool() { _SETSEBOOL="$*"; return 0; }
restorecon() { _RESTORECON="$*"; return 0; }
logger() { _LOGGER="$*"; return 0; }
command() {
  if [ "$1" = "-v" ]; then
    case "$2" in
      getsebool|setsebool|restorecon|logger) echo "/usr/sbin/$2"; return 0 ;;
    esac
  fi
  builtin command "$@"
}
`)
	return b.String()
}

func jalankanSELinux(t *testing.T, stub, panggil string) string {
	t.Helper()
	sh, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash tidak tersedia")
	}
	// `set -e` ikut, karena helper ini memang disambung ke skrip yang
	// memakainya — dan satu perintah yang gagal di dalamnya akan
	// menghentikan pembuatan domain, bukan sekadar melewatkan langkah
	// SELinux-nya.
	//
	// Stub dipasang SESUDAH helper: definisi fungsi yang belakangan yang
	// berlaku, jadi stub di depan akan ditimpa helper dan tesnya diam-diam
	// menguji mesin yang menjalankannya.
	jejak := "\necho \"JEJAK setsebool=[$_SETSEBOOL] restorecon=[$_RESTORECON] logger=[$_LOGGER]\"\n"
	script := "set -e\n" + SELinuxHelper + "\n" + stub + "\n" + panggil + jejak + "echo SELESAI\n"

	out, err := exec.Command(sh, "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("skrip gagal: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "SELESAI") {
		t.Fatalf("skrip berhenti di tengah:\n%s", out)
	}
	return string(out)
}

// Rocky 8.10 apa adanya: enforcing, boolean mati, alatnya terpasang.
func TestSELinux_EnforcingMenyalakanBoolean(t *testing.T) {
	out := jalankanSELinux(t, stubSELinux("1", false, true), "poinhost_selinux_proxy")

	if !strings.Contains(out, "setsebool=[-P httpd_can_network_connect 1]") {
		t.Errorf("boolean tidak dinyalakan:\n%s", out)
	}
	// Perubahan kebijakan keamanan TIDAK boleh diam-diam: dicatat ke syslog
	// server — permanen, di mesin yang diubahnya — bukan cuma ke layar yang
	// mungkin sudah ditutup.
	if !strings.Contains(out, "logger=[-t poinhost") {
		t.Errorf("perubahan tidak dicatat ke syslog server:\n%s", out)
	}
	if !strings.Contains(out, selinuxTanda) {
		t.Errorf("perubahan tidak dilaporkan ke keluaran perintah:\n%s", out)
	}
}

// Boolean yang SUDAH menyala tidak disentuh lagi — menulis ulang setelan
// yang sudah benar hanya menambah baris di log audit server orang.
func TestSELinux_BooleanSudahMenyalaTidakDisentuh(t *testing.T) {
	out := jalankanSELinux(t, stubSELinux("1", true, true), "poinhost_selinux_proxy")
	if !strings.Contains(out, "setsebool=[]") {
		t.Errorf("menyalakan boolean yang sudah menyala:\n%s", out)
	}
	if strings.Contains(out, selinuxTanda) {
		t.Errorf("melaporkan perubahan yang tidak terjadi:\n%s", out)
	}
}

// Permissive BUKAN enforcing: ia mencatat tapi tidak memblokir apa pun, jadi
// tidak ada yang perlu diubah. Melonggarkan kebijakan di situ berarti
// mengubah server yang sedang tidak bermasalah.
func TestSELinux_PermissiveTidakDiubah(t *testing.T) {
	out := jalankanSELinux(t, stubSELinux("0", false, true),
		"poinhost_selinux_proxy\npoinhost_selinux_label /var/www/x")
	if !strings.Contains(out, "setsebool=[] restorecon=[]") {
		t.Errorf("mengubah server yang SELinux-nya permissive:\n%s", out)
	}
}

// Debian/Ubuntu: tidak ada SELinux sama sekali. Helper ini ikut terkirim ke
// sana di SETIAP pembuatan domain, jadi ia wajib tidak melakukan apa pun —
// dan terutama wajib tidak menggagalkan skripnya.
func TestSELinux_TanpaSELinuxTidakMelakukanApaPun(t *testing.T) {
	out := jalankanSELinux(t, stubSELinux("", false, false),
		"poinhost_selinux_proxy\npoinhost_selinux_label /var/www/x")
	if !strings.Contains(out, "setsebool=[] restorecon=[] logger=[]") {
		t.Errorf("bertindak di server tanpa SELinux:\n%s", out)
	}
	if strings.Contains(out, selinuxTanda) {
		t.Errorf("melaporkan perubahan di server tanpa SELinux:\n%s", out)
	}
}

// Enforcing tapi policycoreutils tidak terpasang. Tidak ada yang bisa
// dilakukan, dan itu bukan alasan menggagalkan pembuatan domain.
func TestSELinux_TanpaAlatnyaTetapLanjut(t *testing.T) {
	out := jalankanSELinux(t, stubSELinux("1", false, false),
		"poinhost_selinux_proxy\npoinhost_selinux_label /var/www/x")
	if strings.Contains(out, selinuxTanda) {
		t.Errorf("melaporkan perubahan padahal alatnya tidak ada:\n%s", out)
	}
}

func TestSELinux_LabelDipanggilUntukDirektorinya(t *testing.T) {
	out := jalankanSELinux(t, stubSELinux("1", false, true), selinuxLabel("/var/www/contoh.id/public_html"))
	if !strings.Contains(out, "restorecon=[-R /var/www/contoh.id/public_html]") {
		t.Errorf("restorecon tidak dijalankan untuk webroot:\n%s", out)
	}
}

// Boolean jaringan hanya untuk vhost PROXY. Domain static tidak pernah butuh
// nginx menyambung ke mana pun, dan menyalakannya di sana berarti
// melonggarkan kebijakan server tanpa ada yang memerlukannya.
func TestSelinuxProxy_HanyaUntukVhostProxy(t *testing.T) {
	if got := selinuxProxy(&vhostOptions{Domain: "contoh.id"}); got != "" {
		t.Errorf("domain static ikut menyalakan boolean: %q", got)
	}
	if got := selinuxProxy(&vhostOptions{Domain: "contoh.id", ProxyTarget: "   "}); got != "" {
		t.Errorf("target berisi spasi saja dianggap proxy: %q", got)
	}
	if got := selinuxProxy(&vhostOptions{Domain: "contoh.id", ProxyTarget: "http://127.0.0.1:8080"}); got == "" {
		t.Error("vhost proxy tidak menyalakan boolean")
	}
	if got := selinuxProxy(nil); got != "" {
		t.Errorf("opts nil = %q", got)
	}
}

// Pelajaran truepoinhost_svc: konstanta ini disambung ke skrip lain yang
// tidak selalu berakhir dengan baris baru. Tanpa baris baru di depan, nama
// fungsinya menyatu dengan kata terakhir sebelumnya dan bash menerimanya
// sebagai definisi fungsi bernama LAIN — tanpa satu pun galat sintaks,
// sehingga setiap pemanggilnya gagal diam-diam.
var definisiSELinux = regexp.MustCompile(`(?m)^poinhost_selinux_[a-z]+\(\) \{`)

func TestSELinuxHelper_TidakMenyambungKeBarisSebelumnya(t *testing.T) {
	if !strings.HasPrefix(SELinuxHelper, "\n") {
		t.Fatal("SELinuxHelper tidak diawali baris baru")
	}
	if n := len(definisiSELinux.FindAllString(SELinuxHelper, -1)); n != 4 {
		t.Errorf("fungsi yang terdefinisi di awal baris = %d, mau 4", n)
	}
	if regexp.MustCompile(`[0-9A-Za-z_]poinhost_selinux_`).MatchString(SELinuxHelper) {
		t.Error("ada nama yang menempel pada kata sebelumnya")
	}
}

// Skrip yang benar-benar dikirim ke server harus sah secara sintaks, dengan
// helper sudah tersambung di tempatnya.
func TestSELinuxHelper_SintaksnyaSahSaatDisambung(t *testing.T) {
	sh, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash tidak tersedia")
	}
	opts := &vhostOptions{Domain: "contoh.id", Root: "/var/www/contoh.id/public_html", ProxyTarget: "http://127.0.0.1:8080"}
	gabungan := "set -e\n" + SvcHelper + SELinuxHelper + "\n" +
		selinuxProxy(opts) + selinuxLabel(opts.Root) + "true\n"
	if out, err := exec.Command(sh, "-n", "-c", gabungan).CombinedOutput(); err != nil {
		t.Errorf("sintaks tidak sah: %v\n%s\n--- skrip ---\n%s", err, out, gabungan)
	}
}

// Keputusan "enforcing atau tidak" diuji dengan BERKAS SUNGGUHAN, bukan
// dengan menimpa fungsinya.
//
// Tes lain di file ini memalsukan poinhost_selinux_enforcing supaya bisa
// menguji cabang-cabang di atasnya — dan itu berarti logika fungsi ini
// sendiri tidak tersentuh oleh satu pun dari mereka. Terbukti: saat
// pemeriksaan "= 1" dihapus, seluruh tes tetap hijau. Keadaan permissive
// yang salah dibaca sebagai enforcing berarti poinhost melonggarkan
// kebijakan server yang sedang tidak bermasalah.
func TestSELinuxEnforcing_DibacaDariBerkasnya(t *testing.T) {
	sh, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash tidak tersedia")
	}
	dir := t.TempDir()

	tulis := func(nama, isi string) string {
		p := filepath.Join(dir, nama)
		if err := os.WriteFile(p, []byte(isi), 0o644); err != nil {
			t.Fatal(err)
		}
		return filepath.ToSlash(p)
	}

	kasus := []struct {
		nama string
		path string
		mau  string
	}{
		{"enforcing", tulis("enforcing", "1"), "YA"},
		{"enforcing dengan baris baru", tulis("enforcing_nl", "1\n"), "YA"},
		{"permissive", tulis("permissive", "0"), "TIDAK"},
		{"berkasnya tidak ada", filepath.ToSlash(filepath.Join(dir, "hilang")), "TIDAK"},
		{"isinya kosong", tulis("kosong", ""), "TIDAK"},
	}

	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			script := "set -e\n" + SELinuxHelper + "\n" +
				"export POINHOST_SELINUX_ENFORCE=" + shellQuote(k.path) + "\n" +
				"if poinhost_selinux_enforcing; then echo YA; else echo TIDAK; fi\n"
			out, err := exec.Command(sh, "-c", script).CombinedOutput()
			if err != nil {
				t.Fatalf("skrip gagal: %v\n%s", err, out)
			}
			if got := strings.TrimSpace(string(out)); got != k.mau {
				t.Errorf("= %q, mau %q", got, k.mau)
			}
		})
	}
}
