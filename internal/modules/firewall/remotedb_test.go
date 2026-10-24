package firewall

import "testing"

// Kasus nyata yang dilaporkan: user sudah membuka 5432 "dari mana saja" lewat
// panel Firewall. Fitur remote access TIDAK boleh menambah aturan lagi —
// aturan longgar itu sudah mencakup IP klien mana pun.
func TestSumberTercakupAturanDariManaSaja(t *testing.T) {
	rules := []Rule{{Port: "5432", Protocol: "tcp", Source: "", Action: "allow"}}
	if !sourceAllowedForPort(rules, "182.8.212.16/32", 5432) {
		t.Fatal("aturan dari mana saja harus mencakup IP klien")
	}
	if sourceAllowedForPort(rules, "182.8.212.16/32", 3306) {
		t.Fatal("aturan 5432 tidak boleh terhitung membuka 3306")
	}
}

func TestSourceCovers(t *testing.T) {
	kasus := []struct {
		rule, src string
		want      bool
	}{
		{"", "1.2.3.4", true},
		{"1.2.3.4", "1.2.3.4/32", true},
		{"1.2.3.4/32", "1.2.3.4", true},
		{"1.2.3.0/24", "1.2.3.4", true},
		{"1.2.3.0/24", "1.2.0.0/16", false}, // cuma sebagian
		{"1.2.3.4", "1.2.3.5", false},
		{"10.0.0.0/8", "2001:db8::1", false}, // beda keluarga
		{"2001:db8::/32", "2001:db8::1", true},
		{"bukan-ip", "1.2.3.4", false},
	}
	for _, k := range kasus {
		if got := sourceCovers(k.rule, k.src); got != k.want {
			t.Errorf("sourceCovers(%q, %q) = %v, mau %v", k.rule, k.src, got, k.want)
		}
	}
}

// "Anywhere (v6)" di ufw hanya membuka port untuk IPv6.
func TestBarisV6TidakMencakupKlienIPv4(t *testing.T) {
	rules := []Rule{{Port: "5432", Protocol: "tcp", Action: "allow", IPv6: true}}
	if sourceAllowedForPort(rules, "182.8.212.16", 5432) {
		t.Fatal("aturan (v6) tidak boleh terhitung membuka port untuk IPv4")
	}
	if !sourceAllowedForPort(rules, "2001:db8::1", 5432) {
		t.Fatal("aturan (v6) dari mana saja harus mencakup klien IPv6")
	}
}

// Service firewalld ("postgresql") membuka port lewat ServicePorts, bukan Port.
func TestServiceFirewalldTerhitung(t *testing.T) {
	rules := []Rule{{Service: "postgresql", ServicePorts: "5432/tcp", Action: "allow"}}
	if !sourceAllowedForPort(rules, "182.8.212.16", 5432) {
		t.Fatal("service postgresql harus terhitung membuka 5432")
	}
}

func TestAturanDenyTidakTerhitung(t *testing.T) {
	rules := []Rule{{Port: "5432", Protocol: "tcp", Action: "deny"}}
	if sourceAllowedForPort(rules, "1.2.3.4", 5432) {
		t.Fatal("aturan deny tidak boleh dihitung membuka port")
	}
}

// Pencabutan hanya menyentuh aturan milik fitur ini (di ufw: komentar
// bermarker) dengan sumber+port PERSIS — aturan longgar milik user tetap.
func TestRemoteRuleIDsHanyaMilikFiturIni(t *testing.T) {
	rules := []Rule{
		{ID: "allow from 182.8.212.16 to any port 5432 proto tcp", Port: "5432", Protocol: "tcp", Source: "182.8.212.16", Action: "allow", Owner: remoteAccessMarker},
		{ID: "allow to any port 5432 proto tcp", Port: "5432", Protocol: "tcp", Action: "allow"},
		{ID: "allow from 182.8.212.16 to any port 3306 proto tcp", Port: "3306", Protocol: "tcp", Source: "182.8.212.16", Action: "allow", Owner: remoteAccessMarker},
		{ID: "allow from 182.8.212.0/24 to any port 5432 proto tcp", Port: "5432", Protocol: "tcp", Source: "182.8.212.0/24", Action: "allow", Owner: remoteAccessMarker},
		{ID: "allow from 182.8.212.16 to any port 5432 proto tcp#user", Port: "5432", Protocol: "tcp", Source: "182.8.212.16", Action: "allow"},
	}
	ids := remoteRuleIDs(rules, "ufw", "182.8.212.16/32", 5432)
	if len(ids) != 1 || ids[0] != rules[0].ID {
		t.Fatalf("hanya aturan bermarker dengan sumber+port persis yang boleh dicabut, dapat %v", ids)
	}
	// firewalld tidak punya komentar: cocokkan sumber+port persis.
	fw := remoteRuleIDs(rules, "firewalld", "182.8.212.16", 5432)
	if len(fw) != 2 {
		t.Fatalf("firewalld: dua aturan dengan sumber+port persis, dapat %v", fw)
	}
}

func TestFirewallSource(t *testing.T) {
	kasus := map[string]string{
		"182.8.212.16":    "182.8.212.16",
		"182.8.212.16/32": "182.8.212.16",
		"10.1.2.3/8":      "10.0.0.0/8",
		"2001:db8::1/128": "2001:db8::1",
	}
	for in, want := range kasus {
		got, err := firewallSource(in)
		if err != nil || got != want {
			t.Errorf("firewallSource(%q) = %q, %v; mau %q", in, got, err, want)
		}
	}
	if _, err := firewallSource("x; rm -rf /"); err == nil {
		t.Fatal("sumber sembarang harus ditolak")
	}
}
