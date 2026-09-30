package sitexfer

import (
	"strings"
	"testing"
)

func TestDestProbeScript_ReadsSpaceFromNearestExistingAncestor(t *testing.T) {
	script := destProbeScript([]string{"/var/www/baru.web.id"})

	// Folder tujuan biasanya BELUM ada; `df` atas path yang tidak ada cuma
	// menghasilkan error, jadi skrip harus naik dulu ke leluhur terdekat.
	if !strings.Contains(script, `while [ ! -d "$d" ]`) {
		t.Errorf("tidak naik ke leluhur yang ada: %s", script)
	}
	// df --output= hanya ada di coreutils GNU; server tujuan belum tentu.
	if strings.Contains(script, "--output") {
		t.Errorf("memakai df --output yang tidak portabel: %s", script)
	}
	if !strings.Contains(script, "df -P") {
		t.Errorf("tidak memakai format POSIX df: %s", script)
	}
	// Kekosongan folder harus diperiksa tanpa membaca seluruh isinya.
	if !strings.Contains(script, "-print -quit") {
		t.Errorf("pemeriksaan folder kosong membaca seluruh isi: %s", script)
	}
}

func TestParseDestProbe(t *testing.T) {
	out := strings.Join([]string{
		"PATH\t/var/www/a",
		"EXISTS\t1",
		"NONEMPTY\t1",
		"MOUNT\t/",
		"AVAIL\t1048576",
		"PATH\t/srv/b",
		"EXISTS\t0",
		"MOUNT\t/srv",
		"AVAIL\t2097152",
	}, "\n")

	got := parseDestProbe(out)
	if len(got) != 2 {
		t.Fatalf("jumlah entri = %d, mau 2", len(got))
	}
	a := got["/var/www/a"]
	if !a.Exists || !a.NonEmpty || a.Mount != "/" || a.Avail != 1048576 {
		t.Errorf("/var/www/a = %+v", a)
	}
	// Folder yang belum ada tidak mencetak NONEMPTY sama sekali; itu harus
	// terbaca sebagai kosong, bukan mewarisi nilai path sebelumnya.
	b := got["/srv/b"]
	if b.Exists || b.NonEmpty || b.Mount != "/srv" || b.Avail != 2097152 {
		t.Errorf("/srv/b = %+v", b)
	}
}

// Beberapa folder tujuan bisa berbagi satu filesystem. Memeriksanya satu per
// satu akan menyatakan muat untuk masing-masing padahal jumlahnya tidak
// muat bersama-sama.
func TestSpaceShortfall_SumsPathsSharingAFilesystem(t *testing.T) {
	need := map[string]int64{"/var/www/a": 600, "/var/www/b": 600}
	probes := map[string]destProbe{
		"/var/www/a": {Mount: "/", Avail: 1000},
		"/var/www/b": {Mount: "/", Avail: 1000},
	}
	short := spaceShortfall(need, probes)
	v, ok := short["/"]
	if !ok {
		t.Fatalf("seharusnya kurang, dapat %+v", short)
	}
	if v[0] != 1200 || v[1] != 1000 {
		t.Fatalf("butuh/tersedia = %d/%d, mau 1200/1000", v[0], v[1])
	}
}

func TestSpaceShortfall_SeparateFilesystemsJudgedSeparately(t *testing.T) {
	need := map[string]int64{"/var/www/a": 600, "/srv/b": 600}
	probes := map[string]destProbe{
		"/var/www/a": {Mount: "/", Avail: 1000},
		"/srv/b":     {Mount: "/srv", Avail: 100},
	}
	short := spaceShortfall(need, probes)
	if len(short) != 1 {
		t.Fatalf("hanya /srv yang kurang, dapat %+v", short)
	}
	if _, ok := short["/srv"]; !ok {
		t.Fatalf("mount yang kurang salah: %+v", short)
	}
}

// `df` yang tidak terbaca menghasilkan Avail 0. Itu berarti "tidak tahu",
// bukan "penuh" — memblokir migrasi atas dasar pembacaan yang gagal akan
// menghentikan pekerjaan yang sebenarnya baik-baik saja.
func TestSpaceShortfall_UnknownAvailIsNotTreatedAsFull(t *testing.T) {
	need := map[string]int64{"/var/www/a": 600}
	probes := map[string]destProbe{"/var/www/a": {Mount: "/", Avail: 0}}
	if short := spaceShortfall(need, probes); len(short) != 0 {
		t.Fatalf("avail tidak terbaca seharusnya tidak memblokir, dapat %+v", short)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		512:             "512 B",
		1024:            "1.0 KiB",
		5 * 1024 * 1024: "5.0 MiB",
		3 * 1073741824:  "3.0 GiB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, mau %q", in, got, want)
		}
	}
}
