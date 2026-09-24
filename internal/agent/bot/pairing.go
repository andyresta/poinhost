package bot

import (
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"
)

// PairingTTL adalah masa berlaku satu kode pairing.
const PairingTTL = 10 * time.Minute

// pairingAlphabet sengaja tanpa 0/O/1/I/L: kodenya dibaca dari layar laptop
// lalu diketik ulang di HP, dan karakter yang mirip adalah sumber kegagalan
// yang paling sering terjadi di alur seperti ini.
const pairingAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

const pairingLen = 8

// ErrPairingInvalid dikembalikan untuk kode yang salah, kedaluwarsa, atau
// sudah terpakai. Ketiganya sengaja TIDAK dibedakan dalam pesan ke pengirim:
// memberi tahu "kode benar tapi sudah kedaluwarsa" akan mengonfirmasi tebakan
// yang benar kepada orang yang sedang menebak-nebak.
var ErrPairingInvalid = errors.New("kode pairing tidak berlaku")

// Pairing menyimpan kode pairing sekali-pakai di memori.
//
// Sengaja tidak dipersistenkan: kode hanya berlaku sepuluh menit, dan agent
// yang restart di tengah proses lebih baik membuat user meminta kode baru
// daripada menghidupkan kembali kode lama yang mungkin sudah bocor.
type Pairing struct {
	mu    sync.Mutex
	codes map[string]time.Time // kode -> kedaluwarsa
	now   func() time.Time
}

// NewPairing membuat penyimpan kode pairing kosong.
func NewPairing() *Pairing {
	return &Pairing{codes: make(map[string]time.Time), now: time.Now}
}

// Issue membuat kode baru yang berlaku selama PairingTTL.
func (p *Pairing) Issue() (string, time.Time, error) {
	code, err := randomCode()
	if err != nil {
		return "", time.Time{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sweepLocked()
	exp := p.now().Add(PairingTTL)
	p.codes[code] = exp
	return code, exp, nil
}

// Redeem memakai kode. Kode yang berhasil dipakai langsung hangus, jadi satu
// kode hanya pernah memasukkan satu orang ke allowlist.
func (p *Pairing) Redeem(code string) error {
	norm := NormalizeCode(code)
	if norm == "" {
		return ErrPairingInvalid
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.sweepLocked()

	exp, ok := p.codes[norm]
	if !ok || p.now().After(exp) {
		delete(p.codes, norm)
		return ErrPairingInvalid
	}
	delete(p.codes, norm)
	return nil
}

// Pending melaporkan berapa kode yang masih berlaku — dipakai UI Agent
// Control untuk menampilkan "kode sedang menunggu".
func (p *Pairing) Pending() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sweepLocked()
	return len(p.codes)
}

func (p *Pairing) sweepLocked() {
	now := p.now()
	for code, exp := range p.codes {
		if now.After(exp) {
			delete(p.codes, code)
		}
	}
}

// NormalizeCode menyamakan bentuk kode yang diketik user: huruf besar, tanpa
// spasi maupun tanda hubung, dan prefiks "PH" boleh ikut atau tidak.
//
// Pemisah dibuang LEBIH DULU, baru prefiksnya: user menyalin kode dari layar
// dengan bentuk yang bermacam-macam ("PH-3TZF-CS2M", "ph 3tzf cs2m",
// "3TZFCS2M"), dan menolaknya hanya membuat mereka mengira kodenya rusak.
//
// Prefiks hanya dibuang kalau panjangnya memang pas — P dan H ada di dalam
// alfabet kode, jadi membuang "PH" tanpa syarat akan merusak kode sah yang
// kebetulan diawali dua huruf itu.
func NormalizeCode(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.NewReplacer(" ", "", "-", "", "_", "").Replace(s)
	if len(s) == len("PH")+pairingLen && strings.HasPrefix(s, "PH") {
		s = s[len("PH"):]
	}
	if len(s) != pairingLen {
		return ""
	}
	for _, r := range s {
		if !strings.ContainsRune(pairingAlphabet, r) {
			return ""
		}
	}
	return s
}

// FormatCode menampilkan kode dalam bentuk yang dibaca user.
func FormatCode(code string) string {
	if len(code) != pairingLen {
		return code
	}
	return "PH-" + code[:4] + "-" + code[4:]
}

func randomCode() (string, error) {
	// Rejection sampling: len(alfabet) bukan pembagi 256, jadi modulo
	// langsung akan membuat sebagian karakter lebih sering muncul.
	const max = 256 - (256 % len(pairingAlphabet))
	out := make([]byte, 0, pairingLen)
	buf := make([]byte, pairingLen)
	for len(out) < pairingLen {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if int(b) >= max {
				continue
			}
			out = append(out, pairingAlphabet[int(b)%len(pairingAlphabet)])
			if len(out) == pairingLen {
				break
			}
		}
	}
	return string(out), nil
}
