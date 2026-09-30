package sitexfer

import (
	"strconv"
	"strings"
)

// destProbe kondisi satu folder tujuan di server TUJUAN.
type destProbe struct {
	// Exists folder itu sudah ada.
	Exists bool
	// NonEmpty folder itu ada DAN ada isinya. Ini yang penting, bukan
	// sekadar keberadaannya: folder kosong adalah tujuan yang wajar,
	// sedangkan folder berisi berarti hasil migrasi akan bercampur dengan
	// yang sudah ada di sana.
	NonEmpty bool
	// Mount titik mount filesystem yang menampung folder ini (atau leluhur
	// terdekatnya yang sudah ada, kalau foldernya sendiri belum dibuat).
	Mount string
	// Avail sisa ruang filesystem itu dalam byte.
	Avail int64
}

// destProbeScript memeriksa semua folder tujuan dalam SATU perintah.
//
// Sisa ruang dibaca dari leluhur terdekat yang sudah ada, bukan dari
// foldernya sendiri: folder tujuan biasanya memang belum dibuat, dan `df`
// atas path yang tidak ada hanya menghasilkan error. Yang ingin diketahui
// adalah filesystem mana yang akan menampungnya, dan itu ditentukan oleh
// leluhurnya.
//
// `df -P` (format POSIX) dipakai, bukan `df --output=` yang hanya ada di
// coreutils GNU — server tujuan belum tentu memakainya.
func destProbeScript(paths []string) string {
	var sb strings.Builder
	for _, p := range paths {
		q := shellQuote(p)
		sb.WriteString("p=" + q + "\n")
		sb.WriteString("printf 'PATH\\t%s\\n' \"$p\"\n")
		sb.WriteString("if [ -e \"$p\" ]; then\n")
		sb.WriteString("  printf 'EXISTS\\t1\\n'\n")
		// -print -quit berhenti pada entri pertama; `ls -A` akan membaca
		// seluruh isi folder hanya untuk tahu apakah ia kosong.
		sb.WriteString("  if [ -n \"$(find \"$p\" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null)\" ]; then printf 'NONEMPTY\\t1\\n'; else printf 'NONEMPTY\\t0\\n'; fi\n")
		sb.WriteString("else\n")
		sb.WriteString("  printf 'EXISTS\\t0\\n'\n")
		sb.WriteString("fi\n")
		sb.WriteString("d=\"$p\"\n")
		sb.WriteString("while [ ! -d \"$d\" ] && [ \"$d\" != \"/\" ]; do d=$(dirname \"$d\"); done\n")
		sb.WriteString("printf 'MOUNT\\t%s\\n' \"$(df -P \"$d\" 2>/dev/null | awk 'NR==2{print $6}')\"\n")
		sb.WriteString("printf 'AVAIL\\t%s\\n' \"$(df -Pk \"$d\" 2>/dev/null | awk 'NR==2{printf \"%.0f\", $4*1024}')\"\n")
	}
	sb.WriteString("true\n")
	return sb.String()
}

// parseDestProbe membaca keluaran destProbeScript menjadi peta path ->
// kondisinya.
func parseDestProbe(out string) map[string]destProbe {
	res := map[string]destProbe{}
	current := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		key, val, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch key {
		case "PATH":
			current = val
			if _, seen := res[current]; !seen {
				res[current] = destProbe{}
			}
		case "EXISTS":
			if p, ok := res[current]; ok {
				p.Exists = val == "1"
				res[current] = p
			}
		case "NONEMPTY":
			if p, ok := res[current]; ok {
				p.NonEmpty = val == "1"
				res[current] = p
			}
		case "MOUNT":
			if p, ok := res[current]; ok {
				p.Mount = val
				res[current] = p
			}
		case "AVAIL":
			if p, ok := res[current]; ok {
				p.Avail, _ = strconv.ParseInt(val, 10, 64)
				res[current] = p
			}
		}
	}
	return res
}

// spaceShortfall menghitung, per filesystem, berapa byte yang kurang.
//
// Dikelompokkan per mount point karena beberapa folder tujuan bisa berbagi
// filesystem yang sama: memeriksanya satu per satu akan menyatakan muat
// untuk masing-masing padahal jumlahnya tidak muat bersama-sama.
//
// Mengembalikan peta mount -> [dibutuhkan, tersedia] hanya untuk mount yang
// kekurangan.
func spaceShortfall(need map[string]int64, probes map[string]destProbe) map[string][2]int64 {
	perMount := map[string]int64{}
	availOf := map[string]int64{}
	for path, bytes := range need {
		pr, ok := probes[path]
		if !ok || pr.Mount == "" {
			continue
		}
		perMount[pr.Mount] += bytes
		availOf[pr.Mount] = pr.Avail
	}
	short := map[string][2]int64{}
	for mount, required := range perMount {
		avail := availOf[mount]
		// Avail 0 berarti `df` tidak terbaca, bukan berarti disknya penuh —
		// melaporkannya sebagai kekurangan akan memblokir migrasi atas
		// dasar pembacaan yang gagal.
		if avail <= 0 {
			continue
		}
		if required > avail {
			short[mount] = [2]int64{required, avail}
		}
	}
	return short
}
