package bot

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPairing_KodeBerlakuSekali(t *testing.T) {
	p := NewPairing()
	code, _, err := p.Issue()
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if err := p.Redeem(code); err != nil {
		t.Fatalf("penukaran pertama gagal: %v", err)
	}
	if err := p.Redeem(code); !errors.Is(err, ErrPairingInvalid) {
		t.Errorf("penukaran kedua = %v, mau ErrPairingInvalid", err)
	}
}

func TestPairing_KodeKedaluwarsa(t *testing.T) {
	p := NewPairing()
	now := time.Now()
	p.now = func() time.Time { return now }

	code, exp, _ := p.Issue()
	if exp.Sub(now) != PairingTTL {
		t.Errorf("masa berlaku = %v, mau %v", exp.Sub(now), PairingTTL)
	}

	now = now.Add(PairingTTL + time.Second)
	if err := p.Redeem(code); !errors.Is(err, ErrPairingInvalid) {
		t.Errorf("kode kedaluwarsa diterima: %v", err)
	}
}

// Kode kedaluwarsa tidak boleh menumpuk di memori pada agent yang hidup
// berbulan-bulan.
func TestPairing_MembersihkanKodeKedaluwarsa(t *testing.T) {
	p := NewPairing()
	now := time.Now()
	p.now = func() time.Time { return now }

	for i := 0; i < 5; i++ {
		if _, _, err := p.Issue(); err != nil {
			t.Fatal(err)
		}
	}
	if p.Pending() != 5 {
		t.Fatalf("Pending = %d, mau 5", p.Pending())
	}

	now = now.Add(PairingTTL + time.Second)
	if p.Pending() != 0 {
		t.Errorf("Pending = %d setelah kedaluwarsa, mau 0", p.Pending())
	}
}

// Kode dibaca dari layar laptop lalu diketik di HP. Huruf kecil, spasi, dan
// prefiks yang ikut/tidak ikut tersalin adalah kesalahan yang normal terjadi —
// dan menolaknya hanya akan membuat user mengira kodenya rusak.
func TestNormalizeCode_MenerimaVariasiKetikan(t *testing.T) {
	p := NewPairing()
	code, _, _ := p.Issue()
	formatted := FormatCode(code)

	variasi := []string{
		code,
		strings.ToLower(code),
		formatted,
		strings.ToLower(formatted),
		" " + formatted + " ",
		strings.ReplaceAll(formatted, "-", " "),
	}
	for _, v := range variasi {
		if got := NormalizeCode(v); got != code {
			t.Errorf("NormalizeCode(%q) = %q, mau %q", v, got, code)
		}
	}
}

func TestNormalizeCode_MenolakYangTidakBerbentukKode(t *testing.T) {
	for _, v := range []string{"", "PH-", "ABC", "PH-AAAA-BBBB-CCCC", "PH-0OO1-IIll"} {
		if got := NormalizeCode(v); got != "" {
			t.Errorf("NormalizeCode(%q) = %q, mau kosong", v, got)
		}
	}
}

// Alfabet sengaja tanpa karakter yang mudah tertukar saat dibaca ulang.
func TestPairing_AlfabetTanpaKarakterAmbigu(t *testing.T) {
	for _, r := range "01OIL" {
		if strings.ContainsRune(pairingAlphabet, r) {
			t.Errorf("alfabet memuat karakter ambigu %q", r)
		}
	}

	p := NewPairing()
	for i := 0; i < 200; i++ {
		code, _, err := p.Issue()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != pairingLen {
			t.Fatalf("panjang kode = %d, mau %d", len(code), pairingLen)
		}
		for _, r := range code {
			if !strings.ContainsRune(pairingAlphabet, r) {
				t.Fatalf("kode %q memuat karakter di luar alfabet", code)
			}
		}
	}
}

func TestPairing_KodeTidakBerulang(t *testing.T) {
	p := NewPairing()
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		code, _, err := p.Issue()
		if err != nil {
			t.Fatal(err)
		}
		if seen[code] {
			t.Fatalf("kode %q keluar dua kali dalam 500 percobaan", code)
		}
		seen[code] = true
	}
}

// P dan H ada di dalam alfabet kode, jadi kode sah yang kebetulan diawali
// "PH" tidak boleh ikut terpotong saat prefiks dibuang.
func TestNormalizeCode_TidakMemotongKodeYangDiawaliPH(t *testing.T) {
	const code = "PH234567" // 8 karakter, semuanya dari alfabet kode
	if got := NormalizeCode(code); got != code {
		t.Errorf("NormalizeCode(%q) = %q, mau utuh", code, got)
	}
	if got := NormalizeCode("PH-" + code[:4] + "-" + code[4:]); got != code {
		t.Errorf("bentuk berformat dari kode ber-awalan PH tidak dikenali: %q", got)
	}
}
