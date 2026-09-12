package docker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const distroDetectScript = `. /etc/os-release 2>/dev/null
echo "ID=${ID:-unknown}"
echo "PRETTY=${PRETTY_NAME:-unknown}"
command -v apt-get >/dev/null 2>&1 && echo PM=apt
command -v dnf >/dev/null 2>&1 && echo PM=dnf
command -v yum >/dev/null 2>&1 && echo PM=yum
command -v apk >/dev/null 2>&1 && echo PM=apk
true`

const dockerStatusScript = `echo "BIN=$(command -v docker 2>/dev/null || true)"
if command -v docker >/dev/null 2>&1; then
  VER=$(docker version --format '{{.Server.Version}}' 2>/dev/null || true)
  echo "SERVER_VER=${VER}"
fi
echo "ACTIVE=$(systemctl is-active docker 2>/dev/null || systemctl is-active docker.service 2>/dev/null || echo unknown)"
echo "ENABLED=$(systemctl is-enabled docker 2>/dev/null || systemctl is-enabled docker.service 2>/dev/null || echo unknown)"
if [ "$(systemctl is-active docker 2>/dev/null)" = "unknown" ] || [ -z "$(systemctl is-active docker 2>/dev/null)" ]; then
  if command -v rc-service >/dev/null 2>&1; then
    if rc-service docker status 2>/dev/null | grep -qi started; then echo "ACTIVE=active"; fi
  fi
fi
true`

// engineDetail struct sederhana untuk hasil deteksi distro (dipakai wizard install).
type distroInfo struct {
	ID             string
	Name           string
	PackageManager string
}

// detectDistroInfo mendeteksi distro + package manager via SSH.
func (s *Service) detectDistroInfo(access *dockerAccess) (distroInfo, error) {
	res, err := s.runDocker(access, distroDetectScript, 15*time.Second)
	if err != nil {
		return distroInfo{}, err
	}
	info := distroInfo{ID: "unknown", Name: "unknown"}
	out := ""
	if res != nil {
		out = res.Stdout + "\n" + res.Stderr
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ID=") {
			info.ID = strings.TrimPrefix(line, "ID=")
		}
		if strings.HasPrefix(line, "PRETTY=") {
			info.Name = strings.TrimPrefix(line, "PRETTY=")
		}
		if strings.HasPrefix(line, "PM=") {
			pm := strings.TrimPrefix(line, "PM=")
			if info.PackageManager == "" {
				info.PackageManager = pm
			}
		}
	}
	if strings.Contains(out, "PM=apt") {
		info.PackageManager = "apt"
	}
	return info, nil
}

// DetectEngineStatus membaca status instalasi Docker di server.
func (s *Service) DetectEngineStatus(serverID string) (*EngineStatus, error) {
	if strings.TrimSpace(serverID) == "" {
		return nil, fmt.Errorf("id server wajib diisi")
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	distro, err := s.detectDistroInfo(access)
	if err != nil {
		return nil, err
	}
	st := &EngineStatus{
		DistroID:       distro.ID,
		DistroName:     distro.Name,
		PackageManager: distro.PackageManager,
		CanInstall:     distro.PackageManager != "",
	}

	res, err := s.runDocker(access, dockerStatusScript, 15*time.Second)
	if err != nil {
		// Masih kembalikan distro agar wizard bisa tampil.
		return st, nil
	}
	out := ""
	if res != nil {
		out = res.Stdout + "\n" + res.Stderr
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "BIN="):
			st.Installed = strings.TrimPrefix(line, "BIN=") != ""
		case strings.HasPrefix(line, "SERVER_VER="):
			v := strings.TrimPrefix(line, "SERVER_VER=")
			if v != "" {
				st.Version = v
				st.Installed = true
			}
		case strings.HasPrefix(line, "ACTIVE="):
			st.Active = strings.TrimPrefix(line, "ACTIVE=") == "active"
		case strings.HasPrefix(line, "ENABLED="):
			en := strings.TrimPrefix(line, "ENABLED=")
			st.Enabled = en == "enabled"
		}
	}
	return st, nil
}

// StartEngine mengaktifkan service Docker di server.
func (s *Service) StartEngine(serverID string) (*EngineStatus, error) {
	if strings.TrimSpace(serverID) == "" {
		return nil, fmt.Errorf("id server wajib diisi")
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	st, err := s.DetectEngineStatus(serverID)
	if err != nil {
		return nil, err
	}
	if !st.Installed {
		return nil, fmt.Errorf("Docker belum terpasang di server")
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	script := `set -e
if command -v systemctl >/dev/null 2>&1; then
  systemctl enable docker 2>/dev/null || systemctl enable docker.service 2>/dev/null || true
  systemctl start docker 2>&1 || systemctl restart docker 2>&1 || systemctl start docker.service 2>&1
  systemctl is-active docker 2>/dev/null || systemctl is-active docker.service
elif command -v rc-service >/dev/null 2>&1; then
  rc-update add docker default 2>/dev/null || true
  rc-service docker start 2>&1 || service docker start 2>&1
else
  dockerd >/dev/null 2>&1 &
  sleep 2
fi
docker version --format '{{.Server.Version}}' 2>/dev/null || true
`
	res, err := s.runDocker(access, script, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if res != nil && res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = "gagal menjalankan Docker"
		}
		return nil, mapDockerError(fmt.Errorf("%s", msg))
	}
	return s.DetectEngineStatus(serverID)
}

// aptWaitLock antisipasi dpkg/apt lock yang masih dipegang proses lain
// (mis. unattended-upgrades) — tunggu sebentar sebelum menyerah, daripada
// langsung gagal di percobaan pertama seperti VPS yang baru saja boot.
const aptWaitLock = `export DEBIAN_FRONTEND=noninteractive
for i in $(seq 1 30); do
  if ! fuser /var/lib/dpkg/lock-frontend >/dev/null 2>&1 && ! fuser /var/lib/apt/lists/lock >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
`

// installDockerScript menghasilkan skrip instalasi Docker per package manager,
// memakai skrip resmi get.docker.com (mendukung mayoritas distro Linux populer).
func installDockerScript(pm string) (string, bool) {
	switch pm {
	case "apt":
		return `set -e
` + aptWaitLock + `
echo ">> Memasang dependensi..."
apt-get update
apt-get install -y ca-certificates curl gnupg
echo ">> Menginstall Docker Engine (skrip resmi get.docker.com)..."
curl -fsSL https://get.docker.com -o /tmp/poinhost-get-docker.sh
sh /tmp/poinhost-get-docker.sh
rm -f /tmp/poinhost-get-docker.sh
echo ">> Mengaktifkan service docker..."
systemctl enable docker 2>/dev/null || true
systemctl start docker 2>/dev/null || systemctl restart docker 2>/dev/null || true
sleep 2
docker version
echo ">> Selesai."
`, true
	case "dnf":
		return `set -e
echo ">> Menginstall Docker Engine (skrip resmi get.docker.com)..."
dnf install -y curl ca-certificates || true
curl -fsSL https://get.docker.com -o /tmp/poinhost-get-docker.sh
sh /tmp/poinhost-get-docker.sh
rm -f /tmp/poinhost-get-docker.sh
echo ">> Mengaktifkan service docker..."
systemctl enable docker 2>/dev/null || true
systemctl start docker 2>/dev/null || systemctl restart docker 2>/dev/null || true
sleep 2
docker version
echo ">> Selesai."
`, true
	case "yum":
		return `set -e
echo ">> Menginstall Docker Engine (skrip resmi get.docker.com)..."
yum install -y curl ca-certificates || true
curl -fsSL https://get.docker.com -o /tmp/poinhost-get-docker.sh
sh /tmp/poinhost-get-docker.sh
rm -f /tmp/poinhost-get-docker.sh
echo ">> Mengaktifkan service docker..."
systemctl enable docker 2>/dev/null || true
systemctl start docker 2>/dev/null || systemctl restart docker 2>/dev/null || true
sleep 2
docker version
echo ">> Selesai."
`, true
	case "apk":
		return `set -e
echo ">> Memasang Docker via APK (Alpine)..."
apk add --no-cache docker docker-cli docker-compose 2>/dev/null || apk add --no-cache docker
rc-update add docker default 2>/dev/null || true
service docker start 2>/dev/null || rc-service docker start 2>/dev/null || true
sleep 2
docker version
echo ">> Selesai."
`, true
	default:
		return "", false
	}
}

// runInstallStream menjalankan script install via PTY stream + sentinel exit code.
func (s *Service) runInstallStream(ctx context.Context, access *dockerAccess, script string, onLine func(string) error) error {
	const sentinel = "__poinhost_INSTALL_DONE__:"
	wrapped := "(\n" + script + "\n) 2>&1\necho \"" + sentinel + "$?\""
	exitCode := -1
	streamErr := s.executor.ExecStreamPTY(ctx, access.serverID, access.wrap(wrapped), func(line string) error {
		if strings.HasPrefix(line, sentinel) {
			codeStr := strings.TrimSpace(strings.TrimPrefix(line, sentinel))
			if n, err := strconv.Atoi(codeStr); err == nil {
				exitCode = n
			}
			return nil
		}
		return onLine(line)
	})
	if streamErr != nil {
		return streamErr
	}
	if exitCode != 0 {
		return fmt.Errorf("proses gagal dengan kode keluar %d", exitCode)
	}
	return nil
}

// StreamInstallEngine menginstal Docker sambil mengalirkan output ke onLine.
func (s *Service) StreamInstallEngine(ctx context.Context, serverID string, onLine func(string) error) error {
	if strings.TrimSpace(serverID) == "" {
		return fmt.Errorf("id server wajib diisi")
	}
	if onLine == nil {
		onLine = func(string) error { return nil }
	}
	_ = onLine("Menyiapkan koneksi ke server...")
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}
	_ = onLine("Mendeteksi sistem operasi server...")
	distro, err := s.detectDistroInfo(access)
	if err != nil {
		return err
	}
	_ = onLine(fmt.Sprintf("Distro terdeteksi: %s (paket: %s)", distro.Name, distro.PackageManager))
	script, ok := installDockerScript(distro.PackageManager)
	if !ok {
		return fmt.Errorf("distro tidak didukung untuk instalasi otomatis Docker (package manager: %s)", distro.PackageManager)
	}

	if !s.mutex.TryLock(serverID) {
		_ = onLine("Server sedang dipakai operasi lain, menunggu antrian...")
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				if s.mutex.TryLock(serverID) {
					goto locked
				}
				_ = onLine("Masih menunggu antrian server...")
			}
		}
	}
locked:
	defer s.mutex.Unlock(serverID)

	_ = onLine("Memulai instalasi Docker...")
	if err := s.runInstallStream(ctx, access, script, onLine); err != nil {
		return err
	}
	_ = onLine("Instalasi Docker selesai.")
	return nil
}
