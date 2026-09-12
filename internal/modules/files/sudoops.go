package files

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
)

// execAccess menjalankan satu perintah shell dengan atau tanpa sudo sesuai
// konteks akses — satu-satunya jalur yang dipakai operasi yang memang TIDAK
// punya padanan SFTP (Copy, Search), dan jalur fallback operasi lain saat
// `access.viaSudo()` true.
func (s *Service) execAccess(ctx context.Context, access *fileAccess, inner string, timeout time.Duration, destructive bool) (*sshpool.ExecResult, error) {
	cmd := access.wrapSudo(inner)
	res, err := s.executor.Exec(ctx, access.serverID, timeout, cmd, destructive)
	return res, mapSudoError(err)
}

// listDirSudo menampilkan isi direktori via `find` (bukan SFTP — SFTP
// subsystem selalu jalan sebagai user SSH yang login, tidak bisa "sebagai"
// user lain tanpa re-autentikasi).
func (s *Service) listDirSudo(ctx context.Context, access *fileAccess, dir string) ([]sshpool.FileEntry, error) {
	inner := fmt.Sprintf(
		`find %s -maxdepth 1 -mindepth 1 -printf '%%y\t%%m\t%%s\t%%T@\t%%f\n' 2>/dev/null`,
		shellQuote(dir),
	)
	res, err := s.execAccess(ctx, access, inner, 2*time.Minute, false)
	if err != nil {
		return nil, err
	}

	out := make([]sshpool.FileEntry, 0)
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}
		name := parts[4]
		if name == "" || name == "." || name == ".." {
			continue
		}
		size, _ := strconv.ParseInt(parts[2], 10, 64)
		modUnix, _ := strconv.ParseFloat(parts[3], 64)
		modTime := time.Unix(int64(modUnix), 0).UTC().Format(time.RFC3339)
		modeOct, _ := strconv.ParseUint(parts[1], 8, 32)
		out = append(out, sshpool.FileEntry{
			Name:    name,
			Path:    joinRemotePath(dir, name),
			Size:    size,
			IsDir:   parts[0] == "d",
			Mode:    fmt.Sprintf("%04o", modeOct),
			ModTime: modTime,
		})
	}
	return out, nil
}

// readFileSudo membaca file teks via `cat` sebagai user efektif.
func (s *Service) readFileSudo(ctx context.Context, access *fileAccess, p string) ([]byte, error) {
	inner := "cat -- " + shellQuote(p)
	res, err := s.execAccess(ctx, access, inner, 2*time.Minute, false)
	if err != nil {
		return nil, err
	}
	return []byte(res.Stdout), nil
}

// writeFileSudo menulis file teks via `tee` (base64 di tengah supaya isi
// file apa pun amannya lolos lewat shell tanpa masalah escaping/quoting).
// File besar lewat writeFileSudoViaTemp supaya tidak membuat command line
// raksasa.
func (s *Service) writeFileSudo(ctx context.Context, access *fileAccess, p string, data []byte) error {
	if len(data) > 512*1024 {
		return s.writeFileSudoViaTemp(ctx, access, p, data)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	inner := fmt.Sprintf("echo %s | base64 -d | tee %s > /dev/null", shellQuote(encoded), shellQuote(p))
	_, err := s.execAccess(ctx, access, inner, 2*time.Minute, true)
	return err
}

// writeFileSudoViaTemp menulis file besar: upload dulu ke lokasi sementara
// lewat SFTP sebagai user SSH sendiri (selalu boleh, tidak butuh sudo),
// baru `cp` ke tujuan sebagai user efektif via sudo, lalu hapus sementara.
func (s *Service) writeFileSudoViaTemp(ctx context.Context, access *fileAccess, dest string, data []byte) error {
	tmp := "/tmp/.poinhost-upload-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := s.sftp.WriteFile(ctx, access.serverID, tmp, data); err != nil {
		return err
	}
	defer func() {
		_, _ = s.execAccess(ctx, access, "rm -f "+shellQuote(tmp), 30*time.Second, true)
	}()
	inner := fmt.Sprintf("cp -f %s %s", shellQuote(tmp), shellQuote(dest))
	_, err := s.execAccess(ctx, access, inner, 2*time.Minute, true)
	return err
}

// mkdirSudo membuat direktori (termasuk induk) sebagai user efektif.
func (s *Service) mkdirSudo(ctx context.Context, access *fileAccess, p string) error {
	inner := "mkdir -p " + shellQuote(p)
	_, err := s.execAccess(ctx, access, inner, 2*time.Minute, true)
	return err
}

// renameSudo memindahkan/rename path sebagai user efektif.
func (s *Service) renameSudo(ctx context.Context, access *fileAccess, oldPath, newPath string) error {
	inner := fmt.Sprintf("mv %s %s", shellQuote(oldPath), shellQuote(newPath))
	_, err := s.execAccess(ctx, access, inner, 2*time.Minute, true)
	return err
}

// removeSudo menghapus path (rekursif) sebagai user efektif.
func (s *Service) removeSudo(ctx context.Context, access *fileAccess, p string) error {
	inner := "rm -rf " + shellQuote(p)
	_, err := s.execAccess(ctx, access, inner, 2*time.Minute, true)
	return err
}

// chmodSudo mengubah permission sebagai user efektif.
func (s *Service) chmodSudo(ctx context.Context, access *fileAccess, p string, mode string) error {
	inner := fmt.Sprintf("chmod %s %s", shellQuote(mode), shellQuote(p))
	_, err := s.execAccess(ctx, access, inner, 2*time.Minute, true)
	return err
}

func joinRemotePath(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}
