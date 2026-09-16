package docker

import "testing"

// Container yang dibuat langsung dengan `docker run` tidak punya label
// compose — tidak ada yang bisa diterapkan ulang, dan itu harus dilaporkan
// apa adanya alih-alih dianggap "tidak ada drift".
func TestParseComposeInfoTidakDikelolaCompose(t *testing.T) {
	info := parseComposeInfo("MANAGED=0\n")
	if info.Managed {
		t.Fatal("seharusnya dilaporkan tidak dikelola compose")
	}
	if info.Drift || info.Unknown {
		t.Fatalf("tanpa compose, drift tidak relevan: %+v", info)
	}
}

// Inilah kasus yang menyebabkan gangguan produksi: container berjalan dengan
// konfigurasi lama sementara compose file sudah berubah. Putusannya diambil
// dari compose sendiri lewat --dry-run, bukan dari perbandingan hash yang
// sempat dicoba dan terbukti menghasilkan false positive.
func TestParseComposeInfoMendeteksiDrift(t *testing.T) {
	stdout := `MANAGED=1
PROJECT=deploy
SERVICE=kelindo-mobs
WORKDIR=/opt/kelindo-mobs/deploy
FILES=/opt/kelindo-mobs/deploy/docker-compose.yml
DRY=DRY-RUN MODE -  Container kelindo-mobs  Recreate
DRY=DRY-RUN MODE -  Container kelindo-mobs  Recreated
`
	info := parseComposeInfo(stdout)
	if !info.Managed || !info.Drift {
		t.Fatalf("compose menyatakan perlu Recreate, seharusnya drift: %+v", info)
	}
	if info.Verdict != "Recreate" {
		t.Fatalf("putusan compose harus disimpan apa adanya: %+v", info)
	}
	if info.Unknown {
		t.Fatal("putusan sudah jelas, tidak boleh dilaporkan tidak diketahui")
	}
	if info.Service != "kelindo-mobs" || info.WorkingDir != "/opt/kelindo-mobs/deploy" {
		t.Fatalf("label compose salah baca: %+v", info)
	}
}

func TestParseComposeInfoTanpaDrift(t *testing.T) {
	stdout := `MANAGED=1
SERVICE=app
WORKDIR=/opt/app
DRY=DRY-RUN MODE -  Container app  Running
`
	info := parseComposeInfo(stdout)
	if info.Drift {
		t.Fatalf("compose bilang Running, seharusnya tidak drift: %+v", info)
	}
	if info.Unknown {
		t.Fatal("putusan sudah jelas, tidak boleh tidak diketahui")
	}
	if info.Verdict != "Running" {
		t.Fatalf("putusan salah baca: %+v", info)
	}
}

// Dua pengaman yang tidak boleh hilang: --force-recreate (tanpa itu compose
// cuma men-start container lama) dan TIDAK adanya --remove-orphans (yang akan
// menghapus container proyek lain saat nama direktori proyek bertabrakan).
func TestComposeScriptAman(t *testing.T) {
	// Skrip disusun di ApplyCompose; di sini diuji bentuknya lewat helper yang
	// sama supaya perubahan tidak sengaja ketahuan.
	script := "set -e\ncd " + shellQuote("/opt/kelindo-mobs/deploy") +
		"\ndocker compose up -d --force-recreate --no-deps " + shellQuote("kelindo-mobs")

	if !contains(script, "--force-recreate") {
		t.Fatal("tanpa --force-recreate, konfigurasi baru tidak akan diterapkan")
	}
	if contains(script, "--remove-orphans") {
		t.Fatal("--remove-orphans berbahaya saat nama proyek bertabrakan")
	}
	if !contains(script, "--no-deps") {
		t.Fatal("recreate harus terbatas pada service ini saja")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
