package docker

import (
	"strings"
	"testing"
)

func TestNormalizePortScope(t *testing.T) {
	cases := map[string]string{
		"":          portScopePublic,
		"public":    portScopePublic,
		"Public":    portScopePublic,
		"intranet":  portScopeIntranet,
		"localhost": portScopeLocalhost,
	}
	for in, want := range cases {
		got, err := normalizePortScope(in)
		if err != nil {
			t.Fatalf("normalizePortScope(%q) unexpected error: %v", in, err)
		}
		if got != want {
			t.Fatalf("normalizePortScope(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := normalizePortScope("world"); err == nil {
		t.Fatal("normalizePortScope(\"world\") expected error, got nil")
	}
}

func TestHostIPForScope(t *testing.T) {
	if hostIPForScope(portScopeLocalhost) != "127.0.0.1" {
		t.Fatal("localhost scope must bind 127.0.0.1")
	}
	if hostIPForScope(portScopePublic) != "" {
		t.Fatal("public scope must not restrict bind (0.0.0.0)")
	}
	if hostIPForScope(portScopeIntranet) != "" {
		t.Fatal("intranet scope must bind 0.0.0.0 too — restriction is via firewall, not bind")
	}
}

func TestSyncPortScopeLabels(t *testing.T) {
	ports := []PortMapping{
		{HostPort: 8080, Protocol: "tcp", Scope: portScopeIntranet},
		{HostPort: 443, Protocol: "tcp", Scope: portScopePublic},
		{HostPort: 22, Protocol: "tcp", Scope: portScopeLocalhost},
	}
	labels := syncPortScopeLabels(map[string]string{"unrelated": "keep-me"}, ports)

	if labels["unrelated"] != "keep-me" {
		t.Fatal("non-portscope labels must survive sync")
	}
	if labels[portScopeLabelKey(8080, "tcp")] != portScopeIntranet {
		t.Fatal("intranet scope must be persisted as a label")
	}
	if labels[portScopeLabelKey(22, "tcp")] != portScopeLocalhost {
		t.Fatal("localhost scope must be persisted as a label")
	}
	if _, ok := labels[portScopeLabelKey(443, "tcp")]; ok {
		t.Fatal("public (default) scope should not be stored as a label")
	}

	// Recreate with the intranet port removed — stale label must be dropped,
	// not linger and confuse a future inspect.
	labels = syncPortScopeLabels(labels, []PortMapping{{HostPort: 443, Protocol: "tcp", Scope: portScopePublic}})
	if _, ok := labels[portScopeLabelKey(8080, "tcp")]; ok {
		t.Fatal("stale portscope label for a removed port must be dropped on resync")
	}
	if labels["unrelated"] != "keep-me" {
		t.Fatal("non-portscope labels must still survive a resync")
	}
}

func TestPortScopeApplyScriptRemovesStaleRuleBeforeNarrowing(t *testing.T) {
	// Switching a port from "public" (allow from anywhere) to "intranet"
	// must remove the old broad allow rule FIRST — otherwise it stays in
	// effect alongside the new narrow one and silently defeats it (ufw/
	// firewalld OR all matching allow rules together).
	script := portScopeApplyScript([]PortMapping{{HostPort: 8080, ContainerPort: 8080, Protocol: "tcp", Scope: portScopeIntranet}})

	deleteIdx := strings.Index(script, "ufw delete allow to any port 8080 proto tcp")
	addIdx := strings.Index(script, "ufw allow from 10.0.0.0/8 to any port 8080 proto tcp")
	if deleteIdx < 0 || addIdx < 0 {
		t.Fatalf("expected both a removal of the broad rule and an intranet allow rule, got:\n%s", script)
	}
	if deleteIdx > addIdx {
		t.Fatal("stale broad-allow rule must be removed BEFORE the narrower intranet rule is added")
	}
}

func TestPortScopeApplyScriptLocalhostAddsNoFirewallRule(t *testing.T) {
	script := portScopeApplyScript([]PortMapping{{HostPort: 2222, ContainerPort: 22, Protocol: "tcp", Scope: portScopeLocalhost}})
	if strings.Contains(script, "allow") && strings.Contains(script, "2222") {
		// only the cleanup "delete" lines should mention this port, never a fresh "allow"
		for _, line := range strings.Split(script, "\n") {
			if strings.Contains(line, "2222") && strings.Contains(line, "allow") && !strings.Contains(line, "delete") {
				t.Fatalf("localhost scope must not add a firewall allow rule, got line: %s", line)
			}
		}
	}
}

func TestPortScopeWarning(t *testing.T) {
	ports := []PortMapping{{HostPort: 5432, Protocol: "tcp", Scope: portScopeIntranet}}

	if w := portScopeWarning(ports, "ufw"); w != "" {
		t.Fatalf("no warning expected when a firewall is active, got: %q", w)
	}
	if w := portScopeWarning(ports, "none"); w == "" || !strings.Contains(w, "5432/tcp") {
		t.Fatalf("expected a warning naming the port when no firewall is detected, got: %q", w)
	}
	publicOnly := []PortMapping{{HostPort: 80, Protocol: "tcp", Scope: portScopePublic}}
	if w := portScopeWarning(publicOnly, "none"); w != "" {
		t.Fatalf("public scope needs no warning even without a firewall, got: %q", w)
	}
}
