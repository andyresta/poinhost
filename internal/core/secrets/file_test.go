package secrets

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func newVaultDir(t *testing.T) (string, *fileVault) {
	t.Helper()
	dir := t.TempDir()
	fv, err := newFileVault(dir)
	if err != nil {
		t.Fatalf("newFileVault: %v", err)
	}
	return dir, fv
}

// Jalur normal: kunci dibuat sekali, lalu rahasia yang ditulis instance
// pertama harus terbaca oleh instance berikutnya di direktori yang sama.
func TestFileVault_RahasiaBertahanLintasInstance(t *testing.T) {
	dir, fv := newVaultDir(t)

	if err := fv.Set("server/1/password", "s3cr3t"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	lain, err := newFileVault(dir)
	if err != nil {
		t.Fatalf("buka ulang vault: %v", err)
	}
	got, ok, err := lain.Get("server/1/password")
	if err != nil || !ok || got != "s3cr3t" {
		t.Fatalf("Get = (%q, %v, %v), mau (\"s3cr3t\", true, nil)", got, ok, err)
	}
}

// Inti perbaikannya. secret.key yang ADA tapi ukurannya salah (mis. tulisan
// terpotong saat disk penuh) dulu diperlakukan sama dengan "belum ada":
// kunci baru dibuat dan menimpa file itu, sehingga secrets.json di sebelahnya
// — yang masih terenkripsi dengan kunci lama — hilang selamanya. Sekarang
// harus gagal keras dan TIDAK menyentuh file kuncinya.
func TestFileVault_KunciRusakTidakDitimpa(t *testing.T) {
	dir, fv := newVaultDir(t)
	if err := fv.Set("penting", "jangan-hilang"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	keyPath := filepath.Join(dir, "secret.key")
	asli, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("baca kunci: %v", err)
	}
	if err := os.WriteFile(keyPath, asli[:10], 0o600); err != nil {
		t.Fatalf("potong kunci: %v", err)
	}

	if _, err := newFileVault(dir); !IsKeyUnreadable(err) {
		t.Fatalf("newFileVault err = %v, mau ErrKeyUnreadable", err)
	}

	sesudah, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("baca kunci setelah gagal: %v", err)
	}
	if len(sesudah) != 10 {
		t.Errorf("file kunci ditulis ulang (%d byte) padahal harus dibiarkan apa adanya", len(sesudah))
	}

	// Setelah file kunci dipulihkan, rahasianya harus kembali terbaca —
	// inilah yang mustahil kalau kuncinya sempat ditimpa.
	if err := os.WriteFile(keyPath, asli, 0o600); err != nil {
		t.Fatalf("pulihkan kunci: %v", err)
	}
	pulih, err := newFileVault(dir)
	if err != nil {
		t.Fatalf("buka vault setelah kunci dipulihkan: %v", err)
	}
	got, ok, err := pulih.Get("penting")
	if err != nil || !ok || got != "jangan-hilang" {
		t.Fatalf("Get = (%q, %v, %v), mau (\"jangan-hilang\", true, nil)", got, ok, err)
	}
}

// secret.key yang ada tapi tidak bisa dibaca (permission) juga tidak boleh
// memicu pembuatan kunci baru — sebabnya beda, akibatnya sama fatalnya.
func TestFileVault_KunciTidakTerbacaTidakDitimpa(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode permission POSIX tidak berlaku di windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root menembus mode file, skenario ini tidak bisa diuji sebagai root")
	}

	dir, fv := newVaultDir(t)
	if err := fv.Set("penting", "jangan-hilang"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	keyPath := filepath.Join(dir, "secret.key")
	asli, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("baca kunci: %v", err)
	}
	if err := os.Chmod(keyPath, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(keyPath, 0o600) })

	if _, err := newFileVault(dir); !IsKeyUnreadable(err) {
		t.Fatalf("newFileVault err = %v, mau ErrKeyUnreadable", err)
	}

	if err := os.Chmod(keyPath, 0o600); err != nil {
		t.Fatalf("chmod balik: %v", err)
	}
	sesudah, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("baca kunci: %v", err)
	}
	if string(sesudah) != string(asli) {
		t.Error("isi secret.key berubah padahal pembacaannya gagal")
	}
}

// Kunci yang hilang sama sekali memang boleh dibuat ulang — tanpa ini,
// instalasi baru tidak akan pernah bisa jalan.
func TestFileVault_KunciHilangDibuatBaru(t *testing.T) {
	dir, _ := newVaultDir(t)
	keyPath := filepath.Join(dir, "secret.key")
	if err := os.Remove(keyPath); err != nil {
		t.Fatalf("hapus kunci: %v", err)
	}

	if _, err := newFileVault(dir); err != nil {
		t.Fatalf("newFileVault setelah kunci hilang: %v", err)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("kunci tidak dibuat ulang: %v", err)
	}
	if info.Size() != keyLen {
		t.Errorf("ukuran kunci = %d, mau %d", info.Size(), keyLen)
	}
	if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 {
		t.Errorf("mode kunci = %o, mau 600", perm)
	}
}

// New tidak boleh lagi menelan kegagalan vault file jadi penyimpanan
// in-memory: di mesin tanpa OS keychain itu berarti semua rahasia hilang tiap
// restart tanpa ada yang tahu.
func TestNew_MengembalikanErrorAlihAlihJatuhKeMemori(t *testing.T) {
	dir := t.TempDir()
	if _, err := newFileVault(dir); err != nil {
		t.Fatalf("persiapan: %v", err)
	}
	keyPath := filepath.Join(dir, "secret.key")
	if err := os.WriteFile(keyPath, []byte("pendek"), 0o600); err != nil {
		t.Fatalf("rusakkan kunci: %v", err)
	}

	// keyringAvailable=false: yang diuji memang cabang fallback file.
	v, err := newVault(false, dir)
	if err == nil {
		t.Fatalf("newVault berhasil (%T) padahal vault file rusak", v)
	}
	if !IsKeyUnreadable(err) {
		t.Errorf("err = %v, mau bisa dikenali lewat IsKeyUnreadable", err)
	}
}
