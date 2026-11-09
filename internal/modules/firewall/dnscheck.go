package firewall

import (
	"time"

	"github.com/andyresta/poinhost/internal/core/pkgfail"
)

// periksaJaringan menolak memulai pemasangan kalau server tidak bisa
// menerjemahkan nama yang dibutuhkannya — lihat website.periksaJaringan.
func (s *Service) periksaJaringan(access *firewallAccess, pm, distroID string) error {
	host := pkgfail.HostRepo(pm, distroID)
	return pkgfail.Periksa(host, func(script string) (int, error) {
		res, err := s.run(access, script, 20*time.Second)
		if err != nil || res == nil {
			return 0, errPeriksaTidakSelesai
		}
		return res.ExitCode, nil
	})
}

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

var errPeriksaTidakSelesai = errSentinel("pemeriksaan jaringan tidak selesai")
