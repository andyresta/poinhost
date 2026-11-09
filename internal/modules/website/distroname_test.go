package website

import "testing"

// Pesan "tidak didukung" harus menyebut SERVER MANA. Versi sebelumnya
// mencetak tanda kurung kosong saat package managernya tidak terdeteksi —
// kalimat yang tidak menyebut satu pun hal yang bisa ditindaklanjuti.
func TestNamaDistro(t *testing.T) {
	kasus := []struct {
		nama string
		in   distroInfo
		mau  string
	}{
		{"pakai PRETTY_NAME", distroInfo{ID: "rocky", Name: "Rocky Linux 8.10"}, "Rocky Linux 8.10"},
		{"jatuh ke ID", distroInfo{ID: "opensuse-leap", Name: ""}, "opensuse-leap"},
		// os-release memang mengisi "unknown" lewat ${ID:-unknown}, jadi itu
		// harus diperlakukan sebagai kosong, bukan dicetak apa adanya.
		{"unknown dianggap kosong", distroInfo{ID: "unknown", Name: "unknown"}, "server ini"},
		{"kosong sama sekali", distroInfo{}, "server ini"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			if got := namaDistro(k.in); got != k.mau {
				t.Errorf("namaDistro = %q, mau %q", got, k.mau)
			}
		})
	}
}

// Yang didukung ditentukan oleh package manager, bukan nama distro — jadi
// Rocky, Alma, dan Oracle ikut jalan tanpa disebut satu per satu.
func TestInstallNginxScript_CakupanPackageManager(t *testing.T) {
	for _, pm := range []string{"apt", "dnf", "yum", "apk"} {
		if _, ok := installNginxScript(pm); !ok {
			t.Errorf("%s seharusnya didukung", pm)
		}
	}
	for _, pm := range []string{"zypper", "pacman", "emerge", ""} {
		if _, ok := installNginxScript(pm); ok {
			t.Errorf("%s seharusnya BELUM didukung", pm)
		}
	}
}
