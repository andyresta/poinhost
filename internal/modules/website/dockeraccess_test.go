package website

import (
	"strings"
	"testing"
)

// Jalur jaringan terbuka BUKAN jaminan container bisa masuk: MySQL masih
// menolak akun yang hostnya hanya 'localhost'. Ini kegagalan yang paling
// sering disalahartikan sebagai masalah password, jadi harus dilaporkan.
func TestComputeEnabledMemperingatkanAkunLocalhostSaja(t *testing.T) {
	st := &DBDockerAccessStatus{
		Engine:             "mysql",
		BindAllInterfaces:  true,
		FirewallDetected:   "ufw",
		FirewallRuleActive: true,
		LocalOnlyUsers:     []string{"devpoin", "mkelindo"},
	}
	st.computeEnabled()

	// Jalur jaringannya memang sudah benar — Enabled tidak diturunkan.
	if !st.Enabled {
		t.Fatal("jalur jaringan sudah terbuka, Enabled seharusnya tetap true")
	}
	if !strings.Contains(st.Message, "devpoin") || !strings.Contains(st.Message, "mkelindo") {
		t.Fatalf("akun yang bermasalah harus disebutkan: %q", st.Message)
	}
	if !strings.Contains(st.Message, "is not allowed") {
		t.Fatalf("pesan harus menyebut gejala aslinya supaya mudah dikenali: %q", st.Message)
	}
}

func TestComputeEnabledTanpaAkunBermasalahTidakMemperingatkan(t *testing.T) {
	st := &DBDockerAccessStatus{
		Engine:             "mysql",
		BindAllInterfaces:  true,
		FirewallDetected:   "ufw",
		FirewallRuleActive: true,
	}
	st.computeEnabled()
	if st.Message != "" {
		t.Fatalf("tanpa akun bermasalah tidak boleh ada peringatan: %q", st.Message)
	}
}

// Kalau jalur jaringannya sendiri masih tertutup, peringatan akun tidak
// ditambahkan — masalah yang lebih mendasar dulu yang harus diselesaikan,
// dan menumpuk dua pesan justru mengaburkan mana yang perlu dikerjakan.
func TestComputeEnabledTidakMenumpukPesanSaatJaringanMasihTertutup(t *testing.T) {
	st := &DBDockerAccessStatus{
		Engine:             "mysql",
		BindAllInterfaces:  false,
		FirewallDetected:   "ufw",
		FirewallRuleActive: false,
		LocalOnlyUsers:     []string{"devpoin"},
	}
	st.computeEnabled()
	if st.Enabled {
		t.Fatal("bind-address belum 0.0.0.0, tidak boleh dianggap enabled")
	}
	if strings.Contains(st.Message, "devpoin") {
		t.Fatalf("peringatan akun jangan muncul sebelum jaringannya terbuka: %q", st.Message)
	}
}

// pg_hba TIDAK boleh mematok md5: PostgreSQL 14+ menyimpan password sebagai
// scram-sha-256 dan baris md5 akan menolak login walau passwordnya benar.
func TestPgHbaMengikutiPasswordEncryption(t *testing.T) {
	script := dbDockerAccessApplyScript("postgresql", "apt")
	if !strings.Contains(script, "SHOW password_encryption") {
		t.Fatalf("metode auth harus dibaca dari server:\n%s", script)
	}
	if !strings.Contains(script, "scram-sha-256") {
		t.Fatalf("scram-sha-256 harus didukung:\n%s", script)
	}
	// Pastikan nilainya benar-benar dipakai, bukan sekadar dibaca lalu
	// baris pg_hba tetap ditulis "md5".
	if strings.Contains(script, "            md5\n'") {
		t.Fatalf("baris pg_hba masih mematok md5:\n%s", script)
	}
}
