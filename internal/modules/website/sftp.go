package website

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// SFTPAccountInfo satu akun SFTP ter-chroot untuk satu domain.
type SFTPAccountInfo struct {
	Username string `json:"username"`
	Domain   string `json:"domain"`
	Chroot   string `json:"chroot"`
	HomeDir  string `json:"homeDir"`
	Enabled  bool   `json:"enabled"`
}

// SFTPListResponse daftar akun SFTP milik satu domain.
type SFTPListResponse struct {
	Domain     string            `json:"domain"`
	DomainRoot string            `json:"domainRoot"`
	Accounts   []SFTPAccountInfo `json:"accounts"`
}

// SFTPCreateAccountRequest membuat akun SFTP baru untuk satu domain.
type SFTPCreateAccountRequest struct {
	ServerID string `json:"serverId"`
	Domain   string `json:"domain"`
	Username string `json:"username"`
	Password string `json:"password"`
	Chroot   string `json:"chroot,omitempty"`  // opsional, default folder induk document root
	HomeDir  string `json:"homeDir,omitempty"` // opsional, relatif ke chroot
}

var sftpUsernameRE = regexp.MustCompile(`^[a-z_][a-z0-9_-]{2,31}$`)

func normalizeSFTPUsername(raw string) (string, error) {
	u := strings.ToLower(strings.TrimSpace(raw))
	if !sftpUsernameRE.MatchString(u) {
		return "", errFmt("username tidak valid (huruf kecil/angka/-/_ , 3-32 karakter, diawali huruf atau _)")
	}
	return u, nil
}

// domainBaseFromRoot mengembalikan folder INDUK document root (satu tingkat
// di atas public_html) — inilah yang jadi ChrootDirectory default, BUKAN
// public_html itu sendiri: sshd mewajibkan ChrootDirectory (dan semua
// leluhurnya sampai /) dimiliki root:root & tidak bisa ditulis group/other,
// padahal public_html sengaja dimiliki grup web (lihat prepareWebRootScript)
// supaya PHP-FPM & SFTP bisa sama-sama menulis di sana.
func domainBaseFromRoot(root string) string {
	return strings.TrimSuffix(root, "/public_html")
}

func sftpConfPath(username string) string {
	return "/etc/ssh/sshd_config.d/poinhost-sftp-" + username + ".conf"
}

func buildSFTPConfig(domain, chroot, homeDir, username string) string {
	return fmt.Sprintf(`# poinhost-managed sftp domain=%s chroot=%s home=%s
Match User %s
    ChrootDirectory %s
    ForceCommand internal-sftp -d %s
    AllowTcpForwarding no
    X11Forwarding no
    PasswordAuthentication yes
`, domain, chroot, homeDir, username, chroot, homeDir)
}

// ListSFTPAccounts mengembalikan akun SFTP milik satu domain — SATU
// round-trip SSH (scan semua file drop-in sshd_config.d sekaligus), root
// domain dari getDomain yang sudah ter-cache.
func (s *Service) ListSFTPAccounts(serverID, domain string) (*SFTPListResponse, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	info, err := s.getDomain(serverID, domain)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	script := `set +e
for f in /etc/ssh/sshd_config.d/poinhost-sftp-*.conf; do
  [ -f "$f" ] || continue
  META=$(grep -m1 '^# poinhost-managed sftp ' "$f")
  DOMAIN=$(echo "$META" | sed -n 's/.*domain=\([^ ]*\).*/\1/p')
  CHROOT=$(echo "$META" | sed -n 's/.*chroot=\([^ ]*\).*/\1/p')
  HOME=$(echo "$META" | sed -n 's/.*home=\([^ ]*\).*/\1/p')
  USER=$(grep -m1 '^Match User ' "$f" | awk '{print $3}')
  LOCKED=no
  passwd -S "$USER" 2>/dev/null | awk '{print $2}' | grep -q '^L' && LOCKED=yes
  echo "ACC|$USER|$DOMAIN|$CHROOT|$HOME|$LOCKED"
done
exit 0`

	res, err := s.run(access, script, 15*time.Second)
	if err != nil {
		return nil, err
	}

	accounts := make([]SFTPAccountInfo, 0)
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ACC|") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 6 {
			continue
		}
		if parts[2] != domain {
			continue
		}
		accounts = append(accounts, SFTPAccountInfo{
			Username: parts[1], Domain: parts[2], Chroot: parts[3], HomeDir: parts[4], Enabled: parts[5] != "yes",
		})
	}
	return &SFTPListResponse{Domain: domain, DomainRoot: info.Root, Accounts: accounts}, nil
}

// CreateSFTPAccount membuat akun Linux ter-chroot baru untuk satu domain,
// SEMUA langkah (dedupe check, siapkan rantai kepemilikan chroot, useradd,
// set password, siapkan home dir, tulis drop-in sshd, reload) dalam SATU
// script/round-trip SSH — bukan ~9 exec terpisah seperti homepoin, dengan
// rollback (`trap ... ERR`) yang membersihkan user & config kalau ada
// langkah yang gagal di tengah jalan.
func (s *Service) CreateSFTPAccount(req SFTPCreateAccountRequest) error {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return err
	}
	username, err := normalizeSFTPUsername(req.Username)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.Password) == "" {
		return errFmt("password wajib diisi")
	}
	info, err := s.getDomain(req.ServerID, domain)
	if err != nil {
		return err
	}
	if info.IsSubdomain {
		return errFmt("akun SFTP dibuat dari domain induk (parent), bukan subdomain")
	}

	base := domainBaseFromRoot(info.Root)
	chroot := strings.TrimSpace(req.Chroot)
	if chroot == "" {
		chroot = base
	}
	if !strings.HasPrefix(chroot, base) {
		return errFmt("chroot harus di dalam folder domain (%s)", base)
	}
	homeDir := strings.TrimSpace(req.HomeDir)
	if homeDir == "" {
		if chroot == base && strings.HasSuffix(info.Root, "/public_html") {
			homeDir = "/public_html"
		} else {
			homeDir = "/"
		}
	}
	if !strings.HasPrefix(homeDir, "/") {
		homeDir = "/" + homeDir
	}

	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}
	return s.provisionSFTPAccount(access, domain, username, chroot, homeDir, req.Password, false)
}

// provisionSFTPAccount menjalankan pembuatan akun (lihat CreateSFTPAccount)
// dengan secret berupa password polos, ATAU — kalau encrypted — hash
// /etc/shadow apa adanya (`chpasswd -e`), dipakai migrasi website supaya
// password akun SFTP ikut pindah tanpa pernah diketahui plaintext-nya.
func (s *Service) provisionSFTPAccount(access *websiteAccess, domain, username, chroot, homeDir, secret string, encrypted bool) error {
	chpasswdFlag := ""
	if encrypted {
		chpasswdFlag = " -e"
	}
	confPath := sftpConfPath(username)
	confB64 := base64.StdEncoding.EncodeToString([]byte(buildSFTPConfig(domain, chroot, homeDir, username)))
	homeAbs := chroot
	if homeDir != "/" {
		homeAbs = chroot + homeDir
	}

	script := fmt.Sprintf(`set -e
CONF=%s
if [ -f "$CONF" ]; then echo "__poinhost_EXISTS__"; exit 1; fi
if getent passwd %s >/dev/null 2>&1; then
  SHELL_PATH=$(getent passwd %s | cut -d: -f7)
  case "$SHELL_PATH" in
    */nologin|*/false) pkill -KILL -u %s 2>/dev/null || true; sleep 1; userdel -f %s 2>/dev/null || true ;;
    *) echo "__poinhost_USER_CONFLICT__"; exit 1 ;;
  esac
fi

CHROOT=%s
p="$CHROOT"
while [ "$p" != "/" ] && [ -n "$p" ]; do
  mkdir -p "$p"
  chown root:root "$p"
  chmod 755 "$p"
  p=$(dirname "$p")
done

trap 'rm -f "$CONF"; id %s >/dev/null 2>&1 && { pkill -KILL -u %s 2>/dev/null || true; sleep 1; userdel -f %s 2>/dev/null || true; }; true' ERR

WEBGROUP=www-data
if getent group www-data >/dev/null 2>&1; then WEBGROUP=www-data
elif getent group nginx >/dev/null 2>&1; then WEBGROUP=nginx
elif getent group apache >/dev/null 2>&1; then WEBGROUP=apache
fi

if getent group "$WEBGROUP" >/dev/null 2>&1; then
  useradd -M -s /usr/sbin/nologin -d /nonexistent -G "$WEBGROUP" %s
else
  useradd -M -s /usr/sbin/nologin -d /nonexistent %s
fi
printf '%%s:%%s\n' %s %s | chpasswd%s

HOME_ABS=%s
mkdir -p "$HOME_ABS"
chown -R %s:"$WEBGROUP" "$HOME_ABS"
find "$HOME_ABS" -type d -exec chmod 2775 {} \;
find "$HOME_ABS" -type f -exec chmod 0664 {} \;

mkdir -p /etc/ssh/sshd_config.d
chmod 755 /etc/ssh/sshd_config.d
if ! grep -q '^Include /etc/ssh/sshd_config.d/\*.conf' /etc/ssh/sshd_config 2>/dev/null; then
  echo 'Include /etc/ssh/sshd_config.d/*.conf' >> /etc/ssh/sshd_config
fi

echo %s | base64 -d > "$CONF"

sshd -t
systemctl reload sshd 2>/dev/null || systemctl reload ssh 2>/dev/null || service sshd reload 2>/dev/null || service ssh reload 2>/dev/null
trap - ERR
`,
		shellQuote(confPath),
		shellQuote(username), shellQuote(username), shellQuote(username), shellQuote(username),
		shellQuote(chroot),
		shellQuote(username), shellQuote(username), shellQuote(username),
		shellQuote(username), shellQuote(username),
		shellQuote(username), shellQuote(secret), chpasswdFlag,
		shellQuote(homeAbs),
		shellQuote(username),
		shellQuote(confB64),
	)

	res, err := s.run(access, script, 30*time.Second)
	if err != nil {
		return err
	}
	if strings.Contains(res.Stdout, "__poinhost_EXISTS__") {
		return errFmt("akun SFTP %s sudah ada", username)
	}
	if strings.Contains(res.Stdout, "__poinhost_USER_CONFLICT__") {
		return errFmt("username %s sudah dipakai user Linux lain (bukan buatan poinhost)", username)
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = "gagal membuat akun SFTP"
		}
		return mapWebsiteError(errFmt("%s", msg))
	}
	return nil
}

// DeleteSFTPAccount menghapus akun SFTP — menolak kalau config-nya ternyata
// milik domain lain (proteksi cross-domain).
func (s *Service) DeleteSFTPAccount(serverID, domain, username string) error {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return err
	}
	username, err = normalizeSFTPUsername(username)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}

	script := fmt.Sprintf(`set -e
CONF=%s
if [ ! -f "$CONF" ]; then echo "__poinhost_NOT_FOUND__"; exit 0; fi
OWNER_DOMAIN=$(grep -m1 '^# poinhost-managed sftp ' "$CONF" | sed -n 's/.*domain=\([^ ]*\).*/\1/p')
if [ "$OWNER_DOMAIN" != %s ]; then echo "__poinhost_WRONG_DOMAIN__"; exit 1; fi
rm -f "$CONF"
if getent passwd %s >/dev/null 2>&1; then
  pkill -KILL -u %s 2>/dev/null || true
  sleep 1
  userdel -f %s 2>/dev/null || true
fi
sshd -t
systemctl reload sshd 2>/dev/null || systemctl reload ssh 2>/dev/null || service sshd reload 2>/dev/null || service ssh reload 2>/dev/null
`, shellQuote(sftpConfPath(username)), shellQuote(domain), shellQuote(username), shellQuote(username), shellQuote(username))

	res, err := s.run(access, script, 20*time.Second)
	if err != nil {
		return err
	}
	if strings.Contains(res.Stdout, "__poinhost_WRONG_DOMAIN__") {
		return errFmt("akun %s bukan milik domain %s", username, domain)
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = "gagal menghapus akun SFTP"
		}
		return mapWebsiteError(errFmt("%s", msg))
	}
	return nil
}
