package sshpool

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPClient membungkus operasi transfer file via SFTP, memakai slot shared
// pool (multiplexed) supaya tidak menambah koneksi TCP baru per operasi.
type SFTPClient struct {
	pool *Pool
}

// NewSFTPClient membuat klien SFTP baru.
func NewSFTPClient(pool *Pool) *SFTPClient {
	return &SFTPClient{pool: pool}
}

// FileEntry merepresentasikan satu entri direktori remote.
type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	Mode    string `json:"mode"`
	ModTime string `json:"modTime"`
}

// ListDirectory menampilkan isi direktori remote.
func (c *SFTPClient) ListDirectory(ctx context.Context, serverID, path string) ([]FileEntry, error) {
	client, conn, err := c.open(ctx, serverID)
	if err != nil {
		return nil, err
	}
	defer c.close(serverID, conn, client)

	entries, err := client.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("baca direktori: %w", err)
	}

	out := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, FileEntry{
			Name:    e.Name(),
			Path:    joinPath(path, e.Name()),
			Size:    e.Size(),
			IsDir:   e.IsDir(),
			Mode:    e.Mode().String(),
			ModTime: e.ModTime().Format(time.RFC3339),
		})
	}
	return out, nil
}

// ReadFile membaca isi file teks dari remote.
func (c *SFTPClient) ReadFile(ctx context.Context, serverID, path string) ([]byte, error) {
	client, conn, err := c.open(ctx, serverID)
	if err != nil {
		return nil, err
	}
	defer c.close(serverID, conn, client)

	f, err := client.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// WriteFile menulis isi file teks ke remote.
func (c *SFTPClient) WriteFile(ctx context.Context, serverID, path string, data []byte) error {
	client, conn, err := c.open(ctx, serverID)
	if err != nil {
		return err
	}
	defer c.close(serverID, conn, client)

	f, err := client.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// UploadStream mengunggah file dari reader ke remote secara streaming.
func (c *SFTPClient) UploadStream(ctx context.Context, serverID, path string, r io.Reader) error {
	client, conn, err := c.open(ctx, serverID)
	if err != nil {
		return err
	}
	defer c.close(serverID, conn, client)

	f, err := client.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

// DownloadFile mengunduh file remote ke writer lokal.
func (c *SFTPClient) DownloadFile(ctx context.Context, serverID, path string, w io.Writer) error {
	client, conn, err := c.open(ctx, serverID)
	if err != nil {
		return err
	}
	defer c.close(serverID, conn, client)

	f, err := client.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// Mkdir membuat direktori baru di remote.
func (c *SFTPClient) Mkdir(ctx context.Context, serverID, path string) error {
	client, conn, err := c.open(ctx, serverID)
	if err != nil {
		return err
	}
	defer c.close(serverID, conn, client)
	return client.Mkdir(path)
}

// MkdirAll membuat rantai direktori remote (setara mkdir -p).
func (c *SFTPClient) MkdirAll(ctx context.Context, serverID, path string) error {
	client, conn, err := c.open(ctx, serverID)
	if err != nil {
		return err
	}
	defer c.close(serverID, conn, client)
	return client.MkdirAll(path)
}

// Rename mengganti nama atau memindahkan file/direktori remote.
func (c *SFTPClient) Rename(ctx context.Context, serverID, oldPath, newPath string) error {
	client, conn, err := c.open(ctx, serverID)
	if err != nil {
		return err
	}
	defer c.close(serverID, conn, client)
	return client.Rename(oldPath, newPath)
}

// Remove menghapus file atau direktori kosong di remote.
func (c *SFTPClient) Remove(ctx context.Context, serverID, path string) error {
	client, conn, err := c.open(ctx, serverID)
	if err != nil {
		return err
	}
	defer c.close(serverID, conn, client)
	return client.Remove(path)
}

// Chmod mengubah permission file remote.
func (c *SFTPClient) Chmod(ctx context.Context, serverID, path string, mode os.FileMode) error {
	client, conn, err := c.open(ctx, serverID)
	if err != nil {
		return err
	}
	defer c.close(serverID, conn, client)
	return client.Chmod(path, mode)
}

func (c *SFTPClient) open(ctx context.Context, serverID string) (*sftp.Client, *ssh.Client, error) {
	conn, err := c.pool.Acquire(ctx, serverID, SlotShared)
	if err != nil {
		return nil, nil, err
	}
	client, err := sftp.NewClient(conn)
	if err != nil {
		c.pool.Release(serverID, SlotShared, conn)
		return nil, nil, err
	}
	return client, conn, nil
}

func (c *SFTPClient) close(serverID string, conn *ssh.Client, client *sftp.Client) {
	_ = client.Close()
	c.pool.Release(serverID, SlotShared, conn)
}

func joinPath(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}
