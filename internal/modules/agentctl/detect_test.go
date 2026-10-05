package agentctl

import "testing"

// Agent Mobile Control menjawab {"ok":true}; agent versi lama menjawab
// {"status":"ok"}. Membaca hanya salah satunya membuat app desktop melaporkan
// agent yang sehat sebagai "aktif tapi tidak menjawab health check" — dan
// membuat Install menunggu sampai timeout padahal pemasangannya berhasil.
func TestParseHealth_MenerimaJawabanBaruDanLama(t *testing.T) {
	baru := parseHealth(`{"ok":true,"version":"1.2.0","uptimeSeconds":42}`)
	if baru == nil {
		t.Fatal("jawaban agent baru dianggap tidak sehat")
	}
	if baru.UptimeSeconds != 42 {
		t.Errorf("uptimeSeconds = %d, mau 42", baru.UptimeSeconds)
	}

	if parseHealth(`{"status":"ok","version":"1.0.0"}`) == nil {
		t.Error("jawaban agent lama dianggap tidak sehat")
	}
}

func TestParseHealth_MenolakYangBukanSehat(t *testing.T) {
	// `curl ... || true` pada detectScript membuat stdout KOSONG saat agent
	// tidak menjawab, bukan menghasilkan error — jadi kosong harus ditolak
	// di sini, bukan diam-diam lolos sebagai struct nol.
	for nama, raw := range map[string]string{
		"kosong":      "",
		"bukan json":  "curl: (7) Failed to connect",
		"ok false":    `{"ok":false}`,
		"status lain": `{"status":"starting"}`,
		"json kosong": `{}`,
		"html proxy":  "<html>502</html>",
	} {
		if parseHealth(raw) != nil {
			t.Errorf("%s dianggap sehat: %q", nama, raw)
		}
	}
}
