package docker

import (
	"fmt"
	"strconv"
	"strings"
)

// Port sebuah container di-publish lewat `-p host:container` — begitu
// dipasang, siapa pun yang bisa menjangkau IP mana pun di server ini bisa
// mencapainya, termasuk dari internet, terlepas dari tujuan sebenarnya
// (mis. database internal yang cuma dipakai app lain di server yang sama).
// Scope di bawah ini memberi kontrol eksplisit, tiga tingkat:
//
//   - "public"    — bisa diakses dari mana saja (internet). Default, sama
//     seperti perilaku sebelum fitur ini ada.
//   - "intranet"  — HANYA dari jaringan lokal/LAN (rentang IP privat
//     RFC1918), lewat aturan firewall yang dibatasi. Docker sendiri tidak
//     punya cara membatasi "hanya LAN" waktu bind (cuma bisa bind ke satu
//     IP tertentu, bukan rentang) — makanya port tetap bind ke semua
//     interface (0.0.0.0), dan firewall-lah yang menyaring sumbernya.
//     KALAU tidak ada firewall aktif, pembatasan ini TIDAK benar-benar
//     berlaku (sama terbukanya dengan "public") — RecreateContainer
//     melaporkan ini eksplisit lewat RecreateContainerResponse.Warning,
//     bukan diam-diam.
//   - "localhost" — HANYA dari server itu sendiri (bind ke 127.0.0.1).
//     Ini satu-satunya mode yang proteksinya independen dari firewall:
//     bind loopback membuatnya taK terjangkau dari luar apa pun kondisi
//     firewall-nya.
const (
	portScopePublic    = "public"
	portScopeIntranet  = "intranet"
	portScopeLocalhost = "localhost"
)

// portScopeLabelPrefix menandai container label yang menyimpan pilihan
// scope per port — supaya kebuka lagi RecreateContainerModal setelahnya
// menampilkan pilihan yang SAMA seperti terakhir diset (Docker sendiri
// tidak menyimpan niat "public vs intranet", cuma bind-address mentah,
// dan "intranet" & "public" sama-sama bind ke 0.0.0.0).
const portScopeLabelPrefix = "poinhost.portscope."

// portScopeFirewallMarker menandai aturan firewall yang poinhost tulis
// sendiri untuk fitur ini — dipakai supaya idempotent (dihapus/ditulis
// ulang tiap recreate, bukan menumpuk aturan baru tiap kali).
const portScopeFirewallMarker = "poinhost-port-scope"

// privateLANCIDRs rentang alamat IPv4 privat (RFC1918) — dipakai sebagai
// "intranet" untuk scope port. Ini beda dari dockerBridgeCIDR di modul
// website (rentang bridge network Docker internal) — di sini yang
// dimaksud adalah LAN fisik/virtual tempat server ini berada.
var privateLANCIDRs = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}

// normalizePortScope memvalidasi & menormalkan nilai scope dari request UI.
func normalizePortScope(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "":
		return portScopePublic, nil
	case portScopePublic, portScopeIntranet, portScopeLocalhost:
		return v, nil
	default:
		return "", fmt.Errorf("scope port %q tidak valid", raw)
	}
}

// hostIPForScope menghitung HostIP dari scope — satu-satunya sumber
// kebenaran, bukan input manual (lihat komentar PortMapping.HostIP).
func hostIPForScope(scope string) string {
	if scope == portScopeLocalhost {
		return "127.0.0.1"
	}
	return ""
}

func portScopeLabelKey(hostPort int, proto string) string {
	return portScopeLabelPrefix + strconv.Itoa(hostPort) + "." + proto
}

// syncPortScopeLabels membuang label scope lama lalu menulis ulang sesuai
// ports saat ini — supaya label tidak menumpuk untuk port yang sudah
// dihapus/diganti nomor host-nya.
func syncPortScopeLabels(labels map[string]string, ports []PortMapping) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}
	for k := range labels {
		if strings.HasPrefix(k, portScopeLabelPrefix) {
			delete(labels, k)
		}
	}
	for _, p := range ports {
		if p.Scope == "" || p.Scope == portScopePublic {
			continue
		}
		labels[portScopeLabelKey(p.HostPort, p.Protocol)] = p.Scope
	}
	return labels
}

// portScopeApplyScript membangun SATU skrip firewall untuk SEMUA port
// container sekaligus (satu round-trip SSH, bukan satu panggilan per
// port). Untuk tiap port: hapus dulu SEMUA kemungkinan aturan poinhost
// sebelumnya (allow-dari-mana-saja maupun tiap CIDR intranet) sebelum
// menambah yang baru — kalau tidak, aturan "allow dari mana saja" yang
// basi bisa diam-diam mengalahkan pembatasan intranet yang baru dipilih
// (ufw/firewalld meng-OR semua allow rule yang cocok, jadi satu aturan
// longgar yang tertinggal saja cukup membuat pembatasan baru tidak
// berarti). Tidak pernah memasang/mengaktifkan firewall baru — cuma
// menambah aturan ke firewall yang memang sudah aktif.
func portScopeApplyScript(ports []PortMapping) string {
	if len(ports) == 0 {
		return `echo "FIREWALL=skip"`
	}
	var b strings.Builder
	b.WriteString(`if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi "^Status: active"; then` + "\n")
	b.WriteString("  echo \"FIREWALL=ufw\"\n")
	for _, p := range ports {
		port := strconv.Itoa(p.HostPort)
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		b.WriteString("  ufw delete allow to any port " + port + " proto " + proto + " >/dev/null 2>&1 || true\n")
		for _, cidr := range privateLANCIDRs {
			b.WriteString("  ufw delete allow from " + cidr + " to any port " + port + " proto " + proto + " >/dev/null 2>&1 || true\n")
		}
		switch p.Scope {
		case portScopeIntranet:
			for _, cidr := range privateLANCIDRs {
				b.WriteString("  ufw allow from " + cidr + " to any port " + port + " proto " + proto + " comment '" + portScopeFirewallMarker + "' >/dev/null 2>&1 || true\n")
			}
		case portScopePublic:
			b.WriteString("  ufw allow to any port " + port + " proto " + proto + " comment '" + portScopeFirewallMarker + "' >/dev/null 2>&1 || true\n")
		}
	}
	b.WriteString(`elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state 2>/dev/null | grep -qi running; then` + "\n")
	b.WriteString("  echo \"FIREWALL=firewalld\"\n")
	for _, p := range ports {
		port := strconv.Itoa(p.HostPort)
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		b.WriteString("  firewall-cmd --permanent --remove-port=" + port + "/" + proto + " >/dev/null 2>&1 || true\n")
		for _, cidr := range privateLANCIDRs {
			rule := `rule family="ipv4" source address="` + cidr + `" port port="` + port + `" protocol="` + proto + `" accept`
			b.WriteString("  firewall-cmd --permanent --remove-rich-rule=" + shellQuote(rule) + " >/dev/null 2>&1 || true\n")
		}
		switch p.Scope {
		case portScopeIntranet:
			for _, cidr := range privateLANCIDRs {
				rule := `rule family="ipv4" source address="` + cidr + `" port port="` + port + `" protocol="` + proto + `" accept`
				b.WriteString("  firewall-cmd --permanent --add-rich-rule=" + shellQuote(rule) + " >/dev/null 2>&1 || true\n")
			}
		case portScopePublic:
			b.WriteString("  firewall-cmd --permanent --add-port=" + port + "/" + proto + " >/dev/null 2>&1 || true\n")
		}
	}
	b.WriteString("  firewall-cmd --reload >/dev/null 2>&1 || true\n")
	b.WriteString("else\n")
	b.WriteString("  echo \"FIREWALL=none\"\n")
	b.WriteString("fi\n")
	return b.String()
}

// portScopeWarning menyusun pesan peringatan kalau ada port "intranet"
// tapi tidak ada firewall aktif terdeteksi — di kondisi itu pembatasannya
// TIDAK benar-benar berlaku (sama terbukanya dengan "public"), jadi
// dilaporkan eksplisit alih-alih diam-diam gagal membatasi.
func portScopeWarning(ports []PortMapping, firewallDetected string) string {
	if firewallDetected != "none" {
		return ""
	}
	var intranetPorts []string
	for _, p := range ports {
		if p.Scope == portScopeIntranet {
			intranetPorts = append(intranetPorts, strconv.Itoa(p.HostPort)+"/"+p.Protocol)
		}
	}
	if len(intranetPorts) == 0 {
		return ""
	}
	return "Port " + strings.Join(intranetPorts, ", ") + " diset \"hanya intranet\", tapi tidak ada firewall (ufw/firewalld) aktif yang terdeteksi di server ini — pembatasannya BELUM benar-benar berlaku, port tersebut saat ini sama terbukanya dengan \"Publik\" (bisa diakses dari internet). Aktifkan ufw atau firewalld di server supaya benar-benar terbatas ke jaringan lokal saja."
}
