package firewall

import (
	"github.com/andyresta/poinhost/internal/core/pkgfail"
	"github.com/andyresta/poinhost/internal/core/sshpool"
)

// periksaJaringan menolak memulai pemasangan kalau server tidak bisa
// menerjemahkan nama yang dibutuhkannya — lihat website.periksaJaringan.
func (s *Service) periksaJaringan(access *firewallAccess, pm, distroID string) error {
	host := pkgfail.HostRepo(pm, distroID)
	return pkgfail.Periksa(host, func(script string) (*sshpool.ExecResult, error) {
		return s.run(access, script, pkgfail.BatasPeriksa)
	})
}
