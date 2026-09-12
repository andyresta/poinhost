package files

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// pythonZipCreateScript membuat arsip .zip (setara zip -r) via Python stdlib
// — dipakai kalau binary `zip` tidak terpasang di VPS. Diporting dari
// homepoin, sudah teruji menangani folder kosong (zip -r biasa melewatkannya).
// argv: [archive, source1, source2, ...]
const pythonZipCreateScript = `
import zipfile, os, sys
archive = sys.argv[1]
with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as zf:
    for src in sys.argv[2:]:
        src = os.path.abspath(src)
        parent = os.path.dirname(src)
        if os.path.isdir(src):
            for root, _dirs, files in os.walk(src):
                for name in files:
                    path = os.path.join(root, name)
                    zf.write(path, os.path.relpath(path, parent))
                if not files and not _dirs:
                    arc = os.path.relpath(root, parent) + "/"
                    zf.writestr(arc, "")
        elif os.path.isfile(src):
            zf.write(src, os.path.basename(src))
        else:
            raise SystemExit("sumber tidak ditemukan: " + src)
`

// pythonZipExtractScript mengekstrak .zip ke direktori tujuan (setara unzip -o -d).
// argv: [archive, dest]
const pythonZipExtractScript = `
import zipfile, os, sys
archive, dest = sys.argv[1], sys.argv[2]
os.makedirs(dest, exist_ok=True)
with zipfile.ZipFile(archive, "r") as zf:
    zf.extractall(dest)
`

// buildZipCompressCmd membangun command compress zip dengan fallback python3
// bila binary `zip` tidak terpasang di server (exit 127 / command not found).
func buildZipCompressCmd(archive string, quotedSources []string) string {
	archiveQ := shellQuote(archive)
	joined := strings.Join(quotedSources, " ")
	pyB64 := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(pythonZipCreateScript) + "\n"))
	return fmt.Sprintf(
		`if command -v zip >/dev/null 2>&1; then zip -r %s %s; `+
			`elif command -v python3 >/dev/null 2>&1; then echo %s | base64 -d | python3 - %s %s; `+
			`elif command -v python >/dev/null 2>&1; then echo %s | base64 -d | python - %s %s; `+
			`else echo 'Perintah zip tidak ditemukan di server. Pasang dengan: apt install zip  (atau pastikan python3 tersedia).' >&2; exit 127; fi`,
		archiveQ, joined,
		shellQuote(pyB64), archiveQ, joined,
		shellQuote(pyB64), archiveQ, joined,
	)
}

// buildZipExtractCmd membangun command extract zip dengan fallback python3
// bila binary `unzip` tidak terpasang.
func buildZipExtractCmd(archive, dest string) string {
	archiveQ := shellQuote(archive)
	destQ := shellQuote(dest)
	pyB64 := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(pythonZipExtractScript) + "\n"))
	return fmt.Sprintf(
		`if command -v unzip >/dev/null 2>&1; then unzip -o %s -d %s; `+
			`elif command -v python3 >/dev/null 2>&1; then echo %s | base64 -d | python3 - %s %s; `+
			`elif command -v python >/dev/null 2>&1; then echo %s | base64 -d | python - %s %s; `+
			`else echo 'Perintah unzip tidak ditemukan di server. Pasang dengan: apt install unzip  (atau pastikan python3 tersedia).' >&2; exit 127; fi`,
		archiveQ, destQ,
		shellQuote(pyB64), archiveQ, destQ,
		shellQuote(pyB64), archiveQ, destQ,
	)
}
