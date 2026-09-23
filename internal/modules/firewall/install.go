package firewall

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Instalasi firewall untuk server yang belum punya satu pun.
//
// Prinsip utamanya: INSTALASI TIDAK PERNAH MENYALAKAN FIREWALL. Menyalakan
// tetap lewat SetEnabled, yang selalu mengizinkan port SSH lebih dulu dalam
// langkah yang sama. Ini bukan formalitas — beberapa paket MENYALAKAN
// dirinya sendiri saat dipasang, dan konfigurasi bawaannya hanya membuka
// port 22 (service "ssh"). Server yang SSH-nya di port lain langsung
// terputus di tengah instalasi, termasuk koneksi poinhost yang sedang
// menjalankan instalasi itu:
//
//   - firewalld di Debian/Ubuntu: postinst paket langsung enable + start.
//     Dicegah dengan policy-rc.d (mekanisme resmi Debian untuk melarang
//     maintainer script menyalakan service) selama apt berjalan.
//   - firewalld di RHEL/Fedora/openSUSE: preset systemd meng-enable-nya,
//     jadi ia menyala di reboot berikutnya. Di-disable lagi sesudah instal.
//
// Sebagai lapis kedua, port SSH yang dipakai poinhost langsung dimasukkan
// ke konfigurasi permanen backend yang baru dipasang (ufw: aturan ditambah
// walau ufw belum aktif; firewalld: firewall-offline-cmd) — kalau firewall
// dinyalakan dari luar poinhost (CLI, reboot), SSH tetap terbuka.

// InstallInfo hasil deteksi untuk wizard instalasi.
type InstallInfo struct {
	DistroID       string `json:"distroId"`
	DistroName     string `json:"distroName"`
	PackageManager string `json:"packageManager"`
	// Recommended backend bawaan/lazim distro ini ("ufw" / "firewalld").
	Recommended string `json:"recommended"`
	// Options backend yang bisa dipasang otomatis di distro ini.
	Options   []string `json:"options"`
	Installed []string `json:"installed"`
	// Blocker alasan instalasi tidak bisa dijalankan (mis. firewall lain
	// sudah AKTIF — dua firewall aktif saling menimpa aturan iptables).
	Blocker    string `json:"blocker,omitempty"`
	SSHPort    int    `json:"sshPort"`
	HasDocker  bool   `json:"hasDocker"`
	CanInstall bool   `json:"canInstall"`
}

const installDetectScript = `set +e
. /etc/os-release 2>/dev/null
echo "ID=${ID:-unknown}"
echo "PRETTY=${PRETTY_NAME:-unknown}"
command -v apt-get >/dev/null 2>&1 && echo PM=apt
command -v dnf >/dev/null 2>&1 && echo PM=dnf
command -v yum >/dev/null 2>&1 && echo PM=yum
command -v zypper >/dev/null 2>&1 && echo PM=zypper
command -v pacman >/dev/null 2>&1 && echo PM=pacman
command -v apk >/dev/null 2>&1 && echo PM=apk
command -v ufw >/dev/null 2>&1 && echo INSTALLED=ufw
command -v firewall-cmd >/dev/null 2>&1 && echo INSTALLED=firewalld
command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi "^Status: active" && echo ACTIVE=ufw
command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state 2>/dev/null | grep -qi running && echo ACTIVE=firewalld
systemctl is-active --quiet nftables 2>/dev/null && echo ACTIVE=nftables
command -v docker >/dev/null 2>&1 && echo DOCKER=1
exit 0`

// pmPriority urutan pilihan package manager kalau lebih dari satu ada
// (mis. dnf DAN yum di RHEL 8+ — dnf yang dipakai).
var pmPriority = []string{"apt", "dnf", "yum", "zypper", "pacman", "apk"}

// parseInstallInfo dipisah supaya bisa diuji tanpa SSH.
func parseInstallInfo(out string, sshPort int) *InstallInfo {
	info := &InstallInfo{DistroID: "unknown", DistroName: "unknown", SSHPort: sshPort, Options: []string{}, Installed: []string{}}
	pms := map[string]bool{}
	var active []string
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "ID":
			info.DistroID = strings.ToLower(v)
		case "PRETTY":
			info.DistroName = v
		case "PM":
			pms[v] = true
		case "INSTALLED":
			info.Installed = append(info.Installed, v)
		case "ACTIVE":
			active = append(active, v)
		case "DOCKER":
			info.HasDocker = true
		}
	}
	for _, pm := range pmPriority {
		if pms[pm] {
			info.PackageManager = pm
			break
		}
	}
	info.Recommended, info.Options = recommendBackend(info.PackageManager)
	if len(active) > 0 {
		info.Blocker = "firewall " + strings.Join(active, ", ") + " sudah aktif di server ini — kelola yang itu saja"
	}
	info.CanInstall = info.Blocker == "" && len(info.Options) > 0
	return info
}

// recommendBackend memilih firewall yang lazim per package manager: keluarga
// Debian/Ubuntu (apt) dan Arch (pacman) memakai ufw, keluarga Red Hat & SUSE memakai
// firewalld (bawaan distro itu, terintegrasi dengan SELinux/NetworkManager
// dan tool distro lain). Hanya backend yang paketnya tersedia di repo
// resmi distro yang ditawarkan — ufw di RHEL butuh EPEL, jadi tidak.
func recommendBackend(pm string) (string, []string) {
	switch pm {
	case "apt":
		return "ufw", []string{"ufw", "firewalld"}
	case "dnf", "yum":
		return "firewalld", []string{"firewalld"}
	case "zypper":
		return "firewalld", []string{"firewalld"}
	case "pacman":
		return "ufw", []string{"ufw", "firewalld"}
	case "apk":
		return "ufw", []string{"ufw"}
	}
	return "", nil
}

const aptWaitLock = `for i in $(seq 1 60); do
  if fuser /var/lib/dpkg/lock-frontend >/dev/null 2>&1 || fuser /var/lib/apt/lists/lock >/dev/null 2>&1; then
    [ "$i" = 1 ] && echo ">> Menunggu proses apt lain selesai..."
    sleep 5
  else
    break
  fi
done`

// installFirewallScript skrip instalasi per package manager + backend.
func installFirewallScript(pm, backend string, sshPort int) (string, bool) {
	p := strconv.Itoa(sshPort)
	var install string
	switch pm + "/" + backend {
	case "apt/ufw":
		install = aptWaitLock + `
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ufw`
	case "apt/firewalld":
		install = aptWaitLock + `
export DEBIAN_FRONTEND=noninteractive
# Cegah postinst menyalakan firewalld (lihat catatan di atas file ini).
# policy-rc.d milik admin yang sudah ada tidak ditimpa.
OWN_POLICY=0
if [ ! -e /usr/sbin/policy-rc.d ]; then
  printf '#!/bin/sh\nexit 101\n' > /usr/sbin/policy-rc.d
  chmod +x /usr/sbin/policy-rc.d
  OWN_POLICY=1
fi
trap '[ "$OWN_POLICY" = 1 ] && rm -f /usr/sbin/policy-rc.d' EXIT
apt-get update
apt-get install -y firewalld
[ "$OWN_POLICY" = 1 ] && rm -f /usr/sbin/policy-rc.d
systemctl disable --now firewalld 2>/dev/null || true`
	case "dnf/firewalld":
		install = `dnf install -y firewalld
systemctl disable --now firewalld 2>/dev/null || true`
	case "yum/firewalld":
		install = `yum install -y firewalld
systemctl disable --now firewalld 2>/dev/null || true`
	case "zypper/firewalld":
		install = `zypper --non-interactive install firewalld
systemctl disable --now firewalld 2>/dev/null || true`
	case "pacman/ufw":
		install = `pacman -S --noconfirm --needed ufw`
	case "pacman/firewalld":
		install = `pacman -S --noconfirm --needed firewalld
systemctl disable --now firewalld 2>/dev/null || true`
	case "apk/ufw":
		install = `apk add --no-cache ufw || { echo ">> Paket ufw ada di repo 'community' — aktifkan repo itu di /etc/apk/repositories lalu ulangi."; exit 1; }`
	default:
		return "", false
	}

	var preallow string
	if backend == "ufw" {
		// `ufw allow` saat ufw BELUM aktif hanya menyimpan aturan ke
		// konfigurasi, tidak menyalakan apa pun.
		preallow = `echo ">> Mengizinkan port SSH ` + p + ` lebih dulu (firewall tetap MATI)..."
ufw allow to any port ` + p + ` proto tcp comment ` + shellQuote("SSH poinhost "+firewallRuleMarker) + `
ufw status verbose | head -3`
	} else {
		preallow = `echo ">> Mengizinkan port SSH ` + p + ` di konfigurasi permanen (firewall tetap MATI)..."
firewall-offline-cmd --add-port=` + p + `/tcp
echo "firewalld: $(systemctl is-active firewalld 2>/dev/null) / $(systemctl is-enabled firewalld 2>/dev/null)"`
	}

	return `set -e
echo ">> Memasang ` + backend + `..."
` + install + `
` + preallow + `
echo ">> Selesai. Firewall BELUM dinyalakan — nyalakan dari panel Firewall."
`, true
}

// InstallInfo mendeteksi distro, package manager, dan firewall yang ada.
func (s *Service) InstallInfo(serverID string) (*InstallInfo, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	res, err := s.run(access, installDetectScript, 20*time.Second)
	if err != nil {
		return nil, err
	}
	return parseInstallInfo(res.Stdout, access.sshPort), nil
}

// StreamInstall memasang backend firewall sambil mengalirkan output ke onLine.
func (s *Service) StreamInstall(ctx context.Context, serverID, backend string, onLine func(string) error) error {
	if onLine == nil {
		onLine = func(string) error { return nil }
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}
	_ = onLine("Mendeteksi sistem operasi server...")
	res, err := s.run(access, installDetectScript, 20*time.Second)
	if err != nil {
		return err
	}
	info := parseInstallInfo(res.Stdout, access.sshPort)
	_ = onLine(fmt.Sprintf("Distro: %s (paket: %s)", info.DistroName, info.PackageManager))
	if info.Blocker != "" {
		return fmt.Errorf("%s", info.Blocker)
	}
	allowed := false
	for _, o := range info.Options {
		if o == backend {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("%s tidak bisa dipasang otomatis di distro ini (package manager: %s)", backend, info.PackageManager)
	}
	script, ok := installFirewallScript(info.PackageManager, backend, access.sshPort)
	if !ok {
		return fmt.Errorf("kombinasi %s/%s belum didukung", info.PackageManager, backend)
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	const sentinel = "__poinhost_INSTALL_DONE__:"
	wrapped := "(\n" + script + "\n) 2>&1\necho \"" + sentinel + "$?\""
	exitCode := -1
	streamErr := s.executor.ExecStreamPTY(ctx, serverID, access.wrap(wrapped), func(line string) error {
		if rest, ok := strings.CutPrefix(line, sentinel); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil {
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
		return fmt.Errorf("instalasi gagal (kode keluar %d)", exitCode)
	}
	return nil
}
