package website

import (
	"strings"
	"testing"
)

// `www.` hanya boleh ditambahkan ke nama terdaftar paling atas.
//
// Kasus yang memicu perbaikan ini: api.storepoin.com ditambahkan sebagai
// domain tersendiri (induknya tidak dikelola di server itu), sehingga
// IsSubdomain bernilai false dan poinhost ikut meminta
// www.api.storepoin.com. DNS untuk nama itu tidak ada, dan satu SAN yang
// gagal menjatuhkan seluruh permintaan — jadi sertifikat untuk domain yang
// benar-benar diminta tidak pernah bisa terbit.
func TestIsRegistrableApex(t *testing.T) {
	cases := []struct {
		domain string
		want   bool
	}{
		{"storepoin.com", true},
		{"api.storepoin.com", false},
		{"data.storepoin.com", false},
		// Tiga label tapi tetap apex — inilah sebabnya menghitung titik
		// tidak bisa dipakai sebagai pengganti public suffix list.
		{"example.co.uk", true},
		{"shop.example.co.uk", false},
		// Public suffix itu sendiri bukan nama yang bisa didaftarkan.
		{"co.uk", false},
		{"com", false},
		// Nama tanpa TLD publik: jangan mengarang www untuknya.
		{"localhost", false},
		{"server-internal", false},
	}
	for _, c := range cases {
		if got := isRegistrableApex(c.domain); got != c.want {
			t.Errorf("isRegistrableApex(%q) = %v, mau %v", c.domain, got, c.want)
		}
	}
}

// Skrip pengecekan DNS harus keluar dengan status 0 walau tidak satu pun
// nama yang resolve — "tidak ada yang resolve" adalah jawaban yang sah,
// bukan kegagalan perintah yang perlu dilaporkan sebagai error.
func TestResolvableScript_EndsWithTrue(t *testing.T) {
	script := resolvableScript([]string{"www.example.com"})
	if got := script[len(script)-5:]; got != "true\n" {
		t.Errorf("skrip tidak diakhiri `true`: %q", script)
	}
	if want := "getent ahosts 'www.example.com'"; !strings.Contains(script, want) {
		t.Errorf("skrip tidak memeriksa nama: %q", script)
	}
}
