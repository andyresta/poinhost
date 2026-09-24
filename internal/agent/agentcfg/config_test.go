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
	if err := os.WriteFile(p, []byte(`{"listen":"0.0.0.0:7898","apiToken":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); !errors.Is(err, ErrListenNotLoopback) {
		t.Fatalf("Load = %v, mau ErrListenNotLoopback", err)
	}
}

func TestLoad_MengisiDefault(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(`{"apiToken":"x"}`), 0o600); err != nil {
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

// Telegram yang diaktifkan tanpa token adalah kesalahan konfigurasi yang harus
// terlihat saat start, bukan berupa bot yang diam tanpa penjelasan.
func TestValidate_TelegramAktifTanpaToken(t *testing.T) {
	cfg := &Config{Listen: DefaultListen, Telegram: Telegram{Enabled: true}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate berhasil padahal token kosong")
	}
}

// Hasil pairing harus bertahan melewati restart agent — tanpa itu user harus
// pairing ulang setiap kali agent di-restart, termasuk setelah update.
func TestSaveLoad_MempertahankanAllowlist(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	in := &Config{
		Listen:   DefaultListen,
		APIToken: "rahasia",
		Telegram: Telegram{Enabled: true, Token: "123:ABC", AllowedUserIDs: []int64{11, 22}},
	}
	if err := Save(p, in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// Config memuat token bot dan token API; mode file adalah satu-satunya
	// yang memisahkannya dari user lain di server itu.
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode config = %o, mau 600", perm)
	}

	out, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(out.Telegram.AllowedUserIDs) != 2 || out.Telegram.AllowedUserIDs[0] != 11 {
		t.Errorf("AllowedUserIDs = %v, mau [11 22]", out.Telegram.AllowedUserIDs)
	}
	if out.APIToken != "rahasia" {
		t.Errorf("APIToken = %q", out.APIToken)
	}
}

func TestTelegram_IsAllowed(t *testing.T) {
	tg := Telegram{AllowedUserIDs: []int64{5, 9}}
	if !tg.IsAllowed(9) {
		t.Error("user yang terdaftar ditolak")
	}
	if tg.IsAllowed(7) {
		t.Error("user yang tidak terdaftar diterima")
	}
	if (Telegram{}).IsAllowed(0) {
		t.Error("allowlist kosong menerima user id 0")
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
  "apiToken": "rahasia",
  "selfServer": {"enabled": true, "host": "127.0.0.1", "port": 22, "username": "root"},
  "fiturMasaDepan": {"aktif": true, "nilai": 42},
  "telegram": {"enabled": true, "token": "123:ABC", "allowedUserIds": [], "opsiBaru": "x"}
}`
	if err := os.WriteFile(p, []byte(awal), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Meniru pairing: agent menambahkan satu user lalu menyimpan.
	cfg.Telegram.AllowedUserIDs = append(cfg.Telegram.AllowedUserIDs, 6394436882)
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
	tg, _ := got["telegram"].(map[string]any)
	if tg == nil || tg["opsiBaru"] != "x" {
		t.Errorf("kunci tak dikenal di dalam telegram terhapus: %v", got["telegram"])
	}
	// Yang memang diubah pemanggil harus tertulis.
	ids, _ := tg["allowedUserIds"].([]any)
	if len(ids) != 1 {
		t.Errorf("allowedUserIds = %v, mau berisi satu user", tg["allowedUserIds"])
	}

	// Dan yang paling penting: config tetap bisa dimuat ulang dengan benar.
	ulang, err := Load(p)
	if err != nil {
		t.Fatalf("Load setelah Save: %v", err)
	}
	if !ulang.SelfServer.Enabled {
		t.Error("selfServer.enabled hilang setelah agent menyimpan allowlist")
	}
	if len(ulang.Telegram.AllowedUserIDs) != 1 {
		t.Errorf("AllowedUserIDs = %v", ulang.Telegram.AllowedUserIDs)
	}
}
