package agentctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/agent/agentcfg"
	"github.com/andyresta/poinhost/internal/agent/dist"
)

// AdminSocket adalah unix socket permukaan admin agent.
const AdminSocket = DataDir + "/admin.sock"

// penandaNoAdmin menandai stderr saat socket admin tidak ada.
//
// Penanda, bukan kalimat: kalimatnya disusun di Go dari fakta-fakta yang
// ikut dikumpulkan skrip, supaya yang dibaca layar adalah sebab yang
// BENAR, bukan satu tebakan yang kebetulan sering tepat.
const penandaNoAdmin = "POINHOST_NOADMIN"

// callAdmin memanggil permukaan admin agent lewat SSH.
//
// Lewat unix socket, bukan port TCP agent: socket itu ada di dalam direktori
// data bermode 0700 milik root, sehingga vhost nginx yang menerbitkan agent ke
// internet tidak punya jalan menyentuh persetujuan perangkat sama sekali.
// Autentikasinya adalah izin berkas itu — app desktop sudah naik ke root di
// sesi SSH-nya, dan tidak ada rahasia tambahan yang perlu disimpan di sini.
//
// Socket yang TIDAK ADA punya lebih dari satu sebab, dan versi pertama fungsi
// ini menyebut satu saja: "agent terlalu lama, tekan Update agent". Itu
// menyesatkan untuk agent baru yang kebetulan sedang mati atau sedang
// di-restart, dan untuk agent baru yang socket adminnya gagal dibuka — yang
// di main.go memang SENGAJA tidak fatal. Jadi fakta pembedanya dikumpulkan
// dalam round-trip yang sama: service hidup atau tidak, dan versi berapa yang
// benar-benar terpasang.
func (s *Service) callAdmin(ctx context.Context, serverID, method, path string) (string, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return "", err
	}
	res, err := s.run(ctx, access, skripAdmin(method, path), 30*time.Second)
	if err != nil {
		if res != nil && strings.Contains(res.Stderr, penandaNoAdmin) {
			return "", errors.New(sebabNoAdmin(res.Stderr, dist.Version()))
		}
		return "", fmt.Errorf("agent tidak merespons — pastikan service-nya berjalan dan versinya sudah mendukung persetujuan perangkat: %w", err)
	}
	return strings.TrimSpace(res.Stdout), nil
}

// skripAdmin memanggil permukaan admin, atau — kalau socketnya tidak ada —
// mengumpulkan fakta yang membedakan sebab-sebabnya.
//
// Pengumpulan fakta itu ada di round-trip yang SAMA dengan pemanggilannya,
// bukan di SSH kedua: keadaan yang dilaporkan harus menggambarkan satu
// momen. Socket yang hilang lalu service yang dibaca sedetik kemudian bisa
// menghasilkan cerita yang tidak pernah benar-benar terjadi.
func skripAdmin(method, path string) string {
	return `sock=` + shellQuote(AdminSocket) + `
if [ -S "$sock" ]; then
  curl -fsS --max-time 10 -X ` + method + ` --unix-socket "$sock" http://localhost` + path + `
  exit $?
fi
echo ` + penandaNoAdmin + ` >&2
echo "active=$(systemctl is-active ` + UnitName + ` 2>/dev/null)" >&2
if [ -x ` + BinaryPath + ` ]; then echo "version=$(` + BinaryPath + ` --version 2>/dev/null | head -n1)" >&2; fi
exit 90
`
}

// sebabNoAdmin menyusun kalimat dari fakta yang dikumpulkan skripAdmin.
//
// Tiap cabang menyebut langkah yang benar-benar memperbaiki keadaan itu.
// Yang paling sering terjadi — agent lama yang memang perlu di-update —
// menyebut KEDUA versinya, karena "tekan Update agent" tanpa angka tidak
// menerangkan apa pun bagi orang yang baru saja menekan Pasang.
// bundel diterima sebagai argumen, bukan dibaca dari dist di dalam sini:
// versi yang dibundel berasal dari berkas yang dibuat saat rilis dan TIDAK
// ikut di-commit, jadi fungsi yang membacanya sendiri akan berperilaku beda
// di mesin pengembang dan di CI — dan tesnya ikut berbeda.
func sebabNoAdmin(stderr, bundel string) string {
	kv := parseKV(stderr)
	versi := strings.TrimSpace(kv["version"])
	aktif := strings.TrimSpace(kv["active"]) == "active"
	bundel = strings.TrimSpace(bundel)

	switch {
	case versi == "":
		return "Agent belum terpasang di server ini — pasang dulu dari panel Mobile Control."
	case !aktif:
		return "Agent " + versi + " sedang tidak berjalan, jadi persetujuan perangkat tidak bisa dibuka. Nyalakan agent-nya dulu."
	case decideVersionAction(versi, bundel) == versionUpdate:
		return "Agent " + versi + " di server ini belum punya persetujuan perangkat; versi " + bundel +
			" yang dibawa poinhost sudah. Tekan Update agent untuk memasangnya."
	}
	return "Agent " + versi + " berjalan, tapi socket admin " + AdminSocket +
		" tidak terbuka. Buka Log agent untuk melihat alasannya."
}

func (s *Service) readConfig(ctx context.Context, access *agentAccess) (*agentcfg.Config, error) {
	res, err := s.run(ctx, access, "base64 -w0 < "+ConfigPath+" 2>/dev/null || base64 < "+ConfigPath+" | tr -d '\\n'", 20*time.Second)
	if err != nil {
		return nil, err
	}
	cfg := decodeConfig(strings.TrimSpace(res.Stdout))
	if cfg == nil {
		return nil, fmt.Errorf("tidak bisa membaca %s — agent belum terpasang?", ConfigPath)
	}
	return cfg, nil
}

func (s *Service) writeConfig(ctx context.Context, serverID string, access *agentAccess, cfg *agentcfg.Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := s.uploadTemp(ctx, serverID, tmpConfigPath, append(b, '\n')); err != nil {
		return err
	}
	script := "set -e\ninstall -m 0600 -o root -g root " + tmpConfigPath + " " + ConfigPath + "\nrm -f " + tmpConfigPath + "\n"
	_, err = s.run(ctx, access, script, 30*time.Second)
	return err
}

func trimForError(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
