package website

import (
	"errors"
	"time"

	"github.com/andyresta/poinhost/internal/core/pkgfail"
)

// periksaDNS menolak memulai pemasangan kalau server tidak bisa
// menerjemahkan nama publik sama sekali.
//
// Tanpa ini, dnf atau apt tetap dijalankan, menghabiskan puluhan detik
// mencoba setiap cermin, lalu berhenti dengan layar penuh keluaran yang
// mengarahkan perhatian ke repositori — padahal resolvernya yang mati.
//
// Kegagalan pemeriksaannya SENDIRI (SSH putus, timeout) tidak menghalangi
// pemasangan: yang diketahui cuma "pemeriksaan tidak selesai", bukan
// "DNS rusak", dan menolak memasang atas dasar itu menukar satu kegagalan
// yang jelas dengan satu lagi yang membingungkan.
func (s *Service) periksaDNS(access *websiteAccess) error {
	res, err := s.run(access, pkgfail.SkripPeriksaDNS, 15*time.Second)
	if err != nil || res == nil {
		return nil
	}
	if res.ExitCode != 0 {
		return errors.New(pkgfail.PesanAwal(pkgfail.DNSMati))
	}
	return nil
}
