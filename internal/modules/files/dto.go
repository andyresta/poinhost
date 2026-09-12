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
	AsUser   string `json:"asUser,omitempty"`
}

// CreateFileRequest memuat data pembuatan file kosong baru.
type CreateFileRequest struct {
	ServerID string `json:"serverId"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	AsUser   string `json:"asUser,omitempty"`
}

// RenameRequest memuat data rename/pindah file atau direktori.
type RenameRequest struct {
	ServerID string `json:"serverId"`
	OldPath  string `json:"oldPath"`
	NewPath  string `json:"newPath"`
	AsUser   string `json:"asUser,omitempty"`
}

// DeleteRequest memuat daftar path yang akan dihapus (file atau direktori,
// direktori dihapus rekursif).
type DeleteRequest struct {
	ServerID string   `json:"serverId"`
	Paths    []string `json:"paths"`
	AsUser   string   `json:"asUser,omitempty"`
}

// CompressRequest memuat data kompresi file/direktori menjadi satu arsip.
type CompressRequest struct {
	ServerID    string   `json:"serverId"`
	Sources     []string `json:"sources"`
	ArchivePath string   `json:"archivePath"`
	Format      string   `json:"format"` // "zip" | "tar.gz"
	AsUser      string   `json:"asUser,omitempty"`
}

// ExtractRequest memuat data ekstraksi arsip ke direktori tujuan.
type ExtractRequest struct {
	ServerID    string `json:"serverId"`
	ArchivePath string `json:"archivePath"`
	DestPath    string `json:"destPath"`
	AsUser      string `json:"asUser,omitempty"`
}

// ReadResult berisi isi file teks remote untuk diedit.
type ReadResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int64  `json:"size"`
}

// WriteFileRequest memuat isi baru file teks remote (simpan hasil edit).
type WriteFileRequest struct {
	ServerID string `json:"serverId"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	AsUser   string `json:"asUser,omitempty"`
}

// ChmodRequest memuat perubahan permission file/direktori remote.
type ChmodRequest struct {
	ServerID string `json:"serverId"`
	Path     string `json:"path"`
	Mode     string `json:"mode"` // string oktal, mis. "644" atau "0755"
	AsUser   string `json:"asUser,omitempty"`
}

// CopyRequest memuat data operasi salin file/direktori (beda dari
// Rename/move — sumber tetap ada, salinan baru dibuat di tujuan).
type CopyRequest struct {
	ServerID string   `json:"serverId"`
	Sources  []string `json:"sources"`
	DestPath string   `json:"destPath"`
	AsUser   string   `json:"asUser,omitempty"`
}

// SearchRequest memuat parameter pencarian file/direktori remote (rekursif
// dari Path, cocok substring case-insensitive terhadap Query).
type SearchRequest struct {
	ServerID string `json:"serverId"`
	Path     string `json:"path"`
	Query    string `json:"query"`
	AsUser   string `json:"asUser,omitempty"`
}

// SearchHit merepresentasikan satu hasil pencarian file.
type SearchHit struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
}

// SearchResult berisi seluruh hasil pencarian file remote.
type SearchResult struct {
	Query string      `json:"query"`
	Hits  []SearchHit `json:"hits"`
}

// SystemUser merepresentasikan satu user Linux di server remote yang bisa
// dipilih sebagai "jalankan sebagai" (lihat ListSystemUsers).
type SystemUser struct {
	Username string `json:"username"`
	Home     string `json:"home"`
}
