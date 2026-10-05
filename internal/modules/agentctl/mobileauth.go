package agentctl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// EnrollToken adalah kredensial PENDAFTARAN yang ditempel user ke aplikasi
// mobile.
//
// Bukan kredensial akses: menukarnya di agent hanya menghasilkan perangkat
// yang MENUNGGU, dan HP baru bisa membaca apa pun setelah perangkat itu
// disetujui dari panel ini. Itu yang membuat token ini boleh dikirim lewat
// chat atau diketik ulang tanpa taruhan besar.
//
// Port ikut dikembalikan karena tanpa itu token saja tidak cukup untuk
// menyambung, dan port-nya bisa berbeda dari default kalau agent dipasang
// dengan port lain.
type EnrollToken struct {
	Token string `json:"token"`
	Port  int    `json:"port"`
}

// Device adalah satu HP yang terdaftar di agent.
type Device struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Platform   string    `json:"platform,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
	ApprovedAt time.Time `json:"approvedAt,omitempty"`
}

// APIToken membaca token pendaftaran dari config agent.
//
// Dibaca dari SERVER setiap kali diminta, bukan disimpan di sisi desktop:
// satu-satunya keadaan yang benar ada di config agent, dan menyalinnya ke
// sini hanya menciptakan sumber kedua yang bisa basi setelah token diputar
// dari mesin lain.
func (s *Service) APIToken(ctx context.Context, serverID string) (*EnrollToken, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.readConfig(ctx, access)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.EnrollToken) == "" {
		return nil, fmt.Errorf("config agent belum punya token pendaftaran — buat token baru")
	}
	return &EnrollToken{Token: cfg.EnrollToken, Port: portOf(cfg.Listen)}, nil
}

// RotateAPIToken membuat token pendaftaran baru, menyimpannya, lalu me-restart
// agent.
//
// Restart-nya bagian dari operasi, bukan langkah terpisah yang diserahkan ke
// user: agent membaca config saat start, jadi tanpa restart token lama masih
// berlaku dan yang baru belum — persis keadaan yang membuat user mengira
// pencabutan sudah berjalan padahal belum.
//
// Perangkat yang SUDAH disetujui tidak ikut terputus: tokennya sendiri-sendiri
// dan tersimpan terpisah dari config. Yang hilang hanya kemampuan mendaftarkan
// HP baru dengan token lama.
//
// Config dibaca dulu lalu ditulis ulang utuh, sehingga field lain (selfServer,
// dan apa pun yang ditulis versi desktop lebih baru) tidak ikut hilang.
func (s *Service) RotateAPIToken(ctx context.Context, serverID string) (*EnrollToken, error) {
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
	cfg.EnrollToken = token
	// Nama field lama dibuang saat ini juga. Membiarkannya berarti config
	// menyimpan token yang sudah tidak dihormati siapa pun — rahasia mati
	// yang tetap terbaca di berkas dan di setiap backup yang menyentuhnya.
	cfg.LegacyAPIToken = ""
	if err := s.writeConfig(ctx, serverID, access, cfg); err != nil {
		return nil, err
	}
	if _, err := s.Lifecycle(ctx, serverID, "restart"); err != nil {
		// Token SUDAH tertulis di config. Mengembalikan token lama di sini
		// akan menyesatkan, jadi yang dilaporkan adalah restartnya yang
		// gagal — user bisa menyalakan service-nya dari panel yang sama.
		return nil, fmt.Errorf("token baru tersimpan, tapi agent gagal di-restart: %w", err)
	}
	return &EnrollToken{Token: token, Port: portOf(cfg.Listen)}, nil
}

// Devices membaca daftar perangkat yang terdaftar di agent.
func (s *Service) Devices(ctx context.Context, serverID string) ([]Device, error) {
	out, err := s.callAdmin(ctx, serverID, "GET", "/admin/devices")
	if err != nil {
		return nil, err
	}
	return parseDevices(out)
}

// ApproveDevice menyetujui satu perangkat, sehingga tokennya mulai berlaku.
func (s *Service) ApproveDevice(ctx context.Context, serverID, deviceID string) ([]Device, error) {
	return s.deviceAction(ctx, serverID, "POST", "/admin/devices/"+deviceID+"/approve")
}

// RevokeDevice mencabut akses satu perangkat tanpa menghapusnya, sehingga
// HP-nya tahu aksesnya diambil, bukan sekadar gagal.
func (s *Service) RevokeDevice(ctx context.Context, serverID, deviceID string) ([]Device, error) {
	return s.deviceAction(ctx, serverID, "POST", "/admin/devices/"+deviceID+"/revoke")
}

// DeleteDevice menghapus perangkat sepenuhnya.
func (s *Service) DeleteDevice(ctx context.Context, serverID, deviceID string) ([]Device, error) {
	return s.deviceAction(ctx, serverID, "DELETE", "/admin/devices/"+deviceID)
}

func (s *Service) deviceAction(ctx context.Context, serverID, method, path string) ([]Device, error) {
	out, err := s.callAdmin(ctx, serverID, method, path)
	if err != nil {
		return nil, err
	}
	return parseDevices(out)
}

// parseDevices membaca jawaban permukaan admin.
//
// Jawaban kosong diperlakukan sebagai galat, bukan sebagai daftar kosong:
// `curl -fsS` yang gagal menyambung tidak mencetak apa pun, dan melaporkannya
// sebagai "belum ada perangkat" akan menyembunyikan agent yang mati di balik
// layar yang terlihat normal.
func parseDevices(raw string) ([]Device, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("agent tidak mengembalikan daftar perangkat")
	}
	var body struct {
		Devices []Device `json:"devices"`
		Error   string   `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil, fmt.Errorf("jawaban agent tidak terbaca: %s", trimForError(raw))
	}
	if body.Error != "" {
		return nil, fmt.Errorf("%s", body.Error)
	}
	if body.Devices == nil {
		body.Devices = []Device{}
	}
	return body.Devices, nil
}
