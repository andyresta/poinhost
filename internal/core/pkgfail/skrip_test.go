package pkgfail

import (
	"errors"
	"os/exec"
	"testing"
)

// Menjalankan skrip pemeriksaan di shell SUNGGUHAN, dengan getent palsu.
//
// Tes lain di paket ini hanya mencocokkan teks skripnya; yang itu tidak
// menangkap kesalahan logika maupun sintaks. Dan kesalahan di sini mahal:
// skrip yang keliru akan MEMBLOKIR semua pemasangan dengan tuduhan palsu,
// yang lebih buruk daripada bug yang hendak diperbaikinya.
func jalankanPeriksa(t *testing.T, getentPalsu, hostRepo string) int {
	t.Helper()
	sh, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash tidak tersedia")
	}
	// timeout DIPALSUKAN sebagai fungsi juga. Di server ia menjalankan biner
	// getent, jadi tanpa shim ini stub getent tidak pernah terpakai — dan
	// tes ini akan diam-diam menguji resolver mesin yang menjalankannya,
	// bukan skripnya.
	const shimTimeout = "timeout() { shift; \"$@\"; }\n"
	cmd := exec.Command(sh, "-c", shimTimeout+getentPalsu+"\n"+skripPeriksa(hostRepo))
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		t.Fatalf("menjalankan skrip: %v", err)
	}
	return 0
}

func TestSkripPeriksa_Perilaku(t *testing.T) {
	const rocky = "mirrors.rockylinux.org"

	kasus := []struct {
		nama   string
		getent string
		host   string
		mau    int
	}{
		{
			nama:   "tidak ada nama yang terjawab",
			getent: `getent() { return 2; }`,
			host:   rocky,
			mau:    periksaDNSMati,
		},
		{
			// Inilah yang lolos di versi pertama: resolver sehat, mirror
			// distronya sendiri tidak terjawab, pemasangan tetap dimulai.
			nama:   "resolver sehat tapi mirror distro mati",
			getent: `getent() { case "$2" in *rockylinux*) return 2;; esac; return 0; }`,
			host:   rocky,
			mau:    periksaRepoBuntu,
		},
		{
			nama:   "semua terjawab",
			getent: `getent() { return 0; }`,
			host:   rocky,
			mau:    periksaOK,
		},
		{
			// Distro tak dikenal: tahap kedua dilewati, bukan ditebak.
			nama:   "tanpa host repo, mirror mana pun tidak menentukan",
			getent: `getent() { case "$2" in *rockylinux*) return 2;; esac; return 0; }`,
			host:   "",
			mau:    periksaOK,
		},
		{
			// Menolak memasang karena alat pemeriksanya hilang akan
			// menghalangi pemasangan yang sebenarnya sehat.
			nama:   "getent tidak ada sama sekali",
			getent: `unset -f getent timeout 2>/dev/null; PATH=/nonexistent-poinhost`,
			host:   rocky,
			mau:    periksaOK,
		},
	}

	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			if got := jalankanPeriksa(t, k.getent, k.host); got != k.mau {
				t.Errorf("kode keluar = %d, mau %d", got, k.mau)
			}
		})
	}
}
