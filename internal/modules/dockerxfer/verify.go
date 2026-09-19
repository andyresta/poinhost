package dockerxfer

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/andyresta/poinhost/internal/modules/docker"
	"golang.org/x/crypto/ssh"
)

// Verifikasi di sini SENGAJA ringan: jumlah file + ukuran total per mount,
// dibandingkan sumber vs tujuan — BUKAN checksum per-file. homepoin tidak
// punya verifikasi apa pun (sukses di sana murni berarti perintah SSH-nya
// tidak keluar dengan exit code error); checksum penuh di sisi lain terlalu
// mahal untuk volume besar (harus membaca ulang semua data dua kali di
// kedua sisi). Jumlah file + ukuran adalah titik tengah yang disepakati:
// cukup untuk menangkap kasus nyata seperti "tar berhenti di tengah karena
// koneksi putus" atau "beberapa file tidak ke-tar karena permission",
// tanpa membaca ulang seluruh isi data.
type mountStat struct {
	files int64
	bytes int64
}

func statBindPath(ctx context.Context, client *ssh.Client, hostPath string) (mountStat, error) {
	sess, err := client.NewSession()
	if err != nil {
		return mountStat{}, err
	}
	defer sess.Close()
	cmd := fmt.Sprintf(
		"find %s -type f 2>/dev/null | wc -l; du -sb %s 2>/dev/null | cut -f1",
		shellQuote(hostPath), shellQuote(hostPath),
	)
	return runStatCmd(ctx, sess, cmd)
}

func statVolume(ctx context.Context, client *ssh.Client, dockerSvc *docker.Service, serverID, volumeName string) (mountStat, error) {
	script := fmt.Sprintf(
		"docker run --rm -v %s alpine sh -c 'find /dxfer_from -type f 2>/dev/null | wc -l; du -sb /dxfer_from 2>/dev/null | cut -f1'",
		shellQuote(volumeName+":/dxfer_from:ro"),
	)
	out, err := runWrapped(ctx, client, dockerSvc, serverID, script)
	if err != nil {
		return mountStat{}, err
	}
	return parseStatOutput(out), nil
}

func runStatCmd(ctx context.Context, sess *ssh.Session, cmd string) (mountStat, error) {
	type result struct {
		out string
		err error
	}
	ch := make(chan result, 1)
	var buf strings.Builder
	sess.Stdout = &stringWriter{&buf}
	go func() {
		err := sess.Run(cmd)
		ch <- result{buf.String(), err}
	}()
	select {
	case <-ctx.Done():
		_ = sess.Close()
		return mountStat{}, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return mountStat{}, r.err
		}
		return parseStatOutput(r.out), nil
	}
}

type stringWriter struct{ b *strings.Builder }

func (w *stringWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

func parseStatOutput(out string) mountStat {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var st mountStat
	if len(lines) > 0 {
		st.files, _ = strconv.ParseInt(strings.TrimSpace(lines[0]), 10, 64)
	}
	if len(lines) > 1 {
		st.bytes, _ = strconv.ParseInt(strings.TrimSpace(lines[1]), 10, 64)
	}
	return st
}

// verifyMount membandingkan jumlah file + ukuran total sumber vs tujuan
// untuk satu mount, dan mencatat hasilnya ke job (VerifyMatch/VerifyMismatch).
// Kegagalan MEMBACA statistik (bukan ketidakcocokan datanya) dicatat sebagai
// VerifySkipped dengan alasannya — supaya tidak disalahartikan sebagai
// "datanya beda", padahal cuma perintah stat-nya yang gagal.
func verifyMount(ctx context.Context, srcClient, dstClient *ssh.Client, dockerSvc *docker.Service, srcServerID, dstServerID string, m docker.MigrationMount, job *Job, itemKey string) {
	var srcStat, dstStat mountStat
	var err error

	if m.Named() {
		srcStat, err = statVolume(ctx, srcClient, dockerSvc, srcServerID, m.HostSide)
	} else {
		srcStat, err = statBindPath(ctx, srcClient, m.HostSide)
	}
	if err != nil {
		job.SetItemVerify(itemKey, VerifySkipped, "gagal membaca statistik sumber: "+err.Error())
		return
	}

	if m.Named() {
		dstStat, err = statVolume(ctx, dstClient, dockerSvc, dstServerID, m.HostSide)
	} else {
		dstStat, err = statBindPath(ctx, dstClient, m.HostSide)
	}
	if err != nil {
		job.SetItemVerify(itemKey, VerifySkipped, "gagal membaca statistik tujuan: "+err.Error())
		return
	}

	if srcStat.files == dstStat.files && srcStat.bytes == dstStat.bytes {
		job.SetItemVerify(itemKey, VerifyMatch, fmt.Sprintf("%d file, %s", srcStat.files, formatBytes(srcStat.bytes)))
		return
	}
	job.SetItemVerify(itemKey, VerifyMismatch, fmt.Sprintf(
		"sumber: %d file/%s, tujuan: %d file/%s",
		srcStat.files, formatBytes(srcStat.bytes), dstStat.files, formatBytes(dstStat.bytes),
	))
}

func formatBytes(n int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, units[i])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
