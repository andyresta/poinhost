package docker

import (
	"strings"
	"testing"
)

// Recreate harus membawa kembali entri --add-host.
//
// Kasus nyata yang memicu tes ini: sebuah container memakai
// `host.docker.internal:host-gateway` untuk menjangkau database di host.
// Nama itu TIDAK ada di Linux kecuali dipasang lewat entri ini, jadi setelah
// container dibuat ulang tanpa membawanya, aplikasinya gagal resolve dan
// masuk restart loop — sementara konfigurasi yang terlihat di panel tampak
// tidak berubah sama sekali.
func TestBuildCreateArgs_KeepsExtraHosts(t *testing.T) {
	args, err := buildCreateArgs(&createTemplate{
		Name:       "app",
		Image:      "app:latest",
		ExtraHosts: []string{"host.docker.internal:host-gateway", "db.internal:10.0.0.5"},
	})
	if err != nil {
		t.Fatalf("buildCreateArgs: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--add-host host.docker.internal:host-gateway",
		"--add-host db.internal:10.0.0.5",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("entri hilang: %q\nargs: %s", want, joined)
		}
	}
}

// Entri tanpa pemisah ":" bukan spesifikasi host yang sah; meneruskannya
// hanya membuat `docker create` gagal dengan pesan yang membingungkan.
func TestBuildCreateArgs_SkipsMalformedExtraHosts(t *testing.T) {
	args, err := buildCreateArgs(&createTemplate{
		Name:       "app",
		Image:      "app:latest",
		ExtraHosts: []string{"", "   ", "tanpapemisah", "ok:1.2.3.4"},
	})
	if err != nil {
		t.Fatalf("buildCreateArgs: %v", err)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "tanpapemisah") {
		t.Errorf("entri tidak sah ikut terkirim: %s", joined)
	}
	if !strings.Contains(joined, "--add-host ok:1.2.3.4") {
		t.Errorf("entri sah malah hilang: %s", joined)
	}
	if strings.Count(joined, "--add-host") != 1 {
		t.Errorf("jumlah --add-host tidak sesuai: %s", joined)
	}
}

// Nilainya harus benar-benar dibaca dari inspect, bukan sekadar diteruskan
// kalau kebetulan sudah terisi di template.
func TestInspectToDetail_ReadsExtraHostsFromInspect(t *testing.T) {
	raw := `{
	  "Id": "abc123",
	  "Name": "/app",
	  "Config": {"Image": "app:latest"},
	  "HostConfig": {"ExtraHosts": ["host.docker.internal:host-gateway"], "NetworkMode": "bridge"},
	  "State": {"Status": "running"}
	}`
	inv, err := parseInspectJSON(raw)
	if err != nil {
		t.Fatalf("parseInspectJSON: %v", err)
	}
	_, tpl, err := inspectToDetail(inv)
	if err != nil {
		t.Fatalf("inspectToDetail: %v", err)
	}
	if len(tpl.ExtraHosts) != 1 || tpl.ExtraHosts[0] != "host.docker.internal:host-gateway" {
		t.Fatalf("ExtraHosts = %v, mau [host.docker.internal:host-gateway]", tpl.ExtraHosts)
	}
}

func TestNormalizeExtraHosts(t *testing.T) {
	got, err := normalizeExtraHosts([]string{
		"  host.docker.internal:host-gateway  ",
		"",
		"db.internal:10.0.0.5",
		// Duplikat dibuang: dua entri identik hanya membuat perintah create
		// lebih panjang tanpa mengubah apa pun.
		"db.internal:10.0.0.5",
	})
	if err != nil {
		t.Fatalf("normalizeExtraHosts: %v", err)
	}
	want := []string{"host.docker.internal:host-gateway", "db.internal:10.0.0.5"}
	if len(got) != len(want) {
		t.Fatalf("hasil = %v, mau %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hasil = %v, mau %v", got, want)
		}
	}
}

// Entri terisi separuh dilaporkan, bukan dibuang diam-diam — kalau dibuang,
// salah ketik akan terasa seperti setelan yang "tidak mau tersimpan".
func TestNormalizeExtraHosts_RejectsHalfFilledEntry(t *testing.T) {
	for _, bad := range []string{"tanpapemisah", "host.docker.internal:", ":10.0.0.5", "-awal:1.2.3.4", "ada spasi:1.2.3.4"} {
		if _, err := normalizeExtraHosts([]string{bad}); err == nil {
			t.Errorf("%q seharusnya ditolak", bad)
		}
	}
}
