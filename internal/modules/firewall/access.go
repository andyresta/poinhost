package firewall

import (
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// firewallAccess konteks eksekusi perintah firewall di server remote.
// Polanya sama dengan modul docker/services: sudo murni untuk naik ke root
// kalau user SSH bukan root (ufw/firewall-cmd butuh root).
type firewallAccess struct {
	serverID     string
	sshUser      string
	sshPort      int
	useSudo      bool
	sudoPassword string
}

func (s *Service) resolveAccess(serverID string) (*firewallAccess, error) {
	srv, err := s.servers.Get(serverID)
	if err != nil {
		return nil, err
	}
	access := &firewallAccess{
		serverID: serverID,
		sshUser:  srv.Username,
		// Port SSH diambil dari konfigurasi server di poinhost, BUKAN ditebak
		// dari /etc/ssh/sshd_config: inilah port yang benar-benar dipakai
		// koneksi ini, jadi persis port yang akan mengunci kita kalau
		// firewall diaktifkan tanpa mengizinkannya.
		sshPort: srv.Port,
		useSudo: srv.UseSudo,
	}
	if access.sshPort <= 0 {
		access.sshPort = 22
	}
	if srv.UseSudo && srv.Username != "root" && srv.AuthType == "password" {
		pw, _ := s.servers.SudoPassword(serverID)
		access.sudoPassword = pw
	}
	return access, nil
}

func (a *firewallAccess) wrap(script string) string {
	inner := "bash -c " + shellQuote(script)
	if a.sshUser == "root" || !a.useSudo {
		return inner
	}
	if a.sudoPassword != "" {
		return "echo " + shellQuote(a.sudoPassword) + " | sudo -S -p '' " + inner
	}
	return "sudo -n " + inner
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// --- Validasi masukan ---
//
// Semua nilai di bawah ini ikut tersusun jadi perintah shell. Sama seperti di
// modul services, pendekatannya WHITELIST KETAT sejak awal, bukan sekadar
// mengandalkan satu lapis quoting.

// portRE menerima satu port ("80") atau rentang ("8000:9000" gaya ufw,
// "8000-9000" gaya firewalld dinormalkan belakangan).
var portRE = regexp.MustCompile(`^(\d{1,5})(?::(\d{1,5}))?$`)

func normalizePort(raw string) (string, error) {
	p := strings.TrimSpace(strings.ReplaceAll(raw, "-", ":"))
	m := portRE.FindStringSubmatch(p)
	if m == nil {
		return "", errors.New("port tidak valid — isi angka 1-65535 atau rentang seperti 8000:9000")
	}
	lo, err := strconv.Atoi(m[1])
	if err != nil || lo < 1 || lo > 65535 {
		return "", errors.New("port di luar rentang 1-65535")
	}
	if m[2] != "" {
		hi, err := strconv.Atoi(m[2])
		if err != nil || hi < 1 || hi > 65535 {
			return "", errors.New("port di luar rentang 1-65535")
		}
		if hi <= lo {
			return "", errors.New("batas atas rentang port harus lebih besar dari batas bawah")
		}
	}
	return p, nil
}

var protocols = map[string]bool{"tcp": true, "udp": true}

func normalizeProtocol(raw string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(raw))
	if p == "" {
		p = "tcp"
	}
	if !protocols[p] {
		return "", errors.New("protokol harus tcp atau udp")
	}
	return p, nil
}

var actions = map[string]bool{"allow": true, "deny": true}

func normalizeAction(raw string) (string, error) {
	a := strings.ToLower(strings.TrimSpace(raw))
	if a == "" {
		a = "allow"
	}
	if !actions[a] {
		return "", errors.New("aksi harus allow atau deny")
	}
	return a, nil
}

// normalizeSource menerima alamat IP atau CIDR saja.
//
// Diperiksa dengan net.ParseIP/net.ParseCIDR, BUKAN regex: pola karakter
// seperti "^[0-9a-fA-F:.]+$" tampak cukup tapi meloloskan string seperti
// "beef.cafe" yang seluruhnya huruf heksa — bukan celah keamanan, tapi
// membuat error-nya baru muncul dari ufw dengan pesan yang membingungkan.
//
// Nama host SENGAJA ditolak: hasil resolusinya bisa berubah diam-diam
// sehingga aturan firewall jadi tidak deterministik.
func normalizeSource(raw string) (string, error) {
	src := strings.TrimSpace(raw)
	if src == "" || strings.EqualFold(src, "any") {
		return "", nil // kosong = dari mana saja
	}
	if strings.Contains(src, "/") {
		if _, _, err := net.ParseCIDR(src); err != nil {
			return "", errors.New("CIDR tidak valid — contoh yang benar: 10.0.0.0/8")
		}
		return src, nil
	}
	if net.ParseIP(src) == nil {
		return "", errors.New("sumber harus berupa alamat IP atau CIDR, misalnya 10.0.0.0/8")
	}
	return src, nil
}

// commentRE menjaga komentar tetap aman masuk ke `ufw ... comment '...'`.
var commentRE = regexp.MustCompile(`^[\w \-.:/]{0,64}$`)

func normalizeComment(raw string) (string, error) {
	c := strings.TrimSpace(raw)
	if c == "" {
		return "", nil
	}
	if !commentRE.MatchString(c) {
		return "", errors.New("komentar hanya boleh huruf, angka, spasi, dan tanda - . : / (maksimal 64 karakter)")
	}
	return c, nil
}

// mapFirewallError menerjemahkan kegagalan umum jadi pesan yang bisa
// ditindaklanjuti, bukan melempar stderr mentah ke UI.
func mapFirewallError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "a password is required"),
		strings.Contains(msg, "a terminal is required"),
		strings.Contains(msg, "incorrect password attempt"):
		return errors.New("sudo membutuhkan password — gunakan auth password SSH atau atur NOPASSWD di sudoers")
	case strings.Contains(msg, "access denied"), strings.Contains(msg, "permission denied"):
		return errors.New("tidak punya izin mengatur firewall — aktifkan sudo untuk server ini")
	case strings.Contains(msg, "deadline exceeded"), strings.Contains(msg, "timeout"):
		return errors.New("timeout menjalankan perintah firewall — cek koneksi atau beban server")
	}
	return err
}

// zoneRE membatasi nama zone firewalld. Nilai ini ikut masuk ke baris
// perintah lewat --zone=, jadi divalidasi seperti masukan lain.
var zoneRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

func normalizeZone(raw string) (string, error) {
	z := strings.TrimSpace(raw)
	if z == "" {
		return "", nil // kosong = pakai zone default server
	}
	if !zoneRE.MatchString(z) {
		return "", errors.New("nama zone tidak valid")
	}
	return z, nil
}

// serviceRE membatasi nama service firewalld (mis. "ssh", "http",
// "dhcpv6-client").
var serviceRE = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func normalizeServiceName(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if !serviceRE.MatchString(s) {
		return "", errors.New("nama service firewalld tidak valid")
	}
	return s, nil
}
