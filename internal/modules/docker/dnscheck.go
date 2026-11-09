package docker

import (
	"errors"
	"time"

	"github.com/andyresta/poinhost/internal/core/pkgfail"
)

// periksaDNS menolak memulai pemasangan kalau server tidak bisa
// menerjemahkan nama publik sama sekali — lihat website.periksaDNS untuk
// alasan lengkapnya.
func (s *Service) periksaDNS(access *dockerAccess) error {
	res, err := s.runDocker(access, pkgfail.SkripPeriksaDNS, 15*time.Second)
	if err != nil || res == nil {
		return nil
	}
	if res.ExitCode != 0 {
		return errors.New(pkgfail.PesanAwal(pkgfail.DNSMati))
	}
	return nil
}
