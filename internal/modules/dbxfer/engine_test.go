package dbxfer

import (
	"strings"
	"testing"
)

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// Tanpa "set -o pipefail", "mysqldump | gzip" yang gagal di tengah tetap
// keluar exit 0 (shell hanya melihat exit code proses TERAKHIR dalam
// pipeline) — bug nyata yang pernah terjadi di homepoin. Setiap perintah
// dump/restore di sini HARUS membawa prefix ini.
func TestBuildDumpCmd_MySQL_HasPipefail(t *testing.T) {
	cmd := buildDumpCmd("mysql", "appdb", nil, false)
	if !containsAll(cmd, "set -o pipefail", "mysqldump", "appdb", "gzip -c") {
		t.Fatalf("dump command salah: %s", cmd)
	}
	if strings.Contains(cmd, "DEFINER") {
		t.Errorf("dump command tidak boleh menyinggung DEFINER — restore di sini selalu sebagai root, jadi tidak perlu strip: %s", cmd)
	}
}

func TestBuildDumpCmd_MySQL_GtidPurgedOptional(t *testing.T) {
	with := buildDumpCmd("mysql", "appdb", nil, true)
	if !strings.Contains(with, "--set-gtid-purged=OFF") {
		t.Errorf("gtidPurgedSupported=true harus menyertakan flag: %s", with)
	}
	without := buildDumpCmd("mysql", "appdb", nil, false)
	if strings.Contains(without, "gtid-purged") {
		t.Errorf("gtidPurgedSupported=false TIDAK boleh menyertakan flag (MariaDB lama tidak kenal flag ini): %s", without)
	}
}

func TestBuildDumpCmd_MySQL_TablesQuoted(t *testing.T) {
	cmd := buildDumpCmd("mysql", "appdb", []string{"users", "orders"}, false)
	if !containsAll(cmd, "'users'", "'orders'") {
		t.Fatalf("nama tabel harus di-quote: %s", cmd)
	}
}

func TestBuildRestoreCmd_MySQL_HasPipefail(t *testing.T) {
	cmd := buildRestoreCmd("mysql", "appdb")
	if !containsAll(cmd, "set -o pipefail", "gunzip -c", "mysql -u root", "appdb") {
		t.Fatalf("restore command salah: %s", cmd)
	}
}

func TestBuildDumpCmd_Postgres_HasPipefailAndClean(t *testing.T) {
	cmd := buildDumpCmd("postgresql", "appdb", nil, false)
	// --clean --if-exists: perbaikan atas homepoin, yang tidak memakai
	// flag ini sama sekali — restore ulang ke database yang sudah pernah
	// diisi gagal total di objek pertama yang bentrok (ON_ERROR_STOP=1).
	if !containsAll(cmd, "set -o pipefail", "pg_dump", "--clean", "--if-exists", "gzip -c") {
		t.Fatalf("dump command salah: %s", cmd)
	}
}

func TestBuildDumpCmd_Postgres_TableFlags(t *testing.T) {
	cmd := buildDumpCmd("postgresql", "appdb", []string{"users", "orders"}, false)
	if !containsAll(cmd, "-t 'users'", "-t 'orders'") {
		t.Fatalf("flag -t per tabel hilang: %s", cmd)
	}
}

func TestBuildRestoreCmd_Postgres_HasPipefailAndErrorStop(t *testing.T) {
	cmd := buildRestoreCmd("postgresql", "appdb")
	if !containsAll(cmd, "set -o pipefail", "gunzip -c", "psql", "ON_ERROR_STOP=1", "appdb") {
		t.Fatalf("restore command salah: %s", cmd)
	}
}

// Nama database/tabel yang mengandung karakter shell berbahaya harus
// keluar ter-quote di command yang dihasilkan, bukan disambung mentah.
func TestBuildDumpCmd_QuotesShellMetacharacters(t *testing.T) {
	cmd := buildDumpCmd("mysql", "app'; rm -rf /", nil, false)
	if !strings.Contains(cmd, `'app'\''; rm -rf /'`) {
		t.Fatalf("nama database tidak ter-quote dengan aman: %s", cmd)
	}
}
