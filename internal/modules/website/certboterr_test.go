package website

import "testing"

// Keluaran asli certbot saat validasi HTTP-01 gagal. Bagian yang menjelaskan
// apa-apa ada di stdout (blok Domain/Type/Detail); yang selama ini muncul di
// panel justru baris stderr di bawahnya, yang tidak menyebut domain, sebab,
// maupun cara memperbaikinya.
const certbotFailureOutput = `Requesting a certificate for api.example.com
Certbot failed to authenticate some domains (authenticator: webroot). The Certificate Authority reported these problems:
  Domain: api.example.com
  Type:   unauthorized
  Detail: 104.21.12.66: Invalid response from http://api.example.com/.well-known/acme-challenge/2xPq: 404

Hint: The Certificate Authority failed to download the challenge files from the temporary standalone webserver.
Saving debug log to /var/log/letsencrypt/letsencrypt.log
Some challenges have failed.
Ask for help or search for solutions at https://community.letsencrypt.org.`

func TestCertbotProblem_ExtractsDomainTypeDetail(t *testing.T) {
	got := certbotProblem(certbotFailureOutput)
	want := "Domain: api.example.com · Type:   unauthorized · Detail: 104.21.12.66: Invalid response from http://api.example.com/.well-known/acme-challenge/2xPq: 404"
	if got != want {
		t.Fatalf("certbotProblem =\n%q\nmau\n%q", got, want)
	}
}

// Beberapa domain gagal sekaligus: semuanya harus terbawa, bukan yang
// pertama saja — kalau hanya satu yang ditampilkan, user memperbaiki satu
// domain lalu gagal lagi karena domain berikutnya.
func TestCertbotProblem_KeepsEveryFailedDomain(t *testing.T) {
	raw := `  Domain: a.example.com
  Type:   unauthorized
  Detail: 404
  Domain: b.example.com
  Type:   dns
  Detail: NXDOMAIN`
	got := certbotProblem(raw)
	want := "Domain: a.example.com · Type:   unauthorized · Detail: 404 · Domain: b.example.com · Type:   dns · Detail: NXDOMAIN"
	if got != want {
		t.Fatalf("certbotProblem = %q, mau %q", got, want)
	}
}

// Kegagalan yang bukan kegagalan validasi (mis. certbot tidak terpasang)
// tidak punya blok itu — pemanggil harus bisa tahu dan jatuh ke keluaran
// apa adanya, bukan menampilkan pesan kosong.
func TestCertbotProblem_EmptyWhenNoReportBlock(t *testing.T) {
	if got := certbotProblem("certbot: command not found"); got != "" {
		t.Fatalf("certbotProblem = %q, mau kosong", got)
	}
}
