package filexfer

import (
	"errors"
	"path"
	"sort"
	"strings"
)

// shellQuote membungkus string dengan single-quote yang aman untuk shell
// POSIX. Sama seperti helper bernama sama di modul lain — path sumber dan
// tujuan datang dari pilihan user di UI, jadi tidak pernah boleh
// disambung mentah ke dalam perintah tar.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// NormalizeRemotePath menormalkan path absolut di server remote. Path
// relatif ditolak (bukan diselesaikan terhadap home) supaya tidak ada
// perbedaan tafsir antara sisi sumber dan tujuan, dan komponen ".."
// ditolak sekalian walau path.Clean sudah membereskannya — sebuah path
// yang masih menyisakan ".." sesudah Clean berarti keluar dari root, dan
// itu tidak pernah merupakan pilihan yang disengaja dari file browser.
func NormalizeRemotePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/", nil
	}
	raw = strings.ReplaceAll(raw, "\\", "/")
	if !strings.HasPrefix(raw, "/") {
		return "", errors.New("path harus absolut (diawali /)")
	}
	cleaned := path.Clean(raw)
	if cleaned == "." {
		return "/", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("path tidak valid")
	}
	return cleaned, nil
}

// PruneRedundantPaths membuang path yang bertumpang tindih dengan path lain
// yang juga dipilih, supaya isinya tidak tersalin dua kali: sekali sebagai
// bagian arsip induknya, sekali lagi sebagai arsip tersendiri — kerja ganda
// yang juga membuat taksiran ukuran totalnya salah.
//
// Saat induk dan anak sama-sama dipilih, yang DIPERTAHANKAN adalah yang
// paling spesifik (anaknya) dan induknya dibuang — perilaku yang sama
// dengan homepoin. Konsekuensinya perlu disadari: isi lain di dalam induk
// tidak ikut pindah walau induknya tercentang.
func PruneRedundantPaths(paths []string) []string {
	if len(paths) == 0 {
		return paths
	}

	clean := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = path.Clean(p)
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		clean = append(clean, p)
	}
	if len(clean) <= 1 {
		return clean
	}

	// Terpanjang dulu supaya path yang paling spesifik masuk daftar hasil
	// lebih dulu, dan induknya yang menyusul tertolak karenanya.
	sort.Slice(clean, func(i, j int) bool {
		if len(clean[i]) != len(clean[j]) {
			return len(clean[i]) > len(clean[j])
		}
		return clean[i] < clean[j]
	})

	out := make([]string, 0, len(clean))
	for _, p := range clean {
		drop := false
		for _, kept := range out {
			// p induk dari kept, atau p anak dari kept, atau sama —
			// ketiganya berarti p tidak perlu diarsipkan tersendiri.
			if isUnder(kept, p) || isUnder(p, kept) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// isUnder melaporkan apakah child berada di dalam parent. Perbandingannya
// per-segmen (memakai pemisah "/"), bukan strings.HasPrefix telanjang,
// supaya "/var/www-lama" tidak dianggap berada di dalam "/var/www".
func isUnder(child, parent string) bool {
	if parent == child {
		return true
	}
	if parent == "/" {
		return true
	}
	return strings.HasPrefix(child, parent+"/")
}
