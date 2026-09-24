package agentctl

import (
	"strconv"
	"strings"
)

// Perbandingan versi agent.
//
// Dibuat eksplisit karena membandingkan dua string versi dengan `!=` saja
// menghasilkan dua kesalahan sekaligus: app desktop yang lebih TUA akan
// menawarkan "update" yang sebenarnya menurunkan versi agent di server, dan
// build yang belum dirilis (BundledVersion masih "dev") ikut dianggap versi
// yang layak dituju.

// releaseVersion adalah versi rilis yang bisa dibandingkan urutannya.
type releaseVersion struct {
	parts [3]int
	// pre diisi untuk versi pra-rilis seperti "1.2.0-rc1". Versi pra-rilis
	// dianggap LEBIH TUA dari rilis final dengan angka yang sama, mengikuti
	// konvensi semver.
	pre string
}

// parseVersion menerima "v1.2.3", "1.2.3", "1.2", atau "1.2.3-rc1".
//
// ok=false untuk apa pun yang bukan versi rilis — termasuk "dev", string
// kosong, dan hash commit. Itu disengaja: kalau salah satu sisi tidak bisa
// dibandingkan, jawabannya bukan "tebak", melainkan "jangan tawarkan update".
func parseVersion(raw string) (releaseVersion, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return releaseVersion{}, false
	}

	// Buang metadata build; ia tidak ikut menentukan urutan.
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var pre string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}

	fields := strings.Split(s, ".")
	if len(fields) == 0 || len(fields) > 3 {
		return releaseVersion{}, false
	}
	var v releaseVersion
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return releaseVersion{}, false
		}
		v.parts[i] = n
	}
	v.pre = pre
	return v, true
}

// compareVersions mengembalikan -1, 0, atau 1.
func compareVersions(a, b releaseVersion) int {
	for i := 0; i < 3; i++ {
		switch {
		case a.parts[i] < b.parts[i]:
			return -1
		case a.parts[i] > b.parts[i]:
			return 1
		}
	}
	switch {
	case a.pre == b.pre:
		return 0
	case a.pre == "": // rilis final > pra-rilis
		return 1
	case b.pre == "":
		return -1
	case a.pre < b.pre:
		return -1
	default:
		return 1
	}
}

// versionAction menentukan apa yang boleh ditawarkan UI.
type versionAction int

const (
	// versionNothing: tidak ada yang ditawarkan.
	versionNothing versionAction = iota
	// versionUpdate: yang dibundel benar-benar lebih baru.
	versionUpdate
)

// decideVersionAction menawarkan update HANYA kalau versi yang dibundel
// terbukti lebih baru dari yang terpasang.
//
// Kalau salah satu sisi bukan versi rilis yang bisa diurutkan — mis. `make
// agent` dijalankan tanpa VERSION sehingga nilainya "dev" — jawabannya bukan
// menebak, melainkan tidak menawarkan apa pun. Menawarkan pemasangan yang
// urutannya tidak diketahui sama saja meminta user mempercayai tebakan kita
// soal mana yang lebih baru.
func decideVersionAction(installed, bundled string) versionAction {
	iv, iok := parseVersion(installed)
	bv, bok := parseVersion(bundled)
	if !iok || !bok {
		return versionNothing
	}
	if compareVersions(bv, iv) > 0 {
		return versionUpdate
	}
	return versionNothing
}
