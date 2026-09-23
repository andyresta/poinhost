package sitexfer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/andyresta/poinhost/internal/modules/website"
)

var (
	shop     = website.DomainInfo{Domain: "shop.com", Root: "/var/www/shop.com/public_html"}
	shopAPI  = website.DomainInfo{Domain: "api.shop.com", Parent: "shop.com", IsSubdomain: true, Root: "/var/www/shop.com/api.shop.com"}
	shopBlog = website.DomainInfo{Domain: "blog.shop.com", Parent: "shop.com", IsSubdomain: true, Root: "/var/www/shop.com/blog.shop.com"}
	other    = website.DomainInfo{Domain: "other.com", Root: "/var/www/other.com/public_html"}
	legacy   = website.DomainInfo{Domain: "old.com", Root: "/var/www/vhosts/old.com/httpdocs"}
	all      = []website.DomainInfo{shop, shopAPI, shopBlog, other, legacy}
)

func pathsOf(ps []PlanPath) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.Path+" -"+strings.Join(p.Excludes, ","))
	}
	return out
}

func TestComputePaths(t *testing.T) {
	cases := []struct {
		name string
		sel  []website.DomainInfo
		want []string
	}{
		{"induk saja: subdomain yang tidak dipilih dikecualikan", []website.DomainInfo{shop},
			[]string{"/var/www/shop.com -/var/www/shop.com/api.shop.com,/var/www/shop.com/blog.shop.com"}},
		{"induk + satu subdomain: subdomain tercakup, sisanya dikecualikan", []website.DomainInfo{shop, shopAPI},
			[]string{"/var/www/shop.com -/var/www/shop.com/blog.shop.com"}},
		{"subdomain saja: hanya folder subdomain", []website.DomainInfo{shopBlog},
			[]string{"/var/www/shop.com/blog.shop.com -"}},
		{"layout lama: document root itu sendiri", []website.DomainInfo{legacy},
			[]string{"/var/www/vhosts/old.com/httpdocs -"}},
		{"dua domain terpisah", []website.DomainInfo{other, shopAPI},
			[]string{"/var/www/other.com -", "/var/www/shop.com/api.shop.com -"}},
	}
	for _, c := range cases {
		got := pathsOf(computePaths(c.sel, all))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got  %v\n want %v", c.name, got, c.want)
		}
	}
}

func TestValidateSitePath(t *testing.T) {
	for _, bad := range []string{"/", "/var/www", "/etc", "/home", "relatif"} {
		if validateSitePath(bad) == nil {
			t.Errorf("%q harus ditolak", bad)
		}
	}
	for _, ok := range []string{"/var/www/shop.com", "/home/andy/site", "/srv/web/x"} {
		if err := validateSitePath(ok); err != nil {
			t.Errorf("%q harus diterima: %v", ok, err)
		}
	}
}

func TestTarCommands(t *testing.T) {
	p := PlanPath{Path: "/var/www/shop.com", Excludes: []string{"/var/www/shop.com/blog.shop.com"}}
	got := tarCreateCmd(p, true)
	want := "set -o pipefail; tar --anchored --exclude='var/www/shop.com/blog.shop.com' -C / -c -z -p -f - 'var/www/shop.com'"
	if got != want {
		t.Errorf("tarCreateCmd =\n %s\nmau\n %s", got, want)
	}
	if got := tarExtractCmd(false); got != "tar -C / -x -p --same-owner -f -" {
		t.Errorf("tarExtractCmd = %s", got)
	}
	if s := statScript(p); !strings.Contains(s, `\( -path '/var/www/shop.com/blog.shop.com' \) -prune -o -type f`) {
		t.Errorf("statScript tidak mem-prune exclude: %s", s)
	}
}

func TestOwnerFixScriptAndParse(t *testing.T) {
	owners := parseOwners("33 www-data 33 www-data\n1001 shopftp 33 www-data\n1000 andy 1000 andy\n")
	if len(owners) != 3 {
		t.Fatalf("owners = %+v", owners)
	}
	s := ownerFixScript(PlanPath{Path: "/var/www/shop.com"}, owners)
	for _, want := range []string{
		"if ! id -u 'andy' >/dev/null 2>&1; then chown -R -h --from='+1000' \"$WEBUSER\" \"$P\" && echo FIXED user 'andy'; fi",
		"if ! getent group 'andy' >/dev/null 2>&1; then chown -R -h --from=:'+1000' \":$WEBGROUP\" \"$P\"",
		"P='/var/www/shop.com'",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("ownerFixScript tidak berisi %q:\n%s", want, s)
		}
	}
	if got := fixedOwners("FIXED user andy\nnoise\nFIXED group andy\n"); !reflect.DeepEqual(got, []string{"user andy", "group andy"}) {
		t.Errorf("fixedOwners = %v", got)
	}
}

func TestNormalizeRequest(t *testing.T) {
	if _, err := normalizeRequest(StartRequest{SourceServerID: "a", DestServerID: "a", Domains: []string{"x.com"}}); err == nil {
		t.Error("server sama harus ditolak")
	}
	if _, err := normalizeRequest(StartRequest{SourceServerID: "a", DestServerID: "b"}); err == nil {
		t.Error("tanpa domain harus ditolak")
	}
	r, err := normalizeRequest(StartRequest{SourceServerID: " a ", DestServerID: "b", Domains: []string{"B.com", "a.com", "b.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.SourceServerID != "a" || !reflect.DeepEqual(r.Domains, []string{"a.com", "b.com"}) {
		t.Errorf("normalizeRequest = %+v", r)
	}
}

func TestPathFor(t *testing.T) {
	paths := []PlanPath{{Path: "/var/www/shop.com"}, {Path: "/var/www/other.com"}}
	if got := pathFor(paths, "/var/www/shop.com/api.shop.com"); got != "/var/www/shop.com" {
		t.Errorf("pathFor = %q", got)
	}
	if got := pathFor(paths, "/var/www/shop.community/public_html"); got != "" {
		t.Errorf("prefix nama mirip tidak boleh cocok: %q", got)
	}
}
