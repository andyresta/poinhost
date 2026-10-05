package agentcfg

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Jaminan paling penting di package ini: agent TIDAK BOLEH bisa mendengarkan
// di alamat yang bisa dijangkau jaringan. Agent memegang akses setara root ke
// mesinnya sekaligus kredensial SSH ke semua server terdaftar, jadi satu salah
// ketik di config tidak boleh cukup untuk mengekspos semuanya.
func TestValidateListen_MenolakAlamatPublik(t *testing.T) {
	publik := []string{
		"0.0.0.0:7898",
		"192.168.1.10:7898",
		"10.0.0.5:7898",
		"1.2.3.4:7898",
		"[::]:7898",
		"[2001:db8::1]:7898",
	}
	for _, addr := range publik {
		if err := ValidateListen(addr); !errors.Is(err, ErrListenNotLoopback) {
			t.Errorf("ValidateListen(%q) = %v, mau ditolak", addr, err)
		}
	}
}

func TestValidateListen_MenerimaLoopback(t *testing.T) {
	ok := []string{"127.0.0.1:7898", "127.0.0.53:7898", "localhost:7898", "[::1]:7898"}
	for _, addr := range ok {
		if err := ValidateListen(addr); err != nil {
			t.Errorf("ValidateListen(%q) = %v, mau diterima", addr, err)
		}
	}
}

func TestValidateListen_MenolakBentukSalah(t *testing.T) {
	for _, addr := range []string{"", "127.0.0.1", "bukan-alamat:7898"} {
		if err := ValidateListen(addr); err == nil {
			t.Errorf("ValidateListen(%q) berhasil, mau error", addr)
		}
	}
}

// Config yang alamat listen-nya publik harus membuat agent GAGAL START, bukan
// sekadar mencatat peringatan.
func TestLoad_MenolakConfigDenganListenPublik(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(`{"listen":"0.0.0.0:7898","enrollToken":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); !errors.Is(err, ErrListenNotLoopback) {
		t.Fatalf("Load = %v, mau ErrListenNotLoopback", err)
	}
}

func TestLoad_MengisiDefault(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(`{"enrollToken":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != DefaultListen {
		t.Errorf("Listen = %q, mau %q", cfg.Listen, DefaultListen)
	}
	if cfg.DataDir != DefaultDataDir {
		t.Errorf("DataDir = %q, mau %q", cfg.DataDir, DefaultDataDir)
	}
}


// Konfigurasi harus bertahan melewati restart agent — terutama enrollToken,
// karena tanpa itu seluruh API Mobile Control menolak melayani sampai agent
// dipasang ulang.
func TestSaveLoad_MempertahankanEnrollToken(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	in := &Config{
		Listen:   DefaultListen,
		EnrollToken: "rahasia",
	}
	if err := Save(p, in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// Config memuat token API; mode file adalah satu-satunya yang
	// memisahkannya dari user lain di server itu.
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode config = %o, mau 600", perm)
	}

	out, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.EnrollToken != "rahasia" {
		t.Errorf("EnrollToken = %q", out.EnrollToken)
	}
}


// Regresi dari bug nyata: agent versi lama menyimpan allowlist dengan menulis
// ulang seluruh config dari struct-nya, sehingga blok `selfServer` yang baru
// dikenal app desktop terhapus diam-diam — dan agent kehilangan kemampuan
// mengelola mesinnya sendiri tanpa satu pun pesan error.
//
// Versi desktop dan versi agent memang dirancang bisa berbeda, jadi tidak ada
// sisi yang boleh menghapus field yang tidak dikenalnya.
func TestSave_MempertahankanFieldYangTidakDikenal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")

	// Ditulis oleh app desktop yang lebih baru: memuat blok yang belum ada di
	// struct agent versi ini.
	awal := `{
  "listen": "127.0.0.1:7898",
  "enrollToken": "lama",
  "selfServer": {"enabled": true, "host": "127.0.0.1", "port": 22, "username": "root", "opsiBaru": "x"},
  "fiturMasaDepan": {"aktif": true, "nilai": 42}
}`
	if err := os.WriteFile(p, []byte(awal), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Meniru desktop memutar token API lalu menyimpan.
	cfg.EnrollToken = "rahasia"
	if err := Save(p, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var got map[string]any
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("hasil Save bukan JSON valid: %v", err)
	}

	// Blok tak dikenal di level atas harus utuh.
	if _, ada := got["fiturMasaDepan"]; !ada {
		t.Error("blok tak dikenal di level atas terhapus")
	}
	// Kunci tak dikenal DI DALAM objek yang dikenal juga harus utuh.
	self, _ := got["selfServer"].(map[string]any)
	if self == nil || self["opsiBaru"] != "x" {
		t.Errorf("kunci tak dikenal di dalam selfServer terhapus: %v", got["selfServer"])
	}
	// Yang memang diubah pemanggil harus tertulis.
	if got["enrollToken"] != "rahasia" {
		t.Errorf("enrollToken = %v, mau yang baru", got["enrollToken"])
	}

	// Dan yang paling penting: config tetap bisa dimuat ulang dengan benar.
	ulang, err := Load(p)
	if err != nil {
		t.Fatalf("Load setelah Save: %v", err)
	}
	if !ulang.SelfServer.Enabled {
		t.Error("selfServer.enabled hilang setelah disimpan ulang")
	}
	if ulang.EnrollToken != "rahasia" {
		t.Errorf("EnrollToken = %q, mau %q", ulang.EnrollToken, "rahasia")
	}
}

// Agent yang binary-nya di-update membaca config yang ditulis versi lama,
// dan di versi itu nama tokennya apiToken. Tanpa migrasi, Mobile Control mati
// diam-diam begitu agent di-update: pendaftaran ditutup karena enrollToken
// kosong, padahal user tidak mengubah apa pun.
func TestLoad_MigrasiTokenLama(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	lama := `{
  "listen": "127.0.0.1:7898",
  "apiToken": "rahasia-lama",
  "selfServer": {"enabled": true, "host": "127.0.0.1", "port": 22, "username": "root"}
}`
	if err := os.WriteFile(path, []byte(lama), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.EnrollToken != "rahasia-lama" {
		t.Errorf("EnrollToken = %q, mau nilai apiToken lama", cfg.EnrollToken)
	}
}

// Kalau keduanya ada, yang menang adalah nama baru — config itu ditulis
// desktop versi baru, dan apiToken yang tertinggal di sana hanyalah sisa.
func TestLoad_TokenBaruMengalahkanYangLama(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	isi := `{"listen": "127.0.0.1:7898", "apiToken": "lama", "enrollToken": "baru"}`
	if err := os.WriteFile(path, []byte(isi), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.EnrollToken != "baru" {
		t.Errorf("EnrollToken = %q, mau \"baru\"", cfg.EnrollToken)
	}
}
