package files

import (
	"errors"
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// Skrip di archivetool.go dijalankan dan diperiksa di shell SUNGGUHAN.
//
// Tes lain di file sebelah hanya mencocokkan teksnya, dan itu tidak
// menangkap kesalahan sintaks maupun kesalahan logika di dalam loop.
// Pelajaran yang sama sudah mahal sekali di paket pkgfail: skrip yang
// keliru di sini tidak gagal terbuka — ia salah melaporkan alat yang ada
// sebagai tidak ada, lalu menawarkan pemasangan yang tidak perlu.

// shimCommand mengganti `command` dengan fungsi shell, supaya skrip melihat
// daftar perintah yang kita tentukan, bukan isi mesin yang menjalankan tes.
func shimCommand(punya ...string) string {
	return "command() {\n" +
		"  if [ \"$1\" = \"-v\" ]; then\n" +
		"    case \" " + strings.Join(punya, " ") + " \" in *\" $2 \"*) echo \"/usr/bin/$2\"; return 0;; esac\n" +
		"    return 1\n" +
		"  fi\n" +
		"  builtin command \"$@\"\n" +
		"}\n"
}

func jalankanDeteksi(t *testing.T, punya ...string) (string, int) {
	t.Helper()
	sh, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash tidak tersedia")
	}
	cmd := exec.Command(sh, "-c", shimCommand(punya...)+"\n"+alatDetectScript)
	out, err := cmd.CombinedOutput()
	kode := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("menjalankan skrip: %v", err)
		}
		kode = ee.ExitCode()
	}
	return string(out), kode
}

// Keluaran skrip yang sesungguhnya harus bisa dibaca parseAlatOutput —
// keduanya ditulis terpisah, dan selama ini tidak ada yang membuktikan
// mereka sepakat soal format barisnya.
func TestSkripDeteksi_DibacaParsernya(t *testing.T) {
	out, kode := jalankanDeteksi(t, "dnf", "yum", "tar", "gzip")
	if kode != 0 {
		t.Fatalf("kode keluar = %d, mau 0\n%s", kode, out)
	}

	_, _, pm, ada := parseAlatOutput(out)
	if pm != "dnf" {
		t.Errorf("pm = %q, mau dnf\n%s", pm, out)
	}
	if !ada["tar"] || !ada["gzip"] {
		t.Errorf("tar/gzip tidak terbaca: %v\n%s", ada, out)
	}
	if ada["unzip"] || ada["python3"] {
		t.Errorf("melaporkan perintah yang tidak ada: %v\n%s", ada, out)
	}

	// Dan kesimpulannya: .zip belum bisa, .tar.gz sudah bisa.
	if ready, kurang := siapEkstrak("zip", ada); ready || strings.Join(kurang, ",") != "unzip" {
		t.Errorf("zip: ready=%v kurang=%v", ready, kurang)
	}
	if ready, _ := siapEkstrak("tar.gz", ada); !ready {
		t.Error("tar.gz dilaporkan belum siap padahal tar dan gzip ada")
	}
}

// Server yang benar-benar kosong: tidak ada satu pun package manager dan
// tidak ada satu pun alat. Skripnya tetap harus keluar dengan kode 0 —
// kalau tidak, StreamInstallArchiveTool akan membacanya sebagai sudo yang
// ditolak.
func TestSkripDeteksi_ServerKosongTetapKeluarNol(t *testing.T) {
	out, kode := jalankanDeteksi(t)
	if kode != 0 {
		t.Fatalf("kode keluar = %d, mau 0\n%s", kode, out)
	}
	if strings.Contains(out, "PM=") || strings.Contains(out, "HAVE=") {
		t.Errorf("melaporkan sesuatu padahal tidak ada apa-apa:\n%s", out)
	}
}

// Semua package manager dilaporkan, bukan berhenti di yang pertama —
// prioritasnya ditentukan parseAlatOutput, dan itu hanya bisa bekerja
// kalau skripnya menyebut semuanya.
func TestSkripDeteksi_MelaporkanSemuaPM(t *testing.T) {
	out, _ := jalankanDeteksi(t, "apt-get", "dnf", "yum", "zypper", "pacman", "apk")
	var got []string
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "PM="); ok {
			got = append(got, v)
		}
	}
	sort.Strings(got)
	mau := []string{"apk", "apt-get", "dnf", "pacman", "yum", "zypper"}
	if strings.Join(got, ",") != strings.Join(mau, ",") {
		t.Errorf("PM yang dilaporkan = %v, mau %v\n%s", got, mau, out)
	}
}

// Sintaks tiap cabang skrip pemasangan diperiksa oleh shell sungguhan.
// `bash -n` hanya mengurai, tidak menjalankan — tidak ada paket yang
// dipasang di mesin yang menjalankan tes ini.
func TestInstallPaketScript_SintaksnyaSah(t *testing.T) {
	sh, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash tidak tersedia")
	}
	for _, pm := range pmPrioritas {
		t.Run(pm, func(t *testing.T) {
			script, ok := installPaketScript(pm, []string{"unzip", "tar"})
			if !ok {
				t.Fatalf("%s tidak punya skrip", pm)
			}
			// Dibungkus persis seperti runInstallStream membungkusnya, supaya
			// yang diperiksa adalah teks yang benar-benar dikirim ke server.
			wrapped := "(\n" + script + "\n) 2>&1\necho \"__poinhost_INSTALL_DONE__:$?\""
			cmd := exec.Command(sh, "-n", "-c", wrapped)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("sintaks tidak sah: %v\n%s\n--- skrip ---\n%s", err, out, wrapped)
			}
		})
	}
}
