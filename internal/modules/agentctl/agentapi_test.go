package agentctl

import (
	"os/exec"
	"strings"
	"testing"
)

// Socket admin yang tidak ada punya LEBIH DARI SATU sebab, dan versi pertama
// fungsi ini menyebut satu saja — "agent terlalu lama, tekan Update agent".
// Untuk agent baru yang kebetulan sedang mati, kalimat itu mengirim pemilik
// server memasang ulang sesuatu yang sudah benar.
func TestSebabNoAdmin(t *testing.T) {
	kasus := []struct {
		nama   string
		stderr string
		bundel string
		mau    []string
		tidak  []string
	}{
		{
			nama:   "agent lama, perlu update",
			stderr: penandaNoAdmin + "\nactive=active\nversion=0.2.2",
			bundel: "0.8.0",
			// Kedua versinya disebut: "tekan Update agent" tanpa angka tidak
			// menerangkan apa pun bagi orang yang baru saja menekan Pasang.
			mau: []string{"0.2.2", "0.8.0", "Update agent"},
		},
		{
			nama:   "agent baru tapi sedang mati",
			stderr: penandaNoAdmin + "\nactive=inactive\nversion=0.8.0",
			bundel: "0.8.0",
			mau:    []string{"tidak berjalan", "0.8.0"},
			tidak:  []string{"Update agent"},
		},
		{
			nama:   "sedang restart (systemd: activating)",
			stderr: penandaNoAdmin + "\nactive=activating\nversion=0.8.0",
			bundel: "0.8.0",
			mau:    []string{"tidak berjalan"},
			tidak:  []string{"Update agent"},
		},
		{
			nama:   "belum terpasang sama sekali",
			stderr: penandaNoAdmin + "\nactive=inactive",
			bundel: "0.8.0",
			mau:    []string{"belum terpasang"},
			tidak:  []string{"Update agent"},
		},
		{
			// listenAdmin SENGAJA tidak fatal di main.go, jadi keadaan ini
			// nyata: agent terbaru berjalan sehat, socketnya saja gagal
			// dibuka. Menyuruhnya update tidak akan memperbaiki apa pun.
			nama:   "agent terbaru berjalan, socket gagal dibuka",
			stderr: penandaNoAdmin + "\nactive=active\nversion=0.8.0",
			bundel: "0.8.0",
			mau:    []string{"socket admin", "Log agent"},
			tidak:  []string{"Update agent"},
		},
		{
			// Agent di server LEBIH BARU dari app desktop. Menawarkan update
			// di sini berarti menawarkan penurunan versi sebagai perbaikan.
			nama:   "agent lebih baru dari yang dibundel",
			stderr: penandaNoAdmin + "\nactive=active\nversion=0.9.0",
			bundel: "0.8.0",
			tidak:  []string{"Update agent"},
		},
	}

	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			got := sebabNoAdmin(k.stderr, k.bundel)
			for _, m := range k.mau {
				if !strings.Contains(got, m) {
					t.Errorf("pesan tidak memuat %q:\n%s", m, got)
				}
			}
			for _, m := range k.tidak {
				if strings.Contains(got, m) {
					t.Errorf("pesan memuat %q padahal tidak berlaku di sini:\n%s", m, got)
				}
			}
		})
	}
}

// Versi yang tidak bisa diurutkan (mis. `make agent` tanpa VERSION) tidak
// boleh berubah jadi tawaran update — decideVersionAction sudah menolak
// menebak, dan kalimat di sini harus ikut.
func TestSebabNoAdmin_VersiTidakTerbaca(t *testing.T) {
	got := sebabNoAdmin(penandaNoAdmin+"\nactive=active\nversion=dev", "")
	if strings.Contains(got, "Update agent") {
		t.Errorf("menawarkan update untuk versi yang tidak bisa dibandingkan:\n%s", got)
	}
	if !strings.Contains(got, "dev") {
		t.Errorf("tidak menyebut versi yang terpasang:\n%s", got)
	}
}

// Skripnya dijalankan di shell SUNGGUHAN dengan socket yang pasti tidak ada,
// lalu keluarannya dibaca parseKV — persis seperti sebabNoAdmin membacanya.
// Tanpa ini, satu salah ketik di dalam $(...) cuma akan terlihat sebagai
// pesan yang kehilangan angka versinya, di server orang lain.
func TestSkripAdmin_KeluaranTerbacaParsernya(t *testing.T) {
	sh, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash tidak tersedia")
	}

	// systemctl dan binary agent dipalsukan; yang diuji adalah skripnya,
	// bukan mesin yang menjalankan tes.
	shim := "systemctl() { echo inactive; }\n" +
		"export PATH=/nonexistent-poinhost:$PATH\n"
	cmd := exec.Command(sh, "-c", shim+skripAdmin("GET", "/admin/devices"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("skrip berhasil padahal socketnya tidak ada:\n%s", out)
	}

	teks := string(out)
	if !strings.Contains(teks, penandaNoAdmin) {
		t.Fatalf("penanda tidak dicetak:\n%s", teks)
	}
	kv := parseKV(teks)
	if kv["active"] != "inactive" {
		t.Errorf("active = %q, mau inactive\n%s", kv["active"], teks)
	}
	// Binary agent memang tidak ada di mesin ini, jadi version tidak
	// dicetak — dan sebabNoAdmin harus menyimpulkan "belum terpasang".
	if got := sebabNoAdmin(teks, "0.8.0"); !strings.Contains(got, "belum terpasang") {
		t.Errorf("kesimpulan = %q", got)
	}
}

// Socket yang ADA tidak boleh ikut mencetak penanda — kalau ia tercetak di
// jalur normal, setiap jawaban sukses akan dibaca sebagai kegagalan.
func TestSkripAdmin_JalurNormalTidakMencetakPenanda(t *testing.T) {
	skrip := skripAdmin("POST", "/admin/devices/abc/approve")
	i := strings.Index(skrip, "exit $?")
	j := strings.Index(skrip, penandaNoAdmin)
	if i < 0 || j < 0 {
		t.Fatalf("skrip tidak berbentuk seperti yang diharapkan:\n%s", skrip)
	}
	if j < i {
		t.Error("penanda dicetak sebelum jalur sukses selesai")
	}
	if !strings.Contains(skrip, "-X POST") {
		t.Error("method tidak ikut di perintah curl")
	}
	if !strings.Contains(skrip, "/admin/devices/abc/approve") {
		t.Error("path tidak ikut di perintah curl")
	}
}
