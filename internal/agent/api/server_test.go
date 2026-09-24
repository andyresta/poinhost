package api

import "testing"

// Endpoint impor membaca berkas berdasarkan path yang dikirim pemanggil.
// Tanpa batas, ia berubah jadi alat membaca berkas mana pun di server —
// arahkan ke /etc/shadow lalu baca pesan errornya.
func TestWithinDir(t *testing.T) {
	const dir = "/var/lib/poinhost-agent"

	boleh := []string{
		"/var/lib/poinhost-agent/.import.bundle",
		"/var/lib/poinhost-agent/keys/server-x",
		"/var/lib/poinhost-agent/",
	}
	for _, p := range boleh {
		if !withinDir(dir, p) {
			t.Errorf("withinDir(%q) = false, mau true", p)
		}
	}

	tolak := []string{
		"/etc/shadow",
		"/var/lib/poinhost-agent/../../etc/shadow",
		"/var/lib/poinhost-agent-lain/berkas", // awalan sama, direktori berbeda
		"/tmp/.poinhost-agent.bundle",
		"..",
		"",
	}
	for _, p := range tolak {
		if withinDir(dir, p) {
			t.Errorf("withinDir(%q) = true, mau ditolak", p)
		}
	}

	// DataDir kosong berarti konfigurasinya belum benar — jangan izinkan
	// apa pun, bukan izinkan semuanya.
	if withinDir("", "/var/lib/poinhost-agent/x") {
		t.Error("dir kosong meloloskan path")
	}
}
