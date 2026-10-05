package devices

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Perangkat harus selamat dari restart agent. systemd me-restart agent setiap
// kali token diputar atau service di-restart dari panel; perangkat yang hanya
// ada di memori berarti setiap restart memaksa semua HP mendaftar ulang.
func TestPersisten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	dev, token, err := s.Register("HP Andy", "android")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStatus(dev.ID, StatusApproved); err != nil {
		t.Fatal(err)
	}

	ulang, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := ulang.Authenticate(token)
	if !ok {
		t.Fatal("token tidak dikenali setelah dimuat ulang")
	}
	if got.Status != StatusApproved {
		t.Errorf("status = %q, mau approved", got.Status)
	}
	if got.Name != "HP Andy" || got.Platform != "android" {
		t.Errorf("label hilang: %+v", got)
	}
}

// Berkas yang belum ada adalah keadaan normal agent yang belum pernah
// didaftari HP — bukan galat yang menggagalkan start.
func TestOpen_BerkasBelumAda(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "belum-ada.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(s.List()) != 0 {
		t.Error("daftar tidak kosong")
	}
	if _, ok := s.Authenticate("apa pun"); ok {
		t.Error("token asing diterima")
	}
}

// Yang menunggu harus di atas: itu satu-satunya yang menuntut tindakan, dan
// daftar panjang yang menyembunyikannya membuat permintaan baru terlewat.
func TestList_MenungguDiAtas(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "d.json"))
	lama, _, _ := s.Register("lama", "")
	if _, err := s.SetStatus(lama.ID, StatusApproved); err != nil {
		t.Fatal(err)
	}
	baru, _, _ := s.Register("baru", "")

	list := s.List()
	if len(list) != 2 || list[0].ID != baru.ID {
		t.Errorf("urutan salah: %+v", list)
	}
}

// Menghapus berbeda dari mencabut: token yang dihapus harus jadi asing,
// sementara yang dicabut tetap dikenali supaya HP-nya bisa diberi tahu.
func TestDeleteBerbedaDariRevoke(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "d.json"))
	dicabut, tokenCabut, _ := s.Register("dicabut", "")
	dihapus, tokenHapus, _ := s.Register("dihapus", "")

	if _, err := s.SetStatus(dicabut.ID, StatusRevoked); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(dihapus.ID); err != nil {
		t.Fatal(err)
	}

	if d, ok := s.Authenticate(tokenCabut); !ok || d.Status != StatusRevoked {
		t.Errorf("yang dicabut tidak lagi dikenali: %+v %v", d, ok)
	}
	if _, ok := s.Authenticate(tokenHapus); ok {
		t.Error("token yang dihapus masih dikenali")
	}
	if err := s.Delete("tidak-ada"); err == nil {
		t.Error("menghapus id asing tidak menghasilkan galat")
	}
}

// Nama datang dari HP dan ditampilkan di panel desktop.
func TestRegister_MembersihkanNama(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "d.json"))

	d, _, _ := s.Register("   ", "")
	if d.Name != "Perangkat tanpa nama" {
		t.Errorf("nama kosong = %q", d.Name)
	}
	d, _, _ = s.Register("baris\nbaru\tdan tab", "")
	if strings.ContainsAny(d.Name, "\n\t") {
		t.Errorf("karakter kontrol lolos: %q", d.Name)
	}
	d, _, _ = s.Register(strings.Repeat("x", 500), "")
	if len(d.Name) > 60 {
		t.Errorf("nama tidak dipotong: %d karakter", len(d.Name))
	}
}

// Berkasnya ikut tersalin ke backup mana pun yang menyentuh direktori data
// agent, jadi token apa adanya di sana berarti setiap salinan adalah kunci.
func TestBerkasTidakMemuatTokenApaAdanya(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.json")
	s, _ := Open(path)
	_, token, _ := s.Register("HP", "ios")

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), token) {
		t.Error("token tertulis apa adanya di berkas")
	}
}
