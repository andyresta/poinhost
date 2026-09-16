package firewall

import (
	"regexp"
	"strconv"
	"strings"
)

// Rule satu aturan firewall dalam bentuk yang SAMA untuk semua backend.
//
// Field-nya sengaja hanya irisan yang didukung ufw maupun firewalld: port,
// protokol, sumber, aksi. Hal khas satu backend (zone firewalld, app profile
// ufw) tidak dipaksa masuk ke sini — ditampilkan apa adanya lewat Raw.
type Rule struct {
	// ID dipakai saat menghapus. Untuk ufw berupa spesifikasi aturan yang
	// disusun ulang (BUKAN nomor baris — nomor bergeser setiap ada
	// penghapusan, jadi memakainya berarti balapan dengan perubahan lain).
	// Untuk firewalld berupa rich rule utuh.
	ID       string `json:"id"`
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
	// Source kosong berarti "dari mana saja".
	Source string `json:"source"`
	Action string `json:"action"` // allow | deny
	// IPv6 true untuk baris "(v6)" milik ufw. ufw menampilkan aturan v4 dan
	// v6 sebagai dua baris terpisah, dan itu ditampilkan apa adanya daripada
	// digabung — menggabungkannya akan menyembunyikan kasus saat hanya salah
	// satunya yang terpasang.
	IPv6 bool `json:"ipv6"`
	// Service nama service firewalld ("ssh", "http") kalau aturan ini
	// berbentuk service, bukan port lepas. Di RHEL/CentOS inilah bentuk yang
	// paling lazim dipakai orang, jadi tanpa ini daftar aturan akan tampak
	// KOSONG di server yang disetel dengan cara normal.
	Service string `json:"service"`
	// ServicePorts port yang diwakili service tersebut, apa adanya dari
	// `firewall-cmd --info-service` (mis. "80/tcp 443/tcp"). Dipakai juga
	// oleh penjaga SSH supaya service "ssh" terhitung membuka port 22.
	ServicePorts string `json:"servicePorts"`
	// Owner: "poinhost-docker-access" / "poinhost-port-scope" kalau aturan
	// ini dibuat fitur lain di aplikasi ini, selain itu kosong. Hanya bisa
	// diketahui di ufw (lihat catatan di detectOwner).
	Owner   string `json:"owner"`
	Comment string `json:"comment"`
	// Raw baris asli, ditampilkan untuk aturan yang tidak terparse penuh
	// supaya tetap terlihat user alih-alih hilang diam-diam.
	Raw string `json:"raw"`
}

// Status ringkasan firewall di satu server.
type Status struct {
	// Backend firewall yang AKTIF: "ufw" | "firewalld" | "nftables" | "none".
	Backend string `json:"backend"`
	Active  bool   `json:"active"`
	// Editable false untuk nftables/iptables mentah: aturannya ditampilkan
	// tapi tidak bisa diubah dari sini. Alasannya urutan aturan menentukan
	// hasil dan tidak ada standar persistensi — menulis ke sana berisiko
	// membuat aturan yang lenyap saat reboot tanpa user sadar.
	Editable bool `json:"editable"`
	// Installed semua backend yang TERPASANG, tidak peduli aktif atau tidak.
	// Ditampilkan supaya jelas kenapa satu backend dipilih: di satu server
	// bisa ada ufw, firewalld, dan nftables sekaligus.
	Installed       []string `json:"installed"`
	DefaultIncoming string   `json:"defaultIncoming"`
	DefaultOutgoing string   `json:"defaultOutgoing"`
	Zone            string   `json:"zone"`  // firewalld saja: zone yang sedang ditampilkan
	Zones           []string `json:"zones"` // firewalld saja: seluruh zone yang tersedia
	// SSHPort port yang dipakai koneksi poinhost sendiri ke server ini.
	SSHPort int `json:"sshPort"`
	// SSHAllowed true kalau sudah ada aturan yang mengizinkan SSHPort.
	// Dipakai UI untuk memblokir pengaktifan firewall yang akan mengunci
	// user dari servernya sendiri.
	SSHAllowed bool `json:"sshAllowed"`
	// OwnerDetectable false di firewalld: rich rule tidak punya kolom
	// komentar, jadi asal-usul aturan tidak bisa dipastikan di sana.
	OwnerDetectable bool `json:"ownerDetectable"`
	// DockerSubnets seluruh bridge network Docker yang TERDETEKSI di server,
	// beserta status cakupannya masing-masing. Dideteksi, bukan diasumsikan:
	// pool alamat Docker bisa diubah lewat daemon.json dan network bisa
	// dibuat dengan subnet sembarang.
	DockerSubnets []DockerSubnet `json:"dockerSubnets"`
	// DockerDBAllowed true hanya kalau SELURUH subnet di atas sudah tercakup
	// untuk SEMUA port database.
	DockerDBAllowed bool `json:"dockerDbAllowed"`
}

// ListResponse hasil ListRules.
type ListResponse struct {
	Status Status `json:"status"`
	Rules  []Rule `json:"rules"`
}

// Marker milik fitur lain di aplikasi ini. Aturan bermarker ditandai di UI
// supaya user tidak mengubahnya dari sini lalu bingung kenapa berubah lagi
// sendiri saat fitur asalnya dijalankan.
var knownOwners = []string{"poinhost-docker-access", "poinhost-port-scope"}

func detectOwner(comment string) string {
	for _, o := range knownOwners {
		if strings.Contains(comment, o) {
			return o
		}
	}
	return ""
}

// --- ufw ---

// ufwRuleRE membaca satu baris `ufw status numbered`. Kolomnya dipisah spasi
// berlapis, dan kolom "To" sendiri boleh berisi spasi (mis. "22/tcp (v6)"),
// jadi pemisahnya disandarkan pada kata kunci aksi di tengah baris — bukan
// pada jumlah kolom.
var ufwRuleRE = regexp.MustCompile(`^\[\s*(\d+)\]\s+(.+?)\s\s+(ALLOW|DENY|REJECT|LIMIT)\s+(IN|OUT)\s+(.+?)\s*$`)

// parseUFW membaca keluaran `ufw status numbered` jadi daftar Rule.
func parseUFW(out string) []Rule {
	rules := []Rule{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		m := ufwRuleRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		to, action, dir, from := strings.TrimSpace(m[2]), m[3], m[4], strings.TrimSpace(m[5])
		// Aturan OUT tidak dikelola modul ini (UI-nya soal akses masuk),
		// tapi tetap ditampilkan agar daftarnya jujur.
		comment := ""
		if i := strings.Index(from, "#"); i >= 0 {
			comment = strings.TrimSpace(from[i+1:])
			from = strings.TrimSpace(from[:i])
		}

		r := Rule{
			Action:  strings.ToLower(action),
			Comment: comment,
			Owner:   detectOwner(comment),
			Raw:     strings.TrimSpace(line),
		}
		if r.Action == "reject" {
			r.Action = "deny"
		}
		if strings.Contains(to, "(v6)") || strings.Contains(from, "(v6)") {
			r.IPv6 = true
			to = strings.TrimSpace(strings.ReplaceAll(to, "(v6)", ""))
			from = strings.TrimSpace(strings.ReplaceAll(from, "(v6)", ""))
		}
		r.Port, r.Protocol = splitPortSpec(to)
		if !strings.EqualFold(from, "Anywhere") {
			r.Source = from
		}
		if dir == "OUT" {
			// Ditandai lewat Raw saja; tidak ada field arah karena modul ini
			// hanya membuat aturan masuk.
			r.Comment = strings.TrimSpace(r.Comment + " (OUT)")
		}
		r.ID = ufwRuleID(r)
		rules = append(rules, r)
	}
	return rules
}

// splitPortSpec memecah spesifikasi port jadi port + protokol. Dipakai untuk
// KEDUA backend: "80/tcp", "22", "8000:9000/udp" (gaya ufw), "8000-9000/udp"
// (gaya firewalld), atau nama app profile ufw seperti "OpenSSH" (port
// dikosongkan — profile tidak punya port tunggal yang bisa ditampilkan).
//
// Rentang dinormalkan ke ":" supaya satu model data melayani dua backend yang
// memakai pemisah berbeda. Nilai asli tetap disimpan pemanggilnya untuk ID
// penghapusan, karena firewalld hanya menerima ejaannya sendiri.
func splitPortSpec(spec string) (port, proto string) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", ""
	}
	if i := strings.LastIndex(spec, "/"); i >= 0 {
		port, proto = spec[:i], strings.ToLower(spec[i+1:])
	} else {
		port = spec
	}
	port = strings.ReplaceAll(port, "-", ":")
	// Bukan angka/rentang → app profile atau "Anywhere", bukan port.
	if _, err := strconv.Atoi(strings.SplitN(port, ":", 2)[0]); err != nil {
		return "", proto
	}
	return port, proto
}

// ufwRuleID menyusun ulang spesifikasi aturan untuk `ufw delete <spec>`.
//
// Penghapusan SENGAJA memakai spesifikasi, bukan nomor baris: nomor bergeser
// setiap kali ada aturan lain dihapus, jadi menghapus "nomor 3" berdasarkan
// daftar yang dibaca beberapa detik lalu bisa menghapus aturan yang salah.
func ufwRuleID(r Rule) string {
	if r.Port == "" {
		return "" // app profile / aturan tanpa port: tidak dihapus dari sini
	}
	// Hanya allow/deny yang bisa disusun ulang jadi spesifikasi hapus yang
	// sah. Aksi lain (limit, route) dibiarkan tanpa ID supaya tombol hapusnya
	// mati di UI — lebih baik daripada memunculkan error setelah diklik.
	if r.Action != "allow" && r.Action != "deny" {
		return ""
	}
	var b strings.Builder
	b.WriteString(r.Action)
	if r.Source != "" {
		b.WriteString(" from " + r.Source)
	}
	b.WriteString(" to any port " + r.Port)
	if r.Protocol != "" {
		b.WriteString(" proto " + r.Protocol)
	}
	return b.String()
}

// --- firewalld ---

var richRuleRE = regexp.MustCompile(`source address="([^"]+)"`)
var richPortRE = regexp.MustCompile(`port port="([^"]+)"`)
var richProtoRE = regexp.MustCompile(`protocol="([^"]+)"`)

// parseFirewalld membaca gabungan `--list-ports` dan `--list-rich-rules`.
// Keduanya diberi prefiks di skrip supaya bisa dibedakan di sini.
func parseFirewalld(out string) []Rule {
	rules := []Rule{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		switch {
		case strings.HasPrefix(line, "PORT="):
			// Format "80/tcp" — port terbuka untuk semua sumber.
			spec := strings.TrimPrefix(line, "PORT=")
			for _, item := range strings.Fields(spec) {
				port, proto := splitPortSpec(item)
				if port == "" {
					continue
				}
				r := Rule{Port: port, Protocol: proto, Action: "allow", Raw: item}
				r.ID = "port:" + item
				rules = append(rules, r)
			}
		case strings.HasPrefix(line, "SVC="):
			// Format "SVC=<nama>|<ports>", mis. "SVC=ssh|22/tcp".
			spec := strings.TrimPrefix(line, "SVC=")
			name, ports, _ := strings.Cut(spec, "|")
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			rules = append(rules, Rule{
				Service:      name,
				ServicePorts: strings.TrimSpace(ports),
				Action:       "allow",
				Raw:          name,
				ID:           "service:" + name,
			})
		case strings.HasPrefix(line, "RICH="):
			spec := strings.TrimPrefix(line, "RICH=")
			if spec == "" {
				continue
			}
			r := Rule{Action: "allow", Raw: spec, ID: "rich:" + spec}
			if strings.Contains(spec, " drop") || strings.Contains(spec, " reject") {
				r.Action = "deny"
			}
			if m := richRuleRE.FindStringSubmatch(spec); m != nil {
				r.Source = m[1]
			}
			if m := richPortRE.FindStringSubmatch(spec); m != nil {
				r.Port = strings.ReplaceAll(m[1], "-", ":")
			}
			// protocol="..." muncul dua kali di rich rule (family & port);
			// yang relevan adalah yang tcp/udp.
			for _, m := range richProtoRE.FindAllStringSubmatch(spec, -1) {
				if protocols[strings.ToLower(m[1])] {
					r.Protocol = strings.ToLower(m[1])
				}
			}
			if strings.Contains(spec, `family="ipv6"`) {
				r.IPv6 = true
			}
			rules = append(rules, r)
		}
	}
	return rules
}

// sshAllowed memeriksa apakah ada aturan allow yang mencakup port SSH.
//
// Ini penjaga anti-terkunci: dipakai sebelum mengaktifkan firewall dengan
// kebijakan default deny incoming. Rentang port ikut dihitung — "22:2222"
// mencakup 22.
func sshAllowed(rules []Rule, sshPort int) bool {
	for _, r := range rules {
		if r.Action != "allow" {
			continue
		}
		// Aturan berbentuk service firewalld: portnya ada di ServicePorts
		// ("22/tcp"), bukan di Port. Tanpa cabang ini, server RHEL yang
		// membuka SSH lewat `--add-service=ssh` akan salah dilaporkan
		// "SSH belum diizinkan".
		if r.ServicePorts != "" {
			for _, item := range strings.Fields(r.ServicePorts) {
				port, proto := splitPortSpec(item)
				if port == "" || (proto != "" && proto != "tcp") {
					continue
				}
				if portCovers(port, sshPort) {
					return true
				}
			}
			continue
		}
		if r.Port == "" {
			continue
		}
		if r.Protocol != "" && r.Protocol != "tcp" {
			continue
		}
		if portCovers(r.Port, sshPort) {
			return true
		}
	}
	return false
}

func portCovers(spec string, target int) bool {
	lo, hi, ok := parsePortSpec(spec)
	if !ok {
		return false
	}
	return target >= lo && target <= hi
}

func parsePortSpec(spec string) (lo, hi int, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(spec), ":", 2)
	lo, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	hi = lo
	if len(parts) == 2 {
		hi, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, false
		}
	}
	return lo, hi, true
}

// parseUFWAdded membaca keluaran `ufw show added`, yang dipakai saat ufw
// TERPASANG TAPI MATI — dalam keadaan itu `ufw status` tidak menampilkan
// aturan apa pun.
//
// Formatnya berbeda dari `ufw status numbered`: bukan tabel berkolom,
// melainkan daftar perintah yang bisa dijalankan ulang, dan bentuknya dua
// macam — ringkas ("ufw allow 22/tcp") atau lengkap ("ufw allow from
// 10.0.0.0/8 to any port 3306 proto tcp"). Keduanya ditangani di sini.
func parseUFWAdded(out string) []Rule {
	rules := []Rule{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		line = strings.TrimPrefix(line, "ADDED=")
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ufw ") {
			continue // baris judul "Added user rules (...)"
		}
		if r, ok := parseUFWAddedLine(strings.TrimPrefix(line, "ufw ")); ok {
			rules = append(rules, r)
		}
	}
	return rules
}

func parseUFWAddedLine(spec string) (Rule, bool) {
	r := Rule{Raw: "ufw " + spec}

	// Komentar dikupas dulu supaya isinya (yang boleh berisi spasi) tidak
	// ikut terpecah saat baris dipotong per kata.
	if i := strings.Index(spec, "comment "); i >= 0 {
		r.Comment = strings.Trim(strings.TrimSpace(spec[i+len("comment "):]), "'\"")
		r.Owner = detectOwner(r.Comment)
		spec = strings.TrimSpace(spec[:i])
	}

	fields := strings.Fields(spec)
	if len(fields) == 0 {
		return Rule{}, false
	}
	switch fields[0] {
	case "allow", "deny":
		r.Action = fields[0]
	case "reject":
		r.Action = "deny"
	default:
		// limit/route/dsb tidak dikelola dari sini, tapi tetap ditampilkan.
		r.Action = fields[0]
	}

	for i := 1; i < len(fields); i++ {
		switch fields[i] {
		case "from":
			if i+1 < len(fields) {
				if !strings.EqualFold(fields[i+1], "any") {
					r.Source = fields[i+1]
				}
				i++
			}
		case "port":
			if i+1 < len(fields) {
				r.Port, _ = splitPortSpec(fields[i+1])
				i++
			}
		case "proto":
			if i+1 < len(fields) {
				r.Protocol = strings.ToLower(fields[i+1])
				i++
			}
		case "to", "any", "in", "out":
			// kata sambung sintaks ufw, tidak membawa nilai
		default:
			// Bentuk ringkas: "22/tcp" atau "22" langsung setelah aksi.
			if r.Port == "" {
				if p, proto := splitPortSpec(fields[i]); p != "" {
					r.Port, r.Protocol = p, proto
				}
			}
		}
	}

	if strings.Contains(spec, "::") || strings.Contains(r.Source, ":") {
		r.IPv6 = true
	}
	r.ID = ufwRuleID(r)
	return r, true
}
