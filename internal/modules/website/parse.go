package website

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// errFmt adalah fmt.Errorf tanpa perlu import "fmt" di tiap file pemanggil.
func errFmt(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// parseExitCode mem-parsing kode keluar dari sentinel marker instalasi.
func parseExitCode(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

// shellQuote membungkus argumen shell dengan kutip tunggal aman.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

var domainRE = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

// NormalizeDomain memvalidasi & menormalisasi nama domain (lowercase, tanpa
// trailing dot, tanpa skema/path).
func NormalizeDomain(raw string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(raw))
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimSuffix(d, "/")
	d = strings.TrimSuffix(d, ".")
	if d == "" {
		return "", errors.New("nama domain wajib diisi")
	}
	if len(d) > 253 || !domainRE.MatchString(d) {
		return "", errors.New("nama domain tidak valid")
	}
	return d, nil
}

var subdomainLabelRE = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

// normalizeSubdomainLabel memvalidasi label subdomain (bagian sebelum parent).
func normalizeSubdomainLabel(raw string) (string, error) {
	l := strings.ToLower(strings.TrimSpace(raw))
	if l == "" {
		return "", errors.New("nama subdomain wajib diisi")
	}
	if len(l) > 63 || !subdomainLabelRE.MatchString(l) {
		return "", errors.New("nama subdomain tidak valid (huruf, angka, - saja)")
	}
	return l, nil
}

func subdomainFQDN(parent, label string) string {
	return label + "." + parent
}
