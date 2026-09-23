package sitexfer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/ssh"
)

type countingReader struct {
	r   io.Reader
	add func(int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 && c.add != nil {
		c.add(int64(n))
	}
	return n, err
}

// pipe menyambung stdout srcCmd (server asal) ke stdin dstCmd (server
// tujuan) lewat io.Pipe di memori proses ini — pola yang sama dengan
// filexfer/dbxfer: data tidak pernah ditulis ke disk komputer ini, dan
// memori yang dipakai sebatas potongan yang sedang lewat.
func pipe(ctx context.Context, srcClient, dstClient *ssh.Client, srcCmd, dstCmd string, add func(int64)) error {
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
	dstSess.Stdin = &countingReader{r: pr, add: add}
	dstSess.Stderr = &dstErrBuf

	if err := dstSess.Start(dstCmd); err != nil {
		return fmt.Errorf("mulai di server tujuan: %w", err)
	}
	if err := srcSess.Start(srcCmd); err != nil {
		_ = pw.CloseWithError(err)
		_ = dstSess.Wait()
		return fmt.Errorf("mulai di server asal: %w", err)
	}

	srcDone := make(chan error, 1)
	go func() {
		werr := srcSess.Wait()
		_ = pw.CloseWithError(werr)
		srcDone <- werr
	}()
	dstDone := make(chan error, 1)
	go func() {
		werr := dstSess.Wait()
		// Tujuan selesai (sukses atau gagal) sebelum sumber: tutup pembaca
		// supaya penulis di sisi sumber tidak memblokir selamanya.
		_ = pr.CloseWithError(io.ErrClosedPipe)
		dstDone <- werr
	}()

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
	case dstErr = <-dstDone:
		srcErr = <-srcDone
	case srcErr = <-srcDone:
		dstErr = <-dstDone
	}

	if dstErr != nil {
		return fmt.Errorf("di server tujuan: %s", firstNonEmpty(strings.TrimSpace(dstErrBuf.String()), dstErr.Error()))
	}
	if srcErr != nil {
		return fmt.Errorf("di server asal: %s", firstNonEmpty(strings.TrimSpace(srcErrBuf.String()), srcErr.Error()))
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

type sshClient = *ssh.Client
