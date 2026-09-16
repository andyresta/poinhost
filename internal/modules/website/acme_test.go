package website

import (
	"strings"
	"testing"
)

// Regresi: domain reverse-proxy tidak pernah bisa mendapat sertifikat
// pertamanya karena permintaan validasi ACME ikut diteruskan ke aplikasi di
// belakangnya, yang menjawab 404.
//
// Blok acme-challenge harus ada di vhost HTTP-saja — justru varian itulah yang
// aktif saat sertifikat pertama diterbitkan.
func TestVhostHTTPMenyajikanAcmeChallenge(t *testing.T) {
	proxy := buildVhostConfig(&vhostOptions{
		Domain:      "licence.contoh.com",
		Root:        "/var/www/contoh.com/licence.contoh.com",
		IsSubdomain: true,
		Parent:      "contoh.com",
		ProxyTarget: "http://127.0.0.1:7575",
	})

	if !strings.Contains(proxy, "location ^~ /.well-known/acme-challenge/") {
		t.Fatalf("vhost proxy HTTP wajib menyajikan acme-challenge:\n%s", proxy)
	}
	// Harus SEBELUM location proxy, dan memakai ^~ supaya menang tanpa
	// bergantung pada urutan.
	acme := strings.Index(proxy, "/.well-known/acme-challenge/")
	prox := strings.Index(proxy, "proxy_pass")
	if acme < 0 || prox < 0 || acme > prox {
		t.Fatalf("blok acme harus mendahului proxy_pass:\n%s", proxy)
	}
	if !strings.Contains(proxy, "root /var/www/contoh.com/licence.contoh.com;") {
		t.Fatalf("acme-challenge harus disajikan dari document root domain:\n%s", proxy)
	}

	// Domain static juga ikut punya blok itu — sebelumnya selamat hanya
	// karena kebetulan try_files menyajikannya.
	static := buildVhostConfig(&vhostOptions{Domain: "contoh.com", Root: "/var/www/contoh.com/public_html"})
	if !strings.Contains(static, "location ^~ /.well-known/acme-challenge/") {
		t.Fatalf("vhost static juga harus eksplisit menyajikan acme-challenge:\n%s", static)
	}

	// Varian HTTPS memang sudah punya sejak dulu — dijaga supaya tidak hilang.
	withSSL := buildVhostConfig(&vhostOptions{
		Domain: "contoh.com", Root: "/var/www/contoh.com/public_html",
		SSLEnabled: true, SSLCertificate: "/c.pem", SSLCertificateKey: "/k.pem",
	})
	if strings.Count(withSSL, "/.well-known/acme-challenge/") != 1 {
		t.Fatalf("varian HTTPS harus punya tepat satu blok acme di server :80:\n%s", withSSL)
	}
}
