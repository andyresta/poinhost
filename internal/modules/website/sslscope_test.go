package website

import (
	"strings"
	"testing"
)

// Sertifikat sekarang per domain. `www` TETAP digabung dengan induknya karena
// keduanya dilayani satu vhost lewat server_name — memisahkannya menghasilkan
// sertifikat yang tidak pernah dipakai.
func TestCertExistsScriptJatuhKeIndukUntukSetupLama(t *testing.T) {
	// Subdomain: ada jalur cadangan ke lineage induk.
	sub := certExistsScript("whmail.devpoin.com", "devpoin.com")
	if !strings.Contains(sub, "/etc/letsencrypt/live/whmail.devpoin.com/fullchain.pem") {
		t.Fatalf("lineage sendiri harus dicek lebih dulu:\n%s", sub)
	}
	if !strings.Contains(sub, "/etc/letsencrypt/live/devpoin.com/fullchain.pem") {
		t.Fatalf("harus ada cadangan ke lineage induk:\n%s", sub)
	}
	// Cadangan hanya dipakai kalau SAN induk memang memuat subdomain ini —
	// kalau tidak, subdomain yang TIDAK tercakup akan salah dilaporkan aman.
	if !strings.Contains(sub, `grep -q "DNS:whmail.devpoin.com"`) {
		t.Fatalf("cadangan wajib memverifikasi SAN induk:\n%s", sub)
	}
	if !strings.Contains(sub, "CERT_SHARED=1") {
		t.Fatalf("pemakaian sertifikat induk harus ditandai:\n%s", sub)
	}

	// Domain induk: tidak punya induk, jadi tidak ada cadangan sama sekali.
	parent := certExistsScript("devpoin.com", "")
	if strings.Contains(parent, "CERT_SHARED=1") {
		t.Fatalf("domain induk tidak boleh punya jalur cadangan:\n%s", parent)
	}
}

// Halaman default domain baru menyebut PoinHost dan nama domainnya, serta
// meng-escape nama domain sebelum masuk HTML.
func TestDefaultIndexHTML(t *testing.T) {
	page := defaultIndexHTML("contoh.com")
	for _, want := range []string{"Domain dikelola oleh", "PoinHost", "contoh.com", "prefers-color-scheme"} {
		if !strings.Contains(page, want) {
			t.Fatalf("halaman default kurang %q", want)
		}
	}
	if strings.Contains(page, "Homepoin") {
		t.Fatal("halaman default tidak boleh menyebut Homepoin lagi")
	}

	// Nama domain masuk ke HTML, jadi harus di-escape walau sudah divalidasi
	// di lapisan lain — pertahanan berlapis, bukan mengandalkan satu titik.
	esc := defaultIndexHTML(`a"><script>x</script>`)
	if strings.Contains(esc, "<script>") {
		t.Fatalf("nama domain harus di-escape:\n%s", esc)
	}
}
