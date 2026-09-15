// Package firewall mengelola firewall server remote lewat SSH.
//
// Pendekatannya: DETEKSI BACKEND YANG SEDANG AKTIF, bukan menebak dari OS.
// Satu server bisa punya ufw, firewalld, dan nftables terpasang sekaligus
// (server Ubuntu pun sering begitu), jadi memetakan "Ubuntu → ufw" akan
// menulis aturan ke firewall yang justru sedang mati — dan UI melaporkan
// "aman" padahal portnya terbuka.
//
// ufw dan firewalld bisa diubah dari sini. nftables/iptables mentah SENGAJA
// hanya dibaca: urutan aturannya menentukan hasil dan tidak ada standar
// persistensi, jadi menulis ke sana berisiko menghasilkan aturan yang lenyap
// saat reboot tanpa user sadar. Lebih jujur menampilkannya sambil bilang
// tidak dikelola dari sini.
package firewall

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/servers"
)

// Service mengorkestrasi operasi firewall di server remote.
type Service struct {
	servers  *servers.Service
	executor *sshpool.Executor
	mutex    *sshpool.ServerMutexRegistry
}

// NewService membuat service modul firewall baru.
func NewService(serversSvc *servers.Service, executor *sshpool.Executor, mutex *sshpool.ServerMutexRegistry) *Service {
	return &Service{servers: serversSvc, executor: executor, mutex: mutex}
}

func (s *Service) run(access *firewallAccess, script string, timeout time.Duration) (*sshpool.ExecResult, error) {
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// destructive=true → tanpa retry otomatis: perintah firewall mengubah
	// state, dan mengulanginya saat timeout bisa menerapkan aturan dua kali.
	res, err := s.executor.Exec(ctx, access.serverID, timeout, access.wrap(script), true)
	if err != nil {
		return res, mapFirewallError(err)
	}
	return res, nil
}

// listScript mengambil deteksi backend DAN seluruh aturan dalam satu
// round-trip SSH — bukan satu panggilan untuk status lalu satu lagi untuk
// aturan, yang membuat keduanya bisa menggambarkan dua momen berbeda.
//
// Urutan cabangnya penting, dan dua hal di dalamnya mudah salah:
//
//  1. nftables diakui sebagai backend HANYA kalau `nftables.service` aktif,
//     BUKAN sekadar `nft list ruleset` tidak kosong. Di host ber-Docker
//     ruleset nftables nyaris selalu terisi — itu aturan NAT bikinan Docker,
//     bukan kebijakan firewall. Memakai "ruleset tidak kosong" sebagai
//     penanda membuat server tanpa firewall sama sekali dilaporkan seolah
//     terlindungi, sekaligus menutupi ufw yang terpasang tapi belum menyala.
//
//  2. Saat ufw MATI, `ufw status` tidak menampilkan aturan apa pun, jadi
//     daftarnya diambil dari `ufw show added`. Tanpa ini aturan yang baru
//     ditambahkan user tidak muncul di layar sampai firewall dinyalakan —
//     terlihat seperti fitur yang rusak.
//
// buildListScript menyusun skrip deteksi + pembacaan aturan.
//
// Zone hanya berlaku untuk firewalld. Kosong berarti memakai zone default
// server. Nilainya SUDAH divalidasi pemanggil (normalizeZone).
func buildListScript(zone string) string {
	zoneExpr := "$(firewall-cmd --get-default-zone 2>/dev/null)"
	if zone != "" {
		zoneExpr = zone
	}
	return `set +e
if command -v ufw >/dev/null 2>&1; then echo "INSTALLED=ufw"; fi
if command -v firewall-cmd >/dev/null 2>&1; then echo "INSTALLED=firewalld"; fi
if command -v nft >/dev/null 2>&1; then echo "INSTALLED=nftables"; fi

if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi "^Status: active"; then
  echo "BACKEND=ufw"
  ufw status verbose 2>/dev/null | grep -i "^Default:" | sed 's/^/DEFAULT=/'
  echo "__RULES__"
  ufw status numbered 2>/dev/null
elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state 2>/dev/null | grep -qi running; then
  Z=` + zoneExpr + `
  echo "BACKEND=firewalld"
  echo "ZONE=$Z"
  echo "ZONES=$(firewall-cmd --get-zones 2>/dev/null)"
  echo "DEFAULT=$(firewall-cmd --permanent --zone=$Z --get-target 2>/dev/null)"
  echo "__RULES__"
  echo "PORT=$(firewall-cmd --zone=$Z --list-ports 2>/dev/null)"
  for SVC in $(firewall-cmd --zone=$Z --list-services 2>/dev/null); do
    echo "SVC=$SVC|$(firewall-cmd --info-service=$SVC 2>/dev/null | sed -n 's/^[[:space:]]*ports:[[:space:]]*//p')"
  done
  firewall-cmd --zone=$Z --list-rich-rules 2>/dev/null | sed 's/^/RICH=/'
elif systemctl is-active --quiet nftables 2>/dev/null; then
  echo "BACKEND=nftables"
  echo "__RULES__"
elif command -v ufw >/dev/null 2>&1; then
  echo "BACKEND=ufw"
  echo "INACTIVE=1"
  echo "__RULES__"
  ufw show added 2>/dev/null | sed 's/^/ADDED=/'
elif command -v firewall-cmd >/dev/null 2>&1; then
  echo "BACKEND=firewalld"
  echo "INACTIVE=1"
  echo "__RULES__"
else
  echo "BACKEND=none"
  echo "__RULES__"
fi
exit 0`
}

// ListRules mengembalikan status firewall beserta seluruh aturannya.
//
// zone hanya dipakai firewalld; kosong = zone default server.
func (s *Service) ListRules(serverID, zone string) (*ListResponse, error) {
	if strings.TrimSpace(serverID) == "" {
		return nil, fmt.Errorf("id server wajib diisi")
	}
	z, err := normalizeZone(zone)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	res, err := s.run(access, buildListScript(z), 25*time.Second)
	if err != nil {
		return nil, err
	}
	out := parseList(res.Stdout, access.sshPort)
	return out, nil
}

// parseList dipisah dari ListRules supaya bisa diuji tanpa SSH sama sekali.
func parseList(stdout string, sshPort int) *ListResponse {
	head, body, _ := strings.Cut(stdout, "__RULES__")

	st := Status{Backend: "none", SSHPort: sshPort, Installed: []string{}}
	inactive := false
	for _, line := range strings.Split(head, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		switch {
		case strings.HasPrefix(line, "INSTALLED="):
			st.Installed = append(st.Installed, strings.TrimPrefix(line, "INSTALLED="))
		case strings.HasPrefix(line, "BACKEND="):
			st.Backend = strings.TrimPrefix(line, "BACKEND=")
		case line == "INACTIVE=1":
			inactive = true
		case strings.HasPrefix(line, "ZONES="):
			st.Zones = strings.Fields(strings.TrimPrefix(line, "ZONES="))
		case strings.HasPrefix(line, "ZONE="):
			st.Zone = strings.TrimPrefix(line, "ZONE=")
		case strings.HasPrefix(line, "DEFAULT="):
			st.DefaultIncoming, st.DefaultOutgoing = parseDefaults(strings.TrimPrefix(line, "DEFAULT="))
		}
	}

	st.Active = st.Backend != "none" && !inactive
	switch st.Backend {
	case "ufw":
		st.Editable = true
		st.OwnerDetectable = true
	case "firewalld":
		st.Editable = true
		// Rich rule firewalld tidak punya kolom komentar, jadi asal-usul
		// sebuah aturan tidak bisa dipastikan di sana.
		st.OwnerDetectable = false
	default:
		st.Editable = false
	}

	var rules []Rule
	switch st.Backend {
	case "ufw":
		if inactive {
			rules = parseUFWAdded(body)
		} else {
			rules = parseUFW(body)
		}
	case "firewalld":
		rules = parseFirewalld(body)
	default:
		rules = []Rule{}
	}

	st.SSHAllowed = sshAllowed(rules, sshPort)
	return &ListResponse{Status: st, Rules: rules}
}

// parseDefaults membaca baris kebijakan default. ufw: "Default: deny
// (incoming), allow (outgoing), disabled (routed)". firewalld: satu kata
// target zone ("default"/"ACCEPT"/"DROP").
func parseDefaults(raw string) (incoming, outgoing string) {
	v := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "Default:"))
	if !strings.Contains(v, "(") {
		return v, ""
	}
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		open := strings.Index(part, "(")
		if open < 0 {
			continue
		}
		policy := strings.TrimSpace(part[:open])
		kind := strings.Trim(part[open:], "() ")
		switch kind {
		case "incoming":
			incoming = policy
		case "outgoing":
			outgoing = policy
		}
	}
	return incoming, outgoing
}

// RuleRequest permintaan menambah aturan.
type RuleRequest struct {
	ServerID string `json:"serverId"`
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
	Source   string `json:"source"`
	Action   string `json:"action"`
	Comment  string `json:"comment"`
	// Zone hanya dipakai firewalld; kosong = zone default server.
	Zone string `json:"zone"`
}

// AddRule menambahkan satu aturan ke backend yang sedang aktif.
func (s *Service) AddRule(req RuleRequest) (*ListResponse, error) {
	port, err := normalizePort(req.Port)
	if err != nil {
		return nil, err
	}
	proto, err := normalizeProtocol(req.Protocol)
	if err != nil {
		return nil, err
	}
	source, err := normalizeSource(req.Source)
	if err != nil {
		return nil, err
	}
	action, err := normalizeAction(req.Action)
	if err != nil {
		return nil, err
	}
	comment, err := normalizeComment(req.Comment)
	if err != nil {
		return nil, err
	}
	zone, err := normalizeZone(req.Zone)
	if err != nil {
		return nil, err
	}

	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return nil, err
	}
	backend, err := s.activeBackend(access)
	if err != nil {
		return nil, err
	}

	var script string
	switch backend {
	case "ufw":
		script = ufwAddScript(port, proto, source, action, comment)
	case "firewalld":
		script = firewalldAddScript(port, proto, source, action, zone)
	default:
		return nil, fmt.Errorf("tidak ada firewall yang bisa diatur dari sini (backend: %s)", backend)
	}

	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)
	if _, err := s.run(access, script, 30*time.Second); err != nil {
		return nil, err
	}
	return s.ListRules(req.ServerID, zone)
}

// DeleteRule menghapus satu aturan berdasarkan ID hasil ListRules.
func (s *Service) DeleteRule(serverID, ruleID, zone string) (*ListResponse, error) {
	id := strings.TrimSpace(ruleID)
	if id == "" {
		return nil, fmt.Errorf("aturan ini tidak bisa dihapus dari sini")
	}
	z, err := normalizeZone(zone)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	backend, err := s.activeBackend(access)
	if err != nil {
		return nil, err
	}

	var script string
	switch backend {
	case "ufw":
		// ID sudah berbentuk spesifikasi ufw yang tersusun dari nilai-nilai
		// yang lolos validasi saat dibaca; divalidasi ulang di sini supaya
		// string sembarang dari frontend tidak bisa menumpang masuk.
		if err := validateUFWDeleteSpec(id); err != nil {
			return nil, err
		}
		script = "ufw --force delete " + id
	case "firewalld":
		spec, err := parseFirewalldRuleID(id, z)
		if err != nil {
			return nil, err
		}
		script = spec
	default:
		return nil, fmt.Errorf("aturan firewall ini tidak bisa diubah dari sini (backend: %s)", backend)
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)
	if _, err := s.run(access, script, 30*time.Second); err != nil {
		return nil, err
	}
	return s.ListRules(serverID, z)
}

// SetEnabled menyalakan atau mematikan firewall.
//
// PENGAMAN ANTI-TERKUNCI: sebelum menyalakan, aturan allow untuk port SSH
// yang dipakai koneksi ini dipasang lebih dulu di skrip yang SAMA. Tanpa itu
// `ufw enable` dengan default deny incoming akan memutus sesi SSH saat itu
// juga dan mengunci user dari servernya sendiri — kesalahan yang tidak bisa
// diperbaiki dari aplikasi ini karena satu-satunya jalan masuk sudah tertutup.
func (s *Service) SetEnabled(serverID string, enabled bool) (*ListResponse, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	backend, err := s.activeBackend(access)
	if err != nil {
		return nil, err
	}

	var script string
	switch backend {
	case "ufw":
		if enabled {
			script = ufwEnableScript(access.sshPort)
		} else {
			script = "ufw --force disable"
		}
	case "firewalld":
		if enabled {
			script = firewalldEnableScript(access.sshPort)
		} else {
			script = "systemctl disable --now firewalld"
		}
	default:
		return nil, fmt.Errorf("tidak ada firewall yang bisa diatur dari sini (backend: %s)", backend)
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)
	if _, err := s.run(access, script, 45*time.Second); err != nil {
		return nil, err
	}
	return s.ListRules(serverID, "")
}

// activeBackend membaca backend mana yang berlaku sekarang. Sengaja dibaca
// ulang tiap operasi, bukan dititipkan dari frontend: user bisa saja
// mengubah firewall lewat Terminal di antara dua aksi.
func (s *Service) activeBackend(access *firewallAccess) (string, error) {
	res, err := s.run(access, buildListScript(""), 25*time.Second)
	if err != nil {
		return "", err
	}
	return parseList(res.Stdout, access.sshPort).Status.Backend, nil
}
