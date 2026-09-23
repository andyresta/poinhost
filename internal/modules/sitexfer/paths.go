package sitexfer

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/andyresta/poinhost/internal/modules/website"
)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// within: p sama dengan dir, atau berada di bawahnya.
func within(dir, p string) bool {
	dir, p = path.Clean(dir), path.Clean(p)
	return p == dir || strings.HasPrefix(p, dir+"/")
}

// forbiddenPaths folder yang tidak boleh di-tar utuh sebagai "folder
// situs" — document root yang menunjuk ke sini hampir pasti salah
// konfigurasi, dan mengekstraknya di tujuan bisa menimpa sistem.
var forbiddenPaths = map[string]bool{
	"/": true, "/bin": true, "/boot": true, "/dev": true, "/etc": true, "/home": true,
	"/lib": true, "/lib64": true, "/opt": true, "/proc": true, "/root": true, "/run": true,
	"/sbin": true, "/srv": true, "/sys": true, "/tmp": true, "/usr": true, "/var": true,
	"/var/www": true, "/var/lib": true, "/var/log": true,
}

func validateSitePath(p string) error {
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("folder %q bukan path absolut", p)
	}
	if forbiddenPaths[path.Clean(p)] {
		return fmt.Errorf("folder %q terlalu luas untuk disalin sebagai folder situs", p)
	}
	return nil
}

// computePaths menentukan folder yang disalin untuk domain terpilih.
//
//   - Domain induk: seluruh folder domainnya (/var/www/<domain>, induk
//     public_html) — di situlah subdomain berlayout standar juga tinggal,
//     jadi folder subdomain yang TIDAK dipilih dikecualikan dari tar.
//   - Subdomain yang induknya ikut dipilih dan foldernya di dalam folder
//     induk: sudah tercakup, tidak disalin dua kali.
//   - Subdomain tanpa induknya: hanya document root subdomain itu sendiri.
func computePaths(selected, all []website.DomainInfo) []PlanPath {
	sel := map[string]bool{}
	for _, d := range selected {
		sel[d.Domain] = true
	}
	bases := map[string]string{}
	for _, d := range selected {
		if !d.IsSubdomain {
			bases[d.Domain] = website.DomainBase(d)
		}
	}

	var paths []PlanPath
	for _, d := range selected {
		if d.IsSubdomain {
			if base, ok := bases[d.Parent]; ok && within(base, d.Root) {
				continue
			}
			paths = append(paths, PlanPath{Path: path.Clean(d.Root), Excludes: []string{}})
			continue
		}
		base := path.Clean(bases[d.Domain])
		excl := []string{}
		for _, o := range all {
			if o.Domain == d.Domain || sel[o.Domain] {
				continue
			}
			if within(base, o.Root) && path.Clean(o.Root) != base {
				excl = append(excl, path.Clean(o.Root))
			}
		}
		sort.Strings(excl)
		paths = append(paths, PlanPath{Path: base, Excludes: excl})
	}

	// Buang path yang sudah tercakup path lain (layout tidak standar).
	sort.Slice(paths, func(i, j int) bool { return paths[i].Path < paths[j].Path })
	out := make([]PlanPath, 0, len(paths))
	for _, p := range paths {
		covered := false
		for _, q := range out {
			if within(q.Path, p.Path) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, p)
		}
	}
	return out
}

func relPath(p string) string { return strings.TrimPrefix(path.Clean(p), "/") }

// tarCreateCmd men-tar folder relatif ke "/" supaya path lengkapnya
// terbawa di arsip dan terekstrak ke path yang SAMA di tujuan.
func tarCreateCmd(p PlanPath, compress bool) string {
	var b strings.Builder
	b.WriteString("set -o pipefail; tar --anchored")
	for _, e := range p.Excludes {
		b.WriteString(" --exclude=" + shellQuote(relPath(e)))
	}
	b.WriteString(" -C / -c")
	if compress {
		b.WriteString(" -z")
	}
	b.WriteString(" -p -f - " + shellQuote(relPath(p.Path)))
	return b.String()
}

// tarExtractCmd mengekstrak sebagai root dengan pemilik dipertahankan —
// GNU tar mencocokkan pemilik berdasarkan NAMA user/grup (bukan UID), jadi
// user SFTP yang sudah dibuat ulang di tujuan (UID-nya bisa beda) tetap
// memiliki file-filenya. Pemilik yang namanya tidak ada di tujuan
// diperbaiki sesudahnya oleh ownerFixScript.
func tarExtractCmd(compress bool) string {
	x := "tar -C / -x"
	if compress {
		x += " -z"
	}
	return x + " -p --same-owner -f -"
}

// findPrune awalan `find` yang melewati folder yang dikecualikan.
func findPrune(p PlanPath) string {
	cmd := "find " + shellQuote(p.Path)
	if len(p.Excludes) > 0 {
		parts := make([]string, len(p.Excludes))
		for i, e := range p.Excludes {
			parts[i] = "-path " + shellQuote(e)
		}
		cmd += " \\( " + strings.Join(parts, " -o ") + " \\) -prune -o"
	}
	return cmd
}

// statScript jumlah file + total ukuran (ukuran nyata per file, bukan blok
// disk — sama di kedua sisi walaupun filesystem-nya beda), dipakai untuk
// taksiran ukuran DAN verifikasi.
func statScript(p PlanPath) string {
	return findPrune(p) + " -type f -printf '%s\\n' 2>/dev/null | awk '{n++; s+=$1} END {printf \"%d %d\\n\", n, s}'"
}

func parseStat(out string) (files, bytes int64) {
	f := strings.Fields(out)
	if len(f) >= 2 {
		files, _ = strconv.ParseInt(f[0], 10, 64)
		bytes, _ = strconv.ParseInt(f[1], 10, 64)
	}
	return files, bytes
}

// ownerScanScript daftar unik "uid user gid group" di folder sumber.
func ownerScanScript(p PlanPath) string {
	return findPrune(p) + " -printf '%U %u %G %g\\n' 2>/dev/null | sort -u"
}

type owner struct {
	uid, user, gid, group string
}

func parseOwners(out string) []owner {
	var res []owner
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 4 {
			res = append(res, owner{f[0], f[1], f[2], f[3]})
		}
	}
	return res
}

// ownerFixScript: file yang pemiliknya (nama user/grup) tidak ada di tujuan
// terekstrak dengan UID/GID NUMERIK dari server asal — yang di tujuan bisa
// kosong, atau lebih buruk, kebetulan milik user lain. Semuanya dialihkan
// ke user/grup web tujuan. chown --from hanya menyentuh file dengan pemilik
// numerik persis itu, jadi file milik user yang ada tidak ikut berubah.
func ownerFixScript(p PlanPath, owners []owner) string {
	var b strings.Builder
	b.WriteString(`set +e
WEBUSER=www-data
if id -u www-data >/dev/null 2>&1; then WEBUSER=www-data
elif id -u nginx >/dev/null 2>&1; then WEBUSER=nginx
elif id -u apache >/dev/null 2>&1; then WEBUSER=apache
fi
WEBGROUP=$(id -gn "$WEBUSER" 2>/dev/null || echo "$WEBUSER")
`)
	b.WriteString("P=" + shellQuote(p.Path) + "\n")
	users, groups := map[string]string{}, map[string]string{}
	for _, o := range owners {
		users[o.uid] = o.user
		groups[o.gid] = o.group
	}
	uids := make([]string, 0, len(users))
	for u := range users {
		uids = append(uids, u)
	}
	sort.Strings(uids)
	for _, uid := range uids {
		fmt.Fprintf(&b, "if ! id -u %s >/dev/null 2>&1; then chown -R -h --from=%s \"$WEBUSER\" \"$P\" && echo FIXED user %s; fi\n",
			shellQuote(users[uid]), shellQuote("+"+uid), shellQuote(users[uid]))
	}
	gids := make([]string, 0, len(groups))
	for g := range groups {
		gids = append(gids, g)
	}
	sort.Strings(gids)
	for _, gid := range gids {
		fmt.Fprintf(&b, "if ! getent group %s >/dev/null 2>&1; then chown -R -h --from=:%s \":$WEBGROUP\" \"$P\" && echo FIXED group %s; fi\n",
			shellQuote(groups[gid]), shellQuote("+"+gid), shellQuote(groups[gid]))
	}
	b.WriteString("exit 0\n")
	return b.String()
}

func formatBytes(n int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, units[i])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
