package website

// Instalasi default (lihat dbInstallScript, version=="") sengaja TIDAK
// menambah repo pihak ketiga apa pun — cuma `apt-get install mariadb-server`
// / `apt-get install postgresql` polos, ikut versi apa pun yang jadi
// default di repo BAWAAN distro tersebut. Ini simpel dan aman, tapi
// berarti versi terpasang bisa jauh dari rilis terbaru (mis. Ubuntu 22.04
// cuma menyediakan PostgreSQL 14 secara default) dan TIDAK bisa dipilih.
//
// supportedMariaDBVersions/supportedPostgreSQLVersions di bawah adalah
// versi PINNED yang bisa dipilih eksplisit — dipasang lewat repo RESMI
// vendor-nya sendiri (bukan PPA/mirror pihak ketiga random), pola yang
// sama seperti phpRepoScript (php.go) untuk PHP: repo dipasang otomatis
// begitu user memilih versi tertentu, transparan buat user.
var supportedMariaDBVersions = []string{"10.6", "10.11", "11.4"}
var supportedPostgreSQLVersions = []string{"14", "15", "16", "17", "18"}

// DBSupportedVersions versi PINNED yang bisa dipilih eksplisit saat
// instalasi — di luar ini, opsi "(bawaan distro)" (versi kosong) tetap
// selalu tersedia dan tidak berubah perilakunya.
func DBSupportedVersions(engine string) []string {
	if engine == "postgresql" {
		return append([]string{}, supportedPostgreSQLVersions...)
	}
	return append([]string{}, supportedMariaDBVersions...)
}

// dbVersionedInstallScript memasang versi PINNED lewat repo resmi vendor —
// beda dari jalur default di dbInstallScript yang tidak menambah repo
// apa pun.
func dbVersionedInstallScript(engine, pm, version, dockerAccess string) (string, bool) {
	if engine == "mysql" {
		return mariaDBVersionedInstallScript(pm, version, dockerAccess)
	}
	return postgresVersionedInstallScript(pm, version, dockerAccess)
}

// mariaDBVersionedInstallScript memasang MariaDB versi tertentu lewat
// skrip repo resmi MariaDB Foundation sendiri (mariadb_repo_setup) —
// mendeteksi & menyiapkan repo apt MAUPUN dnf/yum dalam satu skrip yang
// sama, tinggal beda nama paket server-nya (RPM resmi MariaDB pakai
// prefix kapital `MariaDB-`, bukan `mariadb-` seperti paket Debian/Ubuntu).
func mariaDBVersionedInstallScript(pm, version, dockerAccess string) (string, bool) {
	pkg := "mariadb-server"
	if pm == "dnf" || pm == "yum" {
		pkg = "MariaDB-server"
	}
	switch pm {
	case "apt":
		return `set -e
` + aptWaitLock + `
apt-get install -y curl
curl -fsSL https://r.mariadb.com/downloads/mariadb_repo_setup -o /tmp/mariadb_repo_setup
chmod +x /tmp/mariadb_repo_setup
/tmp/mariadb_repo_setup --mariadb-server-version="mariadb-` + version + `"
apt-get install -y ` + pkg + `
systemctl enable mariadb
systemctl start mariadb
mysql -e "CREATE USER IF NOT EXISTS 'root'@'127.0.0.1' IDENTIFIED BY ''; GRANT ALL ON *.* TO 'root'@'127.0.0.1' WITH GRANT OPTION; FLUSH PRIVILEGES;" 2>/dev/null || true
` + dockerAccess + `
echo ">> MariaDB ` + version + ` terpasang (repo resmi MariaDB), siap diakses dari container Docker di host ini."
`, true
	case "dnf", "yum":
		return `set -e
` + pm + ` install -y curl
curl -fsSL https://r.mariadb.com/downloads/mariadb_repo_setup -o /tmp/mariadb_repo_setup
chmod +x /tmp/mariadb_repo_setup
/tmp/mariadb_repo_setup --mariadb-server-version="mariadb-` + version + `"
` + pm + ` install -y ` + pkg + `
systemctl enable mariadb
systemctl start mariadb
mysql -e "CREATE USER IF NOT EXISTS 'root'@'127.0.0.1' IDENTIFIED BY ''; GRANT ALL ON *.* TO 'root'@'127.0.0.1' WITH GRANT OPTION; FLUSH PRIVILEGES;" 2>/dev/null || true
` + dockerAccess + `
echo ">> MariaDB ` + version + ` terpasang (repo resmi MariaDB), siap diakses dari container Docker di host ini."
`, true
	default:
		return "", false
	}
}

// postgresVersionedInstallScript memasang PostgreSQL versi tertentu lewat
// repo resmi PGDG. Beda mendasar apt vs dnf/yum: paket apt.postgresql.org
// membuat SEMUA versi terpasang dipayungi SATU service generik
// `postgresql` (pg_wrapper), sedangkan repo PGDG untuk RHEL/dnf memberi
// tiap versi service TER-VERSI-nya sendiri (`postgresql-16`, dst) supaya
// beberapa versi bisa hidup berdampingan — makanya butuh langkah initdb +
// enable + start eksplisit per versi, dan dbStatusScriptPostgres/DBStart
// harus tahu mencoba nama ter-versi ini juga (lihat komentarnya).
func postgresVersionedInstallScript(pm, version, dockerAccess string) (string, bool) {
	switch pm {
	case "apt":
		return `set -e
` + aptWaitLock + `
apt-get install -y curl ca-certificates gnupg
install -d /usr/share/postgresql-common/pgdg
curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc
. /etc/os-release
echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt $VERSION_CODENAME-pgdg main" > /etc/apt/sources.list.d/pgdg.list
apt-get update
apt-get install -y postgresql-` + version + `
systemctl enable postgresql
systemctl start postgresql
` + dockerAccess + `
echo ">> PostgreSQL ` + version + ` terpasang (repo resmi PGDG), siap diakses dari container Docker di host ini."
`, true
	case "dnf", "yum":
		return `set -e
` + pm + ` install -y https://download.postgresql.org/pub/repos/yum/reporpm/EL-$(rpm -E %rhel)-x86_64/pgdg-redhat-repo-latest.noarch.rpm
` + pm + ` -qy module disable postgresql 2>/dev/null || true
` + pm + ` install -y postgresql` + version + `-server postgresql` + version + `
/usr/pgsql-` + version + `/bin/postgresql-` + version + `-setup initdb 2>/dev/null || true
systemctl enable postgresql-` + version + `
systemctl start postgresql-` + version + `
` + dockerAccess + `
echo ">> PostgreSQL ` + version + ` terpasang (repo resmi PGDG), siap diakses dari container Docker di host ini."
`, true
	default:
		return "", false
	}
}
