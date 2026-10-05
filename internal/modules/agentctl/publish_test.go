package agentctl

import (
	"testing"

	"github.com/andyresta/poinhost/internal/modules/website"
)

// Induk yang dipilih harus yang TERDEKAT, dan hanya satu label di bawahnya.
//
// Kalau induknya salah, modul website membuat vhost dengan server_name yang
// salah pula — dan untuk nama berlapis seperti agent.app.example.com,
// memilih example.com sebagai induk berarti labelnya jadi "agent.app", yang
// bukan subdomain satu tingkat.
func TestSplitSubdomain_MemilihIndukTerdekat(t *testing.T) {
	ada := []website.DomainInfo{
		{Domain: "example.com"},
		{Domain: "app.example.com"},
		{Domain: "lain.test"},
	}

	parent, label, ok := splitSubdomain("agent.app.example.com", ada)
	if !ok || parent != "app.example.com" || label != "agent" {
		t.Errorf("= (%q, %q, %v), mau (app.example.com, agent, true)", parent, label, ok)
	}

	parent, label, ok = splitSubdomain("agent.example.com", ada)
	if !ok || parent != "example.com" || label != "agent" {
		t.Errorf("= (%q, %q, %v), mau (example.com, agent, true)", parent, label, ok)
	}
}

func TestSplitSubdomain_YangBukanSubdomainSatuTingkat(t *testing.T) {
	ada := []website.DomainInfo{
		{Domain: "example.com"},
		// Induk yang sendirinya subdomain tidak boleh dipakai: modul website
		// cuma mengenal satu tingkat parent.
		{Domain: "app.example.com", IsSubdomain: true, Parent: "example.com"},
	}

	// Dua label di bawah induk — dibuat sebagai domain tersendiri.
	if _, _, ok := splitSubdomain("a.b.example.com", ada); ok {
		t.Error("nama dua tingkat diperlakukan sebagai subdomain satu tingkat")
	}
	// Induk tidak dikelola di server ini.
	if _, _, ok := splitSubdomain("agent.lain.test", ada); ok {
		t.Error("induk yang tidak terdaftar dianggap ada")
	}
	// Domain itu sendiri, bukan subdomain siapa pun.
	if _, _, ok := splitSubdomain("example.com", ada); ok {
		t.Error("domain sendiri dianggap subdomain")
	}
	// Akhiran yang sama tapi domain berbeda: notexample.com bukan subdomain
	// example.com. Tanpa titik di depan, perbandingan akhiran meloloskannya.
	if _, _, ok := splitSubdomain("notexample.com", ada); ok {
		t.Error("akhiran yang kebetulan sama dianggap subdomain")
	}
}

// Mengarahkan domain yang sedang melayani website ke agent akan mematikan
// website itu. Inilah satu-satunya langkah merusak di alur Publish, jadi
// pendeteksiannya diuji per bentuk pemakaian.
func TestDomainInUse(t *testing.T) {
	dipakai := map[string]website.DomainInfo{
		"php":        {PHPEnabled: true},
		"proxy lain": {ProxyTarget: "http://127.0.0.1:3000"},
		"rule path":  {ProxyRules: []website.ProxyRule{{Path: "/api", Target: "http://127.0.0.1:9000"}}},
	}
	for nama, d := range dipakai {
		if !domainInUse(d) {
			t.Errorf("%s dianggap kosong", nama)
		}
	}
	if domainInUse(website.DomainInfo{Domain: "kosong.test", Root: "/var/www/kosong.test"}) {
		t.Error("vhost static kosong dianggap terpakai")
	}
}

// Target proxy harus persis cocok dengan yang dicari PublishStatus, kalau
// tidak domain yang sudah diterbitkan tidak akan dikenali sebagai miliknya
// dan panel melaporkannya belum terbit.
func TestAgentProxyTarget_CocokDenganYangDicari(t *testing.T) {
	const port = 7898
	target := agentProxyTarget(port)
	if target != "http://127.0.0.1:7898" {
		t.Fatalf("target = %q", target)
	}

	d := website.DomainInfo{Domain: "agent.example.com", ProxyTarget: target, SSLEnabled: true}
	if d.ProxyTarget != agentProxyTarget(port) {
		t.Error("target tidak cocok dengan dirinya sendiri")
	}
	// Port berbeda bukan milik agent ini.
	if d.ProxyTarget == agentProxyTarget(7899) {
		t.Error("port lain ikut dikenali sebagai agent")
	}
	if got := publishedURL(d.Domain, d.SSLEnabled); got != "https://agent.example.com" {
		t.Errorf("url = %q", got)
	}
	if got := publishedURL(d.Domain, false); got != "http://agent.example.com" {
		t.Errorf("url tanpa ssl = %q", got)
	}
}
