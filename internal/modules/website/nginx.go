package website

import (
	"strings"
	"time"
)

// distroInfo info distro Linux + package manager server (untuk wizard install).
type distroInfo struct {
	ID             string
	Name           string
	PackageManager string
}

const distroDetectScript = `. /etc/os-release 2>/dev/null
echo "ID=${ID:-unknown}"
echo "PRETTY=${PRETTY_NAME:-unknown}"
command -v apt-get >/dev/null 2>&1 && echo PM=apt
command -v dnf >/dev/null 2>&1 && echo PM=dnf
command -v yum >/dev/null 2>&1 && echo PM=yum
command -v apk >/dev/null 2>&1 && echo PM=apk
true`

// detectDistro mendeteksi distro + package manager server (1 round-trip),
// dipakai wizard install SEBELUM komponen yang bersangkutan (nginx/php/
// certbot) tentu terpasang — di luar jalur itu, status distro sudah
// tersedia gratis lewat getNginxStatus (lihat combinedStatusScript).
func (s *Service) detectDistro(access *websiteAccess) (distroInfo, error) {
	res, err := s.run(access, distroDetectScript, 15*time.Second)
	if err != nil {
		return distroInfo{}, err
	}
	return parseDistroOutput(res.Stdout + "\n" + res.Stderr), nil
}

func parseDistroOutput(out string) distroInfo {
	info := distroInfo{ID: "unknown", Name: "unknown"}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "ID="):
			info.ID = strings.TrimPrefix(line, "ID=")
		case strings.HasPrefix(line, "PRETTY="):
			info.Name = strings.TrimPrefix(line, "PRETTY=")
		case strings.HasPrefix(line, "PM="):
			if info.PackageManager == "" {
				info.PackageManager = strings.TrimPrefix(line, "PM=")
			}
		}
	}
	if strings.Contains(out, "PM=apt") {
		info.PackageManager = "apt"
	}
	return info
}

// combinedStatusScript menggabungkan deteksi distro DAN status Nginx dalam
// SATU perintah — homepoin menjalankan ini sebagai dua round-trip terpisah
// (detectDistro lalu nginxStatusScript) di dalam nginx.DetectStatus, dan
// memanggil DetectStatus itu berulang dari domains.List/php.Status/
// ssl.Status tanpa cache sama sekali (lihat riset ARCHITECTURE.md §Website)
// — gabungan + cache di getNginxStatus ini yang menghilangkan sebagian
// besar rasa "lambat pindah menu" di homepoin, BUKAN soal SSH re-handshake
// (yang di poinhost memang sudah tidak pernah terjadi sejak awal, §3).
const combinedStatusScript = distroDetectScript + `
echo "BIN=$(command -v nginx 2>/dev/null || true)"
if command -v nginx >/dev/null 2>&1; then nginx -v 2>&1 | head -1; fi
echo "ACTIVE=$(systemctl is-active nginx 2>/dev/null || echo unknown)"
echo "ENABLED=$(systemctl is-enabled nginx 2>/dev/null || echo unknown)"
true`

// getNginxStatus mengembalikan status Nginx (+ distro) untuk satu server,
// dari cache kalau masih segar (statusCacheTTL), atau lewat SATU round-trip
// SSH kalau tidak.
func (s *Service) getNginxStatus(serverID string) (*NginxStatus, error) {
	s.cacheMu.Lock()
	entry, ok := s.nginxCache[serverID]
	s.cacheMu.Unlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.status, nil
	}

	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	res, err := s.run(access, combinedStatusScript, 15*time.Second)
	if err != nil {
		return nil, err
	}
	out := res.Stdout + "\n" + res.Stderr
	distro := parseDistroOutput(out)

	st := &NginxStatus{
		DistroID:       distro.ID,
		DistroName:     distro.Name,
		PackageManager: distro.PackageManager,
		CanInstall:     distro.PackageManager != "",
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "BIN="):
			st.Installed = strings.TrimPrefix(line, "BIN=") != ""
		case strings.HasPrefix(line, "nginx version:"):
			st.Version = strings.TrimSpace(strings.TrimPrefix(line, "nginx version:"))
			st.Installed = true
		case strings.HasPrefix(line, "ACTIVE="):
			st.Active = strings.TrimPrefix(line, "ACTIVE=") == "active"
		case strings.HasPrefix(line, "ENABLED="):
			st.Enabled = strings.TrimPrefix(line, "ENABLED=") == "enabled"
		}
	}

	s.cacheMu.Lock()
	s.nginxCache[serverID] = nginxStatusCacheEntry{status: st, expiresAt: time.Now().Add(statusCacheTTL)}
	s.cacheMu.Unlock()
	return st, nil
}

// GetNginxStatus adalah versi publik dari getNginxStatus (dipanggil app.go).
func (s *Service) GetNginxStatus(serverID string) (*NginxStatus, error) {
	return s.getNginxStatus(serverID)
}

// StartNginx mengaktifkan service Nginx yang sudah terpasang tapi tidak aktif.
func (s *Service) StartNginx(serverID string) (*NginxStatus, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	st, err := s.getNginxStatus(serverID)
	if err != nil {
		return nil, err
	}
	if !st.Installed {
		return nil, errFmt("Nginx belum terpasang di server")
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	script := `set -e
systemctl enable nginx 2>/dev/null || true
systemctl start nginx 2>&1 || systemctl restart nginx 2>&1
nginx -v 2>&1 | head -1 || true`
	if _, err := s.run(access, script, 20*time.Second); err != nil {
		return nil, err
	}
	s.invalidateNginxCache(serverID)
	return s.getNginxStatus(serverID)
}

// testReload menjalankan `nginx -t` lalu reload/restart dalam SATU Exec
// (bukan dua panggilan terpisah seperti homepoin's nginx.TestReload) — kalau
// tes config gagal, `set -e` menghentikan skrip SEBELUM reload dijalankan,
// jadi vhost yang salah tidak pernah ikut di-reload ke config yang berjalan.
func (s *Service) testReload(access *websiteAccess) error {
	script := `set -e
nginx -t 2>&1
systemctl reload nginx 2>&1 || systemctl restart nginx 2>&1`
	res, err := s.run(access, script, 20*time.Second)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = "nginx -t / reload gagal"
		}
		return mapWebsiteError(errFmt("%s", msg))
	}
	return nil
}

// aptWaitLock antisipasi dpkg/apt lock yang masih dipegang proses lain
// (mis. unattended-upgrades) sebelum menyerah — sama seperti dipakai
// docker/install.go, ditulis inline supaya modul ini tidak bergantung pada
// modul lain yang belum tentu ada.
const aptWaitLock = `export DEBIAN_FRONTEND=noninteractive
for i in $(seq 1 30); do
  if ! fuser /var/lib/dpkg/lock-frontend >/dev/null 2>&1 && ! fuser /var/lib/apt/lists/lock >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
`

// installNginxScript skrip instalasi Nginx per package manager.
func installNginxScript(pm string) (string, bool) {
	switch pm {
	case "apt":
		return `set -e
` + aptWaitLock + `
apt-get update
apt-get install -y nginx
systemctl enable nginx
systemctl start nginx
nginx -t
echo ">> Selesai."
`, true
	case "dnf", "yum":
		bin := pm
		return `set -e
` + bin + ` install -y nginx
systemctl enable nginx
systemctl start nginx
nginx -t
echo ">> Selesai."
`, true
	case "apk":
		return `set -e
apk add --no-cache nginx
rc-update add nginx default 2>/dev/null || true
service nginx start 2>/dev/null || rc-service nginx start 2>/dev/null || true
nginx -t
echo ">> Selesai."
`, true
	default:
		return "", false
	}
}
