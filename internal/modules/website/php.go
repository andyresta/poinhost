package website

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// supportedPHPVersions versi PHP-FPM yang ditawarkan untuk dipasang.
var supportedPHPVersions = []string{"8.1", "8.2", "8.3", "8.4"}

const phpDetectScript = `set +e
for s in /run/php/php*-fpm.sock; do [ -S "$s" ] && echo "SOCK=$s"; done
if command -v dpkg-query >/dev/null 2>&1; then
  dpkg-query -W -f='PKG=${Package}\n' 2>/dev/null | grep -E '^PKG=php[0-9]+\.[0-9]+-fpm$'
fi
for s in /var/opt/remi/php*/run/php-fpm/www.sock; do [ -S "$s" ] && echo "REMISOCK=$s"; done
if command -v rpm >/dev/null 2>&1; then
  rpm -qa 2>/dev/null | grep -E '^php[0-9]+-php-fpm' | sed 's/^/RPM=/'
fi
if ls /etc/apt/sources.list.d/*.list >/dev/null 2>&1 && grep -ls 'ondrej/php\|packages.sury.org/php' /etc/apt/sources.list.d/*.list >/dev/null 2>&1; then
  echo "REPO_APT=1"
fi
rpm -q remi-release >/dev/null 2>&1 && echo "REPO_REMI=1"
true`

var (
	debianSockRE = regexp.MustCompile(`php([0-9]+\.[0-9]+)-fpm\.sock$`)
	debianPkgRE  = regexp.MustCompile(`^PKG=php([0-9]+\.[0-9]+)-fpm$`)
	remiSockRE   = regexp.MustCompile(`php([0-9]{2,3})/run/php-fpm`)
	rpmPkgRE     = regexp.MustCompile(`^RPM=php([0-9]{2,3})-php-fpm`)
)

// remiCompactToVersion mengubah "83" jadi "8.3" (format compact khas paket Remi/RPM).
func remiCompactToVersion(compact string) string {
	if len(compact) < 2 {
		return compact
	}
	return compact[:1] + "." + compact[1:]
}

// PHPStatus mengembalikan status PHP-FPM di server (+ PHP domain tertentu
// kalau domain diisi) — SATU round-trip SSH untuk deteksi versi/repo
// (phpDetectScript), memakai ULANG info distro dari getNginxStatus yang
// sudah ter-cache (bukan re-detect distro lagi seperti homepoin's
// detectPHPStatus), dan info domain dari listDomains yang juga ter-cache —
// total request ini nyaris selalu 1 round-trip SSH baru, bukan ~6 seperti
// homepoin's php.Status.
func (s *Service) PHPStatus(serverID, domain string) (*PHPStatus, error) {
	nginxSt, err := s.getNginxStatus(serverID)
	if err != nil {
		return nil, err
	}
	if !nginxSt.Installed {
		return &PHPStatus{DistroID: nginxSt.DistroID, DistroName: nginxSt.DistroName, PackageManager: nginxSt.PackageManager, CanInstall: nginxSt.CanInstall}, nil
	}

	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	res, err := s.run(access, phpDetectScript, 15*time.Second)
	if err != nil {
		return nil, err
	}

	st := parsePHPDetect(res.Stdout, nginxSt)

	if domain != "" {
		if info, err := s.getDomain(serverID, domain); err == nil {
			st.DomainVersion = info.PHPVersion
			st.DomainEnabled = info.PHPEnabled
		}
	}
	return st, nil
}

func parsePHPDetect(out string, nginxSt *NginxStatus) *PHPStatus {
	seen := map[string]*PHPVersionInfo{}
	var order []string
	ensure := func(v string) *PHPVersionInfo {
		if vi, ok := seen[v]; ok {
			return vi
		}
		vi := &PHPVersionInfo{Version: v}
		seen[v] = vi
		order = append(order, v)
		return vi
	}
	repoConfigured := false

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "SOCK="):
			if m := debianSockRE.FindStringSubmatch(line); m != nil {
				vi := ensure(m[1])
				vi.Socket = strings.TrimPrefix(line, "SOCK=")
				vi.Active = true
			}
		case strings.HasPrefix(line, "PKG="):
			if m := debianPkgRE.FindStringSubmatch(line); m != nil {
				ensure(m[1])
			}
		case strings.HasPrefix(line, "REMISOCK="):
			if m := remiSockRE.FindStringSubmatch(line); m != nil {
				vi := ensure(remiCompactToVersion(m[1]))
				vi.Socket = strings.TrimPrefix(line, "REMISOCK=")
				vi.Active = true
			}
		case strings.HasPrefix(line, "RPM="):
			if m := rpmPkgRE.FindStringSubmatch(line); m != nil {
				ensure(remiCompactToVersion(m[1]))
			}
		case line == "REPO_APT=1", line == "REPO_REMI=1":
			repoConfigured = true
		}
	}
	sort.Strings(order)
	versions := make([]PHPVersionInfo, 0, len(order))
	for _, v := range order {
		versions = append(versions, *seen[v])
	}

	available := make([]string, 0)
	for _, v := range supportedPHPVersions {
		if _, ok := seen[v]; !ok {
			available = append(available, v)
		}
	}

	return &PHPStatus{
		Installed:      len(versions) > 0,
		RepoConfigured: repoConfigured,
		DistroID:       nginxSt.DistroID,
		DistroName:     nginxSt.DistroName,
		PackageManager: nginxSt.PackageManager,
		CanInstall:     nginxSt.PackageManager != "",
		Versions:       versions,
		Available:      available,
	}
}

// phpFastCGISocket path socket PHP-FPM untuk satu versi, sesuai konvensi
// package manager server (Debian/Ubuntu vs RHEL/Remi).
func phpFastCGISocket(pm, version string) string {
	switch pm {
	case "dnf", "yum":
		return "/var/opt/remi/php" + strings.ReplaceAll(version, ".", "") + "/run/php-fpm/www.sock"
	default:
		return "/run/php/php" + version + "-fpm.sock"
	}
}

// phpRepoScript menyiapkan repo pihak ketiga yang menyediakan banyak versi
// PHP sekaligus (Ubuntu: PPA ondrej/php; Debian: sury.org; RHEL: Remi).
func phpRepoScript(pm, distroID string) (string, bool) {
	switch pm {
	case "apt":
		if distroID == "debian" {
			return `set -e
` + aptWaitLock + `
apt-get install -y ca-certificates curl gnupg lsb-release
curl -fsSL https://packages.sury.org/php/apt.gpg -o /usr/share/keyrings/deb_sury-php.gpg
echo "deb [signed-by=/usr/share/keyrings/deb_sury-php.gpg] https://packages.sury.org/php/ $(lsb_release -sc) main" > /etc/apt/sources.list.d/php.list
apt-get update
echo ">> Repo PHP (sury.org) siap."
`, true
		}
		return `set -e
` + aptWaitLock + `
apt-get install -y software-properties-common
add-apt-repository -y ppa:ondrej/php
apt-get update
echo ">> Repo PHP (ondrej/php) siap."
`, true
	case "dnf", "yum":
		return `set -e
` + pm + ` install -y https://rpms.remirepo.net/enterprise/remi-release-$(rpm -E %rhel).rpm
` + pm + ` module reset php -y 2>/dev/null || true
echo ">> Repo PHP (Remi) siap."
`, true
	default:
		return "", false
	}
}

// phpInstallVersionScript memasang satu versi PHP-FPM (+ ekstensi umum).
func phpInstallVersionScript(pm, version string) (string, bool) {
	if version == "" {
		return "", false
	}
	switch pm {
	case "apt":
		return `set -e
` + aptWaitLock + `
apt-get install -y php` + version + `-fpm php` + version + `-cli php` + version + `-mbstring php` + version + `-xml php` + version + `-curl php` + version + `-mysql
systemctl enable php` + version + `-fpm
systemctl restart php` + version + `-fpm
echo ">> PHP ` + version + ` terpasang."
`, true
	case "dnf", "yum":
		compact := strings.ReplaceAll(version, ".", "")
		return `set -e
` + pm + ` module enable -y php:remi-` + version + ` 2>/dev/null || true
` + pm + ` install -y php` + compact + `-php-fpm php` + compact + `-php-cli php` + compact + `-php-mbstring php` + compact + `-php-xml php` + compact + `-php-mysqlnd
systemctl enable php` + compact + `-php-fpm
systemctl restart php` + compact + `-php-fpm
echo ">> PHP ` + version + ` terpasang."
`, true
	default:
		return "", false
	}
}

// PHPSetDomain mengaktifkan satu versi PHP-FPM untuk satu domain — menulis
// ulang `fastcgi_pass` di vhost domain itu ke socket versi yang dipilih lalu
// reload Nginx (lewat rewriteVhost, SATU round-trip). Menyeleksi PHP
// otomatis MELEPAS whole-domain proxy target kalau ada (PHP dan "proxy
// seluruh domain" saling meniadakan) tapi mempertahankan proxy RULE
// per-path yang mungkin sudah ada (keduanya bisa hidup berdampingan —
// lihat buildLocationBlocks).
func (s *Service) PHPSetDomain(req PHPSetDomainRequest) error {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return err
	}
	st, err := s.PHPStatus(req.ServerID, "")
	if err != nil {
		return err
	}
	found := false
	for _, v := range st.Versions {
		if v.Version == req.Version {
			found = true
			break
		}
	}
	if !found {
		return errFmt("PHP %s belum terpasang di server", req.Version)
	}

	info, err := s.getDomain(req.ServerID, domain)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}

	opts := optionsFromInfo(info)
	opts.PHPVersion = req.Version
	opts.FastCGISocket = phpFastCGISocket(st.PackageManager, req.Version)
	opts.ProxyTarget = ""

	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)
	return s.rewriteVhost(access, info.ConfigPath, opts)
}

// PHPDisableDomain mengembalikan domain ke static-only (melepas PHP).
func (s *Service) PHPDisableDomain(serverID, domain string) error {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return err
	}
	info, err := s.getDomain(serverID, domain)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}

	opts := optionsFromInfo(info)
	opts.PHPVersion = ""
	opts.FastCGISocket = ""

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)
	return s.rewriteVhost(access, info.ConfigPath, opts)
}
