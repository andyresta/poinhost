package website

import (
	"strings"
	"testing"
)

func TestMySQLGrantDatabase(t *testing.T) {
	cases := []struct {
		grant    string
		db       string
		global   bool
		ok       bool
	}{
		{"GRANT USAGE ON *.* TO `shop`@`%`", "", true, true},
		{"GRANT ALL PRIVILEGES ON `shop\\_db`.* TO `shop`@`%`", "shop_db", false, true},
		{"GRANT SELECT, INSERT ON `shop`.`orders` TO 'shop'@'localhost'", "shop", false, true},
		{"GRANT EXECUTE ON PROCEDURE `shop`.`p1` TO `shop`@`%`", "shop", false, true},
		{"GRANT SELECT ON shop.* TO shop@localhost", "shop", false, true},
		{"GRANT `app_role`@`%` TO `shop`@`%`", "", false, false},
		{"GRANT PROXY ON ''@'' TO 'root'@'localhost' WITH GRANT OPTION", "", false, false},
	}
	for _, c := range cases {
		db, global, ok := mysqlGrantDatabase(c.grant)
		if db != c.db || global != c.global || ok != c.ok {
			t.Errorf("mysqlGrantDatabase(%q) = (%q, %v, %v), mau (%q, %v, %v)", c.grant, db, global, ok, c.db, c.global, c.ok)
		}
	}
}

func TestUnescapeMySQLBatch(t *testing.T) {
	if got := unescapeMySQLBatch(`a\tb\nc\\d`); got != "a\tb\nc\\d" {
		t.Fatalf("unescape = %q", got)
	}
}

func TestMySQLQuoteEscapesBackslash(t *testing.T) {
	if got := mysqlQuote(`o'k\`); got != `'o''k\\'` {
		t.Fatalf("mysqlQuote = %s", got)
	}
}

func TestHashScheme(t *testing.T) {
	cases := map[string]string{
		"$y$j9T$salt$hash": "$y$",
		"!$6$salt$hash":    "$6$",
		"$2b$05$xyz":       "$2b$",
		"!":                "",
		"":                 "",
	}
	for in, want := range cases {
		if got := hashScheme(in); got != want {
			t.Errorf("hashScheme(%q) = %q, mau %q", in, got, want)
		}
	}
}

// Vhost migrasi tidak boleh menunjuk socket PHP/sertifikat yang belum ada
// di tujuan, tapi proxy rule harus tetap ikut — dan .php harus ditolak
// supaya source PHP tidak terunduh selama PHP belum dipasang.
func TestMigratedVhostOptions(t *testing.T) {
	info := DomainInfo{
		Domain: "shop.example.com", Root: "/var/www/shop.example.com/public_html", Enabled: true,
		PHPVersion: "8.2", PHPEnabled: true,
		SSLEnabled: true, SSLCertificate: "/etc/letsencrypt/live/shop/fullchain.pem", SSLCertificateKey: "/etc/letsencrypt/live/shop/privkey.pem",
		ProxyRules: []ProxyRule{{Path: "/api", Target: "http://127.0.0.1:3000", WebSocket: true}},
	}
	conf := buildVhostConfig(migratedVhostOptions(info))
	for _, bad := range []string{"fastcgi_pass", "ssl_certificate", "listen 443"} {
		if strings.Contains(conf, bad) {
			t.Errorf("vhost migrasi berisi %q:\n%s", bad, conf)
		}
	}
	for _, want := range []string{"location /api", "proxy_pass http://127.0.0.1:3000", "root /var/www/shop.example.com/public_html", "location ~ \\.php$ {\n        return 403;"} {
		if !strings.Contains(conf, want) {
			t.Errorf("vhost migrasi tidak berisi %q:\n%s", want, conf)
		}
	}
}

func TestParsePGDatabaseACL(t *testing.T) {
	items := parsePGDatabaseACL(`{=Tc/postgres,postgres=CTc/postgres,shop=CTc*/postgres,"odd""name"=c/postgres}`)
	if len(items) != 4 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].grantee != "" || strings.Join(items[0].privs, ",") != "TEMPORARY,CONNECT" {
		t.Errorf("PUBLIC = %+v", items[0])
	}
	if items[2].grantee != "shop" || strings.Join(items[2].privs, ",") != "CREATE,TEMPORARY,CONNECT" {
		t.Errorf("shop = %+v", items[2])
	}
	if items[3].grantee != `odd"name` || strings.Join(items[3].privs, ",") != "CONNECT" {
		t.Errorf("quoted = %+v", items[3])
	}
	if parsePGDatabaseACL("") != nil {
		t.Error("ACL kosong harus nil")
	}
}

func TestParseIdentified(t *testing.T) {
	cases := []struct{ sql, plugin, hash string }{
		{"CREATE USER `a`@`%` IDENTIFIED WITH 'caching_sha2_password' AS 0x2441243030 REQUIRE NONE PASSWORD EXPIRE DEFAULT", "caching_sha2_password", ""},
		{"CREATE USER `a`@`%` IDENTIFIED WITH 'mysql_native_password' AS '*6BB4837EB74329105EE4568DDA7DC67ED2CA2AD9' REQUIRE NONE", "mysql_native_password", "*6BB4837EB74329105EE4568DDA7DC67ED2CA2AD9"},
		{"CREATE USER `a`@`localhost` IDENTIFIED BY PASSWORD '*6BB4837EB74329105EE4568DDA7DC67ED2CA2AD9'", "mysql_native_password", "*6BB4837EB74329105EE4568DDA7DC67ED2CA2AD9"},
		{"CREATE USER `a`@`localhost` IDENTIFIED VIA ed25519 USING 'ZIgUREUg5PVgQ6LskhXmO+eZLS0nC8be6HPjYWR4YJY'", "ed25519", ""},
	}
	for _, c := range cases {
		p, h := parseIdentified(c.sql)
		if p != c.plugin || h != c.hash {
			t.Errorf("parseIdentified(%q) = (%q, %q), mau (%q, %q)", c.sql, p, h, c.plugin, c.hash)
		}
	}
	sha := DBUserExport{Engine: "mysql", SourceFlavor: "mysql", Plugin: "caching_sha2_password"}
	if MySQLUserPortable(sha, "mariadb") {
		t.Error("caching_sha2 MySQL → MariaDB tidak boleh dianggap portabel")
	}
	if !MySQLUserPortable(sha, "mysql") {
		t.Error("MySQL → MySQL harus portabel")
	}
	native := DBUserExport{Engine: "mysql", SourceFlavor: "mysql", nativeHash: "*6BB4837EB74329105EE4568DDA7DC67ED2CA2AD9"}
	if !MySQLUserPortable(native, "mariadb") {
		t.Error("mysql_native_password harus portabel lintas MySQL/MariaDB")
	}
}
