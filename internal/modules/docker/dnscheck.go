package docker

import (
	"github.com/andyresta/poinhost/internal/core/pkgfail"
	"github.com/andyresta/poinhost/internal/core/sshpool"
)

// periksaJaringan menolak memulai pemasangan kalau server tidak bisa
// menerjemahkan nama yang dibutuhkannya — lihat website.periksaJaringan.
func (s *Service) periksaJaringan(access *dockerAccess, distro distroInfo) error {
	host := pkgfail.HostRepo(distro.PackageManager, distro.ID)
	return pkgfail.Periksa(host, func(script string) (*sshpool.ExecResult, error) {
		return s.runDocker(access, script, pkgfail.BatasPeriksa)
	})
}
