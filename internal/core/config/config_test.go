package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// At menempatkan SELURUH artefak data di bawah direktori yang diminta —
// inilah yang membuat satu binary bisa jalan sebagai app desktop di
// ~/.poinhost dan sebagai service di /var/lib/poinhost-agent tanpa cabang
// kode sendiri-sendiri.
func TestAt_SemuaPathIkutDirektoriYangDiminta(t *testing.T) {
	dir := t.TempDir()

	cfg, err := At(dir)
	if err != nil {
		t.Fatalf("At: %v", err)
	}

	if cfg.DataDir != dir {
		t.Errorf("DataDir = %q, mau %q", cfg.DataDir, dir)
	}
	if want := filepath.Join(dir, "logs"); cfg.LogDir != want {
		t.Errorf("LogDir = %q, mau %q", cfg.LogDir, want)
	}
	if want := filepath.Join(dir, "poinhost.db"); cfg.DBPath() != want {
		t.Errorf("DBPath = %q, mau %q", cfg.DBPath(), want)
	}
	if want := filepath.Join(dir, "ssh_known_hosts"); cfg.KnownHostsPath() != want {
		t.Errorf("KnownHostsPath = %q, mau %q", cfg.KnownHostsPath(), want)
	}
}

// Path relatif dijadikan absolut di satu tempat, supaya service yang
// working directory-nya berubah (systemd) tidak tiba-tiba menulis data ke
// lokasi lain.
func TestAt_PathRelatifDijadikanAbsolut(t *testing.T) {
	cfg, err := At("data-relatif")
	if err != nil {
		t.Fatalf("At: %v", err)
	}
	if !filepath.IsAbs(cfg.DataDir) {
		t.Errorf("DataDir = %q, mau absolut", cfg.DataDir)
	}
	if !strings.HasSuffix(cfg.DataDir, "data-relatif") {
		t.Errorf("DataDir = %q, mau berakhiran data-relatif", cfg.DataDir)
	}
}

func TestAt_MenolakDirektoriKosong(t *testing.T) {
	if _, err := At("   "); err == nil {
		t.Fatal("At(\"   \") berhasil, mau error")
	}
}

// Parameter SSH/executor tidak boleh ikut berubah hanya karena direktorinya
// beda — agent dan app desktop harus berperilaku sama.
func TestAt_ParameterRuntimeSamaDenganDefault(t *testing.T) {
	t.Setenv(EnvDataDir, t.TempDir())

	def, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	lain, err := At(t.TempDir())
	if err != nil {
		t.Fatalf("At: %v", err)
	}

	if !reflect.DeepEqual(def.SSH, lain.SSH) {
		t.Errorf("SSHConfig berbeda:\ndefault = %+v\nat      = %+v", def.SSH, lain.SSH)
	}
	if !reflect.DeepEqual(def.Executor, lain.Executor) {
		t.Errorf("ExecutorConfig berbeda:\ndefault = %+v\nat      = %+v", def.Executor, lain.Executor)
	}
}

// POINHOST_DATA_DIR memindahkan data app desktop tanpa mengubah kode —
// dipakai untuk dev dan untuk menjalankan dua instance berdampingan.
func TestDefault_MenghormatiEnvDataDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDataDir, dir)

	cfg, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if cfg.DataDir != dir {
		t.Errorf("DataDir = %q, mau %q", cfg.DataDir, dir)
	}
}

// EnsureDirs harus membuat data dir DAN log dir, termasuk saat induknya belum
// ada — instalasi agent membuat /var/lib/poinhost-agent dari nol.
func TestEnsureDirs_MembuatSampaiLevelBersarang(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "belum", "ada", "poinhost-agent")

	cfg, err := At(target)
	if err != nil {
		t.Fatalf("At: %v", err)
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	for _, p := range []string{cfg.DataDir, cfg.LogDir} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if !info.IsDir() {
			t.Errorf("%s bukan direktori", p)
		}
		if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o700 {
			t.Errorf("mode %s = %o, mau 700", p, perm)
		}
	}
}
