package website

import "strings"

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

// Untuk engine "mysql", value yang dikembalikan DBSupportedVersions diberi
// prefix produk ("mariadb-10.11") — BUKAN cuma "10.11" — supaya nanti kalau
// MySQL asli (Oracle) ditambahkan sebagai pilihan kedua, keduanya bisa
// hidup berdampingan di SATU dropdown tanpa field/DTO baru: tinggal isi
// supportedMySQLVersions + mysqlVersionedInstallScript di bawah, lalu
// tambah satu cabang di dbVersionedInstallScript. Tidak perlu mengubah
// dbInstallScript, StreamInstall, frontend, atau wails binding sama sekali
// — semuanya sudah menerima "version" sebagai string bebas.
//
// MySQL asli SENGAJA belum diimplementasikan sekarang: mekanisme resmi
// Oracle (mysql-apt-config .deb / mysql80-community-release RPM) memakai
// nama file ber-versi yang berubah dari waktu ke waktu (beda dari
// mariadb_repo_setup/PGDG yang stabil), jadi lebih rawan basi — utamakan
// MariaDB dulu (drop-in compatible, mekanisme install jauh lebih stabil
// jangka panjang) sampai benar-benar dibutuhkan.
const mariaDBVersionPrefix = "mariadb-"
const mysqlVersionPrefix = "mysql-"

// DBSupportedVersions versi PINNED yang bisa dipilih eksplisit saat
// instalasi — di luar ini, opsi "(bawaan distro)" (versi kosong) tetap
// selalu tersedia dan tidak berubah perilakunya.
func DBSupportedVersions(engine string) []string {
	if engine == "postgresql" {
		return append([]string{}, supportedPostgreSQLVersions...)
	}
	out := make([]string, 0, len(supportedMariaDBVersions))
	for _, v := range supportedMariaDBVersions {
		out = append(out, mariaDBVersionPrefix+v)
	}
	// supportedMySQLVersions belum ada — begitu diisi, loop yang sama
	// ditambahkan di sini dengan mysqlVersionPrefix.
	return out
}

// dbVersionedInstallScript memasang versi PINNED lewat repo resmi vendor —
// beda dari jalur default di dbInstallScript yang tidak menambah repo
// apa pun.
func dbVersionedInstallScript(engine, pm, version, dockerAccess string) (string, bool) {
	if engine != "mysql" {
		return postgresVersionedInstallScript(pm, version, dockerAccess)
	}
	if v, ok := strings.CutPrefix(version, mariaDBVersionPrefix); ok {
		return mariaDBVersionedInstallScript(pm, v, dockerAccess)
	}
	if v, ok := strings.CutPrefix(version, mysqlVersionPrefix); ok {
		return mysqlVersionedInstallScript(pm, v, dockerAccess)
	}
	return "", false
}

// mysqlVersionedInstallScript memasang MySQL ASLI (Oracle) versi tertentu.
// BELUM diimplementasikan — lihat komentar mysqlVersionPrefix di atas
// untuk alasannya. DBSupportedVersions tidak menawarkan versi "mysql-*"
// apa pun sampai ini diisi, jadi jalur ini belum bisa dicapai dari UI sama
// sekali; sengaja dibiarkan di sini (bukan dihapus) sebagai titik
// perluasan yang sudah disiapkan.
func mysqlVersionedInstallScript(pm, version, dockerAccess string) (string, bool) {
	return "", false
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
