package dbxfer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/andyresta/poinhost/internal/modules/website"
	"golang.org/x/crypto/ssh"
)

// countingReader membungkus reader pipe dan melaporkan setiap byte yang
// lewat ke job — pola identik filexfer/dockerxfer.
type countingReader struct {
	r   io.Reader
	job *Job
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 && c.job != nil {
		c.job.AddDoneBytes(int64(n))
	}
	return n, err
}

// buildDumpCmd menyusun pipeline dump SIAP dikirim ke website.Service.
// WrapCommand (yang membungkusnya lewat sudo bila perlu) — sudah termasuk
// prefix "set -o pipefail" (TANPA ini, "mysqldump | gzip" yang gagal di
// tengah tetap keluar exit 0 karena shell hanya melihat exit code proses
// TERAKHIR dalam pipeline; ini bug nyata yang pernah membuat homepoin
// melaporkan migrasi kosong sebagai sukses — lihat ARCHITECTURE.md).
//
// Dump DAN restore di sini berjalan sebagai root (MySQL lewat unix socket,
// sama seperti seluruh modul Database poinhost lainnya) / user OS
// `postgres` (PostgreSQL) — BUKAN sebagai user aplikasi tertentu seperti
// homepoin. Konsekuensinya: tidak perlu kredensial apa pun (tidak ada file
// .cnf/.pgpass sementara di server), dan tidak perlu strip DEFINER=user@host
// dari hasil dump (homepoin melakukan itu karena restore sebagai user
// aplikasi biasa butuh privilege SUPER/SET_USER_ID untuk objek yang
// definer-nya root; root/postgres di sini sudah punya privilege itu).
func buildDumpCmd(engine, database string, tables []string, gtidPurgedSupported bool) string {
	if engine == "mysql" {
		var sb strings.Builder
		sb.WriteString("set -o pipefail; mysqldump -u root --single-transaction --quick --routines --triggers --events --add-drop-table --no-tablespaces")
		if gtidPurgedSupported {
			sb.WriteString(" --set-gtid-purged=OFF")
		}
		sb.WriteString(" ")
		sb.WriteString(shellQuote(database))
		for _, t := range tables {
			sb.WriteString(" ")
			sb.WriteString(shellQuote(t))
		}
		sb.WriteString(" | gzip -c")
		return sb.String()
	}
	// PostgreSQL: --clean --if-exists membuat pg_dump menyisipkan DROP ...
	// IF EXISTS sebelum tiap CREATE, menyamakan perilakunya dengan
	// --add-drop-table di MySQL. TANPA ini (kondisi asli homepoin), restore
	// ulang ke database tujuan yang sudah pernah diisi GAGAL TOTAL di objek
	// pertama yang bentrok ("relation already exists"), karena ON_ERROR_STOP=1
	// menghentikan seluruh restore di kegagalan pertama — beda perlakuan yang
	// tidak disengaja dibanding MySQL, diperbaiki di sini.
	var sb strings.Builder
	sb.WriteString("set -o pipefail; sudo -u postgres pg_dump --clean --if-exists -d ")
	sb.WriteString(shellQuote(database))
	for _, t := range tables {
		sb.WriteString(" -t ")
		sb.WriteString(shellQuote(t))
	}
	sb.WriteString(" | gzip -c")
	return sb.String()
}

// buildRestoreCmd menyusun pipeline restore.
func buildRestoreCmd(engine, destDatabase string) string {
	if engine == "mysql" {
		return "set -o pipefail; gunzip -c | mysql -u root " + shellQuote(destDatabase)
	}
	return "set -o pipefail; gunzip -c | sudo -u postgres psql -d " + shellQuote(destDatabase) + " -v ON_ERROR_STOP=1 -q"
}

// mysqldumpSupportsGtidPurged mendeteksi apakah `mysqldump` di server SUMBER
// mengenal flag --set-gtid-purged, sebelum memakainya. MariaDB versi lama
// TIDAK mengenalinya dan mati dengan "unknown variable" kalau flag ini
// dipaksakan begitu saja — homepoin pernah kena regresi nyata ini.
// Probe dijalankan LANGSUNG (tanpa sudo) karena cuma membaca --help sebuah
// binary, tidak menyentuh apa pun yang butuh privilege.
func mysqldumpSupportsGtidPurged(ctx context.Context, client *ssh.Client) bool {
	if client == nil {
		return false
	}
	sess, err := client.NewSession()
	if err != nil {
		return false
	}
	defer sess.Close()
	done := make(chan error, 1)
	go func() { done <- sess.Run("mysqldump --help 2>&1 | grep -q -- --set-gtid-purged") }()
	select {
	case <-ctx.Done():
		_ = sess.Close()
		return false
	case err := <-done:
		return err == nil
	}
}

// runOneItem menyalurkan satu database: dump di server asal menulis ke
// stdout, disambung lewat io.Pipe di memori proses ini, langsung masuk ke
// stdin restore di server tujuan — pola dan alasannya identik dengan
// filexfer.runOnePath/dockerxfer.runOneMount: arsipnya tidak pernah
// menyentuh disk komputer ini.
func runOneItem(ctx context.Context, srcClient, dstClient *ssh.Client, websiteSvc *website.Service, srcServerID, dstServerID, engine, database, destDatabase string, tables []string, gtidPurgedSupported bool, job *Job) error {
	if srcClient == nil || dstClient == nil {
		return fmt.Errorf("koneksi SSH tidak tersedia")
	}

	dumpCmd, err := websiteSvc.WrapCommand(srcServerID, buildDumpCmd(engine, database, tables, gtidPurgedSupported))
	if err != nil {
		return fmt.Errorf("bungkus perintah dump: %w", err)
	}
	restoreCmd, err := websiteSvc.WrapCommand(dstServerID, buildRestoreCmd(engine, destDatabase))
	if err != nil {
		return fmt.Errorf("bungkus perintah restore: %w", err)
	}

	srcSess, err := srcClient.NewSession()
	if err != nil {
		return fmt.Errorf("buka sesi di server asal: %w", err)
	}
	defer srcSess.Close()

	dstSess, err := dstClient.NewSession()
	if err != nil {
		return fmt.Errorf("buka sesi di server tujuan: %w", err)
	}
	defer dstSess.Close()

	pr, pw := io.Pipe()

	var srcErrBuf, dstErrBuf bytes.Buffer
	srcSess.Stdout = pw
	srcSess.Stderr = &srcErrBuf
	dstSess.Stdin = &countingReader{r: pr, job: job}
	dstSess.Stderr = &dstErrBuf

	bytesBefore := job.doneBytes.Load()

	// Restore dinyalakan dulu — kalau tidak, dump di sisi sumber bisa
	// memblokir menunggu pembaca yang belum ada.
	if err := dstSess.Start(restoreCmd); err != nil {
		return fmt.Errorf("mulai restore di server tujuan: %w", err)
	}
	if err := srcSess.Start(dumpCmd); err != nil {
		_ = pw.CloseWithError(err)
		_ = dstSess.Wait()
		return fmt.Errorf("mulai dump di server asal: %w", err)
	}

	srcDone := make(chan error, 1)
	go func() {
		werr := srcSess.Wait()
		_ = pw.CloseWithError(werr)
		srcDone <- werr
	}()

	dstDone := make(chan error, 1)
	go func() { dstDone <- dstSess.Wait() }()

	var srcErr, dstErr error
	select {
	case <-ctx.Done():
		_ = srcSess.Close()
		_ = dstSess.Close()
		_ = pr.CloseWithError(ctx.Err())
		_ = pw.CloseWithError(ctx.Err())
		<-srcDone
		<-dstDone
		return ctx.Err()
	case e := <-dstDone:
		dstErr = e
		if e != nil {
			_ = pr.CloseWithError(e)
		}
		srcErr = <-srcDone
	case e := <-srcDone:
		srcErr = e
		dstErr = <-dstDone
	}

	if srcErr != nil {
		return fmt.Errorf("dump di server asal gagal: %s", firstNonEmpty(strings.TrimSpace(srcErrBuf.String()), srcErr.Error()))
	}
	if dstErr != nil {
		return fmt.Errorf("restore di server tujuan gagal: %s", firstNonEmpty(strings.TrimSpace(dstErrBuf.String()), dstErr.Error()))
	}

	// Guard "stream kosong": kedua sisi keluar dengan exit 0 TAPI nol byte
	// benar-benar tersalur — bisa terjadi pada kondisi tepi yang tidak
	// tertangkap pipefail (mis. seleksi tabel yang ternyata kosong semua).
	// Ini lapisan pertahanan KEDUA yang independen dari pipefail, sama
	// seperti di homepoin — sukses palsu jenis ini pernah benar-benar
	// terjadi di sana.
	if job.doneBytes.Load()-bytesBefore <= 0 {
		return fmt.Errorf("stream kosong: tidak ada data yang ditransfer (cek mysqldump/pg_dump terpasang di server dan nama tabel yang dipilih)")
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
