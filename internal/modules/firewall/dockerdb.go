package firewall

// Jalan pintas "Izinkan Docker ke database".
//
// Container yang menghubungi MySQL/PostgreSQL di HOST memakai alamat gateway
// bridge Docker (mis. host.docker.internal → 172.17.0.1). Itu alamat host,
// jadi lalu lintasnya masuk rantai INPUT dan kena kebijakan deny firewall —
// bukan rantai FORWARD seperti dugaan umum soal Docker. Akibatnya aplikasi
// di container menggantung menunggu database sampai timeout, sementara port
// yang di-publish tetap terlihat terbuka.
//
// Subnet-nya DIDETEKSI dari server, bukan diasumsikan. Rentang 172.16.0.0/12
// memang default Docker, tapi hanya default: `default-address-pools` di
// daemon.json bisa mengubahnya, dan siapa pun bisa membuat network dengan
// subnet sembarang (10.x, 192.168.x). Aturan firewall yang disusun dari
// asumsi akan diam-diam tidak mencakup network yang sebenarnya dipakai.

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// dockerAccessMarker disamakan dengan paket website supaya aturan yang
// dipasang lewat modul Database dan lewat panel Firewall adalah aturan yang
// SAMA, bukan dua aturan berbeda dengan maksud serupa.
const dockerAccessMarker = "poinhost-docker-access"

// databasePorts port database yang dibukakan. PostgreSQL ikut dibuka walau
// server hanya memakai MySQL (dan sebaliknya): aturan untuk port yang tidak
// ada layanannya tidak membuka apa pun, sementara menebak-nebak engine mana
// yang terpasang justru bisa salah dan menyisakan container yang tetap
// menggantung.
var databasePorts = []string{"3306", "5432"}

// DockerSubnet satu bridge network Docker beserta subnetnya.
type DockerSubnet struct {
	Network string `json:"network"`
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway"`
	// Covered true kalau SELURUH port database sudah punya aturan allow yang
	// mencakup subnet ini. Dihitung per subnet supaya UI bisa menunjukkan
	// network mana yang belum tercakup, bukan sekadar "sudah/belum".
	Covered bool `json:"covered"`
}

// dockerNetScript membaca seluruh subnet bridge Docker dalam SATU perintah
// docker — bukan satu panggilan per network, yang di server dengan belasan
// network akan terasa lambat.
//
// Network "host" dan "none" tidak punya IPAM, jadi otomatis tidak ikut
// keluar.
const dockerNetScript = `set +e
if ! command -v docker >/dev/null 2>&1; then exit 0; fi
docker network ls -q 2>/dev/null | xargs -r docker network inspect -f '{{range .IPAM.Config}}DOCKERNET={{$.Name}}|{{.Subnet}}|{{.Gateway}}
{{end}}' 2>/dev/null
exit 0`

// parseDockerSubnets dipisah supaya bisa diuji tanpa SSH.
func parseDockerSubnets(stdout string) []DockerSubnet {
	out := []DockerSubnet{}
	seen := map[string]bool{}
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if !strings.HasPrefix(line, "DOCKERNET=") {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(line, "DOCKERNET="), "|", 3)
		if len(parts) < 2 {
			continue
		}
		name, subnet := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if subnet == "" {
			continue
		}
		// Subnet IPv6 dilewati: aturan yang dipasang modul ini berbentuk IPv4,
		// dan mencampurnya akan menghasilkan aturan yang ditolak firewall.
		if _, _, err := net.ParseCIDR(subnet); err != nil || strings.Contains(subnet, ":") {
			continue
		}
		if seen[subnet] {
			continue
		}
		seen[subnet] = true
		gw := ""
		if len(parts) >= 3 {
			gw = strings.TrimSpace(parts[2])
		}
		out = append(out, DockerSubnet{Network: name, Subnet: subnet, Gateway: gw})
	}
	return out
}

// DetectDockerSubnets membaca subnet Docker yang benar-benar ada di server.
func (s *Service) DetectDockerSubnets(serverID string) ([]DockerSubnet, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	res, err := s.run(access, dockerNetScript, 25*time.Second)
	if err != nil {
		return nil, err
	}
	return parseDockerSubnets(res.Stdout), nil
}

// cidrCovers true kalau aturan dengan sumber `ruleSrc` mencakup SELURUH
// `subnet`.
//
// Pengecekannya containment penuh, bukan "beririsan": aturan untuk
// 172.17.0.0/16 TIDAK boleh dianggap mencakup 172.16.0.0/12, karena sebagian
// besar alamat di dalamnya tetap tertolak. Kesalahan arah ini yang akan
// membuat UI melaporkan "sudah aman" padahal sebagian container masih
// terblokir.
func cidrCovers(ruleSrc, subnet string) bool {
	if ruleSrc == "" {
		return false // "dari mana saja" bukan izin khusus Docker
	}
	_, ruleNet, err := net.ParseCIDR(ruleSrc)
	if err != nil {
		ip := net.ParseIP(ruleSrc)
		if ip == nil {
			return false
		}
		// Sumber berupa satu IP hanya mencakup subnet /32 yang sama.
		return subnet == ip.String()+"/32"
	}
	subIP, subNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return false
	}
	if !ruleNet.Contains(subIP) {
		return false
	}
	// Prefix aturan harus lebih longgar atau sama, kalau tidak ia cuma
	// mencakup sebagian subnet.
	ruleOnes, _ := ruleNet.Mask.Size()
	subOnes, _ := subNet.Mask.Size()
	return ruleOnes <= subOnes
}

// subnetAllowedForPort true kalau ada aturan allow yang mencakup subnet ini
// untuk port tersebut.
func subnetAllowedForPort(rules []Rule, subnet string, port int) bool {
	for _, r := range rules {
		if r.Action != "allow" || r.Port == "" {
			continue
		}
		if r.Protocol != "" && r.Protocol != "tcp" {
			continue
		}
		if !cidrCovers(r.Source, subnet) {
			continue
		}
		if portCovers(r.Port, port) {
			return true
		}
	}
	return false
}

// markCoverage mengisi field Covered tiap subnet, dan mengembalikan true
// kalau SEMUA subnet sudah tercakup untuk SEMUA port database.
func markCoverage(subnets []DockerSubnet, rules []Rule) bool {
	all := true
	for i := range subnets {
		covered := true
		for _, p := range databasePorts {
			port, err := strconv.Atoi(p)
			if err != nil {
				continue
			}
			if !subnetAllowedForPort(rules, subnets[i].Subnet, port) {
				covered = false
				break
			}
		}
		subnets[i].Covered = covered
		if !covered {
			all = false
		}
	}
	// Tanpa Docker sama sekali, tidak ada yang perlu diizinkan — tapi itu
	// BUKAN "sudah terpasang". Dibedakan oleh pemanggil lewat panjang slice.
	return all && len(subnets) > 0
}

// AllowDockerToDatabase memasang aturan allow dari subnet Docker yang
// TERDETEKSI ke port database host.
//
// Hanya subnet yang belum tercakup yang ditambahkan, jadi menekannya berulang
// tidak menumpuk aturan, dan server yang sudah punya aturan longgar (mis.
// 172.16.0.0/12) tidak ditambahi aturan baru sama sekali.
func (s *Service) AllowDockerToDatabase(serverID, zone string) (*ListResponse, error) {
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
	backend, err := s.activeBackend(access)
	if err != nil {
		return nil, err
	}
	if backend != "ufw" && backend != "firewalld" {
		return nil, fmt.Errorf("firewall di server ini tidak bisa diatur dari sini (backend: %s)", backend)
	}

	subnets, err := s.DetectDockerSubnets(serverID)
	if err != nil {
		return nil, err
	}
	if len(subnets) == 0 {
		return nil, fmt.Errorf("tidak ada bridge network Docker yang terdeteksi di server ini")
	}

	current, err := s.ListRules(serverID, z)
	if err != nil {
		return nil, err
	}

	var missing []string
	for _, sn := range subnets {
		for _, p := range databasePorts {
			port, err := strconv.Atoi(p)
			if err != nil {
				continue
			}
			if !subnetAllowedForPort(current.Rules, sn.Subnet, port) {
				missing = append(missing, sn.Subnet+"|"+p)
			}
		}
	}
	if len(missing) == 0 {
		return current, nil // sudah tercakup seluruhnya
	}

	var script string
	switch backend {
	case "ufw":
		script = ufwDockerDBScript(missing)
	case "firewalld":
		script = firewalldDockerDBScript(missing, z)
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)
	if _, err := s.run(access, script, 60*time.Second); err != nil {
		return nil, err
	}
	return s.ListRules(serverID, z)
}

// ufwDockerDBScript menyusun aturan untuk pasangan "subnet|port" yang belum
// tercakup. `ufw allow` sendiri idempotent, tapi menyaring lebih dulu membuat
// daftar aturan tidak dipenuhi baris yang sebenarnya tidak perlu.
func ufwDockerDBScript(missing []string) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	for _, m := range missing {
		subnet, port, _ := strings.Cut(m, "|")
		b.WriteString("ufw allow from " + subnet + " to any port " + port +
			" proto tcp comment " + shellQuote(dockerAccessMarker) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func firewalldDockerDBScript(missing []string, zone string) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	for _, m := range missing {
		subnet, port, _ := strings.Cut(m, "|")
		rule := `rule family="ipv4" source address="` + subnet +
			`" port port="` + port + `" protocol="tcp" accept`
		b.WriteString("firewall-cmd --permanent " + zoneFlag(zone) +
			"--add-rich-rule=" + shellQuote(rule) + "\n")
	}
	b.WriteString("firewall-cmd --reload")
	return b.String()
}

// PortAllowedFromDocker menjawab satu pertanyaan konkret: apakah container
// Docker di host ini bisa menjangkau port tersebut di host?
//
// Dipakai modul Database supaya panel Settings-nya dan panel Firewall memakai
// SATU sumber kebenaran yang sama. Sebelumnya modul Database punya
// pengeceknya sendiri yang mencari KOMENTAR aturan ("poinhost-docker-access")
// — cara itu salah: aturan yang sah tapi ditulis lewat jalur lain (atau
// manual oleh user) tidak dikenali, sehingga UI melaporkan akses terblokir
// padahal kenyataannya jalan.
//
// Yang dinilai di sini adalah EFEKNYA: setiap subnet Docker yang terdeteksi
// harus tercakup aturan allow untuk port itu — tidak peduli siapa yang
// menulis aturannya.
func (s *Service) PortAllowedFromDocker(serverID string, port int) (allowed bool, backend string, err error) {
	out, err := s.ListRules(serverID, "")
	if err != nil {
		return false, "", err
	}
	backend = out.Status.Backend
	// Tidak ada firewall yang menyaring = tidak ada yang memblokir.
	if !out.Status.Active || backend == "none" {
		return true, backend, nil
	}
	// Firewall aktif tapi Docker tidak terpasang: tidak ada subnet yang perlu
	// diizinkan, jadi tidak ada yang bisa dinyatakan terblokir.
	if len(out.Status.DockerSubnets) == 0 {
		return true, backend, nil
	}
	for _, sn := range out.Status.DockerSubnets {
		if !subnetAllowedForPort(out.Rules, sn.Subnet, port) {
			return false, backend, nil
		}
	}
	return true, backend, nil
}
