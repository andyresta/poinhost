package website

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
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
// Sertifikat sekarang diterbitkan PER DOMAIN, bukan satu sertifikat gabungan
// untuk induk beserta seluruh subdomainnya.
//
// Cara gabungan sebelumnya punya kelemahan yang mahal: menambah SATU subdomain
// berarti meminta ulang SELURUH anggota keluarga dalam satu permintaan, dan
// kalau ada satu saja yang gagal divalidasi, seluruh permintaan gagal. Jadi
// subdomain yang DNS-nya sudah benar ikut tertahan oleh subdomain lain yang
// belum siap. Subdomain juga baru tercakup setelah seseorang menerbitkan ulang
// dari induk, sehingga bisa lama tidak ber-HTTPS tanpa disadari.
//
// Dengan per domain, kegagalan satu domain tidak menular. Kuota Let's Encrypt
// bukan hambatan: batasnya 50 sertifikat per domain terdaftar per minggu.
//
// `www` TETAP digabung dengan induknya karena keduanya satu situs yang sama —
// satu vhost melayani keduanya lewat server_name, jadi memisahkannya justru
// akan menghasilkan sertifikat yang tidak pernah dipakai.
func (s *Service) sanTargets(serverID, domain string) ([]sanTarget, error) {
	domains, err := s.listDomains(serverID)
	if err != nil {
		return nil, err
	}
	for _, d := range domains {
		if d.Domain != domain {
			continue
		}
		if d.IsSubdomain {
			return []sanTarget{{FQDN: d.Domain, Root: d.Root}}, nil
		}
		return []sanTarget{
			{FQDN: domain, Root: d.Root},
			{FQDN: "www." + domain, Root: d.Root},
		}, nil
	}
	return nil, errFmt("domain %s tidak ditemukan", domain)
}

// vhostFamily mengembalikan semua entri vhost yang perlu ditulis ulang saat
// SSL diaktifkan/dinonaktifkan untuk satu domain parent — vhost parent itu
// sendiri (server_name-nya SUDAH mencakup www, jadi www TIDAK punya vhost
// terpisah) plus vhost tiap subdomain-nya.
// vhostFamily mengembalikan vhost yang perlu ditulis ulang saat SSL
// diaktifkan/dinonaktifkan untuk satu domain.
//
// Sekarang HANYA vhost domain itu sendiri, sejalan dengan sertifikat yang
// diterbitkan per domain. Dulu mengaktifkan SSL di induk ikut menulis ulang
// vhost SELURUH subdomainnya — itu membuat satu tindakan diam-diam mengubah
// situs lain, dan subdomain yang sertifikatnya belum ada pun ikut diarahkan ke
// sertifikat induk. `www` tidak punya vhost terpisah (sudah tercakup
// server_name induk), jadi tidak perlu diikutkan.
func (s *Service) vhostFamily(serverID, domain string) ([]DomainInfo, error) {
	domains, err := s.listDomains(serverID)
	if err != nil {
		return nil, err
	}
	for _, d := range domains {
		if d.Domain == domain {
			return []DomainInfo{d}, nil
		}
	}
	return nil, errFmt("domain %s tidak ditemukan", domain)
}

const detectCertbotScript = `if command -v certbot >/dev/null 2>&1; then
  echo "CERTBOT=1"
  certbot --version 2>&1 | head -n1
else
  echo "CERTBOT=0"
fi`

// certExistsScript mencari sertifikat untuk satu domain.
//
// Dicari di lineage domain itu sendiri LEBIH DULU, lalu — kalau ini subdomain —
// jatuh ke lineage induknya. Cadangan itu penting demi setup lama: sebelum
// sertifikat dipecah per domain, subdomain ikut tercakup lewat SAN sertifikat
// induk dan tidak punya lineage sendiri. Tanpa cadangan ini, subdomain yang
// HTTPS-nya sebenarnya berjalan akan dilaporkan "belum ada sertifikat".
//
// parent kosong berarti domain ini bukan subdomain.
func certExistsScript(domain, parent string) string {
	fallback := ""
	if parent != "" {
		fallback = `
if [ ! -f "$CERT" ] && [ -f /etc/letsencrypt/live/` + parent + `/fullchain.pem ]; then
  # Hanya dipakai kalau SAN sertifikat induk memang memuat domain ini.
  if openssl x509 -in /etc/letsencrypt/live/` + parent + `/fullchain.pem -noout -ext subjectAltName 2>/dev/null | grep -q "DNS:` + domain + `"; then
    CERT=/etc/letsencrypt/live/` + parent + `/fullchain.pem
    KEY=/etc/letsencrypt/live/` + parent + `/privkey.pem
    echo "CERT_SHARED=1"
  fi
fi`
	}
	return `CERT=/etc/letsencrypt/live/` + domain + `/fullchain.pem
KEY=/etc/letsencrypt/live/` + domain + `/privkey.pem` + fallback + `
if [ -f "$CERT" ] && [ -f "$KEY" ]; then
  echo "CERT_EXISTS=1"
  echo "CERT_PATH=$CERT"
  echo "KEY_PATH=$KEY"
  openssl x509 -enddate -noout -in "$CERT" 2>/dev/null | sed 's/notAfter=/NOT_AFTER=/'
  # Domain yang BENAR-BENAR tercakup dibaca dari SAN sertifikatnya, bukan
  # disimpulkan dari daftar subdomain yang terdaftar di aplikasi. Keduanya
  # sering berbeda: subdomain yang dibuat SESUDAH sertifikat terbit belum
  # ikut tercakup sampai sertifikatnya diterbitkan ulang.
  openssl x509 -in "$CERT" -noout -ext subjectAltName 2>/dev/null \
    | tr ',' '\n' | sed -n 's/.*DNS:\([^ ,]*\).*/CERT_SAN=\1/p'
else
  echo "CERT_EXISTS=0"
fi`
}

// certbotDeployHookPath adalah hook yang dipanggil certbot OTOMATIS setiap
// kali sertifikat apa pun di server ini benar-benar diperbarui — dari
// SUMBER manapun (timer/cron bawaan paket certbot, cron jaring pengaman
// poinhost sendiri di bawah, atau tombol Renew manual) — bukan sesuatu
// yang poinhost panggil sendiri. Ini mengisi celah nyata di homepoin:
// sertifikat BISA saja sudah diperbarui otomatis oleh OS, tapi file
// sertifikat baru di disk tidak pernah membuat Nginx reload — worker
// process Nginx tetap memakai bytes sertifikat LAMA di memori sampai ada
// reload eksplisit, jadi auto-renewal tanpa hook ini efektif tidak
// berguna kalau tidak ada yang reload Nginx sesudahnya.
const certbotDeployHookPath = "/etc/letsencrypt/renewal-hooks/deploy/poinhost-reload-nginx.sh"

// certbotRenewCronPath adalah jaring pengaman: paket certbot dari apt/dnf
// BIASANYA sudah memasang systemd timer/cron sendiri (mis. `certbot.timer`
// di Debian/Ubuntu), tapi ini tidak dijamin ada/aktif di semua distro atau
// image minimal — cron eksplisit ini memastikan pengecekan renewal tetap
// jalan tanpa bergantung pada perilaku default paket OS. `certbot renew`
// sendiri cuma benar-benar memperbarui sertifikat yang mendekati
// kedaluwarsa (<30 hari), jadi aman dijalankan setiap hari tanpa efek
// samping untuk sertifikat yang belum perlu diperbarui.
const certbotRenewCronPath = "/etc/cron.d/poinhost-certbot-renew"

const certbotDeployHookScript = `#!/bin/sh
# Dikelola poinhost — reload Nginx setiap kali certbot berhasil memperbarui
# sertifikat apa pun di server ini (dipanggil OTOMATIS oleh certbot sendiri,
# baik dari timer/cron bawaan OS maupun tombol Renew manual di poinhost).
nginx -t && (systemctl reload nginx 2>/dev/null || systemctl restart nginx 2>/dev/null || service nginx reload 2>/dev/null || service nginx restart 2>/dev/null)
`

const certbotRenewCronContent = `# Dikelola poinhost — jangan edit manual.
# Jaring pengaman auto-renew Let's Encrypt: certbot cuma benar-benar
# memperbarui sertifikat yang mendekati kedaluwarsa (<30 hari), jadi aman
# dijalankan tiap hari. Reload Nginx ditangani otomatis lewat deploy-hook
# di /etc/letsencrypt/renewal-hooks/deploy/, BUKAN di baris ini.
12 3 * * * root certbot renew --quiet
`

const autoRenewCheckScript = `if [ -f ` + certbotRenewCronPath + ` ]; then echo "AUTORENEW=1"; else echo "AUTORENEW=0"; fi`

// ensureAutoRenew memasang deploy-hook + cron jaring pengaman dalam SATU
// round-trip SSH — idempotent (aman dipanggil berkali-kali, isinya selalu
// ditimpa dengan konten yang sama).
func (s *Service) ensureAutoRenew(access *websiteAccess) error {
	hookB64 := base64.StdEncoding.EncodeToString([]byte(certbotDeployHookScript))
	cronB64 := base64.StdEncoding.EncodeToString([]byte(certbotRenewCronContent))
	script := `set -e
mkdir -p /etc/letsencrypt/renewal-hooks/deploy
echo ` + shellQuote(hookB64) + ` | base64 -d > ` + certbotDeployHookPath + `
chmod 0755 ` + certbotDeployHookPath + `
echo ` + shellQuote(cronB64) + ` | base64 -d > ` + certbotRenewCronPath + `
chmod 0644 ` + certbotRenewCronPath + `
`
	_, err := s.run(access, script, 15*time.Second)
	return err
}

// EnableSSLAutoRenew memasang ulang mekanisme auto-renew secara eksplisit —
// dipakai tombol manual di UI untuk sertifikat yang sudah diterbitkan
// SEBELUM fitur ini ada, atau kalau pemasangan otomatis saat SSLIssue
// gagal karena sebab lain (auto-renew berlaku untuk SELURUH server, bukan
// per-domain, tapi menerima `domain` supaya bisa langsung mengembalikan
// SSLStatus domain yang sedang dibuka di UI).
func (s *Service) EnableSSLAutoRenew(serverID, domain string) (*SSLStatus, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureAutoRenew(access); err != nil {
		return nil, err
	}
	return s.SSLStatus(serverID, domain)
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
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	res, err := s.run(access, detectCertbotScript+"\n"+certExistsScript(domain, info.Parent)+"\n"+autoRenewCheckScript, 15*time.Second)
	if err != nil {
		return nil, err
	}

	st := &SSLStatus{
		Domain:         domain,
		IsParent:       !info.IsSubdomain,
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
		case line == "AUTORENEW=1":
			st.AutoRenewEnabled = true
		case strings.HasPrefix(line, "CERT_SAN="):
			// Domain yang BENAR-BENAR ada di sertifikat.
			st.Certificate.Domains = append(st.Certificate.Domains, strings.TrimPrefix(line, "CERT_SAN="))
		}
	}

	// SANs = RENCANA (apa yang akan diminta kalau diterbitkan ulang).
	// Certificate.Domains = FAKTA (isi sertifikat sekarang).
	//
	// Dulu keduanya diisi dari sumber yang sama, sehingga panel melaporkan
	// rencana sebagai fakta: subdomain yang dibuat SESUDAH sertifikat terbit
	// ditampilkan "tercakup" padahal HTTPS-nya tidak pernah bekerja.
	if sans, err := s.sanTargets(serverID, domain); err == nil {
		have := map[string]bool{}
		for _, d := range st.Certificate.Domains {
			have[strings.ToLower(strings.TrimPrefix(d, "*."))] = true
		}
		for _, t := range sans {
			st.SANs = append(st.SANs, t.FQDN)
			if st.Certificate.Exists && !have[strings.ToLower(t.FQDN)] {
				st.MissingSANs = append(st.MissingSANs, t.FQDN)
			}
		}
	}

	st.CanIssue = st.CertbotInstalled
	st.CanEnable = st.Certificate.Exists && !st.SSLEnabled
	return st, nil
}

// buildIssueCommand menyusun perintah `certbot certonly --webroot` yang
// mencakup seluruh keluarga SAN dalam SATU permintaan sertifikat.
func buildIssueCommand(email, certName string, targets []sanTarget, force bool) string {
	// --expand WAJIB ada: tanpa itu, certbot yang menemukan lineage dengan
	// nama sama tapi daftar domain berbeda akan menolak (atau bertanya, yang
	// dalam mode non-interactive berarti gagal) alih-alih menambahkan domain
	// baru. Inilah yang membuat subdomain yang dibuat belakangan tidak pernah
	// ikut tercakup.
	parts := []string{"certbot", "certonly", "--non-interactive", "--agree-tos", "--expand",
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
	if _, err := s.getDomain(req.ServerID, domain); err != nil {
		return nil, err
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

	// Vhost ditulis ULANG dulu sebelum certbot jalan.
	//
	// Alasannya: vhost yang dibuat versi lama poinhost belum punya blok
	// `/.well-known/acme-challenge/`. Untuk domain reverse-proxy, permintaan
	// validasi ikut diteruskan ke aplikasi di belakangnya dan dijawab 404,
	// sehingga sertifikat pertamanya tidak pernah bisa terbit. Menulis ulang
	// di sini membuat domain lama ikut terperbaiki tanpa perlu dibuat ulang.
	//
	// Isinya tetap sama persis selain penambahan blok itu — options-nya dibaca
	// dari vhost yang ada sekarang, bukan disusun dari asumsi.
	if family, ferr := s.vhostFamily(req.ServerID, domain); ferr == nil {
		for _, member := range family {
			opts := optionsFromInfo(member)
			if rerr := s.rewriteVhost(access, member.ConfigPath, opts); rerr != nil {
				return nil, errFmt("gagal menyiapkan vhost %s untuk validasi: %w", member.Domain, rerr)
			}
		}
	}

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
		// certbot mengubur alasan sebenarnya di tengah keluarannya. Kalau
		// blok laporannya ketemu, itu saja yang ditampilkan — user butuh
		// tahu domain mana yang gagal dan kenapa, bukan lokasi debug log.
		if res != nil {
			if detail := certbotProblem(res.Stdout + "\n" + res.Stderr); detail != "" {
				return nil, mapWebsiteError(errFmt("penerbitan sertifikat gagal — %s", detail))
			}
		}
		return nil, err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(sshpool.FailureDetail(res))
		if detail := certbotProblem(msg); detail != "" {
			msg = detail
		}
		if msg == "" {
			msg = "gagal menerbitkan sertifikat SSL"
		}
		return nil, mapWebsiteError(errFmt("%s", msg))
	}

	// Best-effort — kegagalan di sini TIDAK membatalkan penerbitan sertifikat
	// yang sudah berhasil di atas; kalau gagal, AutoRenewEnabled di status
	// hasil cuma akan terbaca false, dan user masih bisa memasangnya manual
	// lewat EnableSSLAutoRenew.
	_ = s.ensureAutoRenew(access)

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
	// Best-effort, sama seperti di SSLIssue — mencakup kasus sertifikat yang
	// sudah ada sebelum SSL diaktifkan lewat poinhost (mis. diterbitkan
	// manual via SSH sebelumnya).
	_ = s.ensureAutoRenew(access)
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

	// `certbot renew` HANYA memperpanjang masa berlaku dengan daftar domain
	// yang sudah ada, dan itu pun dilewati kalau sertifikatnya belum mendekati
	// kedaluwarsa. Ia TIDAK PERNAH menambah domain baru.
	//
	// Jadi kalau ada subdomain yang belum tercakup, menjalankan renew akan
	// tampak "tidak berefek apa pun" — persis keluhan yang bikin fitur ini
	// terkesan rusak. Kondisi itu dideteksi lebih dulu dan dijelaskan, bukan
	// dibiarkan jadi operasi diam yang membingungkan.
	before, err := s.SSLStatus(serverID, domain)
	if err == nil && len(before.MissingSANs) > 0 {
		// Pesannya sengaja pendek: penjelasan panjang sudah tidak perlu karena
		// panel menyediakan tombol "Terbitkan ulang" tepat di sebelah daftar
		// domain yang belum tercakup.
		return before, errFmt("perpanjangan tidak menambah domain — pakai Terbitkan ulang untuk %s",
			strings.Join(before.MissingSANs, ", "))
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

// certbotProblem menarik blok laporan kegagalan certbot dari keluaran
// mentahnya.
//
// Saat validasi gagal, certbot mencetak alasan sebenarnya sebagai beberapa
// baris berlabel "Domain:", "Type:" dan "Detail:" — sisanya (lokasi debug
// log, ajakan bertanya di forum, saran menjalankan ulang dengan -v) tidak
// membantu user memperbaiki apa pun, tapi justru bagian itulah yang selama
// ini muncul di panel karena ia yang ditulis ke stderr.
//
// Mengembalikan string kosong kalau blok itu tidak ada, supaya pemanggil
// bisa jatuh ke keluaran apa adanya daripada menampilkan pesan kosong.
func certbotProblem(raw string) string {
	var picked []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Domain:"),
			strings.HasPrefix(line, "Type:"),
			strings.HasPrefix(line, "Detail:"):
			picked = append(picked, line)
		}
	}
	return strings.Join(picked, " · ")
}
