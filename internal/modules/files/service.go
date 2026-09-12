// Package files adalah file manager remote via SFTP (browse/upload/download/
// delete/rename/archive), vertical slice ketiga setelah servers & terminal.
//
// Upload & download SENGAJA tidak lewat base64/Blob ala aplikasi web —
// poinhost aplikasi desktop native, jadi keduanya memakai dialog OS asli
// (lihat app.go: runtime.OpenMultipleFilesDialog / runtime.SaveFileDialog)
// dan stream langsung file-descriptor-ke-file-descriptor lewat SFTP, tanpa
// pernah menampung seluruh isi file di memori JS maupun sebagai string
// base64 yang membengkak ~33%. Service ini sendiri cuma menerima PATH lokal
// (dipilih user lewat dialog) — bukan isi file — supaya tetap portable
// terhadap transport apa pun yang memanggilnya.
package files

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
)

// Service mengorkestrasi operasi file manager satu server via SFTP + exec
// (untuk operasi yang SFTP tidak punya primitifnya sendiri: hapus direktori
// tidak kosong, kompres/ekstrak arsip).
type Service struct {
	sftp     *sshpool.SFTPClient
	executor *sshpool.Executor
}

// NewService membuat files.Service baru.
func NewService(sftp *sshpool.SFTPClient, executor *sshpool.Executor) *Service {
	return &Service{sftp: sftp, executor: executor}
}

// List menampilkan isi satu direktori remote, direktori dulu baru file,
// masing-masing terurut nama (case-insensitive) — SFTP tidak menjamin
// urutan tertentu dari server, jadi diurutkan di sini supaya konsisten.
func (s *Service) List(ctx context.Context, serverID, rawPath string) (*ListResult, error) {
	dir, err := NormalizePath(rawPath)
	if err != nil {
		return nil, err
	}
	entries, err := s.sftp.ListDirectory(ctx, serverID, dir)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	parent, _ := ParentPath(dir)
	return &ListResult{Path: dir, Parent: parent, Entries: entries}, nil
}

// Mkdir membuat direktori baru (termasuk direktori induk yang belum ada).
func (s *Service) Mkdir(ctx context.Context, req MkdirRequest) error {
	dir, err := NormalizePath(req.Path)
	if err != nil {
		return err
	}
	target, err := JoinPath(dir, req.Name)
	if err != nil {
		return err
	}
	return s.sftp.MkdirAll(ctx, req.ServerID, target)
}

// CreateFile membuat file kosong baru di direktori yang sedang dibuka.
func (s *Service) CreateFile(ctx context.Context, req CreateFileRequest) error {
	dir, err := NormalizePath(req.Path)
	if err != nil {
		return err
	}
	target, err := JoinPath(dir, req.Name)
	if err != nil {
		return err
	}
	return s.sftp.WriteFile(ctx, req.ServerID, target, []byte{})
}

// Rename mengganti nama atau memindahkan file/direktori remote.
func (s *Service) Rename(ctx context.Context, req RenameRequest) error {
	oldPath, err := NormalizePath(req.OldPath)
	if err != nil {
		return err
	}
	newPath, err := NormalizePath(req.NewPath)
	if err != nil {
		return err
	}
	return s.sftp.Rename(ctx, req.ServerID, oldPath, newPath)
}

// Delete menghapus satu atau lebih file/direktori (direktori dihapus
// rekursif). Path yang gagal dihapus MENGHENTIKAN proses (bukan skip diam-
// diam) supaya user tahu persis mana yang bermasalah, bukan cuma sebagian.
func (s *Service) Delete(ctx context.Context, req DeleteRequest) error {
	if len(req.Paths) == 0 {
		return fmt.Errorf("tidak ada path yang dipilih")
	}
	for _, raw := range req.Paths {
		p, err := NormalizePath(raw)
		if err != nil {
			return err
		}
		if err := s.removePath(ctx, req.ServerID, p); err != nil {
			return fmt.Errorf("hapus %s: %w", p, err)
		}
	}
	return nil
}

// removePath mencoba hapus lewat SFTP dulu (cepat, untuk file & direktori
// kosong) — SFTP REMOVE gagal untuk direktori berisi, baru di situ fallback
// ke `rm -rf` via exec.
func (s *Service) removePath(ctx context.Context, serverID, p string) error {
	if err := s.sftp.Remove(ctx, serverID, p); err == nil {
		return nil
	}
	cmd := fmt.Sprintf("rm -rf %s", shellQuote(p))
	_, err := s.executor.Exec(ctx, serverID, 2*time.Minute, cmd, true)
	return err
}

// Compress mengarsipkan satu/lebih file/direktori menjadi satu file zip
// atau tar.gz ("gzip") via SSH exec (SFTP tidak punya primitif arsip).
func (s *Service) Compress(ctx context.Context, req CompressRequest) error {
	if len(req.Sources) == 0 {
		return fmt.Errorf("tidak ada sumber yang dipilih")
	}
	archive, err := NormalizePath(req.ArchivePath)
	if err != nil {
		return err
	}

	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = "zip"
	}

	quoted := make([]string, 0, len(req.Sources))
	for _, raw := range req.Sources {
		p, err := NormalizePath(raw)
		if err != nil {
			return err
		}
		quoted = append(quoted, shellQuote(p))
	}

	var cmd string
	switch format {
	case "zip":
		cmd = buildZipCompressCmd(archive, quoted)
	case "tar.gz", "tgz":
		cmd = fmt.Sprintf("tar -czf %s %s", shellQuote(archive), strings.Join(quoted, " "))
	default:
		return fmt.Errorf("format arsip tidak didukung: %s", format)
	}

	_, err = s.executor.Exec(ctx, req.ServerID, 10*time.Minute, cmd, true)
	return err
}

// Extract mengekstrak arsip zip atau tar.gz ke direktori tujuan (dibuat
// dulu kalau belum ada). Format ditentukan dari ekstensi nama arsip.
func (s *Service) Extract(ctx context.Context, req ExtractRequest) error {
	archive, err := NormalizePath(req.ArchivePath)
	if err != nil {
		return err
	}
	dest, err := NormalizePath(req.DestPath)
	if err != nil {
		return err
	}

	lower := strings.ToLower(archive)
	var cmd string
	switch {
	case strings.HasSuffix(lower, ".zip"):
		cmd = fmt.Sprintf("mkdir -p %s && ", shellQuote(dest)) + buildZipExtractCmd(archive, dest)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		cmd = fmt.Sprintf("mkdir -p %s && tar -xzf %s -C %s", shellQuote(dest), shellQuote(archive), shellQuote(dest))
	default:
		return fmt.Errorf("format arsip tidak dikenali dari nama file: %s", BaseName(archive))
	}

	_, err = s.executor.Exec(ctx, req.ServerID, 10*time.Minute, cmd, true)
	return err
}

// UploadFromLocalPath meng-upload satu file LOKAL (dipilih user lewat
// dialog OS, lihat app.go) ke direktori remote yang sedang dibuka, memakai
// nama file aslinya.
func (s *Service) UploadFromLocalPath(ctx context.Context, serverID, localPath, remoteDir string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("buka file lokal: %w", err)
	}
	defer f.Close()

	dir, err := NormalizePath(remoteDir)
	if err != nil {
		return err
	}
	target, err := JoinPath(dir, filepath.Base(localPath))
	if err != nil {
		return err
	}

	return s.sftp.UploadStream(ctx, serverID, target, f)
}

// DownloadToLocalPath mengunduh satu file remote ke path LOKAL yang dipilih
// user lewat dialog "Simpan" OS asli (lihat app.go), stream langsung tanpa
// buffer perantara.
func (s *Service) DownloadToLocalPath(ctx context.Context, serverID, remotePath, localPath string) error {
	p, err := NormalizePath(remotePath)
	if err != nil {
		return err
	}
	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("buat file lokal: %w", err)
	}
	defer f.Close()

	if err := s.sftp.DownloadFile(ctx, serverID, p, f); err != nil {
		_ = os.Remove(localPath)
		return err
	}
	return nil
}
