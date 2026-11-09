package website

import (
	"strings"

	"github.com/andyresta/poinhost/internal/core/pkgfail"
	"github.com/andyresta/poinhost/internal/core/sshpool"
)

// periksaJaringan menolak memulai pemasangan kalau server tidak bisa
// menerjemahkan nama yang dibutuhkannya.
//
// Dua tahap, dan tahap keduanya yang penting: resolver boleh saja hidup
// sementara mirror distronya sendiri tidak terjawab. Versi pertama fungsi
// ini hanya memeriksa tahap pertama dan lulus — lalu dnf gagal beberapa
// puluh detik kemudian, yang justru hendak dihindari.
func (s *Service) periksaJaringan(access *websiteAccess, distro distroInfo) error {
	host := pkgfail.HostRepo(distro.PackageManager, strings.ToLower(strings.TrimSpace(distro.ID)))
	return pkgfail.Periksa(host, func(script string) (*sshpool.ExecResult, error) {
		return s.run(access, script, pkgfail.BatasPeriksa)
	})
}

// namaDistro memilih sebutan yang paling berarti bagi pemilik server.
//
// PRETTY_NAME kalau ada ("Rocky Linux 8.10"), karena itu yang ia kenali;
// ID sebagai cadangan; dan kalau /etc/os-release pun tidak terbaca, sebut
// itu apa adanya alih-alih mencetak "unknown".
func namaDistro(d distroInfo) string {
	if n := strings.TrimSpace(d.Name); n != "" && n != "unknown" {
		return n
	}
	if id := strings.TrimSpace(d.ID); id != "" && id != "unknown" {
		return id
	}
	return "server ini"
}
