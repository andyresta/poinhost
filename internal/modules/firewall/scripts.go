package firewall

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// firewallRuleMarker menandai aturan yang dibuat lewat modul ini. Hanya
// terpakai di ufw — lihat catatan OwnerDetectable di Status.
const firewallRuleMarker = "poinhost-firewall"

// --- ufw ---

func ufwAddScript(port, proto, source, action, comment string) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	b.WriteString("ufw ")
	b.WriteString(action)
	if source != "" {
		b.WriteString(" from " + source)
	}
	b.WriteString(" to any port " + port + " proto " + proto)
	tag := firewallRuleMarker
	if comment != "" {
		tag = comment + " " + firewallRuleMarker
	}
	b.WriteString(" comment " + shellQuote(tag))
	return b.String()
}

// ufwDeleteSpecRE menjaga agar hanya spesifikasi yang bentuknya persis
// seperti hasil ufwRuleID yang bisa dijalankan. ID datang dari frontend, dan
// meski disusun backend saat membaca daftar, tidak ada jaminan ia kembali
// utuh — jadi divalidasi ulang, bukan dipercaya.
var ufwDeleteSpecRE = regexp.MustCompile(
	`^(allow|deny)( from [0-9a-fA-F:.]{2,45}(/\d{1,3})?)? to any port \d{1,5}(:\d{1,5})?( proto (tcp|udp))?$`)

func validateUFWDeleteSpec(spec string) error {
	if !ufwDeleteSpecRE.MatchString(spec) {
		return errors.New("spesifikasi aturan tidak valid")
	}
	return nil
}

// ufwEnableScript menyalakan ufw SETELAH memastikan port SSH terbuka.
//
// Urutannya penting dan tidak boleh dibalik: aturan SSH dipasang lebih dulu,
// baru `ufw --force enable`. `--force` dipakai supaya ufw tidak menunggu
// jawaban interaktif "may disrupt existing ssh connections" — prompt itu
// tidak akan pernah terjawab lewat exec non-interaktif dan perintahnya akan
// menggantung sampai timeout.
func ufwEnableScript(sshPort int) string {
	p := strconv.Itoa(sshPort)
	return `set -e
ufw allow to any port ` + p + ` proto tcp comment ` + shellQuote("SSH poinhost "+firewallRuleMarker) + `
ufw --force enable`
}

// --- firewalld ---

// zoneFlag menghasilkan "--zone=X " atau string kosong untuk zone default.
func zoneFlag(zone string) string {
	if zone == "" {
		return ""
	}
	return "--zone=" + zone + " "
}

func firewalldAddScript(port, proto, source, action, zone string) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	if source == "" {
		// Tanpa sumber, port biasa sudah cukup dan lebih mudah dibaca
		// daripada rich rule. Hanya berlaku untuk allow: firewalld tidak
		// punya "remove-port" yang berarti deny — default zone-nya yang
		// menentukan, jadi deny tetap lewat rich rule.
		if action == "allow" {
			b.WriteString("firewall-cmd --permanent " + zoneFlag(zone) + "--add-port=" + port + "/" + proto + "\n")
			b.WriteString("firewall-cmd --reload")
			return b.String()
		}
		rule := `rule family="ipv4" port port="` + port + `" protocol="` + proto + `" drop`
		b.WriteString("firewall-cmd --permanent " + zoneFlag(zone) + "--add-rich-rule=" + shellQuote(rule) + "\n")
		b.WriteString("firewall-cmd --reload")
		return b.String()
	}
	family := "ipv4"
	if strings.Contains(source, ":") {
		family = "ipv6"
	}
	verb := "accept"
	if action == "deny" {
		verb = "drop"
	}
	rule := `rule family="` + family + `" source address="` + source + `" port port="` + port +
		`" protocol="` + proto + `" ` + verb
	b.WriteString("firewall-cmd --permanent " + zoneFlag(zone) + "--add-rich-rule=" + shellQuote(rule) + "\n")
	b.WriteString("firewall-cmd --reload")
	return b.String()
}

// richRuleSafeRE membatasi rich rule yang boleh dikirim balik untuk dihapus
// ke karakter yang memang dipakai sintaks rich rule — tidak ada `$`, backtick,
// titik koma, atau tanda kutip tunggal yang bisa keluar dari quoting.
var richRuleSafeRE = regexp.MustCompile(`^[\w \-.:/="]{1,300}$`)

var portSpecSafeRE = regexp.MustCompile(`^\d{1,5}(-\d{1,5})?/(tcp|udp)$`)

// parseFirewalldRuleID mengubah ID hasil ListRules jadi perintah penghapusan.
func parseFirewalldRuleID(id, zone string) (string, error) {
	z := zoneFlag(zone)
	switch {
	case strings.HasPrefix(id, "port:"):
		spec := strings.TrimPrefix(id, "port:")
		if !portSpecSafeRE.MatchString(spec) {
			return "", errors.New("spesifikasi port tidak valid")
		}
		return "set -e\nfirewall-cmd --permanent " + z + "--remove-port=" + spec + "\nfirewall-cmd --reload", nil
	case strings.HasPrefix(id, "service:"):
		// Bentuk aturan paling lazim di RHEL/CentOS: akses dibuka lewat nama
		// service, bukan nomor port.
		name, err := normalizeServiceName(strings.TrimPrefix(id, "service:"))
		if err != nil {
			return "", err
		}
		return "set -e\nfirewall-cmd --permanent " + z + "--remove-service=" + name + "\nfirewall-cmd --reload", nil
	case strings.HasPrefix(id, "rich:"):
		spec := strings.TrimPrefix(id, "rich:")
		if !richRuleSafeRE.MatchString(spec) || !strings.HasPrefix(spec, "rule ") {
			return "", errors.New("rich rule tidak valid")
		}
		return "set -e\nfirewall-cmd --permanent " + z + "--remove-rich-rule=" + shellQuote(spec) + "\nfirewall-cmd --reload", nil
	}
	return "", errors.New("aturan ini tidak bisa dihapus dari sini")
}

// firewalldEnableScript menyalakan firewalld dengan pengaman yang sama
// seperti ufw: port SSH dibuka lebih dulu, BARU service-nya dinyalakan.
//
// Urutan ini tidak boleh dibalik. Menyalakan dulu lalu membuka port berarti
// ada jeda saat firewalld sudah menegakkan kebijakan zone-nya tapi aturan SSH
// belum ada — dan kalau zone default-nya memblokir SSH, sesi ini putus
// sebelum perintah berikutnya sempat jalan.
//
// Saat firewalld masih mati, `firewall-cmd --permanent` tidak bisa dipakai
// (butuh daemon-nya hidup di banyak versi), jadi konfigurasinya ditulis lewat
// firewall-offline-cmd. Perintah --permanent sesudah service hidup tetap
// dijalankan sebagai jaring pengaman untuk versi yang tidak punya
// firewall-offline-cmd.
func firewalldEnableScript(sshPort int) string {
	p := strconv.Itoa(sshPort)
	return `set -e
if command -v firewall-offline-cmd >/dev/null 2>&1; then
  firewall-offline-cmd --add-port=` + p + `/tcp >/dev/null 2>&1 || true
fi
systemctl enable --now firewalld
firewall-cmd --permanent --add-port=` + p + `/tcp
firewall-cmd --reload`
}
