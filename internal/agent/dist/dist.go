// Package dist menyediakan binary poinhost-agent yang dibundel ke dalam app
// desktop, supaya instalasi ke server tidak perlu mengunduh apa pun.
//
// Isinya dibangun CI saat rilis dan tidak di-commit; lihat README.md di folder
// ini untuk alasan dan cara membangunnya secara lokal.
package dist

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Yang di-embed adalah DIREKTORI bin/, bukan pola "*.gz".
//
// Bedanya menentukan: pola glob gagal saat kompilasi kalau tidak cocok dengan
// satu file pun, jadi `//go:embed *.gz` akan mematahkan `go build` di setiap
// clone bersih yang belum membangun binary agent. Embed direktori cukup
// membutuhkan satu file di dalamnya — bin/README.md yang memang di-commit —
// dan file .gz ikut terbawa begitu CI membuatnya.
//
//go:embed bin
var files embed.FS

// ErrNotBundled dikembalikan kalau build ini tidak membawa binary agent.
var ErrNotBundled = errors.New("build ini tidak membawa binary poinhost-agent")

// Arch adalah arsitektur target Linux yang didukung.
type Arch string

// Arsitektur yang dibundel.
const (
	AMD64 Arch = "amd64"
	ARM64 Arch = "arm64"
)

// ArchFromUname memetakan keluaran `uname -m` ke Arch.
func ArchFromUname(uname string) (Arch, error) {
	switch uname {
	case "x86_64", "amd64":
		return AMD64, nil
	case "aarch64", "arm64":
		return ARM64, nil
	default:
		return "", fmt.Errorf("arsitektur %q belum didukung agent (yang ada: x86_64 dan aarch64)", uname)
	}
}

func name(a Arch) string { return "bin/poinhost-agent-linux-" + string(a) + ".gz" }

// Available melaporkan apakah binary untuk arsitektur itu ikut dibundel.
func Available(a Arch) bool {
	_, err := files.Open(name(a))
	return err == nil
}

// Bundled melaporkan apakah build ini membawa binary untuk semua arsitektur.
func Bundled() bool { return Available(AMD64) && Available(ARM64) }

// Version mengembalikan versi binary agent yang dibundel, atau string kosong
// kalau build ini tidak membawanya.
//
// Versinya dibaca dari berkas yang ditulis `make agent` BERSAMAAN dengan
// binary-nya, bukan dari -ldflags terpisah saat app desktop dikompilasi. Dua
// sumber terpisah bisa menyimpang — dan versi yang salah di sini membuat UI
// menawarkan update yang keliru, atau menyembunyikan update yang seharusnya
// ada.
func Version() string {
	b, err := files.ReadFile("bin/version.txt")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Compressed mengembalikan binary agent APA ADANYA dalam bentuk ter-gzip,
// beserta checksum SHA-256 dari isi yang sudah didekompresi.
//
// Inilah yang dikirim ke server: bentuk terkompresinya kira-kira separuh
// ukuran binary utuh, dan unggahan SFTP adalah bagian terlama dari pemasangan
// maupun update. Checksum tetap dihitung atas isi FINAL, sehingga yang
// diverifikasi di server adalah binary yang benar-benar akan dipasang, bukan
// arsipnya.
func Compressed(a Arch) (gz []byte, sha256Hex string, err error) {
	raw, err := files.ReadFile(name(a))
	if err != nil {
		return nil, "", fmt.Errorf("%w (arsitektur %s)", ErrNotBundled, a)
	}
	_, sum, err := Binary(a)
	if err != nil {
		return nil, "", err
	}
	return raw, sum, nil
}

// Binary mengembalikan binary agent yang sudah didekompresi beserta checksum
// SHA-256-nya. Checksum dipakai untuk memverifikasi hasil unggahan di sisi
// server sebelum binary dipasang, bukan sekadar percaya SFTP selesai tanpa
// error.
func Binary(a Arch) (data []byte, sha256Hex string, err error) {
	f, err := files.Open(name(a))
	if err != nil {
		return nil, "", fmt.Errorf("%w (arsitektur %s)", ErrNotBundled, a)
	}
	defer func() { _ = f.Close() }()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, "", fmt.Errorf("baca binary agent %s: %w", a, err)
	}
	defer func() { _ = zr.Close() }()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, zr); err != nil {
		return nil, "", fmt.Errorf("dekompresi binary agent %s: %w", a, err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:]), nil
}
