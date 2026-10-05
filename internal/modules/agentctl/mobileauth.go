package agentctl

import (
	"context"
	"fmt"
	"strings"
)

// AccessToken adalah kredensial yang dimasukkan user ke aplikasi mobile.
//
// Port ikut dikembalikan karena tanpa itu token saja tidak cukup untuk
// menyambung, dan port-nya bisa berbeda dari default kalau agent dipasang
// dengan port lain.
type AccessToken struct {
	Token string `json:"token"`
	Port  int    `json:"port"`
}

// APIToken membaca token Mobile Control dari config agent.
//
// Dibaca dari SERVER setiap kali diminta, bukan disimpan di sisi desktop:
// satu-satunya keadaan yang benar ada di config agent, dan menyalinnya ke
// sini hanya menciptakan sumber kedua yang bisa basi setelah token diputar
// dari mesin lain.
func (s *Service) APIToken(ctx context.Context, serverID string) (*AccessToken, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.readConfig(ctx, access)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.APIToken) == "" {
		return nil, fmt.Errorf("config agent belum punya apiToken — buat token baru")
	}
	return &AccessToken{Token: cfg.APIToken, Port: portOf(cfg.Listen)}, nil
}

// RotateAPIToken membuat token baru, menyimpannya, lalu me-restart agent.
//
// Restart-nya bagian dari operasi, bukan langkah terpisah yang diserahkan ke
// user: agent membaca config saat start, jadi tanpa restart token lama masih
// berlaku dan yang baru belum — persis keadaan yang membuat user mengira
// pencabutan sudah berjalan padahal belum.
//
// Config dibaca dulu lalu ditulis ulang utuh, sehingga field lain (selfServer,
// dan apa pun yang ditulis versi desktop lebih baru) tidak ikut hilang.
func (s *Service) RotateAPIToken(ctx context.Context, serverID string) (*AccessToken, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.readConfig(ctx, access)
	if err != nil {
		return nil, err
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	cfg.APIToken = token
	if err := s.writeConfig(ctx, serverID, access, cfg); err != nil {
		return nil, err
	}
	if _, err := s.Lifecycle(ctx, serverID, "restart"); err != nil {
		// Token SUDAH tertulis di config. Mengembalikan token lama di sini
		// akan menyesatkan, jadi yang dilaporkan adalah restartnya yang
		// gagal — user bisa menyalakan service-nya dari panel yang sama.
		return nil, fmt.Errorf("token baru tersimpan, tapi agent gagal di-restart: %w", err)
	}
	return &AccessToken{Token: token, Port: portOf(cfg.Listen)}, nil
}
