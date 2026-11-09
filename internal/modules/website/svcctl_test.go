package website

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Skrip yang MEMANGGIL poinhost_svc harus ikut MEMBAWA definisinya.
//
// Yang paling mudah terlewat adalah deploy hook certbot: ia ditulis ke disk
// dan dijalankan certbot sendiri, di luar sesi poinhost, jadi definisi yang
// hanya ada di skrip pemasangnya tidak akan ikut ke sana.
func TestSkrip_MembawaDefinisiSvc(t *testing.T) {
	skrip := map[string]string{
		"combinedStatusScript":    combinedStatusScript,
		"certbotDeployHookScript": certbotDeployHookScript,
	}
	for _, pm := range []string{"apt", "dnf", "yum", "apk"} {
		s, ok := installNginxScript(pm)
		if !ok {
			t.Fatalf("installNginxScript(%q) tidak didukung", pm)
		}
		skrip["installNginxScript/"+pm] = s
	}

	for nama, s := range skrip {
		if !strings.Contains(s, "poinhost_svc") {
			continue
		}
		if !strings.Contains(s, "poinhost_svc() {") {
			t.Errorf("%s memanggil poinhost_svc tanpa membawa definisinya", nama)
		}
	}
}

// Keberadaan systemctl saja tidak membuktikan systemd berjalan: binarinya ada
// di banyak image container yang PID 1-nya bukan systemd. /run/systemd/system
// hanya ada saat systemd benar-benar menjadi init.
func TestSvcHelper_TidakMenebakDariBinariSaja(t *testing.T) {
	if !strings.Contains(SvcHelper, "/run/systemd/system") {
		t.Error("tidak memeriksa /run/systemd/system")
	}
	if !strings.Contains(SvcHelper, "rc-service") || !strings.Contains(SvcHelper, "rc-update") {
		t.Error("tidak menangani OpenRC")
	}
	// Reload yang gagal harus jatuh ke restart — itu perilaku yang sudah ada
	// sebelum helper ini, dan menghilangkannya akan membuat perubahan vhost
	// diam-diam tidak pernah aktif.
	if !strings.Contains(SvcHelper, "systemctl restart") {
		t.Error("reload tidak jatuh ke restart di jalur systemd")
	}
}

// Nginx TIDAK boleh lagi dikendalikan lewat systemctl langsung di mana pun
// di paket ini. Satu saja yang tertinggal berarti Alpine memasang nginx lalu
// gagal di langkah berikutnya — jebakan yang justru hendak dibuang.
func TestPaket_TidakAdaLagiSystemctlNginx(t *testing.T) {
	berkas, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`systemctl[^"\n]*nginx`)
	for _, f := range berkas {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllString(string(b), -1) {
			t.Errorf("%s masih mengendalikan nginx lewat systemctl: %q", f, m)
		}
	}
}
