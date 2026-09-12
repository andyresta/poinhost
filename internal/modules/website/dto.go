package website

// ProxyRule satu aturan reverse-proxy berbasis path di dalam satu vhost
// (mis. "/api" -> "http://127.0.0.1:3000") — alternatif dari static/PHP
// untuk sebagian path, bukan untuk keseluruhan domain.
type ProxyRule struct {
	Path      string `json:"path"`
	Target    string `json:"target"`
	WebSocket bool   `json:"webSocket,omitempty"`
}

// DomainInfo satu domain/subdomain yang dikelola poinhost di server.
type DomainInfo struct {
	Domain      string `json:"domain"`
	Parent      string `json:"parent,omitempty"`
	IsSubdomain bool   `json:"isSubdomain"`
	ServerNames string `json:"serverNames"`
	Root        string `json:"root"`
	ConfigPath  string `json:"configPath"`
	Enabled     bool   `json:"enabled"`

	PHPVersion string `json:"phpVersion,omitempty"`
	PHPEnabled bool   `json:"phpEnabled"`

	ProxyTarget string      `json:"proxyTarget,omitempty"`
	ProxyRules  []ProxyRule `json:"proxyRules,omitempty"`

	SSLEnabled        bool   `json:"sslEnabled"`
	SSLCertificate    string `json:"sslCertificate,omitempty"`
	SSLCertificateKey string `json:"sslCertificateKey,omitempty"`
}

// NginxStatus status instalasi & service Nginx di server (untuk wizard).
type NginxStatus struct {
	Installed      bool   `json:"installed"`
	Version        string `json:"version,omitempty"`
	Active         bool   `json:"active"`
	Enabled        bool   `json:"enabled"`
	DistroID       string `json:"distroId,omitempty"`
	DistroName     string `json:"distroName,omitempty"`
	PackageManager string `json:"packageManager,omitempty"`
	CanInstall     bool   `json:"canInstall"`
}

// ListResponse daftar domain + status Nginx di satu server.
type ListResponse struct {
	Nginx   NginxStatus  `json:"nginx"`
	Domains []DomainInfo `json:"domains"`
}

// DomainRequest permintaan yang menunjuk satu domain di satu server.
type DomainRequest struct {
	ServerID string `json:"serverId"`
	Domain   string `json:"domain"`
}

// CreateDomainRequest membuat domain (parent) baru — static-only, tanpa PHP/SSL.
type CreateDomainRequest struct {
	ServerID string `json:"serverId"`
	Domain   string `json:"domain"`
}

// CreateSubdomainRequest membuat subdomain di bawah domain parent yang sudah ada.
type CreateSubdomainRequest struct {
	ServerID  string `json:"serverId"`
	Parent    string `json:"parent"`
	Subdomain string `json:"subdomain"`
}

// DeleteDomainRequest menghapus domain/subdomain (dan opsional document root-nya).
type DeleteDomainRequest struct {
	ServerID   string `json:"serverId"`
	Domain     string `json:"domain"`
	RemoveRoot bool   `json:"removeRoot"`
}

// SetEnabledRequest mengaktifkan/menonaktifkan satu domain (tanpa menghapusnya).
type SetEnabledRequest struct {
	ServerID string `json:"serverId"`
	Domain   string `json:"domain"`
	Enabled  bool   `json:"enabled"`
}

// CreateWebsiteRequest alur "Buat Website" gabungan — domain + PHP (opsional) +
// SSL (opsional) dalam SATU submit, beda dari homepoin yang mengharuskan 3
// kunjungan halaman terpisah (Domains -> PHP -> SSL) untuk hasil yang sama.
type CreateWebsiteRequest struct {
	ServerID   string `json:"serverId"`
	Domain     string `json:"domain"`
	PHPVersion string `json:"phpVersion,omitempty"` // "" = static-only
	EnableSSL  bool   `json:"enableSsl"`
	SSLEmail   string `json:"sslEmail,omitempty"`
}

// CreateWebsiteResult hasil alur gabungan — Warnings berisi langkah opsional
// (PHP/SSL) yang gagal TANPA membatalkan domain yang sudah berhasil dibuat,
// supaya kegagalan SSL (mis. DNS belum diarahkan) tidak menghapus balik situs
// yang baru saja jadi.
type CreateWebsiteResult struct {
	Domain   DomainInfo `json:"domain"`
	Warnings []string   `json:"warnings,omitempty"`
}

// ---------------------------------------------------------------------
// PHP
// ---------------------------------------------------------------------

// PHPVersionInfo satu versi PHP-FPM yang terpasang di server.
type PHPVersionInfo struct {
	Version string `json:"version"`
	Socket  string `json:"socket"`
	Active  bool   `json:"active"`
}

// PHPStatus status PHP-FPM di server + PHP domain tertentu (kalau diisi).
type PHPStatus struct {
	Installed      bool             `json:"installed"`
	RepoConfigured bool             `json:"repoConfigured"`
	DistroID       string           `json:"distroId,omitempty"`
	DistroName     string           `json:"distroName,omitempty"`
	PackageManager string           `json:"packageManager,omitempty"`
	CanInstall     bool             `json:"canInstall"`
	Versions       []PHPVersionInfo `json:"versions"`
	Available      []string         `json:"available"`
	DomainVersion  string           `json:"domainVersion,omitempty"`
	DomainEnabled  bool             `json:"domainEnabled"`
}

// PHPInstallRequest memasang satu versi PHP-FPM baru di server.
type PHPInstallRequest struct {
	ServerID string `json:"serverId"`
	Version  string `json:"version"`
}

// PHPSetDomainRequest mengaktifkan satu versi PHP untuk satu domain.
type PHPSetDomainRequest struct {
	ServerID string `json:"serverId"`
	Domain   string `json:"domain"`
	Version  string `json:"version"`
}

// ---------------------------------------------------------------------
// SSL
// ---------------------------------------------------------------------

// SSLCertificateInfo info sertifikat Let's Encrypt yang tersimpan di server.
type SSLCertificateInfo struct {
	Domains  []string `json:"domains,omitempty"`
	NotAfter string   `json:"notAfter,omitempty"`
	CertPath string   `json:"certPath,omitempty"`
	KeyPath  string   `json:"keyPath,omitempty"`
	Exists   bool     `json:"exists"`
}

// SSLStatus status SSL untuk satu domain (parent) beserta keluarga SAN-nya.
type SSLStatus struct {
	Domain           string             `json:"domain"`
	IsParent         bool               `json:"isParent"`
	CertbotInstalled bool               `json:"certbotInstalled"`
	CanInstall       bool               `json:"canInstall"`
	DistroID         string             `json:"distroId,omitempty"`
	DistroName       string             `json:"distroName,omitempty"`
	PackageManager   string             `json:"packageManager,omitempty"`
	SSLEnabled       bool               `json:"sslEnabled"`
	CanIssue         bool               `json:"canIssue"`
	CanEnable        bool               `json:"canEnable"`
	SANs             []string           `json:"sans,omitempty"`
	Certificate      SSLCertificateInfo `json:"certificate"`
	NginxInstalled   bool               `json:"nginxInstalled"`
	Message          string             `json:"message,omitempty"`
}

// SSLIssueRequest menerbitkan sertifikat Let's Encrypt baru untuk satu domain
// (mencakup domain itu + www + semua subdomain-nya, lihat collectSANs).
type SSLIssueRequest struct {
	ServerID string `json:"serverId"`
	Domain   string `json:"domain"`
	Email    string `json:"email"`
}
