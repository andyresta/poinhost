package servers

import (
	"errors"
	"testing"
)

// fakeVault implementasi secrets.Vault in-memory untuk pengujian — bisa
// dipaksa gagal pada key tertentu (failSet) supaya migrateLegacyPasswords
// diuji benar-benar best-effort (satu server gagal tidak menghalangi yang
// lain).
type fakeVault struct {
	data    map[string]string
	failSet map[string]bool
}

func newFakeVault() *fakeVault {
	return &fakeVault{data: map[string]string{}, failSet: map[string]bool{}}
}

func (v *fakeVault) Get(key string) (string, bool, error) {
	val, ok := v.data[key]
	return val, ok, nil
}

func (v *fakeVault) Set(key, value string) error {
	if v.failSet[key] {
		return errors.New("simulated vault failure")
	}
	v.data[key] = value
	return nil
}

func (v *fakeVault) Delete(key string) error {
	delete(v.data, key)
	return nil
}

func TestMigrateLegacyPasswords_MovesToVaultAndClearsColumn(t *testing.T) {
	repo := newTestRepo(t)
	if err := repo.Create(&Server{ID: "s1", Name: "a", Host: "h", Username: "u", AuthType: "password"}); err != nil {
		t.Fatalf("Create s1: %v", err)
	}
	if _, err := repo.db.Exec(`UPDATE servers SET password_enc = ? WHERE id = ?`, "old-plaintext-pw", "s1"); err != nil {
		t.Fatalf("seed password lama: %v", err)
	}

	vault := newFakeVault()
	svc := &Service{repo: repo, vault: vault}

	if err := svc.migrateLegacyPasswords(); err != nil {
		t.Fatalf("migrateLegacyPasswords: %v", err)
	}

	got, ok, _ := vault.Get(vaultKeyForServerPassword("s1"))
	if !ok || got != "old-plaintext-pw" {
		t.Fatalf("password lama seharusnya sudah ada di vault, got %q ok=%v", got, ok)
	}

	legacy, err := repo.ListLegacyPasswords()
	if err != nil {
		t.Fatalf("ListLegacyPasswords: %v", err)
	}
	if len(legacy) != 0 {
		t.Fatalf("password_enc seharusnya kosong sesudah migrasi, got %v", legacy)
	}

	// Idempotent: menjalankan ulang tidak boleh error atau berubah apa-apa
	// lagi (tidak ada lagi legacy password yang tersisa untuk dibaca).
	if err := svc.migrateLegacyPasswords(); err != nil {
		t.Fatalf("migrateLegacyPasswords kedua kali: %v", err)
	}
}

func TestMigrateLegacyPasswords_OneFailureDoesNotBlockOthers(t *testing.T) {
	repo := newTestRepo(t)
	if err := repo.Create(&Server{ID: "bad", Name: "a", Host: "h", Username: "u", AuthType: "password"}); err != nil {
		t.Fatalf("Create bad: %v", err)
	}
	if err := repo.Create(&Server{ID: "good", Name: "b", Host: "h", Username: "u", AuthType: "password"}); err != nil {
		t.Fatalf("Create good: %v", err)
	}
	if _, err := repo.db.Exec(`UPDATE servers SET password_enc = 'pw' WHERE id IN ('bad', 'good')`); err != nil {
		t.Fatalf("seed password lama: %v", err)
	}

	vault := newFakeVault()
	vault.failSet[vaultKeyForServerPassword("bad")] = true
	svc := &Service{repo: repo, vault: vault}

	if err := svc.migrateLegacyPasswords(); err == nil {
		t.Fatal("migrateLegacyPasswords seharusnya mengembalikan error untuk server yang gagal")
	}

	if _, ok, _ := vault.Get(vaultKeyForServerPassword("bad")); ok {
		t.Fatal("server yang gagal di-set ke vault seharusnya tidak tercatat ada di vault")
	}
	if got, ok, _ := vault.Get(vaultKeyForServerPassword("good")); !ok || got != "pw" {
		t.Fatalf("server lain seharusnya tetap termigrasi walau satu server lain gagal, got %q ok=%v", got, ok)
	}

	legacy, err := repo.ListLegacyPasswords()
	if err != nil {
		t.Fatalf("ListLegacyPasswords: %v", err)
	}
	if _, stillThere := legacy["bad"]; !stillThere {
		t.Fatal("password_enc server yang gagal migrasi seharusnya TIDAK dihapus (belum aman dipindah)")
	}
	if _, stillThere := legacy["good"]; stillThere {
		t.Fatal("password_enc server yang berhasil migrasi seharusnya sudah dikosongkan")
	}
}
