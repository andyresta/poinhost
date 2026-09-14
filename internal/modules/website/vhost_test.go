package website

import "testing"

// Domain lama buatan homepoin, marker & isi meniru persis apa yang ditulis
// homepoin (lihat internal/modules/servers/hosting/domains/vhost_proxy.go
// di repo homepoin) — bukan format poinhost sendiri.
const legacyHomepoinVhost = `# homepoin-managed domain: legacy.example.com php=8.3
server {
    listen 80;
    server_name legacy.example.com www.legacy.example.com;

    root /var/www/legacy.example.com/public_html;
    index index.php index.html index.htm;

    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }

    location ~ \.php$ {
        fastcgi_pass unix:/run/php/php8.3-fpm.sock;
    }
}
`

func TestParseVhostFileRecognizesLegacyHomepoinDomain(t *testing.T) {
	info, ok := parseVhostFile("/etc/nginx/conf.d/homepoin-legacy.example.com.conf", legacyHomepoinVhost, true)
	if !ok {
		t.Fatalf("expected legacy homepoin vhost to parse")
	}
	if info.Domain != "legacy.example.com" {
		t.Fatalf("domain = %q, want legacy.example.com", info.Domain)
	}
	if info.PHPVersion != "8.3" || !info.PHPEnabled {
		t.Fatalf("php version/enabled = %q/%v, want 8.3/true", info.PHPVersion, info.PHPEnabled)
	}
	if info.Root != "/var/www/legacy.example.com/public_html" {
		t.Fatalf("root = %q", info.Root)
	}
	if info.ConfigPath != "/etc/nginx/conf.d/homepoin-legacy.example.com.conf" {
		t.Fatalf("configPath = %q", info.ConfigPath)
	}
}

func TestParseVhostFileRecognizesLegacyHomepoinSubdomainAndProxy(t *testing.T) {
	content := `# homepoin-managed subdomain: dev.example.com parent=example.com proxy=http://127.0.0.1:3000 ssl=on ssl_cert=/etc/letsencrypt/live/dev.example.com/fullchain.pem ssl_key=/etc/letsencrypt/live/dev.example.com/privkey.pem
# homepoin-proxy path=/api target=http://127.0.0.1:8080 ws=1
server {
    root /var/www/example.com/dev.example.com;
    server_name dev.example.com;
}
`
	info, ok := parseVhostFile("/etc/nginx/conf.d/homepoin-dev.example.com.conf", content, true)
	if !ok {
		t.Fatalf("expected legacy homepoin subdomain vhost to parse")
	}
	if !info.IsSubdomain || info.Parent != "example.com" {
		t.Fatalf("isSubdomain/parent = %v/%q, want true/example.com", info.IsSubdomain, info.Parent)
	}
	if info.ProxyTarget != "http://127.0.0.1:3000" || !info.SSLEnabled {
		t.Fatalf("proxyTarget/sslEnabled = %q/%v", info.ProxyTarget, info.SSLEnabled)
	}
	if len(info.ProxyRules) != 1 || info.ProxyRules[0].Path != "/api" || info.ProxyRules[0].Target != "http://127.0.0.1:8080" || !info.ProxyRules[0].WebSocket {
		t.Fatalf("proxyRules = %+v", info.ProxyRules)
	}
}

func TestParseVhostFileIgnoresHomepoinPortForwardFiles(t *testing.T) {
	if _, ok := parseVhostFile("/etc/nginx/conf.d/homepoin-port-4500.conf", "server {}\n", true); ok {
		t.Fatalf("expected homepoin-port-* file to be skipped, it is not a domain vhost")
	}
}

func TestParseVhostFileStillRecognizesNativePoinhostDomain(t *testing.T) {
	opts := &vhostOptions{Domain: "native.example.com", Root: "/var/www/native.example.com/public_html", PHPVersion: "8.3"}
	content := buildVhostConfig(opts)
	info, ok := parseVhostFile("/etc/nginx/conf.d/poinhost-native.example.com.conf", content, true)
	if !ok {
		t.Fatalf("expected native poinhost vhost to still parse")
	}
	if info.Domain != "native.example.com" || info.PHPVersion != "8.3" {
		t.Fatalf("info = %+v", info)
	}
}

func TestConfigFilePathVariants(t *testing.T) {
	enabled, disabled := configFilePathVariants("/etc/nginx/conf.d/homepoin-legacy.example.com.conf")
	if enabled != "/etc/nginx/conf.d/homepoin-legacy.example.com.conf" ||
		disabled != "/etc/nginx/conf.d/homepoin-legacy.example.com.conf.disabled" {
		t.Fatalf("enabled/disabled = %q/%q", enabled, disabled)
	}

	enabled, disabled = configFilePathVariants("/etc/nginx/conf.d/homepoin-legacy.example.com.conf.disabled")
	if enabled != "/etc/nginx/conf.d/homepoin-legacy.example.com.conf" ||
		disabled != "/etc/nginx/conf.d/homepoin-legacy.example.com.conf.disabled" {
		t.Fatalf("enabled/disabled (from disabled input) = %q/%q", enabled, disabled)
	}
}

func TestInferMissingParentsFillsLegacyHomepoinSubdomainParent(t *testing.T) {
	// Meniru apa yang benar-benar dihasilkan parseVhostFile untuk subdomain
	// lama buatan homepoin: IsSubdomain sudah true (dari marker "# homepoin-
	// managed subdomain: ..."), tapi Parent kosong karena marker homepoin
	// tidak pernah menyertakan field "parent=" (beda dari marker poinhost).
	domains := []DomainInfo{
		{Domain: "example.com", IsSubdomain: false},
		{Domain: "dev.example.com", IsSubdomain: true, Parent: ""},
		{Domain: "other.example.org", IsSubdomain: false},
	}
	inferMissingParents(domains)

	if domains[1].Parent != "example.com" {
		t.Fatalf("dev.example.com parent = %q, want example.com", domains[1].Parent)
	}
	if domains[0].Parent != "" || domains[0].IsSubdomain {
		t.Fatalf("top-level domain must stay untouched, got %+v", domains[0])
	}
}

func TestInferMissingParentsDoesNotOverrideExplicitParent(t *testing.T) {
	domains := []DomainInfo{
		{Domain: "example.com", IsSubdomain: false},
		{Domain: "app.example.com", IsSubdomain: true, Parent: "example.com"},
	}
	inferMissingParents(domains)
	if domains[1].Parent != "example.com" {
		t.Fatalf("explicit parent must be preserved, got %q", domains[1].Parent)
	}
}

func TestInferMissingParentsSkipsTopLevelDomains(t *testing.T) {
	// "shop.example.com" ditulis sebagai domain top-level SENDIRI (bukan
	// subdomain "example.com") — tidak boleh disatukan hanya karena
	// namanya kebetulan berakhiran nama domain lain di server yang sama.
	domains := []DomainInfo{
		{Domain: "example.com", IsSubdomain: false},
		{Domain: "shop.example.com", IsSubdomain: false, Parent: ""},
	}
	inferMissingParents(domains)
	if domains[1].Parent != "" || domains[1].IsSubdomain {
		t.Fatalf("top-level domain must not be reclassified, got %+v", domains[1])
	}
}

func TestIsManagedVhostFilename(t *testing.T) {
	cases := map[string]bool{
		"poinhost-example.com.conf": true,
		"homepoin-example.com.conf": true,
		"homepoin-port-4500.conf":   false,
		"other-vendor.example.conf": false,
		"poinhost-example.com.txt":  false,
	}
	for name, want := range cases {
		if got := isManagedVhostFilename(name); got != want {
			t.Errorf("isManagedVhostFilename(%q) = %v, want %v", name, got, want)
		}
	}
}
