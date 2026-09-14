package website

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"time"
)

const listVhostsScript = `set +e
for f in /etc/nginx/conf.d/` + confPrefix + `*.conf /etc/nginx/conf.d/` + confPrefix + `*.conf.disabled /etc/nginx/conf.d/` + legacyConfPrefix + `*.conf /etc/nginx/conf.d/` + legacyConfPrefix + `*.conf.disabled; do
  [ -f "$f" ] || continue
  case "$f" in */` + legacyPortConfPrefix + `*) continue ;; esac
  echo "===POINHOST_VHOST_START==="
  echo "PATH=$f"
  cat "$f"
  echo "===POINHOST_VHOST_END==="
done
exit 0`

// listDomains mengembalikan semua domain yang dikelola poinhost di server —
// SATU round-trip SSH total (ls+cat semua vhost sekaligus lewat marker),
// bukan "ls lalu cat per file" seperti homepoin (lihat riset di
// ARCHITECTURE.md §Website: itu N+1 round-trip untuk N domain). Hasilnya
// di-cache singkat (statusCacheTTL) karena dipanggil dari List/PHP-status/
// SSL-status yang sering dibuka bergantian dalam satu sesi klik.
//
// Ikut men-scan prefix legacyConfPrefix (vhost lama buatan homepoin) supaya
// domain yang sudah ada di server SEBELUM pindah ke poinhost tetap muncul
// & bisa dikelola langsung — bukan "hilang" sampai dibuat ulang manual.
// Begitu domain itu diedit lewat poinhost (PHP/SSL/proxy), isinya ditulis
// ulang ke format marker poinhost sendiri (lihat buildMetaComment), tapi
// nama filenya (prefix homepoin-) sengaja TIDAK diganti — tidak ada alasan
// rename selain kosmetik, dan rename berarti round-trip SSH tambahan.
func (s *Service) listDomains(serverID string) ([]DomainInfo, error) {
	s.cacheMu.Lock()
	entry, ok := s.domainCache[serverID]
	s.cacheMu.Unlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.domains, nil
	}

	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	res, err := s.run(access, listVhostsScript, 20*time.Second)
	if err != nil {
		return nil, err
	}

	domains := parseVhostBlocks(res.Stdout)
	sort.Slice(domains, func(i, j int) bool { return domains[i].Domain < domains[j].Domain })

	s.cacheMu.Lock()
	s.domainCache[serverID] = domainListCacheEntry{domains: domains, expiresAt: time.Now().Add(statusCacheTTL)}
	s.cacheMu.Unlock()
	return domains, nil
}

func parseVhostBlocks(stdout string) []DomainInfo {
	domains := make([]DomainInfo, 0)
	lines := strings.Split(stdout, "\n")
	i := 0
	for i < len(lines) {
		if strings.TrimSpace(lines[i]) != "===POINHOST_VHOST_START===" {
			i++
			continue
		}
		i++
		if i >= len(lines) || !strings.HasPrefix(lines[i], "PATH=") {
			continue
		}
		path := strings.TrimPrefix(lines[i], "PATH=")
		i++
		var body strings.Builder
		for i < len(lines) && strings.TrimSpace(lines[i]) != "===POINHOST_VHOST_END===" {
			body.WriteString(lines[i])
			body.WriteByte('\n')
			i++
		}
		i++ // skip END marker
		enabled := !strings.HasSuffix(path, ".disabled")
		if info, ok := parseVhostFile(path, body.String(), enabled); ok {
			domains = append(domains, info)
		}
	}
	return domains
}

// List mengembalikan status Nginx + daftar domain di satu server.
func (s *Service) List(serverID string) (*ListResponse, error) {
	nginxSt, err := s.getNginxStatus(serverID)
	if err != nil {
		return nil, err
	}
	if !nginxSt.Installed {
		return &ListResponse{Nginx: *nginxSt, Domains: []DomainInfo{}}, nil
	}
	domains, err := s.listDomains(serverID)
	if err != nil {
		return nil, err
	}
	return &ListResponse{Nginx: *nginxSt, Domains: domains}, nil
}

// getDomain mencari satu domain dari daftar ter-cache — dipakai PHP/SSL
// untuk membaca konfigurasi terkini sebelum menulis ulang vhost.
func (s *Service) getDomain(serverID, domain string) (DomainInfo, error) {
	domains, err := s.listDomains(serverID)
	if err != nil {
		return DomainInfo{}, err
	}
	for _, d := range domains {
		if d.Domain == domain {
			return d, nil
		}
	}
	return DomainInfo{}, errFmt("domain %s tidak ditemukan", domain)
}

// DomainRoot mengembalikan document root satu domain — dipakai tab Files
// di menu Website untuk mengunci FilesPanel ke root itu (lihat
// DomainFilesTab.tsx). Baca dari listDomains yang sudah ter-cache, jadi
// biasanya 0 round-trip SSH tambahan.
func (s *Service) DomainRoot(serverID, domain string) (string, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return "", err
	}
	info, err := s.getDomain(serverID, domain)
	if err != nil {
		return "", err
	}
	return info.Root, nil
}

// optionsFromInfo merekonstruksi vhostOptions dari DomainInfo yang sudah
// ter-parsing — dipakai saat PHP/SSL cuma perlu mengubah SATU aspek vhost
// (versi PHP, status SSL) tanpa kehilangan proxy rule / field lain yang
// sudah ada di file vhost saat ini.
func optionsFromInfo(info DomainInfo) *vhostOptions {
	return &vhostOptions{
		Domain:            info.Domain,
		Root:              info.Root,
		IsSubdomain:       info.IsSubdomain,
		Parent:            info.Parent,
		PHPVersion:        info.PHPVersion,
		ProxyTarget:       info.ProxyTarget,
		ProxyWebSocket:    info.ProxyWebSocket,
		ProxyRules:        info.ProxyRules,
		SSLEnabled:        info.SSLEnabled,
		SSLCertificate:    info.SSLCertificate,
		SSLCertificateKey: info.SSLCertificateKey,
	}
}

// rewriteVhost menulis ulang isi vhost yang SUDAH ADA dengan opts baru,
// backup+rollback otomatis kalau `nginx -t` gagal — SATU round-trip SSH
// (tulis + test + reload + hapus backup, atau tulis + test-gagal +
// kembalikan backup), bukan tulis-lalu-test-lalu-reload terpisah.
func (s *Service) rewriteVhost(access *websiteAccess, path string, opts *vhostOptions) error {
	content := buildVhostConfig(opts)
	b64 := base64.StdEncoding.EncodeToString([]byte(content))
	script := fmt.Sprintf(`set -e
TARGET=%s
BACKUP="$TARGET.bak.$$"
cp "$TARGET" "$BACKUP"
trap 'mv "$BACKUP" "$TARGET" 2>/dev/null || true' ERR
echo %s | base64 -d > "$TARGET"
nginx -t
systemctl reload nginx || systemctl restart nginx
rm -f "$BACKUP"
trap - ERR
`, shellQuote(path), shellQuote(b64))
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
			msg = "gagal menulis ulang konfigurasi vhost"
		}
		return mapWebsiteError(errFmt("%s", msg))
	}
	s.invalidateDomainCache(access.serverID)
	return nil
}

// provisionDomain membuat webroot + vhost + reload dalam SATU round-trip
// SSH (bukan ~8 round-trip terpisah seperti homepoin's provisionDomain),
// dengan rollback otomatis (hapus vhost yang baru ditulis) kalau `nginx -t`
// gagal setelahnya.
func (s *Service) provisionDomain(access *websiteAccess, domain, root string, opts *vhostOptions) error {
	enabledPath := configFilePath(domain, true)
	disabledPath := configFilePath(domain, false)
	vhostContent := buildVhostConfig(opts)
	vhostB64 := base64.StdEncoding.EncodeToString([]byte(vhostContent))
	indexB64 := base64.StdEncoding.EncodeToString([]byte(defaultIndexHTML))

	script := fmt.Sprintf(`set -e
ENABLED=%s
DISABLED=%s
if [ -f "$ENABLED" ] || [ -f "$DISABLED" ]; then
  echo "__poinhost_EXISTS__"
  exit 1
fi
%s
echo %s | base64 -d > %s/index.html
%s
trap 'rm -f "$ENABLED"' ERR
echo %s | base64 -d > "$ENABLED"
nginx -t
systemctl reload nginx || systemctl restart nginx
trap - ERR
`, shellQuote(enabledPath), shellQuote(disabledPath),
		prepareWebRootScript(root, access.sshUser),
		shellQuote(indexB64), shellQuote(root),
		prepareWebRootScript(root, access.sshUser),
		shellQuote(vhostB64))

	res, err := s.run(access, script, 30*time.Second)
	if err != nil {
		return err
	}
	if strings.Contains(res.Stdout, "__poinhost_EXISTS__") {
		return errFmt("domain %s sudah ada di server ini", domain)
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = "gagal membuat website"
		}
		return mapWebsiteError(errFmt("%s", msg))
	}
	return nil
}

// Create membuat domain (parent) baru — static-only, tanpa PHP/SSL. Untuk
// alur sekali-jalan dengan PHP/SSL langsung aktif, lihat CreateWebsite.
func (s *Service) Create(req CreateDomainRequest) (DomainInfo, error) {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return DomainInfo{}, err
	}
	nginxSt, err := s.getNginxStatus(req.ServerID)
	if err != nil {
		return DomainInfo{}, err
	}
	if !nginxSt.Installed {
		return DomainInfo{}, errFmt("Nginx belum terpasang di server — install dulu lewat wizard di atas")
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return DomainInfo{}, err
	}

	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)

	root := webRootPath(domain)
	opts := &vhostOptions{Domain: domain, Root: root}
	if err := s.provisionDomain(access, domain, root, opts); err != nil {
		return DomainInfo{}, err
	}
	s.invalidateDomainCache(req.ServerID)
	return DomainInfo{Domain: domain, ServerNames: domain + " www." + domain, Root: root, ConfigPath: configFilePath(domain, true), Enabled: true}, nil
}

// CreateSubdomain membuat subdomain baru di bawah domain parent yang sudah ada.
func (s *Service) CreateSubdomain(req CreateSubdomainRequest) (DomainInfo, error) {
	parent, err := NormalizeDomain(req.Parent)
	if err != nil {
		return DomainInfo{}, err
	}
	label, err := normalizeSubdomainLabel(req.Subdomain)
	if err != nil {
		return DomainInfo{}, err
	}
	domain, err := NormalizeDomain(subdomainFQDN(parent, label))
	if err != nil {
		return DomainInfo{}, err
	}

	if _, err := s.getDomain(req.ServerID, parent); err != nil {
		return DomainInfo{}, errFmt("domain induk %s tidak ditemukan", parent)
	}

	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return DomainInfo{}, err
	}

	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)

	root := subWebRootPath(parent, label)
	opts := &vhostOptions{Domain: domain, Root: root, IsSubdomain: true, Parent: parent}
	if err := s.provisionDomain(access, domain, root, opts); err != nil {
		return DomainInfo{}, err
	}
	s.invalidateDomainCache(req.ServerID)
	return DomainInfo{Domain: domain, Parent: parent, IsSubdomain: true, ServerNames: domain, Root: root, ConfigPath: configFilePath(domain, true), Enabled: true}, nil
}

// Delete menghapus vhost domain (dan opsional document root-nya).
func (s *Service) Delete(req DeleteDomainRequest) error {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return err
	}
	info, err := s.getDomain(req.ServerID, domain)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}

	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)

	enabledPath, disabledPath := configFilePathVariants(info.ConfigPath)
	rmRoot := ""
	if req.RemoveRoot && info.Root != "" {
		rmRoot = "rm -rf " + shellQuote(info.Root) + "\n"
	}
	script := fmt.Sprintf(`set -e
rm -f %s %s
%snginx -t
systemctl reload nginx || systemctl restart nginx
`, shellQuote(enabledPath), shellQuote(disabledPath), rmRoot)

	if _, err := s.run(access, script, 20*time.Second); err != nil {
		return err
	}
	s.invalidateDomainCache(req.ServerID)
	return nil
}

// SetEnabled mengaktifkan/menonaktifkan satu domain (mv .conf <-> .conf.disabled).
func (s *Service) SetEnabled(req SetEnabledRequest) error {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return err
	}
	info, err := s.getDomain(req.ServerID, domain)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}

	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)

	enabledPath, disabledPath := configFilePathVariants(info.ConfigPath)
	from, to := disabledPath, enabledPath
	if !req.Enabled {
		from, to = enabledPath, disabledPath
	}
	script := fmt.Sprintf(`set -e
FROM=%s
TO=%s
if [ ! -f "$FROM" ]; then echo "__poinhost_MISSING__"; exit 1; fi
mv "$FROM" "$TO"
trap 'mv "$TO" "$FROM" 2>/dev/null || true' ERR
nginx -t
systemctl reload nginx || systemctl restart nginx
trap - ERR
`, shellQuote(from), shellQuote(to))

	res, err := s.run(access, script, 20*time.Second)
	if err != nil {
		return err
	}
	if strings.Contains(res.Stdout, "__poinhost_MISSING__") {
		return errFmt("file konfigurasi domain %s tidak ditemukan", domain)
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = "gagal mengubah status domain"
		}
		return mapWebsiteError(errFmt("%s", msg))
	}
	s.invalidateDomainCache(req.ServerID)
	return nil
}

// CreateWebsite adalah alur "Buat Website" gabungan: domain + PHP (opsional)
// + SSL (opsional) dalam SATU submit — perbaikan UX dari homepoin, yang
// memaksa tiga kunjungan halaman terpisah (Domains -> PHP -> SSL) untuk
// hasil akhir yang sama. Kegagalan langkah PHP/SSL (opsional) TIDAK
// membatalkan domain yang sudah berhasil dibuat — dilaporkan sebagai
// warning, bukan rollback, karena situs statis yang sudah jadi tetap
// berguna walau mis. penerbitan SSL gagal (DNS belum diarahkan, dst).
func (s *Service) CreateWebsite(req CreateWebsiteRequest) (*CreateWebsiteResult, error) {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return nil, err
	}
	info, err := s.Create(CreateDomainRequest{ServerID: req.ServerID, Domain: domain})
	if err != nil {
		return nil, err
	}

	var warnings []string

	if req.PHPVersion != "" {
		if err := s.PHPSetDomain(PHPSetDomainRequest{ServerID: req.ServerID, Domain: domain, Version: req.PHPVersion}); err != nil {
			warnings = append(warnings, "PHP "+req.PHPVersion+" gagal diaktifkan: "+err.Error())
		} else {
			info.PHPVersion = req.PHPVersion
			info.PHPEnabled = true
		}
	}

	if req.EnableSSL {
		if _, err := s.SSLIssue(SSLIssueRequest{ServerID: req.ServerID, Domain: domain, Email: req.SSLEmail}); err != nil {
			warnings = append(warnings, "Sertifikat SSL gagal diterbitkan: "+err.Error())
		} else if _, err := s.SSLEnable(req.ServerID, domain); err != nil {
			warnings = append(warnings, "SSL gagal diaktifkan: "+err.Error())
		} else {
			info.SSLEnabled = true
		}
	}

	return &CreateWebsiteResult{Domain: info, Warnings: warnings}, nil
}
