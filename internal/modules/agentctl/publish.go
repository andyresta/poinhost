package agentctl

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/andyresta/poinhost/internal/modules/website"
)

// Menerbitkan agent ke internet.
//
// Agent sendiri TIDAK pernah mendengarkan di luar 127.0.0.1 (dipaksa
// agentcfg.ValidateListen), jadi yang menjangkaunya dari HP adalah vhost
// nginx di server yang sama yang mem-proxy ke port loopback itu, dengan TLS
// dari certbot. Keduanya sudah dikelola modul website, jadi di sini tidak ada
// satu pun baris config nginx atau perintah certbot yang baru — fungsi ini
// hanya merangkai langkah yang sudah ada dalam urutan yang benar.
//
// Alasan jalurnya begitu, dan bukan sekadar membuat agent bind 0.0.0.0:
// terminasi TLS, rate limit, dan log akses adalah pekerjaan yang sudah
// dilakukan nginx dengan benar, dan menaruhnya di dalam agent berarti
// menulis ulang semuanya di proses yang berjalan sebagai root.

// PublishInfo adalah keadaan jalur HP -> agent di satu server.
type PublishInfo struct {
	Port int `json:"port"`

	NginxInstalled   bool `json:"nginxInstalled"`
	CertbotInstalled bool `json:"certbotInstalled"`

	// Candidates adalah domain yang sudah dikelola poinhost di server ini,
	// supaya user memilih, bukan mengetik ulang. Yang sudah dipakai website
	// lain ikut didaftar tapi ditandai Taken — menyembunyikannya hanya
	// membuat user bingung kenapa domainnya tidak muncul.
	Candidates []PublishCandidate `json:"candidates"`

	// Domain terisi kalau ada vhost yang memang menunjuk ke agent.
	Domain     string `json:"domain,omitempty"`
	SSLEnabled bool   `json:"sslEnabled"`
	URL        string `json:"url,omitempty"`

	Problem string `json:"problem,omitempty"`
}

// PublishCandidate satu domain yang bisa dipilih.
type PublishCandidate struct {
	Domain string `json:"domain"`
	// Taken: domain ini sudah melayani sesuatu yang lain (PHP, proxy ke
	// target lain, atau punya aturan proxy per-path). Memakainya berarti
	// mematikan situs itu, jadi Publish menolaknya.
	Taken bool `json:"taken"`
	// Agent: domain ini sudah menunjuk ke agent.
	Agent bool `json:"agent"`
}

// PublishRequest adalah pilihan user di form.
type PublishRequest struct {
	ServerID string `json:"serverId"`
	Domain   string `json:"domain"`
	// Email dipakai certbot. Kosong berarti terbitkan tanpa TLS dulu —
	// berguna saat DNS-nya baru saja diarahkan dan belum menyebar.
	Email string `json:"email"`
}

func agentProxyTarget(port int) string {
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

// PublishStatus melaporkan jalur HP -> agent apa adanya.
func (s *Service) PublishStatus(ctx context.Context, serverID string) (*PublishInfo, error) {
	port, err := s.agentPort(ctx, serverID)
	if err != nil {
		return nil, err
	}
	return s.publishInfo(serverID, port, "")
}

// publishInfo menyusun laporan dari daftar domain yang sedang ada.
func (s *Service) publishInfo(serverID string, port int, problem string) (*PublishInfo, error) {
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

	out := &PublishInfo{
		Port:             port,
		NginxInstalled:   list.Nginx.Installed,
		CertbotInstalled: certbot,
		Candidates:       make([]PublishCandidate, 0, len(list.Domains)),
		Problem:          problem,
	}

	target := agentProxyTarget(port)
	for _, d := range list.Domains {
		mine := d.ProxyTarget == target
		out.Candidates = append(out.Candidates, PublishCandidate{
			Domain: d.Domain,
			Taken:  !mine && domainInUse(d),
			Agent:  mine,
		})
		if mine {
			out.Domain = d.Domain
			out.SSLEnabled = d.SSLEnabled
			out.URL = publishedURL(d.Domain, d.SSLEnabled)
		}
	}
	return out, nil
}

// domainInUse: vhost ini sudah melayani sesuatu, jadi mengarahkannya ke agent
// akan mematikan yang sekarang berjalan.
func domainInUse(d website.DomainInfo) bool {
	return d.ProxyTarget != "" || d.PHPEnabled || len(d.ProxyRules) > 0
}

func publishedURL(domain string, ssl bool) string {
	if ssl {
		return "https://" + domain
	}
	return "http://" + domain
}

// Publish mengarahkan satu domain ke agent: vhost dibuat kalau belum ada,
// di-proxy ke port loopback agent, lalu — kalau email diisi — sertifikatnya
// diterbitkan dan langsung dipasang.
//
// Berbeda dari tab SSL di modul Website, yang sengaja memisahkan "terbitkan"
// dari "aktifkan" supaya situs production tidak berubah ke HTTPS sebelum
// user memverifikasi sertifikatnya. Di sini tidak ada yang bisa rusak:
// domainnya memang baru dibuat untuk agent, dan hasil yang dicari user adalah
// satu URL https yang bisa langsung ditempel ke HP.
func (s *Service) Publish(ctx context.Context, req PublishRequest) (*PublishInfo, error) {
	port, err := s.agentPort(ctx, req.ServerID)
	if err != nil {
		return nil, err
	}
	domain, err := website.NormalizeDomain(req.Domain)
	if err != nil {
		return nil, err
	}

	list, err := s.web.List(req.ServerID)
	if err != nil {
		return nil, err
	}
	if !list.Nginx.Installed {
		return nil, fmt.Errorf("nginx belum terpasang di server ini — pasang dulu dari modul Website")
	}

	target := agentProxyTarget(port)
	existing, found := findDomain(list.Domains, domain)
	switch {
	case found && existing.ProxyTarget == target:
		// Sudah menunjuk ke agent: lanjut saja ke tahap TLS. Menjalankan
		// ulang Publish adalah cara memperbaiki sertifikat yang gagal
		// terbit sebelumnya.
	case found && domainInUse(existing):
		// Satu-satunya langkah yang benar-benar merusak di alur ini.
		return nil, fmt.Errorf("%s sudah dipakai website lain — pilih domain atau subdomain lain", domain)
	case !found:
		if err := s.createPublishVhost(req.ServerID, domain, list.Domains); err != nil {
			return nil, err
		}
	}

	if err := s.web.ProxySetDomain(website.ProxySetDomainRequest{
		ServerID: req.ServerID,
		Domain:   domain,
		Target:   target,
	}); err != nil {
		return nil, fmt.Errorf("gagal mengarahkan %s ke agent: %w", domain, err)
	}

	email := strings.TrimSpace(req.Email)
	if email == "" {
		return s.publishInfo(req.ServerID, port, "Terbit tanpa TLS. Isi email lalu terbitkan lagi untuk HTTPS.")
	}
	if _, err := s.web.SSLIssue(website.SSLIssueRequest{
		ServerID: req.ServerID,
		Domain:   domain,
		Email:    email,
	}); err != nil {
		// Proxy-nya SUDAH terpasang dan domainnya sudah melayani lewat
		// HTTP, jadi kegagalan TLS tidak membatalkan apa pun — ia cuma
		// menyisakan satu langkah yang bisa diulang setelah DNS benar.
		return s.publishInfo(req.ServerID, port, "Sertifikat gagal terbit: "+trimForError(err.Error()))
	}
	if _, err := s.web.SSLEnable(req.ServerID, domain); err != nil {
		return s.publishInfo(req.ServerID, port, "Sertifikat terbit tapi belum terpasang di vhost: "+trimForError(err.Error()))
	}
	return s.publishInfo(req.ServerID, port, "")
}

// createPublishVhost membuat vhost untuk domain yang belum terdaftar.
//
// Kalau induknya sudah dikelola di server ini, ia didaftarkan sebagai
// SUBDOMAIN — bukan sekadar supaya rapi di daftar, tapi karena vhost domain
// biasa ikut memasang `www.<nama>` di server_name, dan `www.agent.example.com`
// adalah nama yang tidak pernah ada.
func (s *Service) createPublishVhost(serverID, domain string, existing []website.DomainInfo) error {
	if parent, label, ok := splitSubdomain(domain, existing); ok {
		if _, err := s.web.CreateSubdomain(website.CreateSubdomainRequest{
			ServerID:  serverID,
			Parent:    parent,
			Subdomain: label,
		}); err != nil {
			return fmt.Errorf("gagal membuat subdomain %s: %w", domain, err)
		}
		return nil
	}
	if _, err := s.web.Create(website.CreateDomainRequest{ServerID: serverID, Domain: domain}); err != nil {
		return fmt.Errorf("gagal membuat domain %s: %w", domain, err)
	}
	return nil
}

// splitSubdomain mencari induk TERDEKAT yang dikelola di server ini.
//
// Terdekat, bukan yang pertama ketemu: untuk agent.app.example.com dengan
// app.example.com dan example.com sama-sama terdaftar, induknya adalah
// app.example.com. Induk yang sendirinya subdomain dilewati — modul website
// hanya mengenal satu tingkat parent.
func splitSubdomain(domain string, existing []website.DomainInfo) (parent, label string, ok bool) {
	for _, d := range existing {
		if d.IsSubdomain || !strings.HasSuffix(domain, "."+d.Domain) {
			continue
		}
		if len(d.Domain) <= len(parent) {
			continue
		}
		parent = d.Domain
	}
	if parent == "" {
		return "", "", false
	}
	label = strings.TrimSuffix(domain, "."+parent)
	// Modul website hanya membuat subdomain satu label; yang lebih dalam
	// dibuat sebagai domain tersendiri.
	if label == "" || strings.Contains(label, ".") {
		return "", "", false
	}
	return parent, label, true
}

// Unpublish melepas proxy ke agent.
//
// Vhost-nya SENGAJA tidak dihapus: ia mungkin dibuat user jauh sebelum ini,
// dan menghapus vhost beserta document root-nya karena user menekan "berhenti
// terbitkan" adalah kejutan yang tidak bisa dibatalkan. Yang dilepas hanya
// proxy-nya; menghapus domainnya tetap ada di modul Website.
func (s *Service) Unpublish(ctx context.Context, serverID string) (*PublishInfo, error) {
	port, err := s.agentPort(ctx, serverID)
	if err != nil {
		return nil, err
	}
	info, err := s.publishInfo(serverID, port, "")
	if err != nil {
		return nil, err
	}
	if info.Domain == "" {
		return info, nil
	}
	if err := s.web.ProxyDisableDomain(serverID, info.Domain); err != nil {
		return nil, err
	}
	return s.publishInfo(serverID, port, "")
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

func findDomain(list []website.DomainInfo, domain string) (website.DomainInfo, bool) {
	for _, d := range list {
		if d.Domain == domain {
			return d, true
		}
	}
	return website.DomainInfo{}, false
}
