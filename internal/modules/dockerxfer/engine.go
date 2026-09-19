package dockerxfer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/andyresta/poinhost/internal/modules/docker"
	"golang.org/x/crypto/ssh"
)

// helperImage adalah image dipakai untuk membaca/menulis isi named volume
// lewat container sementara (tidak ada cara lain yang portable — lokasi
// filesystem sebenarnya sebuah named volume di host adalah detail internal
// Docker yang tidak boleh diasumsikan, lihat docker.IsNamedVolume).
//
// Asumsi yang perlu diketahui admin: server harus bisa menarik image ini
// kalau belum ada secara lokal (didokumentasikan di ARCHITECTURE.md) — sama
// seperti homepoin yang juga memakai container bantu untuk kasus ini.
const helperImage = "alpine:3"

// countingReader membungkus reader pipe dan melaporkan setiap byte yang
// lewat ke job, supaya progress dihitung dari data yang benar-benar
// tersalurkan, sama seperti filexfer.
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

// runWrapped menjalankan satu skrip docker-adjacent di sesi SSH baru pada
// client yang sudah dibuka, dibungkus lewat lapisan sudo server itu
// (docker.Service.WrapCommand) — dipakai untuk semua perintah yang
// menyentuh Docker di luar tar streaming (scan ukuran volume, verifikasi).
func runWrapped(ctx context.Context, client *ssh.Client, dockerSvc *docker.Service, serverID, script string) (string, error) {
	wrapped, err := dockerSvc.WrapCommand(serverID, script)
	if err != nil {
		return "", err
	}
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()

	var out, errBuf bytes.Buffer
	sess.Stdout = &out
	sess.Stderr = &errBuf

	done := make(chan error, 1)
	go func() { done <- sess.Run(wrapped) }()

	select {
	case <-ctx.Done():
		_ = sess.Close()
		return "", ctx.Err()
	case err := <-done:
		if err != nil {
			msg := strings.TrimSpace(errBuf.String())
			if msg == "" {
				msg = err.Error()
			}
			return out.String(), fmt.Errorf("%s", msg)
		}
	}
	return out.String(), nil
}

// scanMountSize menaksir ukuran mentah satu mount (bind lewat `du -sb`
// langsung, named volume lewat container bantu read-only). Best-effort:
// gagal berarti 0, transfer tetap jalan tanpa persentase yang akurat untuk
// item itu.
func scanMountSize(ctx context.Context, client *ssh.Client, dockerSvc *docker.Service, serverID string, m docker.MigrationMount) int64 {
	var script string
	if m.Named() {
		script = fmt.Sprintf("docker run --rm -v %s alpine du -sb /dxfer_from 2>/dev/null | cut -f1",
			shellQuote(m.HostSide+":/dxfer_from:ro"))
	} else {
		sess, err := client.NewSession()
		if err != nil {
			return 0
		}
		defer sess.Close()
		var out bytes.Buffer
		sess.Stdout = &out
		done := make(chan error, 1)
		go func() { done <- sess.Run("du -sb " + shellQuote(m.HostSide) + " 2>/dev/null | cut -f1") }()
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
	out, err := runWrapped(ctx, client, dockerSvc, serverID, script)
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	return n
}

// bindTarSrcCmd/bindTarDstCmd menyusun perintah tar langsung atas path host
// (bind mount) — identik dengan pola filexfer.
func bindTarSrcCmd(hostPath string, compress bool) string {
	x := "tar c"
	if compress {
		x += "z"
	}
	x += "f -"
	return x + " -C " + shellQuote(hostPath) + " ."
}

func bindTarDstCmd(hostPath string, compress bool) string {
	x := "tar x"
	if compress {
		x += "z"
	}
	x += "f -"
	return "mkdir -p " + shellQuote(hostPath) + " && " + x + " -C " + shellQuote(hostPath)
}

// volumeTarSrcCmd/volumeTarDstCmd menyusun perintah tar yang berjalan di
// DALAM container bantu, memasang named volume ke /dxfer_from (baca) atau
// /dxfer_to (tulis) — inilah yang membedakannya dari bind mount, yang
// tar-nya langsung membaca path host tanpa Docker terlibat sama sekali.
func volumeTarSrcCmd(volumeName string, compress bool) string {
	x := "tar c"
	if compress {
		x += "z"
	}
	x += "f -"
	return fmt.Sprintf("docker run --rm -v %s alpine %s -C /dxfer_from .",
		shellQuote(volumeName+":/dxfer_from:ro"), x)
}

func volumeTarDstCmd(volumeName string, compress bool) string {
	x := "tar x"
	if compress {
		x += "z"
	}
	x += "f -"
	return fmt.Sprintf("docker run -i --rm -v %s alpine %s -C /dxfer_to",
		shellQuote(volumeName+":/dxfer_to"), x)
}

// runOneMount menyalurkan satu mount (bind ATAU named volume) dari server
// asal ke server tujuan, lewat io.Pipe di memori proses ini — pola dan
// alasannya identik dengan filexfer.runOnePath: tidak pernah menyentuh disk
// atau memori komputer ini secara utuh, hanya potongan yang sedang lewat.
//
// Beda dari filexfer: kedua sisi kadang perlu dibungkus lewat sudo (kalau
// mount-nya named volume, karena harus lewat `docker run`) — makanya
// perintahnya sudah "final" (hasil WrapCommand) sebelum di-Start, bukan
// perintah tar telanjang.
func runOneMount(ctx context.Context, srcClient, dstClient *ssh.Client, dockerSvc *docker.Service, srcServerID, dstServerID string, m docker.MigrationMount, compress bool, job *Job) error {
	if srcClient == nil || dstClient == nil {
		return fmt.Errorf("koneksi SSH tidak tersedia")
	}

	var srcCmd, dstCmd string
	if m.Named() {
		if err := dockerSvc.EnsureNamedVolume(dstServerID, m.HostSide); err != nil {
			return fmt.Errorf("siapkan volume %s di tujuan: %w", m.HostSide, err)
		}
		srcCmd = volumeTarSrcCmd(m.HostSide, compress)
		dstCmd = volumeTarDstCmd(m.HostSide, compress)
	} else {
		srcCmd = bindTarSrcCmd(m.HostSide, compress)
		dstCmd = bindTarDstCmd(m.HostSide, compress)
	}

	srcWrapped, err := dockerSvc.WrapCommand(srcServerID, srcCmd)
	if err != nil {
		return fmt.Errorf("bungkus perintah sumber: %w", err)
	}
	dstWrapped, err := dockerSvc.WrapCommand(dstServerID, dstCmd)
	if err != nil {
		return fmt.Errorf("bungkus perintah tujuan: %w", err)
	}
	// Bind mount tidak butuh Docker sama sekali untuk membaca/menulis path
	// host-nya — tar langsung, tanpa lapisan sudo docker, sama seperti
	// filexfer memindahkan folder biasa (server perlu bisa baca/tulis path
	// itu lewat user SSH-nya, bukan lewat akses Docker).
	if !m.Named() {
		srcWrapped = srcCmd
		dstWrapped = dstCmd
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

	// Tujuan dinyalakan dulu — kalau tidak, tar/container bantu di sisi
	// sumber bisa memblokir menunggu pembaca yang belum ada.
	if err := dstSess.Start(dstWrapped); err != nil {
		return fmt.Errorf("mulai tulis di server tujuan: %w", err)
	}
	if err := srcSess.Start(srcWrapped); err != nil {
		_ = pw.CloseWithError(err)
		_ = dstSess.Wait()
		return fmt.Errorf("mulai baca di server asal: %w", err)
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
		return fmt.Errorf("baca di server asal gagal: %s", firstNonEmpty(strings.TrimSpace(srcErrBuf.String()), srcErr.Error()))
	}
	if dstErr != nil {
		return fmt.Errorf("tulis di server tujuan gagal: %s", firstNonEmpty(strings.TrimSpace(dstErrBuf.String()), dstErr.Error()))
	}
	return nil
}

// runImage menyalurkan konten image container. Kalau image punya
// RepoDigests (ditarik dari registry, bukan build lokal), sisi tujuan cukup
// `docker pull` — jauh lebih cepat dan tidak memakai bandwidth SSH sama
// sekali. Selain itu, fallback save|gzip → gunzip|load lewat io.Pipe yang
// sama dengan mount, TAPI (beda dari homepoin) kompresinya ikut flag
// `compress` yang sama dipakai mount, bukan selalu di-gzip terlepas dari
// pilihan user.
func runImage(ctx context.Context, srcClient, dstClient *ssh.Client, dockerSvc *docker.Service, srcServerID, dstServerID, image string, compress bool, job *Job) (skipped bool, err error) {
	already, err := dockerSvc.ImageExistsOnServer(dstServerID, image)
	if err == nil && already {
		return true, nil
	}

	hasDigest, _ := dockerSvc.ImageHasRegistryDigest(srcServerID, image)
	if hasDigest {
		if err := dockerSvc.PullImage(dstServerID, image); err == nil {
			return false, nil
		}
		// Pull gagal (registry tidak terjangkau dari tujuan, dsb) — jangan
		// menyerah, coba jalur streaming di bawah sebagai fallback yang
		// sama seperti dipakai image tanpa digest sama sekali.
	}

	if srcClient == nil || dstClient == nil {
		return false, fmt.Errorf("koneksi SSH tidak tersedia")
	}

	saveCmd := "docker save " + shellQuote(image)
	loadCmd := "docker load"
	if compress {
		saveCmd += " | gzip"
		loadCmd = "gunzip | " + loadCmd
	}

	srcWrapped, err := dockerSvc.WrapCommand(srcServerID, saveCmd)
	if err != nil {
		return false, fmt.Errorf("bungkus perintah save image: %w", err)
	}
	dstWrapped, err := dockerSvc.WrapCommand(dstServerID, loadCmd)
	if err != nil {
		return false, fmt.Errorf("bungkus perintah load image: %w", err)
	}

	srcSess, err := srcClient.NewSession()
	if err != nil {
		return false, fmt.Errorf("buka sesi di server asal: %w", err)
	}
	defer srcSess.Close()
	dstSess, err := dstClient.NewSession()
	if err != nil {
		return false, fmt.Errorf("buka sesi di server tujuan: %w", err)
	}
	defer dstSess.Close()

	pr, pw := io.Pipe()
	var srcErrBuf, dstErrBuf bytes.Buffer
	srcSess.Stdout = pw
	srcSess.Stderr = &srcErrBuf
	dstSess.Stdin = &countingReader{r: pr, job: job}
	dstSess.Stderr = &dstErrBuf

	if err := dstSess.Start(dstWrapped); err != nil {
		return false, fmt.Errorf("mulai docker load di tujuan: %w", err)
	}
	if err := srcSess.Start(srcWrapped); err != nil {
		_ = pw.CloseWithError(err)
		_ = dstSess.Wait()
		return false, fmt.Errorf("mulai docker save di asal: %w", err)
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
		return false, ctx.Err()
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
		return false, fmt.Errorf("docker save di server asal gagal: %s", firstNonEmpty(strings.TrimSpace(srcErrBuf.String()), srcErr.Error()))
	}
	if dstErr != nil {
		return false, fmt.Errorf("docker load di server tujuan gagal: %s", firstNonEmpty(strings.TrimSpace(dstErrBuf.String()), dstErr.Error()))
	}
	return false, nil
}

// mountHasExistingData memeriksa apakah sisi TUJUAN sebuah mount sudah
// berisi data sebelum transfer dimulai — dipakai untuk peringatan
// non-blocking (lihat catatan Warning di dto.go). homepoin menggabung/
// menimpa isi volume/bind tujuan yang sudah ada TANPA memeriksa atau
// memberitahu ini sama sekali; di sini setidaknya terlihat di UI sebelum
// datanya benar-benar tertimpa/tergabung.
func mountHasExistingData(ctx context.Context, client *ssh.Client, dockerSvc *docker.Service, serverID string, m docker.MigrationMount) bool {
	if m.Named() {
		script := fmt.Sprintf(
			"docker volume inspect %s >/dev/null 2>&1 || exit 0; "+
				"docker run --rm -v %s alpine sh -c '[ -n \"$(ls -A /dxfer_from 2>/dev/null)\" ] && echo yes || echo no'",
			shellQuote(m.HostSide), shellQuote(m.HostSide+":/dxfer_from:ro"),
		)
		out, err := runWrapped(ctx, client, dockerSvc, serverID, script)
		return err == nil && strings.TrimSpace(out) == "yes"
	}
	sess, err := client.NewSession()
	if err != nil {
		return false
	}
	defer sess.Close()
	var out bytes.Buffer
	sess.Stdout = &out
	cmd := fmt.Sprintf("[ -d %s ] && [ -n \"$(ls -A %s 2>/dev/null)\" ] && echo yes || echo no", shellQuote(m.HostSide), shellQuote(m.HostSide))
	if err := sess.Run(cmd); err != nil {
		return false
	}
	return strings.TrimSpace(out.String()) == "yes"
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
