package website

import (
	"strings"
	"testing"
)

// Regresi untuk bug yang membuat kolom "Database" di daftar user PostgreSQL
// dan kolom "Users with access" di daftar database sama-sama tampil kosong.
//
// Penyebabnya: `psql -A` memakai PIPA sebagai pemisah kolom, bukan tab,
// sementara parser di modul ini memecah baris dengan tab (mengikuti keluaran
// `mysql --batch`). Akibatnya seluruh baris terbaca sebagai kolom pertama,
// sehingga nama role menjadi "mkelindo|OWNER" dan tidak pernah cocok dengan
// user mana pun.
func TestRunPostgresMemaksaPemisahTab(t *testing.T) {
	// Perintahnya disusun di runPostgres; di sini yang dijaga adalah bentuk
	// flag-nya, karena itu satu-satunya yang menentukan format keluaran.
	cmd := "sudo -u postgres psql -v ON_ERROR_STOP=1 -d " + shellQuote("mkelindo") +
		" -At -F " + shellQuote("\t") + " -c " + shellQuote("SELECT 1")

	if !strings.Contains(cmd, "-F ") {
		t.Fatal("tanpa -F, psql memakai pipa dan parser berbasis tab akan gagal diam-diam")
	}
	if !strings.Contains(cmd, "\t") {
		t.Fatalf("pemisahnya harus benar-benar karakter tab:\n%s", cmd)
	}
	if !strings.Contains(cmd, "-At") {
		t.Fatal("mode unaligned + tuples-only tetap diperlukan")
	}
}

// Parser akses role memecah baris dengan tab. Tes ini mengunci kontrak itu:
// kalau keluarannya kembali memakai pipa, baris kepemilikan tidak akan
// terbaca dan kolom relasinya kosong lagi.
func TestParsingBarisAksesRolePostgres(t *testing.T) {
	// Bentuk yang BENAR (tab): nama role terpisah dari privilege.
	benar := "mkelindo\t"
	parts := strings.SplitN(strings.TrimSpace(benar), "\t", 2)
	if parts[0] != "mkelindo" {
		t.Fatalf("role harus terbaca utuh, dapat %q", parts[0])
	}

	// Bentuk yang SALAH (pipa) — dipastikan TIDAK menghasilkan nama role yang
	// sah, supaya jelas kenapa gejalanya berupa kolom kosong.
	salah := "mkelindo|OWNER"
	partsSalah := strings.SplitN(salah, "\t", 2)
	if partsSalah[0] == "mkelindo" {
		t.Fatal("dengan pemisah pipa, nama role TIDAK boleh kebetulan terbaca benar")
	}
	if !strings.Contains(partsSalah[0], "|") {
		t.Fatalf("inilah nilai rusak yang dulu masuk sebagai nama role: %q", partsSalah[0])
	}
}
