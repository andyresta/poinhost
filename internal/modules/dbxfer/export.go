package dbxfer

import (
	"context"

	"github.com/andyresta/poinhost/internal/modules/website"
	"golang.org/x/crypto/ssh"
)

// Fungsi-fungsi di sini adalah seam untuk modul migrasi website
// (internal/modules/sitexfer), yang memindahkan database sebagai BAGIAN
// dari satu situs — dump/restore/verifikasinya harus identik dengan
// migrasi database mandiri, jadi dipinjam dari sini, bukan disalin.

// DumpCommand perintah dump satu database utuh (belum dibungkus sudo).
func DumpCommand(engine, database string, gtidPurgedSupported bool) string {
	return buildDumpCmd(engine, database, nil, gtidPurgedSupported)
}

// RestoreCommand perintah restore satu database (belum dibungkus sudo;
// membaca dump dari stdin — bungkus dengan website.WrapStreamingCommand).
func RestoreCommand(engine, database string) string {
	return buildRestoreCmd(engine, database)
}

// MysqldumpSupportsGtidPurged lihat mysqldumpSupportsGtidPurged.
func MysqldumpSupportsGtidPurged(ctx context.Context, client *ssh.Client) bool {
	return mysqldumpSupportsGtidPurged(ctx, client)
}

// VerifyDatabase membandingkan jumlah baris SEMUA tabel sumber vs tujuan.
func VerifyDatabase(websiteSvc *website.Service, srcServerID, dstServerID, engine, database string) (verify, detail string) {
	tables, err := websiteSvc.DBTableNames(srcServerID, engine, database)
	if err != nil {
		return VerifySkipped, "gagal membaca daftar tabel: " + err.Error()
	}
	return verifyDatabase(websiteSvc, srcServerID, dstServerID, engine, database, database, tables)
}
