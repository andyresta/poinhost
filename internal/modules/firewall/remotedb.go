package firewall

// Aturan firewall untuk "remote access database" — klien EKSTERNAL (Navicat,
// DBeaver, aplikasi di server lain) yang menghubungi MySQL/PostgreSQL di
// server ini lewat jaringan.
//
// Beda dengan jalan pintas Docker (dockerdb.go): di sana sumbernya subnet
// Docker yang dideteksi otomatis, di sini sumbernya IP/CIDR yang DIPILIH
// user. Yang dicek tetap efeknya, bukan komentar aturan: kalau user sudah
// membuka port itu "dari mana saja" lewat panel Firewall, tidak ada aturan
// baru yang ditambahkan.

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// remoteAccessMarker disamakan dengan paket website — aturan bermarker ini
// dipasang (dan hanya aturan bermarker ini yang dicabut) oleh fitur remote
// access di modul Database.
const remoteAccessMarker = "poinhost-remote-access"

// sourceNet mengubah sumber ("1.2.3.4", "1.2.3.0/24", "::1") jadi jaringan
// kanonik. Satu IP dianggap /32 (IPv4) atau /128 (IPv6).
func sourceNet(src string) (*net.IPNet, bool) {
	src = strings.TrimSpace(src)
	if strings.Contains(src, "/") {
		_, n, err := net.ParseCIDR(src)
		if err != nil {
			return nil, false
		}
		return n, true
	}
	ip := net.ParseIP(src)
	if ip == nil {
		return nil, false
	}
	if v4 := ip.To4(); v4 != nil {
		return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}, true
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, true
}

// sourceCovers true kalau aturan bersumber ruleSrc mencakup SELURUH src.
//
// Berbeda dengan cidrCovers (khusus Docker), aturan tanpa sumber ("dari mana
// saja") DIHITUNG mencakup: bagi klien remote, port yang dibuka untuk semua
// orang memang sudah bisa dijangkau.
func sourceCovers(ruleSrc, src string) bool {
	sn, ok := sourceNet(src)
	if !ok {
		return false
	}
	if strings.TrimSpace(ruleSrc) == "" {
		return true
	}
	rn, ok := sourceNet(ruleSrc)
	if !ok {
		return false
	}
	ruleOnes, ruleBits := rn.Mask.Size()
	subOnes, subBits := sn.Mask.Size()
	if ruleBits != subBits {
		return false // IPv4 vs IPv6
	}
	return rn.Contains(sn.IP) && ruleOnes <= subOnes
}

// ruleAllowsPort true kalau aturan allow ini membuka port TCP tersebut,
// termasuk aturan berbentuk service firewalld ("postgresql" → 5432/tcp).
func ruleAllowsPort(r Rule, port int) bool {
	if r.Action != "allow" {
		return false
	}
	if r.ServicePorts != "" {
		for _, item := range strings.Fields(r.ServicePorts) {
			p, proto := splitPortSpec(item)
			if p == "" || (proto != "" && proto != "tcp") {
				continue
			}
			if portCovers(p, port) {
				return true
			}
		}
		return false
	}
	if r.Port == "" || (r.Protocol != "" && r.Protocol != "tcp") {
		return false
	}
	return portCovers(r.Port, port)
}

// sourceAllowedForPort true kalau ada aturan allow yang mencakup src untuk
// port tersebut.
func sourceAllowedForPort(rules []Rule, src string, port int) bool {
	sn, ok := sourceNet(src)
	if !ok {
		return false
	}
	isV4 := sn.IP.To4() != nil
	for _, r := range rules {
		// Baris "(v6)" ufw hanya berlaku untuk lalu lintas IPv6 — "Anywhere
		// (v6)" tidak membuka port untuk klien IPv4.
		if r.IPv6 && isV4 {
			continue
		}
		if ruleAllowsPort(r, port) && sourceCovers(r.Source, src) {
			return true
		}
	}
	return false
}

// remoteRuleIDs mencari aturan yang SEBELUMNYA dipasang fitur remote access
// untuk src+port persis ini — bukan aturan yang kebetulan mencakup. Di ufw
// pemiliknya terbaca dari komentar; di firewalld (rich rule tanpa komentar)
// yang dicocokkan sumber+port persis, dan pemanggil hanya memanggil ini
// untuk aturan yang tercatat dipasang poinhost sendiri.
func remoteRuleIDs(rules []Rule, backend, src string, port int) []string {
	want, ok := sourceNet(src)
	if !ok {
		return nil
	}
	p := strconv.Itoa(port)
	var ids []string
	for _, r := range rules {
		if r.ID == "" || r.Action != "allow" || r.Port != p {
			continue
		}
		if r.Protocol != "" && r.Protocol != "tcp" {
			continue
		}
		got, ok := sourceNet(r.Source)
		if !ok || got.String() != want.String() {
			continue
		}
		if backend == "ufw" && r.Owner != remoteAccessMarker {
			continue
		}
		ids = append(ids, r.ID)
	}
	return ids
}

// firewallSource bentuk sumber yang dikirim ke ufw/firewalld: satu host
// ditulis sebagai IP polos (ufw menampilkannya begitu juga), selain itu CIDR.
func firewallSource(src string) (string, error) {
	n, ok := sourceNet(src)
	if !ok {
		return "", fmt.Errorf("sumber harus berupa alamat IP atau CIDR")
	}
	ones, bits := n.Mask.Size()
	if ones == bits {
		return n.IP.String(), nil
	}
	return n.String(), nil
}

// PortAllowedFromSource menjawab: apakah klien dari src bisa menjangkau port
// ini lewat firewall yang AKTIF? Tanpa firewall aktif jawabannya true —
// tidak ada yang menyaring.
func (s *Service) PortAllowedFromSource(serverID string, port int, src string) (allowed bool, backend string, err error) {
	res, backend, err := s.PortAllowedFromSources(serverID, port, []string{src})
	if err != nil {
		return false, "", err
	}
	return res[0], backend, nil
}

// PortAllowedFromSources versi banyak sumber dari PortAllowedFromSource —
// satu kali baca aturan untuk seluruh daftar, bukan satu round-trip SSH per
// sumber.
func (s *Service) PortAllowedFromSources(serverID string, port int, srcs []string) (allowed []bool, backend string, err error) {
	out, err := s.ListRules(serverID, "")
	if err != nil {
		return nil, "", err
	}
	backend = out.Status.Backend
	allowed = make([]bool, len(srcs))
	for i, src := range srcs {
		allowed[i] = !out.Status.Active || backend == "none" || sourceAllowedForPort(out.Rules, src, port)
	}
	return allowed, backend, nil
}

// AllowSourceToPort memasang satu aturan allow src → port/tcp bermarker
// remote access. Hanya untuk ufw/firewalld yang aktif.
func (s *Service) AllowSourceToPort(serverID string, port int, src string) error {
	source, err := firewallSource(src)
	if err != nil {
		return err
	}
	_, err = s.AddRule(RuleRequest{
		ServerID: serverID,
		Port:     strconv.Itoa(port),
		Protocol: "tcp",
		Source:   source,
		Action:   "allow",
		Comment:  remoteAccessMarker,
	})
	return err
}

// RemoveSourceFromPort mencabut aturan yang dipasang AllowSourceToPort untuk
// src+port ini. Aturan lain (milik user, atau yang lebih longgar) tidak
// disentuh. Tidak ada firewall aktif = tidak ada yang perlu dicabut.
func (s *Service) RemoveSourceFromPort(serverID string, port int, src string) error {
	out, err := s.ListRules(serverID, "")
	if err != nil {
		return err
	}
	backend := out.Status.Backend
	if backend != "ufw" && backend != "firewalld" {
		return nil
	}
	for _, id := range remoteRuleIDs(out.Rules, backend, src, port) {
		if _, err := s.DeleteRule(serverID, id, ""); err != nil {
			return err
		}
	}
	return nil
}
