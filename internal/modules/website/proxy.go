package website

import (
	"regexp"
	"strings"
)

// ProxyStatus status reverse-proxy satu domain — bagian dari vhost yang
// SAMA dipakai PHP/SSL (ProxyTarget/ProxyRules di DomainInfo), tab ini
// cuma UI+API untuk mengubah field itu, BUKAN mekanisme baru. Server-wide
// port-forward proxy (menu "Reverse Proxy" terpisah di homepoin, tidak
// terikat satu domain) sengaja tidak ikut di-porting pada tahap ini.
type ProxyStatus struct {
	Domain      string      `json:"domain"`
	Mode        string      `json:"mode"` // "static" | "php" | "proxy" | "mixed"
	ProxyTarget string      `json:"proxyTarget,omitempty"`
	ProxyRules  []ProxyRule `json:"proxyRules"`
	PHPEnabled  bool        `json:"phpEnabled"`
	PHPVersion  string      `json:"phpVersion,omitempty"`
}

// ProxySetDomainRequest mengaktifkan reverse-proxy untuk SELURUH domain
// (menggantikan static/PHP di path "/") ke satu target.
type ProxySetDomainRequest struct {
	ServerID  string `json:"serverId"`
	Domain    string `json:"domain"`
	Target    string `json:"target"`
	WebSocket bool   `json:"webSocket"`
}

// ProxyRuleRequest menambah/mengganti satu aturan proxy per-path.
type ProxyRuleRequest struct {
	ServerID  string `json:"serverId"`
	Domain    string `json:"domain"`
	Path      string `json:"path"`
	Target    string `json:"target"`
	WebSocket bool   `json:"webSocket"`
}

var proxyTargetRE = regexp.MustCompile(`^https?://[^\s'"]+$`)

func normalizeProxyPath(raw string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

func validateProxyTarget(target string) error {
	if !proxyTargetRE.MatchString(strings.TrimSpace(target)) {
		return errFmt("target proxy tidak valid — harus URL http:// atau https://, tanpa spasi/kutip")
	}
	return nil
}

// ProxyStatus membaca status proxy satu domain — dari getDomain yang sudah
// ter-cache (lihat domains.go), jadi biasanya 0 round-trip SSH tambahan.
func (s *Service) ProxyStatus(serverID, domain string) (*ProxyStatus, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	info, err := s.getDomain(serverID, domain)
	if err != nil {
		return nil, err
	}
	mode := "static"
	switch {
	case info.ProxyTarget != "":
		mode = "proxy"
	case info.PHPEnabled:
		mode = "php"
	}
	if info.ProxyTarget == "" && len(info.ProxyRules) > 0 {
		mode = "mixed"
	}
	return &ProxyStatus{
		Domain:      domain,
		Mode:        mode,
		ProxyTarget: info.ProxyTarget,
		ProxyRules:  info.ProxyRules,
		PHPEnabled:  info.PHPEnabled,
		PHPVersion:  info.PHPVersion,
	}, nil
}

// ProxySetDomain mengaktifkan proxy untuk SELURUH domain ke satu target,
// menggantikan static/PHP di path "/" (lihat buildLocationBlocks — proxy
// whole-domain selalu menang atas PHP).
func (s *Service) ProxySetDomain(req ProxySetDomainRequest) error {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return err
	}
	if err := validateProxyTarget(req.Target); err != nil {
		return err
	}
	info, err := s.getDomain(req.ServerID, domain)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}

	opts := optionsFromInfo(info)
	opts.ProxyTarget = strings.TrimSpace(req.Target)
	opts.ProxyWebSocket = req.WebSocket

	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)
	return s.rewriteVhost(access, info.ConfigPath, opts)
}

// ProxyDisableDomain melepas proxy whole-domain (kembali ke static/PHP).
func (s *Service) ProxyDisableDomain(serverID, domain string) error {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return err
	}
	info, err := s.getDomain(serverID, domain)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}

	opts := optionsFromInfo(info)
	opts.ProxyTarget = ""

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)
	return s.rewriteVhost(access, info.ConfigPath, opts)
}

// ProxySetRule menambah atau mengganti (berdasarkan Path) satu aturan proxy
// per-path — bisa hidup berdampingan dengan PHP/static di path lain.
func (s *Service) ProxySetRule(req ProxyRuleRequest) error {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return err
	}
	path := normalizeProxyPath(req.Path)
	if err := validateProxyTarget(req.Target); err != nil {
		return err
	}
	info, err := s.getDomain(req.ServerID, domain)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}

	rules := make([]ProxyRule, 0, len(info.ProxyRules)+1)
	replaced := false
	for _, r := range info.ProxyRules {
		if r.Path == path {
			rules = append(rules, ProxyRule{Path: path, Target: strings.TrimSpace(req.Target), WebSocket: req.WebSocket})
			replaced = true
			continue
		}
		rules = append(rules, r)
	}
	if !replaced {
		rules = append(rules, ProxyRule{Path: path, Target: strings.TrimSpace(req.Target), WebSocket: req.WebSocket})
	}

	opts := optionsFromInfo(info)
	opts.ProxyRules = rules

	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)
	return s.rewriteVhost(access, info.ConfigPath, opts)
}

// ProxyDeleteRule menghapus satu aturan proxy per-path.
func (s *Service) ProxyDeleteRule(serverID, domain, path string) error {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return err
	}
	path = normalizeProxyPath(path)
	info, err := s.getDomain(serverID, domain)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}

	rules := make([]ProxyRule, 0, len(info.ProxyRules))
	for _, r := range info.ProxyRules {
		if r.Path != path {
			rules = append(rules, r)
		}
	}
	opts := optionsFromInfo(info)
	opts.ProxyRules = rules

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)
	return s.rewriteVhost(access, info.ConfigPath, opts)
}
