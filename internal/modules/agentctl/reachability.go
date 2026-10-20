package agentctl

import (
	"context"
	"strconv"
)

// Mengenali jalur dari HP ke agent — TANPA membuatnya.
//
// Agent tidak pernah mendengarkan di luar 127.0.0.1 (dipaksa
// agentcfg.ValidateListen), jadi yang menjangkaunya dari HP adalah vhost
// nginx yang mem-proxy ke port loopback itu, dengan TLS dari certbot.
// Membuat vhost itu adalah pekerjaan modul Website, dan tetap di sana:
// menduplikasinya di sini berarti ada dua tempat yang membuat vhost, dengan
// dua perangkat aturan yang lama-lama berbeda.
//
// Yang tersisa di sini hanya PENGENALAN: mencari domain yang kebetulan sudah
// mem-proxy ke port agent, supaya panel Mobile Control bisa menampilkan URL
// yang dipakai HP tanpa user mengetiknya ulang.

// Reachability adalah keadaan jalur HP -> agent di satu server.
type Reachability struct {
	Port int `json:"port"`

	NginxInstalled   bool `json:"nginxInstalled"`
	CertbotInstalled bool `json:"certbotInstalled"`

	// Domain terisi kalau ada vhost yang memang menunjuk ke agent.
	Domain     string `json:"domain,omitempty"`
	SSLEnabled bool   `json:"sslEnabled"`
	URL        string `json:"url,omitempty"`
}

func agentProxyTarget(port int) string {
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

// Reachability melaporkan jalur HP -> agent apa adanya.
func (s *Service) Reachability(ctx context.Context, serverID string) (*Reachability, error) {
	port, err := s.agentPort(ctx, serverID)
	if err != nil {
		return nil, err
	}
	return s.reachability(serverID, port)
}

func (s *Service) reachability(serverID string, port int) (*Reachability, error) {
	list, err := s.web.List(serverID)
	if err != nil {
		return nil, err
	}
	certbot, cerr := s.web.CertbotInstalled(serverID)
	if cerr != nil {
		// Tidak fatal: yang gagal cuma pengetahuan tentang certbot, dan
		// sisa laporannya tetap berguna.
		certbot = false
	}

	out := &Reachability{
		Port:             port,
		NginxInstalled:   list.Nginx.Installed,
		CertbotInstalled: certbot,
	}

	target := agentProxyTarget(port)
	for _, d := range list.Domains {
		if d.ProxyTarget != target {
			continue
		}
		out.Domain = d.Domain
		out.SSLEnabled = d.SSLEnabled
		out.URL = publishedURL(d.Domain, d.SSLEnabled)
		break
	}
	return out, nil
}

func publishedURL(domain string, ssl bool) string {
	if ssl {
		return "https://" + domain
	}
	return "http://" + domain
}

// agentPort membaca port agent dari config DI SERVER.
//
// Bukan DefaultPort: agent bisa dipasang di port lain, dan mem-proxy ke port
// yang salah menghasilkan 502 yang terlihat seperti agent mati.
func (s *Service) agentPort(ctx context.Context, serverID string) (int, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return 0, err
	}
	cfg, err := s.readConfig(ctx, access)
	if err != nil {
		return 0, err
	}
	return portOf(cfg.Listen), nil
}
