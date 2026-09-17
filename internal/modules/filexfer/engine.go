package filexfer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

// countingReader membungkus reader pipe dan melaporkan setiap byte yang
// lewat ke job, supaya progress dihitung dari data yang BENAR-BENAR
// tersalurkan ke server tujuan, bukan dari tebakan di sisi sumber.
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

// scanSize menaksir ukuran mentah satu path sumber lewat `du -sb`. Sifatnya
// best-effort: kalau gagal (du tidak ada, path tidak terbaca) hasilnya 0 dan
// transfer tetap jalan — hanya persentasenya yang tidak tersedia.
func scanSize(ctx context.Context, client *ssh.Client, srcPath string) int64 {
	if client == nil {
		return 0
	}
	sess, err := client.NewSession()
	if err != nil {
		return 0
	}
	defer sess.Close()

	var out bytes.Buffer
	sess.Stdout = &out
	cmd := "du -sb " + shellQuote(srcPath) + " 2>/dev/null | cut -f1"

	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()

	select {
	case <-ctx.Done():
		_ = sess.Close()
		return 0
	case err := <-done:
		if err != nil {
			return 0
		}
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(out.String()), 10, 64)
	return n
}

// buildTarSrcCmd menyusun perintah tar di sisi sumber yang menulis arsip ke
// stdout. `-C <induk> <nama>` dipakai (bukan tar atas path absolut) supaya
// entri di dalam arsip relatif terhadap induknya: mengekstraknya di tujuan
// menghasilkan <tujuan>/<nama>, bukan mereplikasi seluruh pohon path absolut.
func buildTarSrcCmd(srcPath string, exclude []string, compress bool) string {
	dir := path.Dir(srcPath)
	base := path.Base(srcPath)

	var sb strings.Builder
	sb.WriteString("tar c")
	if compress {
		sb.WriteString("z")
	}
	sb.WriteString("f - -C ")
	sb.WriteString(shellQuote(dir))
	for _, pat := range exclude {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		sb.WriteString(" --exclude=")
		sb.WriteString(shellQuote(pat))
	}
	sb.WriteString(" ")
	sb.WriteString(shellQuote(base))
	return sb.String()
}

// buildTarDstCmd menyusun perintah ekstrak di sisi tujuan yang membaca arsip
// dari stdin. Folder tujuan dibuat lebih dulu supaya transfer ke folder yang
// belum ada tidak gagal di byte pertama.
func buildTarDstCmd(destPath string, compress bool) string {
	x := "tar x"
	if compress {
		x += "z"
	}
	x += "f -"
	return "mkdir -p " + shellQuote(destPath) + " && " + x + " -C " + shellQuote(destPath)
}

// runOnePath menyalurkan satu path sumber: `tar c` di server asal menulis ke
// stdout, disambung lewat io.Pipe di memori proses ini, langsung masuk ke
// stdin `tar x` di server tujuan.
//
// Arsipnya tidak pernah menyentuh disk komputer ini dan tidak pernah
// terkumpul utuh di memori — yang ada cuma potongan yang sedang lewat pipe,
// jadi memindahkan folder 50 GB memakai memori yang sama dengan memindahkan
// folder 50 MB.
func runOnePath(ctx context.Context, srcClient, dstClient *ssh.Client, srcPath, destPath string, exclude []string, compress bool, job *Job) error {
	if srcClient == nil || dstClient == nil {
		return fmt.Errorf("koneksi SSH tidak tersedia")
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

	// Sisi tujuan dinyalakan DULU: begitu sisi sumber mulai menulis, harus
	// sudah ada yang membaca, kalau tidak tar sumber memblokir di write.
	if err := dstSess.Start(buildTarDstCmd(destPath, compress)); err != nil {
		return fmt.Errorf("mulai ekstrak di server tujuan: %w", err)
	}
	if err := srcSess.Start(buildTarSrcCmd(srcPath, exclude, compress)); err != nil {
		_ = pw.CloseWithError(err)
		_ = dstSess.Wait()
		return fmt.Errorf("mulai arsip di server asal: %w", err)
	}

	srcDone := make(chan error, 1)
	go func() {
		werr := srcSess.Wait()
		// Menutup ujung tulis pipe adalah yang memberi EOF ke tar tujuan;
		// kalau sumber gagal, error-nya ikut dipropagasi supaya tujuan
		// berhenti dengan error, bukan menganggap arsipnya lengkap.
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
			// Tujuan mati duluan (disk penuh, tidak punya izin tulis):
			// putuskan pipe supaya tar sumber tidak menunggu selamanya
			// pada write ke pembaca yang sudah tidak ada.
			_ = pr.CloseWithError(e)
		}
		srcErr = <-srcDone
	case e := <-srcDone:
		srcErr = e
		dstErr = <-dstDone
	}

	if srcErr != nil {
		return fmt.Errorf("tar di server asal gagal: %s", firstNonEmpty(strings.TrimSpace(srcErrBuf.String()), srcErr.Error()))
	}
	if dstErr != nil {
		return fmt.Errorf("tar di server tujuan gagal: %s", firstNonEmpty(strings.TrimSpace(dstErrBuf.String()), dstErr.Error()))
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
