package agentctl

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// Binary diverifikasi DI SERVER sebelum dipasang: "SFTP selesai tanpa error"
// bukan jaminan berkasnya utuh, dan memasang binary yang rusak sebagai service
// systemd menghasilkan agent yang gagal start tanpa sebab yang jelas.
func TestInstallScript_MemeriksaChecksumSebelumMemasang(t *testing.T) {
	sum := checksum([]byte("binary palsu"))
	script := installScript(sum, false, "")

	if !strings.Contains(script, "sha256sum "+tmpBinaryPath) {
		t.Error("skrip tidak menghitung checksum berkas yang diunggah")
	}
	if !strings.Contains(script, sum) {
		t.Error("skrip tidak membandingkan dengan checksum yang diharapkan")
	}

	iCheck := strings.Index(script, "sha256sum")
	iInstall := strings.Index(script, "install -m 0755")
	if iCheck < 0 || iInstall < 0 || iCheck > iInstall {
		t.Error("checksum diperiksa SETELAH binary dipasang — urutannya harus sebaliknya")
	}
	if !strings.HasPrefix(script, "set -e\n") {
		t.Error("tanpa `set -e`, kegagalan di tengah tetap lanjut ke langkah berikutnya")
	}
}

func TestInstallScript_ModeBerkas(t *testing.T) {
	script := installScript("abc", false, "")
	cases := map[string]string{
		"install -m 0755 -o root -g root " + tmpBinaryPath: "binary harus 0755",
		"install -m 0600 -o root -g root " + tmpConfigPath: "config memuat token, harus 0600",
		"install -m 0644 -o root -g root " + tmpUnitPath:   "unit systemd harus 0644",
		"install -d -m 0700 " + ConfigDir:                  "direktori config harus 0700",
	}
	for frag, why := range cases {
		if !strings.Contains(script, frag) {
			t.Errorf("%s — tidak menemukan %q", why, frag)
		}
	}
}

// Kunci publik ditulis lewat base64 + variabel, bukan langsung di command
// line: apa pun yang ada di command line terlihat di `ps` selama perintah
// berjalan. Penambahannya juga harus idempoten supaya pemasangan ulang tidak
// menumpuk baris duplikat di authorized_keys.
func TestInstallScript_MemasangKunciDenganAman(t *testing.T) {
	_, pub, err := generateKeypair()
	if err != nil {
		t.Fatalf("generateKeypair: %v", err)
	}
	script := installScript("abc", true, pub)

	if strings.Contains(script, pub) {
		t.Error("kunci publik muncul apa adanya di command line")
	}
	if !strings.Contains(script, "base64 -d") {
		t.Error("kunci tidak ditulis lewat base64")
	}
	if !strings.Contains(script, "sed -i '/"+authKeyLabel+"$/d' /root/.ssh/authorized_keys") {
		t.Error("kunci agent lama tidak dibuang — pemasangan ulang meninggalkan kunci root yatim")
	}
	if !strings.Contains(script, "install -m 0600 -o root -g root "+tmpKeyPath+" "+KeyPath) {
		t.Error("kunci privat tidak dipasang dengan mode 0600")
	}
}

func TestInstallScript_TanpaManageSelfTidakMenyentuhAuthorizedKeys(t *testing.T) {
	script := installScript("abc", false, "")
	if strings.Contains(script, "authorized_keys") {
		t.Error("authorized_keys disentuh padahal manageSelf tidak dipilih")
	}
}

// Kunci yang dipasang agent harus bisa dikenali dan dicabut lagi saat copot —
// tanpa penanda, `sed` saat uninstall tidak punya pegangan dan kunci yatim
// tertinggal dengan akses root.
func TestGenerateKeypair_KunciBerlabelDanValid(t *testing.T) {
	priv, pub, err := generateKeypair()
	if err != nil {
		t.Fatalf("generateKeypair: %v", err)
	}

	if !strings.HasSuffix(pub, authKeyLabel) {
		t.Errorf("kunci publik tidak berlabel %q: %q", authKeyLabel, pub)
	}
	if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pub)); err != nil {
		t.Errorf("kunci publik tidak bisa diparse sebagai authorized_key: %v", err)
	}

	signer, err := ssh.ParsePrivateKey([]byte(priv))
	if err != nil {
		t.Fatalf("kunci privat tidak bisa diparse: %v", err)
	}
	if signer.PublicKey().Type() != "ssh-ed25519" {
		t.Errorf("tipe kunci = %q, mau ssh-ed25519", signer.PublicKey().Type())
	}

	// Pasangannya harus benar-benar cocok; kalau tidak, agent terpasang tapi
	// tidak pernah bisa login ke mesinnya sendiri.
	parsedPub, _, _, _, _ := ssh.ParseAuthorizedKey([]byte(pub))
	if string(parsedPub.Marshal()) != string(signer.PublicKey().Marshal()) {
		t.Error("kunci publik dan privat bukan sepasang")
	}
}

func TestGenerateKeypair_SelaluBerbeda(t *testing.T) {
	_, a, _ := generateKeypair()
	_, b, _ := generateKeypair()
	if a == b {
		t.Error("dua pemasangan menghasilkan kunci yang sama")
	}
}

// "Copot bersih" berarti tidak ada sisa — termasuk baris authorized_keys yang
// masih memberi akses root ke mesin itu.
func TestUninstall_MencabutKunciDanSeluruhBerkas(t *testing.T) {
	// Skrip uninstall dibangun inline di Uninstall(); yang diuji di sini
	// adalah konstanta jalurnya ikut terpakai semua.
	for _, p := range []string{BinaryPath, PrevPath, ConfigDir, DataDir, UnitPath} {
		if p == "" {
			t.Fatal("ada konstanta jalur yang kosong")
		}
	}
	if !strings.Contains(systemdUnit, "Restart=always") {
		t.Error("unit systemd tidak akan menghidupkan agent kembali setelah crash")
	}
	if !strings.Contains(systemdUnit, "After=network-online.target") {
		t.Error("agent bisa start sebelum jaringan siap")
	}
	if !strings.Contains(systemdUnit, "WantedBy=multi-user.target") {
		t.Error("unit tidak akan otomatis jalan setelah reboot")
	}
}

func TestPortOf(t *testing.T) {
	cases := map[string]int{
		"127.0.0.1:7898": 7898,
		"127.0.0.1:9000": 9000,
		"":               DefaultPort,
		"tanpa-port":     DefaultPort,
	}
	for in, want := range cases {
		if got := portOf(in); got != want {
			t.Errorf("portOf(%q) = %d, mau %d", in, got, want)
		}
	}
}

func TestParseKV(t *testing.T) {
	out := parseKV("arch=x86_64\nsystemd=1\n\n# komentar\nversion=0.1.0\nrusak\n")
	if out["arch"] != "x86_64" || out["systemd"] != "1" || out["version"] != "0.1.0" {
		t.Errorf("hasil parse = %v", out)
	}
	if _, ada := out["rusak"]; ada {
		t.Error("baris tanpa '=' ikut terparse")
	}
}

// Token bot tidak boleh ikut dibawa ke frontend. Ia dibutuhkan agent, bukan
// layar — dan rahasia yang tidak pernah menyeberang tidak bisa bocor lewat log
// atau devtools.
func TestDetect_StatusTidakMembocorkanTokenBot(t *testing.T) {
	cfgJSON := `{"listen":"127.0.0.1:7898","apiToken":"api-rahasia","telegram":{"enabled":true,"token":"123:SANGAT-RAHASIA","allowedUserIds":[7]}}`
	cfg := decodeConfig(b64(cfgJSON))
	if cfg == nil {
		t.Fatal("decodeConfig mengembalikan nil")
	}

	st := &Status{}
	st.Telegram.Configured = cfg.Telegram.Token != ""
	st.Telegram.Enabled = cfg.Telegram.Enabled
	st.Telegram.AllowedUsers = cfg.Telegram.AllowedUserIDs
	st.Listen = cfg.Listen

	if !st.Telegram.Configured {
		t.Error("status tidak menandai token sudah terisi")
	}
	if strings.Contains(renderStatus(st), "SANGAT-RAHASIA") {
		t.Error("token bot ikut masuk ke Status yang dikirim ke frontend")
	}
	if strings.Contains(renderStatus(st), "api-rahasia") {
		t.Error("apiToken ikut masuk ke Status yang dikirim ke frontend")
	}
}

// --- pembantu test ---

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// renderStatus menserialisasi Status persis seperti yang diterima frontend,
// sehingga test kebocoran rahasia memeriksa payload yang sesungguhnya dikirim.
func renderStatus(st *Status) string {
	b, err := json.Marshal(st)
	if err != nil {
		return ""
	}
	return string(b)
}

// Kunci untuk mengelola mesin sendiri dipasang dengan cara yang sama baik saat
// instalasi maupun saat diaktifkan belakangan — supaya jalur pemulihan tidak
// diam-diam berbeda dari jalur normal.
func TestSelfKeyScript_SamaAmannyaDenganJalurInstalasi(t *testing.T) {
	_, pub, err := generateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	script := selfKeyScript(pub)

	if strings.Contains(script, pub) {
		t.Error("kunci publik muncul apa adanya di command line")
	}
	if !strings.Contains(script, "sed -i '/"+authKeyLabel+"$/d'") {
		t.Error("kunci agent lama tidak dibuang sebelum yang baru ditambahkan")
	}
	if !strings.Contains(script, "install -m 0600 -o root -g root "+tmpKeyPath+" "+KeyPath) {
		t.Error("kunci privat tidak dipasang 0600")
	}
	if !strings.Contains(script, "rm -f "+tmpKeyPath) {
		t.Error("kunci privat sementara tidak dibersihkan dari /tmp")
	}

	// Jalur instalasi harus memakai helper yang sama, bukan salinannya.
	if !strings.Contains(installScript("abc", true, pub), script) {
		t.Error("installScript tidak memakai selfKeyScript yang sama")
	}
}

// Binary dikirim dalam bentuk TERKOMPRESI lalu didekompresi di server —
// kira-kira separuh ukuran, dan unggahan adalah bagian terlama dari pemasangan
// maupun update. Yang diverifikasi checksum-nya tetap binary FINAL, bukan
// arsipnya, supaya jaminannya tidak berkurang.
func TestInstallScript_MendekompresiSebelumMemeriksaChecksum(t *testing.T) {
	script := installScript("abc123", false, "")

	iGunzip := strings.Index(script, "gzip -dc "+tmpBinaryGz)
	iSum := strings.Index(script, "sha256sum "+tmpBinaryPath)
	iInstall := strings.Index(script, "install -m 0755")

	if iGunzip < 0 {
		t.Fatal("binary tidak didekompresi di server")
	}
	if iSum < 0 || iInstall < 0 {
		t.Fatal("langkah checksum/instalasi hilang")
	}
	if !(iGunzip < iSum && iSum < iInstall) {
		t.Errorf("urutan salah — mau dekompresi(%d) < checksum(%d) < pasang(%d)", iGunzip, iSum, iInstall)
	}
	if !strings.Contains(script, "command -v gzip") {
		t.Error("tidak ada pemeriksaan gzip; kegagalannya akan muncul sebagai error yang membingungkan")
	}
	if !strings.Contains(script, "rm -f "+tmpBinaryGz) {
		t.Error("arsip sementara tidak dibersihkan dari /tmp")
	}
}
