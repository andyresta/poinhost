package sshpool

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

const (
	barisA = "10.0.0.9 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAAAA"
	barisB = "10.0.0.9 ssh-rsa AAAAB3NzaC1yc2EAAAADAQABBBBB"
	barisC = "192.168.1.5 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICCCC"
)

func tulisKnownHosts(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ssh_known_hosts")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// LinesFor dipakai untuk menyalin host key yang SUDAH disetujui user ke agent.
// Kalau ia mengembalikan baris milik host lain, agent akan mempercayai kunci
// yang tidak pernah disetujui untuk host itu.
func TestLinesFor_HanyaHostYangDiminta(t *testing.T) {
	s := NewKnownHostsStore(tulisKnownHosts(t, barisA, barisB, barisC, "# komentar", ""))

	got := s.LinesFor("10.0.0.9")
	if len(got) != 2 {
		t.Fatalf("dapat %d baris, mau 2: %v", len(got), got)
	}
	for _, l := range got {
		if !strings.HasPrefix(l, "10.0.0.9 ") {
			t.Errorf("baris host lain ikut terbawa: %q", l)
		}
	}

	// Hostname dari klien SSH selalu memuat port.
	if len(s.LinesFor("10.0.0.9:22")) != 2 {
		t.Error("hostname berport tidak cocok dengan entri yang sama")
	}
	if len(s.LinesFor("10.0.0.99")) != 0 {
		t.Error("host dengan awalan sama ikut cocok")
	}
	if len(s.LinesFor("tidak-ada")) != 0 {
		t.Error("host yang tidak ada mengembalikan baris")
	}
}

func TestLinesFor_BerkasTidakAda(t *testing.T) {
	s := NewKnownHostsStore(filepath.Join(t.TempDir(), "belum-ada"))
	if got := s.LinesFor("10.0.0.9"); got != nil {
		t.Errorf("mau nil, dapat %v", got)
	}
}

// Agent menyimpan known_hosts untuk BANYAK server sekaligus, jadi menulis
// ulang entri satu host tidak boleh menyentuh host lain.
func TestReplaceLinesFor_TidakMenyentuhHostLain(t *testing.T) {
	p := tulisKnownHosts(t, barisA, barisC)
	s := NewKnownHostsStore(p)

	baru := "10.0.0.9 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBARU"
	if err := s.ReplaceLinesFor("10.0.0.9", []string{baru}); err != nil {
		t.Fatalf("ReplaceLinesFor: %v", err)
	}

	got := s.LinesFor("10.0.0.9")
	if len(got) != 1 || got[0] != baru {
		t.Errorf("entri host yang ditulis ulang = %v, mau [%q]", got, baru)
	}
	if lain := s.LinesFor("192.168.1.5"); len(lain) != 1 || lain[0] != barisC {
		t.Errorf("entri host lain berubah: %v", lain)
	}

	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode known_hosts = %o, mau 600", perm)
	}
}

// Menulis ulang dua kali dengan isi yang sama tidak boleh menumpuk duplikat.
func TestReplaceLinesFor_Idempoten(t *testing.T) {
	s := NewKnownHostsStore(tulisKnownHosts(t, barisC))
	for i := 0; i < 3; i++ {
		if err := s.ReplaceLinesFor("10.0.0.9", []string{barisA, barisB}); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.LinesFor("10.0.0.9"); len(got) != 2 {
		t.Errorf("dapat %d baris setelah 3x tulis, mau 2", len(got))
	}
}

// Satu host boleh punya beberapa host key — agent menanam SELURUH
// /etc/ssh/ssh_host_*_key.pub mesinnya, jadi 127.0.0.1 punya baris ecdsa,
// ed25519, dan rsa sekaligus. Klien hanya menegosiasikan satu di antaranya.
//
// Verifikasi yang berhenti di baris pertama yang host-nya cocok akan menolak
// sambungan yang benar-benar sah sebagai SSH_HOST_KEY_MISMATCH, hanya karena
// yang dinegosiasikan bukan kebetulan yang tertulis paling atas. Itulah yang
// membuat agent tidak pernah bisa membaca mesinnya sendiri.
func TestVerify_HostDenganBeberapaJenisKunci(t *testing.T) {
	ed, edLine := kunciUji(t, "127.0.0.1", "ed25519")
	_, ecLine := kunciUji(t, "127.0.0.1", "ecdsa")

	// ecdsa ditulis LEBIH DULU, persis seperti urutan glob di server
	// (ssh_host_ecdsa_key.pub sebelum ssh_host_ed25519_key.pub).
	s := NewKnownHostsStore(tulisKnownHosts(t, ecLine, edLine))
	if err := s.verify("127.0.0.1:22", ed); err != nil {
		t.Fatalf("kunci ed25519 yang sah ditolak: %v", err)
	}
}

func TestVerify_KunciBerubahUntukJenisYangSama(t *testing.T) {
	_, lamaLine := kunciUji(t, "127.0.0.1", "ed25519")
	baru, _ := kunciUji(t, "127.0.0.1", "ed25519")

	s := NewKnownHostsStore(tulisKnownHosts(t, lamaLine))
	var mm *HostKeyMismatch
	if err := s.verify("127.0.0.1:22", baru); !errors.As(err, &mm) {
		t.Fatalf("kunci yang berubah diterima, err = %v", err)
	}
	if mm.OldFingerprint == "" {
		t.Error("fingerprint lama tidak ikut dilaporkan")
	}
}

// Host dikenal, tapi tidak untuk jenis kunci yang ditawarkan: itu bukan
// "berubah", itu belum pernah dipercaya.
func TestVerify_JenisKunciYangBelumPernahDipercaya(t *testing.T) {
	ed, _ := kunciUji(t, "127.0.0.1", "ed25519")
	_, ecLine := kunciUji(t, "127.0.0.1", "ecdsa")

	s := NewKnownHostsStore(tulisKnownHosts(t, ecLine))
	var mm *HostKeyMismatch
	if err := s.verify("127.0.0.1:22", ed); !errors.As(err, &mm) {
		t.Fatalf("jenis kunci yang tidak dikenal diterima, err = %v", err)
	}
	if mm.OldFingerprint != "" {
		t.Errorf("dilaporkan sebagai perubahan kunci (lama=%q), bukan host baru", mm.OldFingerprint)
	}
}

// kunciUji membuat host key baru beserta baris known_hosts-nya.
func kunciUji(t *testing.T, host, jenis string) (ssh.PublicKey, string) {
	t.Helper()
	var signer ssh.Signer
	switch jenis {
	case "ed25519":
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		signer, err = ssh.NewSignerFromKey(priv)
		if err != nil {
			t.Fatal(err)
		}
	case "ecdsa":
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		signer, err = ssh.NewSignerFromKey(priv)
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("jenis tidak dikenal: %s", jenis)
	}
	pub := signer.PublicKey()
	line := host + " " + pub.Type() + " " +
		base64.StdEncoding.EncodeToString(pub.Marshal())
	return pub, line
}
