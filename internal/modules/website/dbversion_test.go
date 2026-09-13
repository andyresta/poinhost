package website

import (
	"strings"
	"testing"
)

func TestDBSupportedVersionsDoesNotAliasInternalSlice(t *testing.T) {
	v1 := DBSupportedVersions("postgresql")
	v1[0] = "tampered"
	v2 := DBSupportedVersions("postgresql")
	if v2[0] == "tampered" {
		t.Fatal("DBSupportedVersions must return a defensive copy, not the shared backing array")
	}
	if len(DBSupportedVersions("mysql")) == 0 {
		t.Fatal("expected at least one supported MariaDB version")
	}
}

func TestDbInstallScriptDefaultVersionUnchanged(t *testing.T) {
	script, ok := dbInstallScript("mysql", "apt", "")
	if !ok {
		t.Fatal("expected apt mysql default install to be supported")
	}
	if strings.Contains(script, "mariadb_repo_setup") || strings.Contains(script, "pgdg") {
		t.Fatalf("default (unpinned) install must not touch any vendor repo, got:\n%s", script)
	}
	if !strings.Contains(script, "apt-get install -y mariadb-server") {
		t.Fatal("default mysql install must still install the plain distro mariadb-server package")
	}
}

func TestDbInstallScriptPinnedVersionRoutesToVersionedScript(t *testing.T) {
	script, ok := dbInstallScript("mysql", "apt", "10.11")
	if !ok {
		t.Fatal("expected pinned mariadb install to be supported on apt")
	}
	if !strings.Contains(script, `mariadb-server-version="mariadb-10.11"`) {
		t.Fatalf("pinned version must be threaded into mariadb_repo_setup, got:\n%s", script)
	}
	// Docker access must still be applied on the versioned path too — it's
	// the same automatic step as the default install path.
	if !strings.Contains(script, "POINHOST_DOCKER_ACCESS_DONE") {
		t.Fatal("pinned install must still run the docker-access script (same as default install)")
	}
}

func TestMariaDBVersionedInstallScriptPackageNamePerPM(t *testing.T) {
	apt, ok := mariaDBVersionedInstallScript("apt", "10.11", "")
	if !ok || !strings.Contains(apt, "apt-get install -y mariadb-server") {
		t.Fatalf("apt package name should be lowercase mariadb-server, got:\n%s", apt)
	}
	dnf, ok := mariaDBVersionedInstallScript("dnf", "10.11", "")
	if !ok || !strings.Contains(dnf, "dnf install -y MariaDB-server") {
		t.Fatalf("dnf package name should be capitalized MariaDB-server (official RPM repo naming), got:\n%s", dnf)
	}
	if _, ok := mariaDBVersionedInstallScript("zypper", "10.11", ""); ok {
		t.Fatal("unsupported package manager must return ok=false")
	}
}

func TestPostgresVersionedInstallScriptDiffersAptVsDnf(t *testing.T) {
	apt, ok := postgresVersionedInstallScript("apt", "16", "")
	if !ok {
		t.Fatal("expected apt postgres pinned install to be supported")
	}
	if !strings.Contains(apt, "postgresql-16") || strings.Contains(apt, "module disable") {
		t.Fatalf("apt PGDG install must install postgresql-<version> and never touch dnf modules, got:\n%s", apt)
	}
	// apt PGDG packages are all managed by ONE generic service regardless of version.
	if !strings.Contains(apt, "systemctl start postgresql\n") {
		t.Fatalf("apt PGDG must start the generic postgresql service, got:\n%s", apt)
	}

	dnf, ok := postgresVersionedInstallScript("dnf", "16", "")
	if !ok {
		t.Fatal("expected dnf postgres pinned install to be supported")
	}
	if !strings.Contains(dnf, "module disable") {
		t.Fatal("dnf PGDG install must disable the distro's own postgresql module first (conflicts otherwise)")
	}
	// RHEL/PGDG packages use a VERSION-SPECIFIC service unit, unlike apt.
	if !strings.Contains(dnf, "systemctl start postgresql-16") {
		t.Fatalf("dnf PGDG must start the version-specific postgresql-<version> service, got:\n%s", dnf)
	}
}
