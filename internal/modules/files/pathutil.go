package files

import (
	"errors"
	"path"
	"strings"
)

// NormalizePath menormalkan path absolut remote dan menolak path relatif.
func NormalizePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/", nil
	}
	if !strings.HasPrefix(raw, "/") {
		return "", errors.New("path harus absolut")
	}
	cleaned := path.Clean(raw)
	if cleaned == "." {
		return "/", nil
	}
	return cleaned, nil
}

// JoinPath menggabungkan direktori dengan satu nama entri baru secara aman
// (menolak "/", "." dan ".." supaya tidak bisa dipakai untuk traversal atau
// diam-diam menulis ke direktori lain).
func JoinPath(dir, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("nama wajib diisi")
	}
	if strings.Contains(name, "/") || name == "." || name == ".." {
		return "", errors.New("nama tidak valid")
	}
	dir, err := NormalizePath(dir)
	if err != nil {
		return "", err
	}
	if dir == "/" {
		return "/" + name, nil
	}
	return dir + "/" + name, nil
}

// ParentPath mengembalikan direktori induk dari path absolut.
func ParentPath(p string) (string, error) {
	p, err := NormalizePath(p)
	if err != nil {
		return "", err
	}
	if p == "/" {
		return "/", nil
	}
	parent := path.Dir(p)
	if parent == "." {
		return "/", nil
	}
	return parent, nil
}

// BaseName mengembalikan nama file/folder terakhir dari path.
func BaseName(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "/"
	}
	return path.Base(p)
}

// shellQuote membungkus argumen shell dengan kutip tunggal aman.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
