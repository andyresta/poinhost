package files

import "github.com/andyresta/poinhost/internal/core/sshpool"

// ListResult berisi daftar entri direktori remote.
type ListResult struct {
	Path    string              `json:"path"`
	Parent  string              `json:"parent"`
	Entries []sshpool.FileEntry `json:"entries"`
}

// MkdirRequest memuat data pembuatan direktori baru.
type MkdirRequest struct {
	ServerID string `json:"serverId"`
	Path     string `json:"path"`
	Name     string `json:"name"`
}

// CreateFileRequest memuat data pembuatan file kosong baru.
type CreateFileRequest struct {
	ServerID string `json:"serverId"`
	Path     string `json:"path"`
	Name     string `json:"name"`
}

// RenameRequest memuat data rename/pindah file atau direktori.
type RenameRequest struct {
	ServerID string `json:"serverId"`
	OldPath  string `json:"oldPath"`
	NewPath  string `json:"newPath"`
}

// DeleteRequest memuat daftar path yang akan dihapus (file atau direktori,
// direktori dihapus rekursif).
type DeleteRequest struct {
	ServerID string   `json:"serverId"`
	Paths    []string `json:"paths"`
}

// CompressRequest memuat data kompresi file/direktori menjadi satu arsip.
type CompressRequest struct {
	ServerID    string   `json:"serverId"`
	Sources     []string `json:"sources"`
	ArchivePath string   `json:"archivePath"`
	Format      string   `json:"format"` // "zip" | "tar.gz"
}

// ExtractRequest memuat data ekstraksi arsip ke direktori tujuan.
type ExtractRequest struct {
	ServerID    string `json:"serverId"`
	ArchivePath string `json:"archivePath"`
	DestPath    string `json:"destPath"`
}
