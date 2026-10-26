package website

import (
	"strings"
	"testing"
)

// Path hanya pemisah: "/fixunit/api/login" harus sampai sebagai "/api/login".
// nginx membuang prefix hanya kalau location DAN proxy_pass sama-sama
// berakhir "/" — keduanya diperiksa, karena salah satu saja menghasilkan
// URI rusak ("//api/login" atau prefix yang tetap terbawa).
func TestProxyRuleStripPathMembuangPrefix(t *testing.T) {
	block := buildProxyLocationBlock(ProxyRule{Path: "/fixunit", Target: "http://127.0.0.1:3001", StripPath: true})
	for _, want := range []string{
		"location /fixunit/ {",
		"proxy_pass http://127.0.0.1:3001/;",
		"proxy_set_header X-Forwarded-Prefix /fixunit;",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("blok tidak memuat %q:\n%s", want, block)
		}
	}
	// target yang sudah punya path tetap dipakai sebagai prefix tujuan
	withPath := buildProxyLocationBlock(ProxyRule{Path: "/fixunit", Target: "http://127.0.0.1:3001/app", StripPath: true})
	if !strings.Contains(withPath, "proxy_pass http://127.0.0.1:3001/app/;") {
		t.Fatalf("target ber-path harus diakhiri slash:\n%s", withPath)
	}
}

// Perilaku lama (path diteruskan utuh) tidak boleh berubah untuk aturan yang
// sudah ada.
func TestProxyRuleTanpaStripTetapSepertiLama(t *testing.T) {
	block := buildProxyLocationBlock(ProxyRule{Path: "/api", Target: "http://127.0.0.1:3002"})
	if !strings.Contains(block, "location /api {") || !strings.Contains(block, "proxy_pass http://127.0.0.1:3002;") {
		t.Fatalf("aturan lama berubah:\n%s", block)
	}
	if strings.Contains(block, "X-Forwarded-Prefix") {
		t.Fatalf("header prefix hanya untuk mode strip:\n%s", block)
	}
}

// Pilihan strip disimpan di vhost itu sendiri (satu-satunya sumber
// kebenaran) dan harus terbaca balik — kalau tidak, menyimpan aturan LAIN di
// domain yang sama akan diam-diam mengembalikan aturan ini ke mode lama.
func TestProxyRuleStripPathTerbacaBalik(t *testing.T) {
	opts := &vhostOptions{Domain: "app.devpoin.com", Root: "/var/www/app.devpoin.com/public_html",
		ProxyRules: []ProxyRule{
			{Path: "/fixunit", Target: "http://127.0.0.1:3001", StripPath: true, WebSocket: true},
			{Path: "/api", Target: "http://127.0.0.1:3002"},
		}}
	info, ok := parseVhostFile("/etc/nginx/conf.d/poinhost-app.devpoin.com.conf", buildVhostConfig(opts), true)
	if !ok || len(info.ProxyRules) != 2 {
		t.Fatalf("aturan tidak terbaca: %+v", info)
	}
	got := map[string]ProxyRule{}
	for _, r := range info.ProxyRules {
		got[r.Path] = r
	}
	if r := got["/fixunit"]; !r.StripPath || !r.WebSocket {
		t.Fatalf("/fixunit harus tetap strip+ws: %+v", r)
	}
	if got["/api"].StripPath {
		t.Fatal("/api tidak boleh ikut jadi strip")
	}
}

func TestValidateProxyPath(t *testing.T) {
	for _, ok := range []string{"/", "/api", "/fixunit", "/v1/users", "/a.b_c~d-e"} {
		if err := validateProxyPath(ok); err != nil {
			t.Errorf("%q seharusnya valid: %v", ok, err)
		}
	}
	// path ditulis apa adanya ke `location <path> {` — karakter seperti ini
	// bisa menyisipkan direktif nginx sembarang.
	for _, bad := range []string{"/x { return 200; } location /y", "/a b", "/a;b", "/a'b", "/a\nb", "//api", "/a$b"} {
		if err := validateProxyPath(bad); err == nil {
			t.Errorf("%q seharusnya ditolak", bad)
		}
	}
}
