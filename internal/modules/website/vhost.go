package website

import (
	"fmt"
	"strings"
)

// confPrefix awalan nama file vhost yang dikelola poinhost — membedakannya
// dari vhost lain yang mungkin sudah ada di server sebelum poinhost dipakai
// (vhost non-poinhost tidak pernah disentuh/ditampilkan sama sekali).
const confPrefix = "poinhost-"

// legacyConfPrefix awalan file vhost domain buatan homepoin (produk
// sebelumnya, format vhost & marker metadata-nya nyaris identik — poinhost
// memang meniru polanya). Dikenali JUGA supaya domain yang sudah dibuat
// lewat homepoin sebelum pindah ke poinhost tetap kebaca & bisa dikelola,
// tidak "hilang" begitu saja dari daftar. legacyPortConfPrefix dikecualikan
// karena itu file proxy port-forward homepoin (modul lain, bukan vhost
// domain) yang kebetulan berbagi awalan "homepoin-" yang sama.
const legacyConfPrefix = "homepoin-"
const legacyPortConfPrefix = "homepoin-port-"

// isManagedVhostFilename true kalau nama file (tanpa suffix .disabled) cocok
// salah satu prefix vhost domain yang dikenali poinhost (native atau warisan
// homepoin).
func isManagedVhostFilename(base string) bool {
	if !strings.HasSuffix(base, ".conf") {
		return false
	}
	if strings.HasPrefix(base, legacyPortConfPrefix) {
		return false
	}
	return strings.HasPrefix(base, confPrefix) || strings.HasPrefix(base, legacyConfPrefix)
}

// webRootPath path document root standar untuk domain parent.
func webRootPath(domain string) string {
	return "/var/www/" + domain + "/public_html"
}

// subWebRootPath path document root untuk subdomain — sibling dari
// public_html milik parent, dinamai dari FQDN subdomain-nya sendiri supaya
// tidak pernah bentrok dengan public_html atau subdomain lain.
func subWebRootPath(parent, label string) string {
	return "/var/www/" + parent + "/" + label + "." + parent
}

// configFilePath path file vhost Nginx untuk satu domain (enabled/disabled
// dibedakan lewat suffix, bukan direktori terpisah — cukup `mv` untuk
// toggle enable/disable tanpa menulis ulang isi file).
func configFilePath(domain string, enabled bool) string {
	p := "/etc/nginx/conf.d/" + confPrefix + domain + ".conf"
	if !enabled {
		p += ".disabled"
	}
	return p
}

// configFilePathVariants mengembalikan path enabled & disabled dari path
// vhost yang SUDAH ADA di server, dari prefix APAPUN (poinhost- ATAU
// legacyConfPrefix). Dipakai Delete/SetEnabled alih-alih configFilePath
// (yang selalu menduga prefix poinhost-) supaya operasi mv/rm menyasar file
// yang sungguh-sungguh ada, termasuk untuk domain warisan homepoin.
func configFilePathVariants(actualPath string) (enabledPath, disabledPath string) {
	if strings.HasSuffix(actualPath, ".disabled") {
		return strings.TrimSuffix(actualPath, ".disabled"), actualPath
	}
	return actualPath, actualPath + ".disabled"
}

// vhostOptions parameter pembangunan satu vhost — SATU builder options-based
// untuk static/PHP/proxy sekaligus (beda dari homepoin yang punya dua
// tingkat: BuildVhostConfig "legacy" static/PHP-only lalu
// BuildVhostConfigWithOptions terpisah untuk varian proxy — di sini semua
// jalur, termasuk pembuatan domain baru yang paling sederhana, lewat SATU
// fungsi yang sama, supaya tidak ada dua tempat berbeda yang bisa mendrift).
type vhostOptions struct {
	Domain        string
	Root          string
	IsSubdomain   bool
	Parent        string
	PHPVersion    string
	FastCGISocket string

	ProxyTarget    string
	ProxyWebSocket bool
	ProxyRules     []ProxyRule

	SSLEnabled        bool
	SSLCertificate    string
	SSLCertificateKey string
}

// buildMetaComment membentuk baris komentar metadata yang disisipkan ke
// vhost, dipakai parseVhostFile untuk merekonstruksi DomainInfo tanpa
// database terpisah sama sekali — satu-satunya "sumber kebenaran" adalah
// file vhost itu sendiri di server.
func buildMetaComment(o *vhostOptions) string {
	kind := "domain"
	if o.IsSubdomain {
		kind = "subdomain"
	}
	line := fmt.Sprintf("# poinhost-managed %s: %s", kind, o.Domain)
	if o.IsSubdomain && o.Parent != "" {
		line += " parent=" + o.Parent
	}
	if o.PHPVersion != "" {
		line += " php=" + o.PHPVersion
	}
	if o.ProxyTarget != "" {
		line += " proxy=" + o.ProxyTarget
		if o.ProxyWebSocket {
			line += " proxy_ws=1"
		}
	}
	if o.SSLEnabled {
		line += " ssl=on"
		if o.SSLCertificate != "" {
			line += " ssl_cert=" + o.SSLCertificate
		}
		if o.SSLCertificateKey != "" {
			line += " ssl_key=" + o.SSLCertificateKey
		}
	}
	var b strings.Builder
	b.WriteString(line)
	b.WriteByte('\n')
	for _, r := range o.ProxyRules {
		b.WriteString(fmt.Sprintf("# poinhost-proxy path=%s target=%s", r.Path, r.Target))
		if r.WebSocket {
			b.WriteString(" ws=1")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func buildProxyLocationBlock(r ProxyRule) string {
	var b strings.Builder
	fmt.Fprintf(&b, "    location %s {\n", r.Path)
	fmt.Fprintf(&b, "        proxy_pass %s;\n", r.Target)
	b.WriteString("        proxy_http_version 1.1;\n")
	b.WriteString("        proxy_set_header Host $host;\n")
	b.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
	b.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	b.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
	if r.WebSocket {
		b.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
		b.WriteString("        proxy_set_header Connection \"upgrade\";\n")
	}
	b.WriteString("    }\n")
	return b.String()
}

// buildLocationBlocks menyusun blok location sesuai prioritas: proxy rule
// per-path dulu (path lebih panjang lebih dulu, supaya mengalahkan yang
// lebih umum), lalu proxy seluruh domain, lalu PHP-FPM, terakhir static.
func buildLocationBlocks(o *vhostOptions) string {
	var b strings.Builder
	rules := append([]ProxyRule{}, o.ProxyRules...)
	for i := 0; i < len(rules); i++ {
		for j := i + 1; j < len(rules); j++ {
			if len(rules[j].Path) > len(rules[i].Path) {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}
	for _, r := range rules {
		b.WriteString(buildProxyLocationBlock(r))
	}

	if o.ProxyTarget != "" {
		b.WriteString(buildProxyLocationBlock(ProxyRule{Path: "/", Target: o.ProxyTarget, WebSocket: o.ProxyWebSocket}))
		return b.String()
	}

	if o.PHPVersion != "" {
		socket := o.FastCGISocket
		if socket == "" {
			socket = "/run/php/php" + o.PHPVersion + "-fpm.sock"
		}
		b.WriteString("    location / {\n        try_files $uri $uri/ /index.php?$query_string;\n    }\n")
		b.WriteString("    location ~ \\.php$ {\n")
		b.WriteString("        include snippets/fastcgi-php.conf;\n")
		fmt.Fprintf(&b, "        fastcgi_pass unix:%s;\n", socket)
		b.WriteString("    }\n")
		return b.String()
	}

	b.WriteString("    location / {\n        try_files $uri $uri/ =404;\n    }\n")
	return b.String()
}

// buildVhostConfig membentuk isi lengkap file vhost (HTTP saja, atau
// HTTP-redirect + HTTPS kalau SSLEnabled dengan cert+key lengkap).
func buildVhostConfig(o *vhostOptions) string {
	serverNames := o.Domain
	if !o.IsSubdomain {
		serverNames = o.Domain + " www." + o.Domain
	}
	indexLine := "index index.html index.htm;"
	if o.PHPVersion != "" && o.ProxyTarget == "" {
		indexLine = "index index.php index.html index.htm;"
	}
	locations := buildLocationBlocks(o)
	meta := buildMetaComment(o)

	if o.SSLEnabled && o.SSLCertificate != "" && o.SSLCertificateKey != "" {
		var b strings.Builder
		b.WriteString(meta)
		fmt.Fprintf(&b, `server {
    listen 80;
    listen [::]:80;
    server_name %s;

    root %s;

    access_log /var/log/nginx/%s.access.log;
    error_log /var/log/nginx/%s.error.log;

    location ^~ /.well-known/acme-challenge/ {
        default_type "text/plain";
        root %s;
    }
    location / {
        return 301 https://$host$request_uri;
    }
}
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name %s;

    ssl_certificate %s;
    ssl_certificate_key %s;

    root %s;
    %s

    access_log /var/log/nginx/%s.access.log;
    error_log /var/log/nginx/%s.error.log;

%s}
`, serverNames, o.Root, o.Domain, o.Domain, o.Root,
			serverNames, o.SSLCertificate, o.SSLCertificateKey, o.Root, indexLine,
			o.Domain, o.Domain, locations)
		return b.String()
	}

	var b strings.Builder
	b.WriteString(meta)
	fmt.Fprintf(&b, `server {
    listen 80;
    listen [::]:80;
    server_name %s;

    root %s;
    %s

    access_log /var/log/nginx/%s.access.log;
    error_log /var/log/nginx/%s.error.log;

%s}
`, serverNames, o.Root, indexLine, o.Domain, o.Domain, locations)
	return b.String()
}

// parseVhostFile mem-parsing isi satu file vhost jadi DomainInfo. Sengaja
// line-scan sederhana (bukan parser Nginx penuh) — cukup untuk vhost yang
// SELALU ditulis lewat buildVhostConfig di atas, konsisten dengan pendekatan
// homepoin (yang juga tidak pernah butuh parser Nginx umum, karena tidak
// pernah membaca vhost buatan orang lain).
func parseVhostFile(path, content string, enabled bool) (DomainInfo, bool) {
	base := path[strings.LastIndex(path, "/")+1:]
	base = strings.TrimSuffix(base, ".disabled")
	if !isManagedVhostFilename(base) {
		return DomainInfo{}, false
	}

	info := DomainInfo{ConfigPath: path, Enabled: enabled}
	var proxyRules []ProxyRule

	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		// Marker metadata homepoin ("# homepoin-managed ...", "# homepoin-proxy
		// ...") berformat & berarti IDENTIK dengan marker poinhost sendiri —
		// dinormalkan ke marker poinhost supaya lewat satu jalur parsing yang
		// sama di bawah, bukan digandakan case-by-case.
		if strings.HasPrefix(line, "# homepoin-managed ") || strings.HasPrefix(line, "# homepoin-proxy ") {
			line = "# poinhost-" + strings.TrimPrefix(line, "# homepoin-")
		}
		switch {
		case strings.HasPrefix(line, "# poinhost-managed domain:"):
			info.Domain = strings.TrimSpace(strings.TrimPrefix(line, "# poinhost-managed domain:"))
			info.Domain, info.PHPVersion, info.ProxyTarget, info.ProxyWebSocket, info.SSLEnabled, info.SSLCertificate, info.SSLCertificateKey =
				splitMetaFields(info.Domain)
		case strings.HasPrefix(line, "# poinhost-managed subdomain:"):
			info.IsSubdomain = true
			rest := strings.TrimSpace(strings.TrimPrefix(line, "# poinhost-managed subdomain:"))
			info.Domain, info.PHPVersion, info.ProxyTarget, info.ProxyWebSocket, info.SSLEnabled, info.SSLCertificate, info.SSLCertificateKey, info.Parent = splitMetaFieldsSub(rest)
		case strings.HasPrefix(line, "# poinhost-proxy "):
			rule := ProxyRule{}
			for _, tok := range strings.Fields(strings.TrimPrefix(line, "# poinhost-proxy ")) {
				if v, ok := strings.CutPrefix(tok, "path="); ok {
					rule.Path = v
				} else if v, ok := strings.CutPrefix(tok, "target="); ok {
					rule.Target = v
				} else if tok == "ws=1" {
					rule.WebSocket = true
				}
			}
			if rule.Path != "" {
				proxyRules = append(proxyRules, rule)
			}
		case strings.HasPrefix(line, "root "):
			info.Root = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(line, "root ")), ";")
		case strings.HasPrefix(line, "server_name "):
			info.ServerNames = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(line, "server_name ")), ";")
		}
	}
	info.ProxyRules = proxyRules
	info.PHPEnabled = info.PHPVersion != "" && info.ProxyTarget == ""
	if info.Domain == "" {
		return DomainInfo{}, false
	}
	return info, true
}

// splitMetaFields memisahkan "example.com php=8.3 ssl=on ssl_cert=... ssl_key=..."
// jadi domain + field-field metadata.
func splitMetaFields(raw string) (domain, phpVersion, proxyTarget string, proxyWebSocket bool, sslEnabled bool, sslCert, sslKey string) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return "", "", "", false, false, "", ""
	}
	domain = fields[0]
	for _, f := range fields[1:] {
		switch {
		case strings.HasPrefix(f, "php="):
			phpVersion = strings.TrimPrefix(f, "php=")
		case strings.HasPrefix(f, "proxy="):
			proxyTarget = strings.TrimPrefix(f, "proxy=")
		case f == "proxy_ws=1":
			proxyWebSocket = true
		case f == "ssl=on":
			sslEnabled = true
		case strings.HasPrefix(f, "ssl_cert="):
			sslCert = strings.TrimPrefix(f, "ssl_cert=")
		case strings.HasPrefix(f, "ssl_key="):
			sslKey = strings.TrimPrefix(f, "ssl_key=")
		}
	}
	return domain, phpVersion, proxyTarget, proxyWebSocket, sslEnabled, sslCert, sslKey
}

// splitMetaFieldsSub sama seperti splitMetaFields, plus field "parent=" yang
// cuma ada di baris metadata subdomain.
func splitMetaFieldsSub(raw string) (domain, phpVersion, proxyTarget string, proxyWebSocket, sslEnabled bool, sslCert, sslKey, parent string) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return "", "", "", false, false, "", "", ""
	}
	domain = fields[0]
	for _, f := range fields[1:] {
		switch {
		case strings.HasPrefix(f, "parent="):
			parent = strings.TrimPrefix(f, "parent=")
		case strings.HasPrefix(f, "php="):
			phpVersion = strings.TrimPrefix(f, "php=")
		case strings.HasPrefix(f, "proxy="):
			proxyTarget = strings.TrimPrefix(f, "proxy=")
		case f == "proxy_ws=1":
			proxyWebSocket = true
		case f == "ssl=on":
			sslEnabled = true
		case strings.HasPrefix(f, "ssl_cert="):
			sslCert = strings.TrimPrefix(f, "ssl_cert=")
		case strings.HasPrefix(f, "ssl_key="):
			sslKey = strings.TrimPrefix(f, "ssl_key=")
		}
	}
	return domain, phpVersion, proxyTarget, proxyWebSocket, sslEnabled, sslCert, sslKey, parent
}

// prepareWebRootScript menyiapkan document root: buat direktori, atur
// pemilik & permission supaya user SSH (lewat Files/SFTP) DAN PHP-FPM
// (lewat grup web) sama-sama bisa baca/tulis, tanpa salah satu terkunci.
func prepareWebRootScript(root, sshUser string) string {
	return fmt.Sprintf(`set -e
ROOT=%s
mkdir -p "$ROOT"
WEBUSER=www-data
if id -u www-data >/dev/null 2>&1; then WEBUSER=www-data
elif id -u nginx >/dev/null 2>&1; then WEBUSER=nginx
elif id -u apache >/dev/null 2>&1; then WEBUSER=apache
fi
WEBGROUP=$(id -gn "$WEBUSER" 2>/dev/null || echo "$WEBUSER")
OWNER=%s
if ! id -u "$OWNER" >/dev/null 2>&1; then OWNER=$WEBUSER; fi
chown -R "$OWNER":"$WEBGROUP" "$ROOT"
usermod -aG "$WEBGROUP" "$OWNER" 2>/dev/null || true
find "$ROOT" -type d -exec chmod 2775 {} \;
find "$ROOT" -type f -exec chmod 664 {} \;
chmod 2775 "$ROOT"
`, shellQuote(root), shellQuote(sshUser))
}

const defaultIndexHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>Website baru</title></head>
<body style="font-family:sans-serif;text-align:center;padding-top:10%">
<h1>Website ini sudah aktif</h1>
<p>Unggah file ke document root untuk mulai.</p>
</body></html>
`
