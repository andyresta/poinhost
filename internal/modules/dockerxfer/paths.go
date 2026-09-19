package dockerxfer

import "strings"

// shellQuote membungkus string dengan single-quote yang aman untuk shell
// POSIX. Helper bernama sama ada juga di filexfer dan docker — sengaja
// diduplikasi kecil daripada membuat sub-paket bersama untuk satu fungsi
// satu baris.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
