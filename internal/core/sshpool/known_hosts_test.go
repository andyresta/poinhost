package sshpool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
