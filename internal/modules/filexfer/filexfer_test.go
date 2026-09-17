package filexfer

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeRemotePath(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "/", false},
		{"/", "/", false},
		{"/var/www/", "/var/www", false},
		{"/var//www/./html", "/var/www/html", false},
		{"/var/www/../log", "/var/log", false},
		{"var/www", "", true},
		{"../etc", "", true},
		{"/..", "/", false},
	}
	for _, c := range cases {
		got, err := NormalizeRemotePath(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("NormalizeRemotePath(%q) seharusnya error, dapat %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeRemotePath(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("NormalizeRemotePath(%q) = %q, mau %q", c.in, got, c.want)
		}
	}
}

// Memilih induk DAN anaknya sekaligus adalah kejadian biasa saat mencentang
// daftar: tanpa pemangkasan, isi anak tersalin dua kali dan taksiran ukuran
// totalnya ikut terhitung dobel.
//
// Yang dipertahankan adalah pilihan paling SPESIFIK — induknya yang dibuang,
// mengikuti perilaku homepoin. Arah ini penting dan sengaja dikunci di tes:
// membaliknya diam-diam mengubah data apa yang benar-benar ikut pindah.
func TestPruneRedundantPaths_KeepsMostSpecificAndDropsParent(t *testing.T) {
	got := PruneRedundantPaths([]string{"/var/www/html", "/var/www", "/etc/nginx"})
	want := []string{"/etc/nginx", "/var/www/html"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PruneRedundantPaths = %v, mau %v", got, want)
	}
}

// Root dipilih bersama salah satu isinya: root harus ikut dibuang seperti
// induk mana pun. homepoin melewatkan kasus ini karena membandingkan
// prefix "/" + "/" sehingga tidak pernah cocok.
func TestPruneRedundantPaths_RootIsTreatedAsParent(t *testing.T) {
	got := PruneRedundantPaths([]string{"/", "/var"})
	want := []string{"/var"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PruneRedundantPaths = %v, mau %v", got, want)
	}
}

// Kemiripan awalan string BUKAN kemiripan path: /var/www-lama berdiri
// sendiri walau namanya diawali persis seperti /var/www.
func TestPruneRedundantPaths_SiblingWithSharedPrefixSurvives(t *testing.T) {
	got := PruneRedundantPaths([]string{"/var/www", "/var/www-lama"})
	want := []string{"/var/www", "/var/www-lama"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PruneRedundantPaths = %v, mau %v", got, want)
	}
}

func TestPruneRedundantPaths_DedupesAndCleans(t *testing.T) {
	got := PruneRedundantPaths([]string{"/opt/app/", "/opt/app", "/opt//app", ""})
	want := []string{"/opt/app"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PruneRedundantPaths = %v, mau %v", got, want)
	}
}

// `-C <induk> <nama>` adalah yang membuat arsip berisi entri relatif; kalau
// berubah jadi tar atas path absolut, ekstraksi di tujuan akan membangun
// ulang seluruh pohon path dari root, bukan menaruh isinya di folder tujuan.
func TestBuildTarSrcCmd_UsesParentDirAndBasename(t *testing.T) {
	cmd := buildTarSrcCmd("/var/www/html", nil, true)
	if !strings.Contains(cmd, "-C '/var/www'") {
		t.Errorf("tidak pindah ke folder induk: %s", cmd)
	}
	if !strings.HasSuffix(cmd, "'html'") {
		t.Errorf("tidak mengarsipkan basename saja: %s", cmd)
	}
	if !strings.HasPrefix(cmd, "tar czf -") {
		t.Errorf("tidak menulis arsip terkompresi ke stdout: %s", cmd)
	}
}

func TestBuildTarSrcCmd_ExcludeAndNoCompress(t *testing.T) {
	cmd := buildTarSrcCmd("/srv/app", []string{"node_modules", " ", "*.log"}, false)
	if !strings.HasPrefix(cmd, "tar cf -") {
		t.Errorf("kompresi seharusnya mati: %s", cmd)
	}
	if !strings.Contains(cmd, "--exclude='node_modules'") || !strings.Contains(cmd, "--exclude='*.log'") {
		t.Errorf("pola exclude hilang: %s", cmd)
	}
	if strings.Contains(cmd, "--exclude=' '") {
		t.Errorf("pola kosong seharusnya dibuang: %s", cmd)
	}
}

// Folder tujuan dibuat lebih dulu di perintah yang SAMA — kalau tidak,
// transfer ke folder yang belum ada gagal sebelum satu byte pun terkirim.
func TestBuildTarDstCmd_CreatesDestinationFirst(t *testing.T) {
	cmd := buildTarDstCmd("/srv/baru", true)
	if !strings.HasPrefix(cmd, "mkdir -p '/srv/baru' && ") {
		t.Errorf("folder tujuan tidak dibuat dulu: %s", cmd)
	}
	if !strings.Contains(cmd, "tar xzf - -C '/srv/baru'") {
		t.Errorf("ekstraksi tidak membaca stdin ke folder tujuan: %s", cmd)
	}
}

// Nama file dengan kutip tunggal harus tetap jadi SATU argumen; kalau
// lolos mentah, sisa namanya akan ditafsirkan shell sebagai perintah.
func TestShellQuote_EscapesSingleQuote(t *testing.T) {
	got := shellQuote("/tmp/it's here")
	want := `'/tmp/it'\''s here'`
	if got != want {
		t.Fatalf("shellQuote = %s, mau %s", got, want)
	}
}

func TestIsUnder(t *testing.T) {
	cases := []struct {
		child, parent string
		want          bool
	}{
		{"/var/www/html", "/var/www", true},
		{"/var/www", "/var/www", true},
		{"/var/www-lama", "/var/www", false},
		{"/var", "/var/www", false},
		{"/apa/pun", "/", true},
	}
	for _, c := range cases {
		if got := isUnder(c.child, c.parent); got != c.want {
			t.Errorf("isUnder(%q, %q) = %v, mau %v", c.child, c.parent, got, c.want)
		}
	}
}
