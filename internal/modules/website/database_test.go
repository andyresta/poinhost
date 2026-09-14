package website

import "strings"
import "testing"

// Regresi bug nyata: mysql -h 127.0.0.1 -u root gagal konsisten di server
// sungguhan (dikonfirmasi via instalasi MariaDB 10.11 asli) — MariaDB
// mencocokkan koneksi TCP loopback itu ke akun root@localhost (auth
// unix_socket, SELALU menolak TCP), bukan root@127.0.0.1 yang dibuat
// khusus untuk ini, meski password benar. Lihat komentar panjang di
// runMySQL (database.go) untuk detail lengkap.
func TestMySQLAdminCommandUsesLocalSocketNotTCP(t *testing.T) {
	cmd := mysqlAdminCommand("SHOW DATABASES")
	if !strings.Contains(cmd, "mysql -u root") {
		t.Fatalf("expected local-socket mysql -u root, got: %s", cmd)
	}
	if strings.Contains(cmd, "-h 127.0.0.1") || strings.Contains(cmd, "-h127.0.0.1") {
		t.Fatalf("must not reintroduce -h 127.0.0.1 (TCP), which fails against real MariaDB: %s", cmd)
	}
}

func TestDBDockerAccessStatusScriptUsesLocalSocketForMySQL(t *testing.T) {
	script := dbDockerAccessStatusScript("mysql")
	if strings.Contains(script, "-h 127.0.0.1") {
		t.Fatalf("docker-access status check must not use -h 127.0.0.1 for MySQL: %s", script)
	}
	if !strings.Contains(script, "mysql -u root") {
		t.Fatalf("expected local-socket mysql -u root in status script: %s", script)
	}
}

// Bootstrap akun root@127.0.0.1 berpassword kosong (dulu dibuat supaya
// mysql -h 127.0.0.1 -u root bisa login) sudah tidak diperlukan lagi sejak
// admin ops pindah ke unix socket — dan sebaiknya memang tidak pernah
// dibuat lagi (akun root ber-password kosong yang bisa dijangkau TCP,
// meski cuma loopback, adalah beban keamanan yang tidak perlu).
func TestDBInstallScriptDoesNotCreateEmptyPasswordRootAt127(t *testing.T) {
	for _, pm := range []string{"apt", "dnf", "yum"} {
		script, ok := dbInstallScript("mysql", pm, "")
		if !ok {
			t.Fatalf("dbInstallScript(mysql, %s, \"\") not ok", pm)
		}
		if strings.Contains(script, "root'@'127.0.0.1'") {
			t.Fatalf("pm=%s: must not bootstrap root@127.0.0.1 anymore: %s", pm, script)
		}
	}
}

func TestMariaDBVersionedInstallScriptDoesNotCreateEmptyPasswordRootAt127(t *testing.T) {
	script, ok := dbVersionedInstallScript("mysql", "apt", "mariadb-10.11", "")
	if !ok {
		t.Fatalf("dbVersionedInstallScript not ok")
	}
	if strings.Contains(script, "root'@'127.0.0.1'") {
		t.Fatalf("must not bootstrap root@127.0.0.1 anymore: %s", script)
	}
}
