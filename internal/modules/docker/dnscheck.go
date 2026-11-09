package docker

import (
	"time"

	"github.com/andyresta/poinhost/internal/core/pkgfail"
)

// periksaJaringan menolak memulai pemasangan kalau server tidak bisa
// menerjemahkan nama yang dibutuhkannya — lihat website.periksaJaringan.
func (s *Service) periksaJaringan(access *dockerAccess, distro distroInfo) error {
	host := pkgfail.HostRepo(distro.PackageManager, distro.ID)
	return pkgfail.Periksa(host, func(script string) (int, error) {
		res, err := s.runDocker(access, script, 20*time.Second)
		if err != nil || res == nil {
			return 0, errPeriksaTidakSelesai
		}
		return res.ExitCode, nil
	})
}

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

var errPeriksaTidakSelesai = errSentinel("pemeriksaan jaringan tidak selesai")
