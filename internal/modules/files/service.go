// Package files adalah file manager remote via SFTP (browse/upload/download/
// delete/rename/archive/copy/search/chmod/edit), vertical slice ketiga
// setelah servers & terminal.
//
// Upload & download SENGAJA tidak lewat base64/Blob ala aplikasi web —
// poinhost aplikasi desktop native, jadi keduanya memakai dialog OS asli
// (lihat app.go: runtime.OpenMultipleFilesDialog / runtime.SaveFileDialog)
// dan stream langsung file-descriptor-ke-file-descriptor lewat SFTP kalau
// beroperasi sebagai user SSH sendiri. Kalau beroperasi "sebagai" user lain
// (asUser + sudo, lihat access.go/sudoops.go), SFTP tidak bisa dipakai sama
// sekali (subsystem SFTP selalu jalan sebagai user yang login SSH) — jalur
// itu jatuh ke shell command lewat sudo, dan TIDAK benar-benar streaming
// (isi file dibaca penuh ke memori dulu) — batasan yang diterima sama
// seperti homepoin, dicatat di ARCHITECTURE.md.
package files

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/servers"
)

// maxEditableFileSize membatasi ukuran file yang boleh dibuka di editor
// dalam app (sama seperti batas homepoin) — mencegah UI membeku menampung
// file besar di textarea/CodeMirror, dan mencegah salah pakai file manager
// ini untuk file biner besar yang memang bukan untuk diedit sebagai teks.
const maxEditableFileSize = 2 * 1024 * 1024 // 2MB

// Service mengorkestrasi operasi file manager satu server via SFTP + exec
// (untuk operasi yang SFTP tidak punya primitifnya sendiri: hapus direktori
// tidak kosong, kompres/ekstrak arsip, copy, search) — dan lewat sudo kalau
// beroperasi sebagai user selain yang login SSH (lihat access.go).
type Service struct {
	sftp     *sshpool.SFTPClient
	executor *sshpool.Executor
	servers  *servers.Service
}

// NewService membuat files.Service baru.
func NewService(sftp *sshpool.SFTPClient, executor *sshpool.Executor, serversSvc *servers.Service) *Service {
	return &Service{sftp: sftp, executor: executor, servers: serversSvc}
}

// List menampilkan isi satu direktori remote, direktori dulu baru file,
// masing-masing terurut nama (case-insensitive).
func (s *Service) List(ctx context.Context, serverID, rawPath, asUser string) (*ListResult, error) {
	dir, err := NormalizePath(rawPath)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(ctx, serverID, asUser)
	if err != nil {
		return nil, err
	}

	var entries []sshpool.FileEntry
	if access.viaSudo() {
		entries, err = s.listDirSudo(ctx, access, dir)
	} else {
		entries, err = s.sftp.ListDirectory(ctx, serverID, dir)
	}
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
	access, err := s.resolveAccess(ctx, req.ServerID, req.AsUser)
	if err != nil {
		return err
	}
	dir, err := NormalizePath(req.Path)
	if err != nil {
		return err
	}
	target, err := JoinPath(dir, req.Name)
	if err != nil {
		return err
	}
	if access.viaSudo() {
		return s.mkdirSudo(ctx, access, target)
	}
	return s.sftp.MkdirAll(ctx, req.ServerID, target)
}

// CreateFile membuat file kosong baru di direktori yang sedang dibuka.
func (s *Service) CreateFile(ctx context.Context, req CreateFileRequest) error {
	access, err := s.resolveAccess(ctx, req.ServerID, req.AsUser)
	if err != nil {
		return err
	}
	dir, err := NormalizePath(req.Path)
	if err != nil {
		return err
	}
	target, err := JoinPath(dir, req.Name)
	if err != nil {
		return err
	}
	if access.viaSudo() {
		return s.writeFileSudo(ctx, access, target, []byte{})
	}
	return s.sftp.WriteFile(ctx, req.ServerID, target, []byte{})
}

// Rename mengganti nama atau memindahkan file/direktori remote.
func (s *Service) Rename(ctx context.Context, req RenameRequest) error {
	access, err := s.resolveAccess(ctx, req.ServerID, req.AsUser)
	if err != nil {
		return err
	}
	oldPath, err := NormalizePath(req.OldPath)
	if err != nil {
		return err
	}
	newPath, err := NormalizePath(req.NewPath)
	if err != nil {
		return err
	}
	if access.viaSudo() {
		return s.renameSudo(ctx, access, oldPath, newPath)
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
	access, err := s.resolveAccess(ctx, req.ServerID, req.AsUser)
	if err != nil {
		return err
	}
	for _, raw := range req.Paths {
		p, err := NormalizePath(raw)
		if err != nil {
			return err
		}
		if err := s.removePath(ctx, access, p); err != nil {
			return fmt.Errorf("hapus %s: %w", p, err)
		}
	}
	return nil
}

// removePath mencoba hapus lewat SFTP dulu (cepat, untuk file & direktori
// kosong, kalau bukan via sudo) — SFTP Remove gagal untuk direktori berisi,
// baru di situ fallback ke `rm -rf` via exec. Jalur sudo selalu lewat exec
// langsung (SFTP tidak relevan sama sekali begitu beda user).
func (s *Service) removePath(ctx context.Context, access *fileAccess, p string) error {
	if access.viaSudo() {
		return s.removeSudo(ctx, access, p)
	}
	if err := s.sftp.Remove(ctx, access.serverID, p); err == nil {
		return nil
	}
	cmd := "rm -rf " + shellQuote(p)
	_, err := s.executor.Exec(ctx, access.serverID, 2*time.Minute, cmd, true)
	return err
}

// Compress mengarsipkan satu/lebih file/direktori menjadi satu file zip
// atau tar.gz ("gzip") via SSH exec — SFTP tidak punya primitif arsip sama
// sekali, jadi ini SELALU lewat execAccess (sudo-aware secara otomatis).
func (s *Service) Compress(ctx context.Context, req CompressRequest) error {
	access, err := s.resolveAccess(ctx, req.ServerID, req.AsUser)
	if err != nil {
		return err
	}
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

	_, err = s.execAccess(ctx, access, cmd, 10*time.Minute, true)
	return err
}

// Extract mengekstrak arsip zip atau tar.gz ke direktori tujuan (dibuat
// dulu kalau belum ada). Format ditentukan dari ekstensi nama arsip.
func (s *Service) Extract(ctx context.Context, req ExtractRequest) error {
	access, err := s.resolveAccess(ctx, req.ServerID, req.AsUser)
	if err != nil {
		return err
	}
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

	_, err = s.execAccess(ctx, access, cmd, 10*time.Minute, true)
	return err
}

// Copy menyalin satu/lebih file/direktori ke lokasi tujuan (sumber tetap
// ada, beda dari Rename). SFTP tidak punya primitif copy — selalu lewat
// `cp -r` via execAccess.
func (s *Service) Copy(ctx context.Context, req CopyRequest) error {
	access, err := s.resolveAccess(ctx, req.ServerID, req.AsUser)
	if err != nil {
		return err
	}
	if len(req.Sources) == 0 {
		return fmt.Errorf("tidak ada sumber yang dipilih")
	}
	dest, err := NormalizePath(req.DestPath)
	if err != nil {
		return err
	}

	quoted := make([]string, 0, len(req.Sources))
	for _, raw := range req.Sources {
		p, err := NormalizePath(raw)
		if err != nil {
			return err
		}
		quoted = append(quoted, shellQuote(p))
	}

	cmd := fmt.Sprintf("cp -r %s %s", strings.Join(quoted, " "), shellQuote(dest))
	_, err = s.execAccess(ctx, access, cmd, 5*time.Minute, true)
	return err
}

// Search mencari file/direktori di bawah Path yang namanya mengandung Query
// (case-insensitive, rekursif). Dibatasi 200 hasil supaya tidak membanjiri
// UI kalau direktori pencarian sangat besar.
func (s *Service) Search(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	access, err := s.resolveAccess(ctx, req.ServerID, req.AsUser)
	if err != nil {
		return nil, err
	}
	root, err := NormalizePath(req.Path)
	if err != nil {
		return nil, err
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, fmt.Errorf("kata kunci pencarian wajib diisi")
	}

	// find -iname pakai glob shell, bukan regex — tanda kutip tunggal di
	// query bisa lepas dari pembungkusnya, jadi dibuang saja (nama file
	// dengan kutip tunggal sangat jarang, tidak sepadan kompleksitasnya).
	pattern := strings.ReplaceAll(query, "'", "")
	cmd := fmt.Sprintf(
		`find %s -iname '*%s*' -printf '%%y\t%%p\n' 2>/dev/null | head -n 200`,
		shellQuote(root), pattern,
	)

	res, err := s.execAccess(ctx, access, cmd, 2*time.Minute, false)
	if err != nil {
		return nil, err
	}

	hits := make([]SearchHit, 0)
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		hits = append(hits, SearchHit{Path: parts[1], Name: BaseName(parts[1]), IsDir: parts[0] == "d"})
	}
	return &SearchResult{Query: query, Hits: hits}, nil
}

// ReadFile membaca isi file teks remote untuk ditampilkan di editor.
// Menolak file di atas maxEditableFileSize supaya UI tidak membeku.
func (s *Service) ReadFile(ctx context.Context, serverID, rawPath, asUser string) (*ReadResult, error) {
	access, err := s.resolveAccess(ctx, serverID, asUser)
	if err != nil {
		return nil, err
	}
	p, err := NormalizePath(rawPath)
	if err != nil {
		return nil, err
	}

	var data []byte
	if access.viaSudo() {
		data, err = s.readFileSudo(ctx, access, p)
	} else {
		data, err = s.sftp.ReadFile(ctx, serverID, p)
	}
	if err != nil {
		return nil, err
	}
	if len(data) > maxEditableFileSize {
		return nil, fmt.Errorf("file terlalu besar untuk diedit di aplikasi (maks %d MB)", maxEditableFileSize/1024/1024)
	}
	return &ReadResult{Path: p, Content: string(data), Size: int64(len(data))}, nil
}

// WriteFile menulis isi baru file teks remote (dipakai saat menyimpan hasil
// edit) — overwrite penuh, bukan patch, konsisten dengan CreateFile/upload.
func (s *Service) WriteFile(ctx context.Context, req WriteFileRequest) error {
	access, err := s.resolveAccess(ctx, req.ServerID, req.AsUser)
	if err != nil {
		return err
	}
	p, err := NormalizePath(req.Path)
	if err != nil {
		return err
	}
	if access.viaSudo() {
		return s.writeFileSudo(ctx, access, p, []byte(req.Content))
	}
	return s.sftp.WriteFile(ctx, req.ServerID, p, []byte(req.Content))
}

// Chmod mengubah permission file atau direktori remote.
func (s *Service) Chmod(ctx context.Context, req ChmodRequest) error {
	access, err := s.resolveAccess(ctx, req.ServerID, req.AsUser)
	if err != nil {
		return err
	}
	p, err := NormalizePath(req.Path)
	if err != nil {
		return err
	}
	if access.viaSudo() {
		return s.chmodSudo(ctx, access, p, req.Mode)
	}
	mode, err := parseFileMode(req.Mode)
	if err != nil {
		return err
	}
	return s.sftp.Chmod(ctx, req.ServerID, p, mode)
}

// parseFileMode mengurai string mode oktal (mis. "644" atau "0755").
func parseFileMode(raw string) (os.FileMode, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("mode permission wajib diisi")
	}
	raw = strings.TrimPrefix(raw, "0")
	n, err := strconv.ParseUint(raw, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("format permission tidak valid: %s", raw)
	}
	return os.FileMode(n), nil
}

// UploadFromLocalPath meng-upload satu file LOKAL (dipilih user lewat
// dialog OS, lihat app.go) ke direktori remote yang sedang dibuka, memakai
// nama file aslinya.
func (s *Service) UploadFromLocalPath(ctx context.Context, serverID, localPath, remoteDir, asUser string) error {
	access, err := s.resolveAccess(ctx, serverID, asUser)
	if err != nil {
		return err
	}
	dir, err := NormalizePath(remoteDir)
	if err != nil {
		return err
	}
	target, err := JoinPath(dir, filepath.Base(localPath))
	if err != nil {
		return err
	}

	if access.viaSudo() {
		// Sudo tidak bisa stream langsung (perlu lewat shell) — baca penuh
		// dulu ke memori, batasan yang diterima sama seperti homepoin.
		data, err := os.ReadFile(localPath)
		if err != nil {
			return fmt.Errorf("baca file lokal: %w", err)
		}
		return s.writeFileSudoViaTemp(ctx, access, target, data)
	}

	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("buka file lokal: %w", err)
	}
	defer f.Close()
	return s.sftp.UploadStream(ctx, serverID, target, f)
}

// DownloadToLocalPath mengunduh satu file remote ke path LOKAL yang dipilih
// user lewat dialog "Simpan" OS asli (lihat app.go), stream langsung tanpa
// buffer perantara (kecuali jalur sudo — lihat catatan di UploadFromLocalPath).
func (s *Service) DownloadToLocalPath(ctx context.Context, serverID, remotePath, localPath, asUser string) error {
	access, err := s.resolveAccess(ctx, serverID, asUser)
	if err != nil {
		return err
	}
	p, err := NormalizePath(remotePath)
	if err != nil {
		return err
	}

	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("buat file lokal: %w", err)
	}
	defer f.Close()

	if access.viaSudo() {
		data, err := s.readFileSudo(ctx, access, p)
		if err != nil {
			_ = os.Remove(localPath)
			return err
		}
		if _, err := f.Write(data); err != nil {
			_ = os.Remove(localPath)
			return err
		}
		return nil
	}

	if err := s.sftp.DownloadFile(ctx, serverID, p, f); err != nil {
		_ = os.Remove(localPath)
		return err
	}
	return nil
}
