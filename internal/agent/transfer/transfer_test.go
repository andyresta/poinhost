package transfer

import (
	"strings"
	"testing"
)

func contohBundle() *Bundle {
	return &Bundle{Servers: []Server{{
		ID: "abc", Name: "prod", Host: "10.0.0.9", Port: 22, Username: "root",
		AuthType: "key", KeyPath: "/var/lib/poinhost-agent/keys/server-abc",
		KeyFileB64: "a2V5", Tags: []string{"linked"},
		KnownHosts: []string{"10.0.0.9 ssh-ed25519 AAAAC3Nz"},
	}}}
}

func TestSealOpen_PulangPergi(t *testing.T) {
	in := contohBundle()
	sealed, err := Seal(in, "rahasia")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if in.Version != Version {
		t.Errorf("Seal tidak mengisi Version: %d", in.Version)
	}

	out, err := Open(sealed, "rahasia")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(out.Servers) != 1 || out.Servers[0].ID != "abc" {
		t.Fatalf("isi bundle berubah: %+v", out.Servers)
	}
	if len(out.Servers[0].KnownHosts) != 1 {
		t.Error("host key tidak ikut — agent akan gagal dengan SSH_HOST_KEY_MISMATCH")
	}
}

// Kredensial tidak boleh terbaca dari berkas paket. Berkas ini mendarat di
// /tmp server sebelum diimpor, jadi isinya harus tidak berguna bagi siapa pun
// yang menemukannya.
func TestSeal_TidakMenyimpanKredensialTerbuka(t *testing.T) {
	in := contohBundle()
	in.Servers[0].Password = "password-sangat-rahasia"
	sealed, err := Seal(in, "passphrase")
	if err != nil {
		t.Fatal(err)
	}
	for _, bocor := range []string{"password-sangat-rahasia", "prod", "10.0.0.9", "a2V5"} {
		if strings.Contains(string(sealed), bocor) {
			t.Errorf("%q terbaca apa adanya di paket terenkripsi", bocor)
		}
	}
}

func TestOpen_PassphraseSalahDitolak(t *testing.T) {
	sealed, err := Seal(contohBundle(), "benar")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(sealed, "salah"); err == nil {
		t.Fatal("passphrase salah diterima")
	}
}

// Versi paket yang tidak dikenal harus ditolak dengan jelas, bukan diproses
// sebagian — desktop dan agent memang bisa berbeda versi.
func TestOpen_VersiTidakDikenalDitolak(t *testing.T) {
	b := contohBundle()
	sealed, err := Seal(b, "p")
	if err != nil {
		t.Fatal(err)
	}
	// Bungkus ulang dengan versi masa depan.
	b.Version = Version + 99
	sealed2, err := sealRaw(b, "p")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(sealed2, "p"); err == nil {
		t.Error("versi paket tak dikenal diterima")
	}
	if _, err := Open(sealed, "p"); err != nil {
		t.Errorf("versi yang dikenal ikut ditolak: %v", err)
	}
}
