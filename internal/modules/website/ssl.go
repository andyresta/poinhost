package website

import (
	"strings"
	"time"
)

// sanTarget satu pasangan FQDN+webroot yang perlu divalidasi certbot lewat
// HTTP-01 (webroot mode) saat menerbitkan/memperbarui sertifikat.
type sanTarget struct {
	FQDN string
	Root string
}

// sanTargets mengumpulkan seluruh anggota "keluarga" SAN satu domain parent:
// domain itu sendiri, www.<domain> (berbagi document root yang sama, karena
// satu vhost mencakup keduanya lewat server_name), dan semua subdomain yang
// menunjuk domain ini sebagai Parent. Dulu di homepoin ini dihitung dengan
// memanggil ULANG domains.List (yang sendirinya N+1 round-trip) — di sini
// tinggal baca dari listDomains yang sudah SATU round-trip & ter-cache,
// jadi collectSANs pada praktiknya nyaris gratis (lihat riset ARCHITECTURE.md
// §Website: ini adalah sumber utama lambatnya tab SSL di homepoin).
func (s *Service) sanTargets(serverID, domain string) ([]sanTarget, error) {
	domains, err := s.listDomains(serverID)
	if err != nil {
		return nil, err
	}
	var parentRoot string
	found := false
	for _, d := range domains {
		if d.Domain == domain && !d.IsSubdomain {
			parentRoot = d.Root
			found = true
			break
		}
	}
	if !found {
		return nil, errFmt("domain %s tidak ditemukan (atau bukan domain induk)", domain)
	}

	targets := []sanTarget{{FQDN: domain, Root: parentRoot}, {FQDN: "www." + domain, Root: parentRoot}}
	for _, d := range domains {
		if d.IsSubdomain && d.Parent == domain {
			targets = append(targets, sanTarget{FQDN: d.Domain, Root: d.Root})
		}
	}
	return targets, nil
}

// vhostFamily mengembalikan semua entri vhost yang perlu ditulis ulang saat
// SSL diaktifkan/dinonaktifkan untuk satu domain parent — vhost parent itu
// sendiri (server_name-nya SUDAH mencakup www, jadi www TIDAK punya vhost
// terpisah) plus vhost tiap subdomain-nya.
func (s *Service) vhostFamily(serverID, domain string) ([]DomainInfo, error) {
	domains, err := s.listDomains(serverID)
	if err != nil {
		return nil, err
	}
	var family []DomainInfo
	for _, d := range domains {
		if d.Domain == domain && !d.IsSubdomain {
			family = append(family, d)
		} else if d.IsSubdomain && d.Parent == domain {
			family = append(family, d)
		}
	}
	if len(family) == 0 {
		return nil, errFmt("domain %s tidak ditemukan", domain)
	}
	return family, nil
}

const detectCertbotScript = `if command -v certbot >/dev/null 2>&1; then
  echo "CERTBOT=1"
  certbot --version 2>&1 | head -n1
else
  echo "CERTBOT=0"
fi`

func certExistsScript(domain string) string {
	return `CERT=/etc/letsencrypt/live/` + domain + `/fullchain.pem
KEY=/etc/letsencrypt/live/` + domain + `/privkey.pem
if [ -f "$CERT" ] && [ -f "$KEY" ]; then
  echo "CERT_EXISTS=1"
  echo "CERT_PATH=$CERT"
  echo "KEY_PATH=$KEY"
  openssl x509 -enddate -noout -in "$CERT" 2>/dev/null | sed 's/notAfter=/NOT_AFTER=/'
else
  echo "CERT_EXISTS=0"
fi`
}

// SSLStatus membaca status SSL untuk satu domain parent — certbot-detect +
// cert-exists digabung jadi SATU round-trip SSH (bukan dua panggilan
// terpisah seperti homepoin), plus sanTargets yang sekarang murah (lihat
// atas). SSL cuma berlaku untuk domain PARENT, bukan subdomain — satu
// sertifikat mencakup seluruh keluarga lewat SAN.
func (s *Service) SSLStatus(serverID, domain string) (*SSLStatus, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	nginxSt, err := s.getNginxStatus(serverID)
	if err != nil {
		return nil, err
	}
	if !nginxSt.Installed {
		return &SSLStatus{Domain: domain, NginxInstalled: false, Message: "Nginx belum terpasang di server"}, nil
	}

	info, err := s.getDomain(serverID, domain)
	if err != nil {
		return nil, err
	}
	if info.IsSubdomain {
		return &SSLStatus{Domain: domain, IsParent: false, NginxInstalled: true, Message: "SSL cuma bisa diaktifkan dari domain induk (parent), bukan subdomain — subdomain otomatis ikut tercakup lewat SAN saat SSL induknya diaktifkan"}, nil
	}

	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	res, err := s.run(access, detectCertbotScript+"\n"+certExistsScript(domain), 15*time.Second)
	if err != nil {
		return nil, err
	}

	st := &SSLStatus{
		Domain:         domain,
		IsParent:       true,
		DistroID:       nginxSt.DistroID,
		DistroName:     nginxSt.DistroName,
		PackageManager: nginxSt.PackageManager,
		CanInstall:     nginxSt.PackageManager != "",
		SSLEnabled:     info.SSLEnabled,
		NginxInstalled: true,
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "CERTBOT=1":
			st.CertbotInstalled = true
		case strings.HasPrefix(line, "CERT_EXISTS="):
			st.Certificate.Exists = strings.TrimPrefix(line, "CERT_EXISTS=") == "1"
		case strings.HasPrefix(line, "CERT_PATH="):
			st.Certificate.CertPath = strings.TrimPrefix(line, "CERT_PATH=")
		case strings.HasPrefix(line, "KEY_PATH="):
			st.Certificate.KeyPath = strings.TrimPrefix(line, "KEY_PATH=")
		case strings.HasPrefix(line, "NOT_AFTER="):
			st.Certificate.NotAfter = strings.TrimSpace(strings.TrimPrefix(line, "NOT_AFTER="))
		}
	}

	if sans, err := s.sanTargets(serverID, domain); err == nil {
		for _, t := range sans {
			st.SANs = append(st.SANs, t.FQDN)
		}
		st.Certificate.Domains = st.SANs
	}

	st.CanIssue = st.CertbotInstalled
	st.CanEnable = st.Certificate.Exists && !st.SSLEnabled
	return st, nil
}

// buildIssueCommand menyusun perintah `certbot certonly --webroot` yang
// mencakup seluruh keluarga SAN dalam SATU permintaan sertifikat.
func buildIssueCommand(email, certName string, targets []sanTarget, force bool) string {
	parts := []string{"certbot", "certonly", "--non-interactive", "--agree-tos",
		"--cert-name", shellQuote(certName), "--email", shellQuote(email)}
	if force {
		parts = append(parts, "--force-renewal")
	} else {
		parts = append(parts, "--keep-until-expiring")
	}
	for _, t := range targets {
		parts = append(parts, "--webroot", "-w", shellQuote(t.Root), "-d", shellQuote(t.FQDN))
	}
	return strings.Join(parts, " ")
}

// SSLIssue menerbitkan sertifikat Let's Encrypt baru (webroot mode) untuk
// satu domain parent + seluruh keluarganya (www + subdomain) — TIDAK
// otomatis mengaktifkan SSL di vhost (lihat SSLEnable terpisah), supaya
// user bisa memverifikasi dulu sertifikatnya berhasil terbit sebelum situs
// production ikut berubah ke HTTPS.
func (s *Service) SSLIssue(req SSLIssueRequest) (*SSLStatus, error) {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return nil, err
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		return nil, errFmt("email wajib diisi untuk menerbitkan sertifikat Let's Encrypt")
	}
	info, err := s.getDomain(req.ServerID, domain)
	if err != nil {
		return nil, err
	}
	if info.IsSubdomain {
		return nil, errFmt("SSL cuma bisa diterbitkan dari domain induk (parent)")
	}

	targets, err := s.sanTargets(req.ServerID, domain)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return nil, err
	}

	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)

	var mkdirs strings.Builder
	mkdirs.WriteString("set -e\n")
	seen := map[string]bool{}
	for _, t := range targets {
		if seen[t.Root] {
			continue
		}
		seen[t.Root] = true
		mkdirs.WriteString("mkdir -p " + shellQuote(t.Root+"/.well-known/acme-challenge") + "\n")
	}
	script := mkdirs.String() + buildIssueCommand(email, domain, targets, false)

	res, err := s.run(access, script, 180*time.Second)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = "gagal menerbitkan sertifikat SSL"
		}
		return nil, mapWebsiteError(errFmt("%s", msg))
	}

	return s.SSLStatus(req.ServerID, domain)
}

// SSLEnable menerapkan sertifikat yang sudah terbit ke vhost domain parent
// DAN seluruh subdomain-nya (satu cert, banyak vhost), lalu reload Nginx —
// masing-masing lewat rewriteVhost (SATU round-trip per vhost).
func (s *Service) SSLEnable(serverID, domain string) (*SSLStatus, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	st, err := s.SSLStatus(serverID, domain)
	if err != nil {
		return nil, err
	}
	if !st.Certificate.Exists {
		return nil, errFmt("sertifikat SSL belum ada untuk %s — terbitkan dulu", domain)
	}

	family, err := s.vhostFamily(serverID, domain)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	for _, member := range family {
		opts := optionsFromInfo(member)
		opts.SSLEnabled = true
		opts.SSLCertificate = st.Certificate.CertPath
		opts.SSLCertificateKey = st.Certificate.KeyPath
		if err := s.rewriteVhost(access, member.ConfigPath, opts); err != nil {
			return nil, errFmt("gagal menerapkan SSL ke %s: %w", member.Domain, err)
		}
	}
	return s.SSLStatus(serverID, domain)
}

// SSLDisable melepas SSL dari vhost domain parent + subdomain-nya (sertifikat
// di server TIDAK dihapus, cuma tidak lagi dipakai vhost — bisa diaktifkan
// lagi kapan saja tanpa perlu menerbitkan ulang).
func (s *Service) SSLDisable(serverID, domain string) (*SSLStatus, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	family, err := s.vhostFamily(serverID, domain)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	for _, member := range family {
		opts := optionsFromInfo(member)
		opts.SSLEnabled = false
		opts.SSLCertificate = ""
		opts.SSLCertificateKey = ""
		if err := s.rewriteVhost(access, member.ConfigPath, opts); err != nil {
			return nil, errFmt("gagal melepas SSL dari %s: %w", member.Domain, err)
		}
	}
	return s.SSLStatus(serverID, domain)
}

// SSLRenew memperbarui sertifikat via `certbot renew`, lalu reload Nginx
// dalam SATU Exec gabungan (renew + test + reload) supaya sertifikat baru
// langsung dipakai tanpa panggilan terpisah.
func (s *Service) SSLRenew(serverID, domain string) (*SSLStatus, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	script := `set -e
certbot renew --cert-name ` + shellQuote(domain) + ` --non-interactive
nginx -t
systemctl reload nginx || systemctl restart nginx`
	if _, err := s.run(access, script, 120*time.Second); err != nil {
		return nil, err
	}
	return s.SSLStatus(serverID, domain)
}

// installCertbotScript skrip instalasi certbot per package manager (tidak
// ada dukungan apk/Alpine, sama seperti homepoin).
func installCertbotScript(pm string) (string, bool) {
	switch pm {
	case "apt":
		return `set -e
` + aptWaitLock + `
apt-get install -y certbot
command -v certbot
echo ">> Certbot terpasang."
`, true
	case "dnf", "yum":
		return `set -e
` + pm + ` install -y certbot
command -v certbot
echo ">> Certbot terpasang."
`, true
	default:
		return "", false
	}
}
