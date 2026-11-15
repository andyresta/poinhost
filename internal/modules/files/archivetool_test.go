package files

import (
	"strings"
	"testing"
)

func TestArchiveFormat(t *testing.T) {
	kasus := map[string]string{
		"/tmp/situs.zip":     "zip",
		"/tmp/SITUS.ZIP":     "zip",
		"/tmp/backup.tar.gz": "tar.gz",
		"/tmp/backup.tgz":    "tar.gz",
	}
	for path, mau := range kasus {
		got, ok := archiveFormat(path)
		if !ok || got != mau {
			t.Errorf("archiveFormat(%q) = %q,%v — mau %q", path, got, ok, mau)
		}
	}
	for _, path := range []string{"/tmp/a.rar", "/tmp/a.gz", "/tmp/a.tar", "/tmp/zip"} {
		if _, ok := archiveFormat(path); ok {
			t.Errorf("archiveFormat(%q) menerima format yang tidak diekstrak Extract", path)
		}
	}
}

// Syarat tiap format BEDA, dan di situlah letak bugnya kalau disamakan:
// zip butuh SALAH SATU dari unzip/python, tar.gz butuh tar DAN gzip.
func TestSiapEkstrak(t *testing.T) {
	kasus := []struct {
		nama   string
		format string
		ada    []string
		ready  bool
		kurang []string
	}{
		{"zip, unzip ada", "zip", []string{"unzip"}, true, nil},
		{"zip, cuma python3", "zip", []string{"python3"}, true, nil},
		{"zip, cuma python", "zip", []string{"python"}, true, nil},
		{"zip, kosong", "zip", nil, false, []string{"unzip"}},
		{"zip, tar ada tapi tidak menolong", "zip", []string{"tar", "gzip"}, false, []string{"unzip"}},
		{"targz, lengkap", "tar.gz", []string{"tar", "gzip"}, true, nil},
		{"targz, gzip hilang", "tar.gz", []string{"tar"}, false, []string{"gzip"}},
		{"targz, tar hilang", "tar.gz", []string{"gzip"}, false, []string{"tar"}},
		{"targz, kosong", "tar.gz", nil, false, []string{"tar", "gzip"}},
		// python TIDAK menolong tar.gz — fallback python cuma ada untuk zip.
		{"targz, python tidak menolong", "tar.gz", []string{"python3"}, false, []string{"tar", "gzip"}},
	}

	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			ada := map[string]bool{}
			for _, c := range k.ada {
				ada[c] = true
			}
			ready, kurang := siapEkstrak(k.format, ada)
			if ready != k.ready {
				t.Errorf("ready = %v, mau %v", ready, k.ready)
			}
			if strings.Join(kurang, ",") != strings.Join(k.kurang, ",") {
				t.Errorf("kurang = %v, mau %v", kurang, k.kurang)
			}
		})
	}
}

// Setiap perintah yang bisa dilaporkan kurang HARUS punya nama paket.
// Tanpa jaminan ini, modal bisa menawarkan pemasangan yang ternyata tidak
// memasang apa pun lalu melapor "Selesai" — kegagalan yang jauh lebih
// membingungkan daripada exit 127 yang digantikannya.
func TestSetiapYangKurangPunyaPaket(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		_, kurang := siapEkstrak(format, map[string]bool{})
		if len(kurang) == 0 {
			t.Fatalf("%s: tidak ada yang kurang di server kosong — pemeriksaannya tidak menguji apa pun", format)
		}
		paket := paketUntukSemua(kurang)
		if len(paket) != len(kurang) {
			t.Errorf("%s: kurang %v tapi paketnya cuma %v", format, kurang, paket)
		}
	}
}

func TestParseAlatOutput(t *testing.T) {
	out := `ID=rocky
PRETTY=Rocky Linux 8.10 (Green Obsidian)
PM=dnf
PM=yum
HAVE=tar
HAVE=gzip`
	id, nama, pm, ada := parseAlatOutput(out)
	if id != "rocky" {
		t.Errorf("id = %q", id)
	}
	if nama != "Rocky Linux 8.10 (Green Obsidian)" {
		t.Errorf("nama = %q", nama)
	}
	// dnf DAN yum ada di RHEL 8+; dnf yang dipakai.
	if pm != "dnf" {
		t.Errorf("pm = %q, mau dnf", pm)
	}
	if !ada["tar"] || !ada["gzip"] || ada["unzip"] {
		t.Errorf("ada = %v", ada)
	}
}

// Yang dicari binarinya (`apt-get`), yang disebut keluarganya (`apt`).
// pkgfail.HostRepo dan installPaketScript keduanya memakai "apt".
func TestParseAlatOutput_AptGetDisebutApt(t *testing.T) {
	if _, _, pm, _ := parseAlatOutput("ID=debian\nPM=apt-get"); pm != "apt" {
		t.Errorf("pm = %q, mau apt", pm)
	}
}

func TestParseAlatOutput_TanpaOsRelease(t *testing.T) {
	id, nama, pm, _ := parseAlatOutput("")
	if id != "unknown" || nama != "unknown" || pm != "" {
		t.Errorf("id=%q nama=%q pm=%q", id, nama, pm)
	}
}

// Setiap package manager yang BISA terdeteksi harus punya cabang skrip.
// Kalau tidak, ArchiveTool menolak dengan Blocker — benar, tapi berarti
// fiturnya diam-diam tidak jalan di distro itu.
func TestInstallPaketScript_SemuaPMYangTerdeteksi(t *testing.T) {
	for _, pm := range pmPrioritas {
		script, ok := installPaketScript(pm, []string{"unzip"})
		if !ok {
			t.Errorf("%s terdeteksi tapi tidak punya skrip instalasi", pm)
			continue
		}
		if !strings.HasPrefix(script, "set -e\n") {
			t.Errorf("%s: skrip tidak diawali set -e — langkah yang gagal akan dilewati diam-diam", pm)
		}
		if !strings.Contains(script, "unzip") {
			t.Errorf("%s: nama paket tidak ikut di skrip", pm)
		}
		if perintahManual(pm, []string{"unzip"}) == "" {
			t.Errorf("%s: tidak punya perintah manual untuk ditampilkan", pm)
		}
	}
}

// Instalasi berjalan di sesi SSH tanpa manusia di depannya. Satu prompt
// "Do you want to continue? [Y/n]" akan menggantung sampai timeout.
func TestInstallPaketScript_TidakInteraktif(t *testing.T) {
	mau := map[string]string{
		"apt":    "-y",
		"dnf":    "-y",
		"yum":    "-y",
		"zypper": "--non-interactive",
		"pacman": "--noconfirm",
		"apk":    "--no-cache",
	}
	for pm, flag := range mau {
		script, _ := installPaketScript(pm, []string{"unzip"})
		if !strings.Contains(script, flag) {
			t.Errorf("%s: skrip tidak memuat %q — bisa menggantung menunggu jawaban", pm, flag)
		}
	}
	// apt punya satu prompt lagi (konfigurasi paket) yang hanya dimatikan
	// oleh DEBIAN_FRONTEND, bukan oleh -y.
	script, _ := installPaketScript("apt", []string{"unzip"})
	if !strings.Contains(script, "DEBIAN_FRONTEND=noninteractive") {
		t.Error("apt: DEBIAN_FRONTEND tidak diset")
	}
}

func TestInstallPaketScript_TanpaPaketAtauPMAsing(t *testing.T) {
	if _, ok := installPaketScript("apt", nil); ok {
		t.Error("menyusun skrip pemasangan tanpa satu pun paket")
	}
	if _, ok := installPaketScript("emerge", []string{"unzip"}); ok {
		t.Error("package manager yang tidak didukung dianggap bisa")
	}
}

// `sudo` polos MENGGANTUNG menunggu prompt password di TTY yang tidak ada
// pada sesi exec SSH. Yang dipakai harus -S (ada password) atau -n (tidak).
func TestRootAccessWrap(t *testing.T) {
	root := (&rootAccess{sshUser: "root"}).wrap("id -u")
	if strings.Contains(root, "sudo") {
		t.Errorf("user root masih lewat sudo: %s", root)
	}
	if !strings.HasPrefix(root, "sh -c ") {
		t.Errorf("tidak dibungkus sh -c: %s", root)
	}

	tanpaPw := (&rootAccess{sshUser: "deploy"}).wrap("id -u")
	if !strings.HasPrefix(tanpaPw, "sudo -n ") {
		t.Errorf("tanpa password tidak memakai sudo -n: %s", tanpaPw)
	}

	denganPw := (&rootAccess{sshUser: "deploy", sudoPassword: "rahasia"}).wrap("id -u")
	if !strings.Contains(denganPw, "sudo -S -p ''") {
		t.Errorf("dengan password tidak memakai sudo -S: %s", denganPw)
	}
	if !strings.Contains(denganPw, `'rahasia'`) {
		t.Errorf("password tidak dikutip dengan benar: %s", denganPw)
	}
}

// Alpine tidak memasang bash secara bawaan, dan modul ini memang dipakai di
// sana (apk ada di daftar yang didukung).
func TestRootAccessWrap_TidakButuhBash(t *testing.T) {
	for _, a := range []*rootAccess{
		{sshUser: "root"},
		{sshUser: "deploy"},
		{sshUser: "deploy", sudoPassword: "x"},
	} {
		if strings.Contains(a.wrap("echo hai"), "bash") {
			t.Errorf("skrip dibungkus bash: %s", a.wrap("echo hai"))
		}
	}
}

// Skrip deteksi tidak boleh pernah gagal: yang dicari justru perintah yang
// TIDAK ada, dan kode keluar bukan nol dari pemeriksaannya sendiri akan
// terbaca sebagai "sudo ditolak" di StreamInstallArchiveTool.
func TestAlatDetectScript_SelaluKeluarNol(t *testing.T) {
	if !strings.HasPrefix(alatDetectScript, "set +e\n") {
		t.Error("skrip deteksi tidak mematikan set -e")
	}
	if !strings.HasSuffix(strings.TrimSpace(alatDetectScript), "exit 0") {
		t.Error("skrip deteksi tidak berakhir dengan exit 0")
	}
	// Semua yang dibaca siapEkstrak harus benar-benar diperiksa skripnya.
	for _, c := range []string{"unzip", "tar", "gzip", "python3", "python"} {
		if !strings.Contains(alatDetectScript, c) {
			t.Errorf("skrip deteksi tidak memeriksa %q", c)
		}
	}
	for _, pm := range []string{"apt-get", "dnf", "yum", "zypper", "pacman", "apk"} {
		if !strings.Contains(alatDetectScript, pm) {
			t.Errorf("skrip deteksi tidak memeriksa %q", pm)
		}
	}
}
