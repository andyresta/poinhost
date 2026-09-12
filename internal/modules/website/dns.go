package website

import (
	"fmt"
	"net"
	"strings"
)

// DNSRecord satu baris record dalam preview zona DNS.
type DNSRecord struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Note    string `json:"note,omitempty"`
}

// DNSPreviewResponse hasil hitung zona DNS untuk satu domain parent — murni
// komputasi Go dari data yang SUDAH ada (Server.Host + listDomains), TANPA
// exec SSH sama sekali. poinhost (sama seperti homepoin) tidak menjalankan
// DNS server apa pun di VPS — ini cuma generator file zona BIND untuk
// di-import manual ke provider DNS (Cloudflare, dst).
type DNSPreviewResponse struct {
	Domain     string      `json:"domain"`
	ServerHost string      `json:"serverHost"`
	HostValid  bool        `json:"hostValid"`
	IPKind     string      `json:"ipKind"` // "ipv4" | "ipv6" | "invalid"
	TTL        int         `json:"ttl"`
	Records    []DNSRecord `json:"records"`
	ZoneText   string      `json:"zoneText"`
	Filename   string      `json:"filename"`
}

const dnsTTL = 3600

// DNSPreview menghitung preview zona DNS domain parent — apex + www + semua
// subdomain-nya, mengarah ke IP server. Sengaja TIDAK exec SSH: listDomains
// yang dipakai di sini sama dengan yang dipakai List/PHP/SSL, jadi biasanya
// sudah ter-cache dan gratis dipanggil dari sini juga.
func (s *Service) DNSPreview(serverID, domain string) (*DNSPreviewResponse, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	srv, err := s.servers.Get(serverID)
	if err != nil {
		return nil, err
	}
	info, err := s.getDomain(serverID, domain)
	if err != nil {
		return nil, err
	}
	if info.IsSubdomain {
		return nil, errFmt("DNS cuma bisa dihitung dari domain induk (parent), bukan subdomain")
	}

	host := strings.TrimSpace(srv.Host)
	ip := net.ParseIP(host)
	ipKind := "invalid"
	hostValid := false
	recordType := "A"
	if ip != nil {
		hostValid = true
		if ip.To4() != nil {
			ipKind = "ipv4"
			recordType = "A"
		} else {
			ipKind = "ipv6"
			recordType = "AAAA"
		}
	}

	domains, err := s.listDomains(serverID)
	if err != nil {
		return nil, err
	}

	records := []DNSRecord{
		{Type: recordType, Name: "@", Content: host, TTL: dnsTTL, Note: "apex -> IP server"},
		{Type: "CNAME", Name: "www", Content: domain + ".", TTL: dnsTTL, Note: "www -> apex"},
	}
	for _, d := range domains {
		if d.IsSubdomain && d.Parent == domain {
			label := strings.TrimSuffix(d.Domain, "."+domain)
			records = append(records, DNSRecord{Type: recordType, Name: label, Content: host, TTL: dnsTTL, Note: "subdomain Website"})
		}
	}

	zoneText := renderBINDZone(domain, host, records)

	return &DNSPreviewResponse{
		Domain:     domain,
		ServerHost: host,
		HostValid:  hostValid,
		IPKind:     ipKind,
		TTL:        dnsTTL,
		Records:    records,
		ZoneText:   zoneText,
		Filename:   domain + ".zone",
	}, nil
}

func renderBINDZone(domain, host string, records []DNSRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "; Dibuat oleh poinhost untuk import DNS (mis. Cloudflare)\n")
	fmt.Fprintf(&b, "; Domain: %s\n", domain)
	fmt.Fprintf(&b, "; Target IP: %s\n", host)
	fmt.Fprintf(&b, "$ORIGIN %s.\n", domain)
	fmt.Fprintf(&b, "$TTL %d\n", dnsTTL)
	for _, r := range records {
		name := r.Name
		if name == "" {
			name = "@"
		}
		content := r.Content
		if r.Type == "CNAME" && !strings.HasSuffix(content, ".") {
			content += "."
		}
		comment := ""
		if r.Note != "" {
			comment = " ; " + r.Note
		}
		fmt.Fprintf(&b, "%-16s %d IN %-6s %s%s\n", name, r.TTL, r.Type, content, comment)
	}
	return b.String()
}
