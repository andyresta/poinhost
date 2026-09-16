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
//
// Scope memberi kontrol eksplisit, DUA pilihan saja:
//
//   - "public"  — bind ke 0.0.0.0, bisa dijangkau dari internet.
//   - "private" — bind ke 127.0.0.1, HANYA bisa dijangkau dari dalam server
//     itu sendiri. Reverse proxy di host (nginx) tetap bisa meneruskan ke
//     sini; dari luar server, portnya tidak ada sama sekali.
//
// SEBELUMNYA ada pilihan ketiga, "intranet" (hanya dari LAN), yang bekerja
// dengan membiarkan port bind ke 0.0.0.0 lalu memasang aturan ufw/firewalld
// yang membatasi sumbernya ke rentang RFC1918. Itu DIHAPUS karena tidak
// benar-benar berlaku: aturan ufw hidup di rantai INPUT, sedangkan trafik ke
// port container yang di-publish masuk lewat PREROUTING (DNAT) lalu FORWARD
// melalui rantai DOCKER-USER — tidak pernah menyentuh INPUT. Hasilnya port
// bertanda "intranet" sama terbukanya dengan "public", padahal UI menyatakan
// sebaliknya. Membatasi port Docker secara benar butuh aturan di DOCKER-USER,
// dan itu lapisan yang jauh lebih rapuh untuk dikelola otomatis daripada
// sekadar memilih alamat bind.
//
// Karena itu pembatasan sekarang dilakukan MURNI lewat alamat bind. Tidak ada
// aturan firewall yang ditulis fitur ini sama sekali, jadi tidak ada lagi
// kemungkinan UI menjanjikan proteksi yang tidak terjadi.
const (
	portScopePublic  = "public"
	portScopePrivate = "private"
)

// portScopeLabelPrefix menandai container label yang menyimpan pilihan scope
// per port.
//
// Sebenarnya sekarang scope bisa disimpulkan langsung dari alamat bind
// (127.0.0.1 = private, selain itu public), tapi labelnya tetap ditulis
// supaya pilihan user terbaca eksplisit dan tidak bergantung pada penafsiran
// bind-address — termasuk kalau suatu saat ada bentuk bind lain.
const portScopeLabelPrefix = "poinhost.portscope."

// normalizePortScope memvalidasi & menormalkan nilai scope dari request UI.
//
// Nilai lama "intranet" dan "localhost" masih diterima demi container yang
// terlanjur berlabel itu, dan keduanya dipetakan ke "private" — untuk
// "localhost" artinya sama persis, sedangkan "intranet" DINAIKKAN
// proteksinya: yang dulu hanya dijanjikan kini benar-benar berlaku.
func normalizePortScope(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "":
		return portScopePublic, nil
	case portScopePublic:
		return portScopePublic, nil
	case portScopePrivate, "localhost", "intranet":
		return portScopePrivate, nil
	default:
		return "", fmt.Errorf("scope port %q tidak valid", raw)
	}
}

// hostIPForScope menghitung HostIP dari scope — satu-satunya sumber
// kebenaran, bukan input manual (lihat komentar PortMapping.HostIP).
func hostIPForScope(scope string) string {
	if scope == portScopePrivate {
		return "127.0.0.1"
	}
	return ""
}

func portScopeLabelKey(hostPort int, proto string) string {
	return portScopeLabelPrefix + strconv.Itoa(hostPort) + "." + proto
}

// syncPortScopeLabels membuang label scope lama lalu menulis ulang sesuai
// port yang sekarang — supaya label tidak menumpuk untuk port yang sudah
// tidak dipakai lagi.
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
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		scope := p.Scope
		if scope == "" {
			scope = portScopePublic
		}
		labels[portScopeLabelKey(p.HostPort, proto)] = scope
	}
	return labels
}
