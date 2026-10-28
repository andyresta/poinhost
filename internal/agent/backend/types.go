package backend

import "time"

// ServerInfo adalah mesin yang diurus agent ini, seperti dilihat lapisan API.
//
// Tipe ini dulu tinggal di paket bot, karena bot-lah satu-satunya yang
// memakainya. Setelah konektor Telegram dibuang, pemakainya tinggal API
// HTTP — jadi tempatnya pindah ke sini, di sisi yang memilikinya.
type ServerInfo struct {
	ID   string
	Name string
	Host string
}

// Status adalah snapshot kesehatan satu server.
type Status struct {
	Connection    string
	OSName        string
	Hostname      string
	UptimeSeconds int64
	CPUCores      int
	CPUUsedPct    float64
	MemUsedMB     int64
	MemTotalMB    int64
	DiskUsedGB    float64
	DiskTotalGB   float64
	CheckedAt     time.Time
	Stale         bool
	Error         string
}

// ServiceInfo satu unit systemd, seperti dilihat lapisan API.
type ServiceInfo struct {
	Name          string
	Description   string
	ActiveState   string
	SubState      string
	UnitFileState string
	Running       bool
	Enabled       bool
	CanEnable     bool
}

// FileEntry satu entri direktori di mesin ini.
type FileEntry struct {
	Name    string
	Path    string
	Size    int64
	IsDir   bool
	Mode    string
	ModTime string
}

// FileListing isi satu direktori.
type FileListing struct {
	Path    string
	Parent  string
	Entries []FileEntry
}

// FileText isi satu file teks.
type FileText struct {
	Path    string
	Content string
	Size    int64
}

// WebsiteInfo satu vhost nginx di mesin ini.
type WebsiteInfo struct {
	Domain      string
	Parent      string
	IsSubdomain bool
	Root        string
	Enabled     bool
	PHPVersion  string
	PHPEnabled  bool
	ProxyTarget string
	SSLEnabled  bool
}

// WebsiteListing seluruh vhost beserta keadaan nginx-nya.
type WebsiteListing struct {
	NginxInstalled bool
	NginxActive    bool
	NginxVersion   string
	Sites          []WebsiteInfo
}

// WebsiteSSL keadaan sertifikat satu domain.
type WebsiteSSL struct {
	Domain           string
	SSLEnabled       bool
	AutoRenewEnabled bool
	CertExists       bool

	// NotAfter kapan sertifikatnya kedaluwarsa, dalam RFC3339 kalau
	// formatnya terbaca — apa adanya kalau tidak. Inilah satu-satunya angka
	// di modul ini yang punya tenggat, dan karena itu satu-satunya yang
	// benar-benar pantas dilihat dari HP.
	NotAfter string

	// ExpiresInDays sisa hari, nil kalau tanggalnya tidak terbaca.
	//
	// Dihitung di sini, bukan di HP: `openssl -enddate` mengeluarkan format
	// yang hanya dipahami openssl, dan menyalin penguraiannya ke tiap klien
	// berarti tiap klien punya kesempatan sendiri untuk salah membacanya.
	ExpiresInDays *int

	CertDomains []string

	// MissingSANs domain yang terdaftar tapi BELUM tercakup sertifikat.
	// Selama ini tidak kosong, HTTPS untuk domain itu tidak bekerja walau
	// statusnya "SSL aktif" — dan `certbot renew` tidak memperbaikinya.
	MissingSANs []string
}

// ContainerInfo satu container Docker di mesin ini.
type ContainerInfo struct {
	ID      string
	Name    string
	Image   string
	Status  string
	State   string
	Ports   string
	Created string
}

// ContainerListing seluruh container beserta keadaan engine-nya.
type ContainerListing struct {
	DockerOK   bool
	Version    string
	Containers []ContainerInfo
}

// Reader adalah satu-satunya jalan lapisan API menyentuh dunia poinhost.
//
// Sengaja berupa interface sempit, bukan *servers.Service langsung: itu
// membuat perilaku endpoint bisa diuji tanpa SSH, database, maupun
// jaringan — dan menegaskan bahwa API tidak punya kemampuan lain selain
// yang tercantum di sini. Semuanya baca; tidak ada satu pun operasi yang
// mengubah server, sesuai tahap pertama Mobile Control.
//
// Tidak ada ListServers di sini, dan itu disengaja. Agent yang di-update
// dari versi lama bisa MASIH menyimpan server yang dulu ditautkan ke
// dalamnya; interface yang hanya bisa menyebut Self membuat server-server
// itu mustahil ikut terlapor lewat API, tanpa bergantung pada lapisan di
// atasnya ingat menyaring.
type Reader interface {
	// Self adalah mesin tempat agent berjalan. false berarti agent belum
	// (atau gagal) mendaftarkan dirinya sendiri.
	Self() (ServerInfo, bool)
	Status(id string) (Status, bool)

	// Services adalah daftar unit systemd di mesin ini. Bool keduanya
	// melaporkan apakah mesinnya memakai systemd sama sekali — server tanpa
	// systemd bukan kegagalan, cuma tidak punya modul ini.
	Services() ([]ServiceInfo, bool, error)

	// Files membaca isi satu direktori di mesin ini.
	//
	// Path-nya TIDAK dibatasi ke folder tertentu, mengikuti modul Files di
	// app desktop. Itu keputusan pemilik proyek, diambil setelah akibatnya
	// disebutkan: perangkat yang bisa memanggil ini bisa membaca apa pun
	// yang bisa dibaca root, termasuk kunci SSH dan devices.json milik agent
	// sendiri. Yang membatasinya bukan path, melainkan SIAPA yang boleh
	// memanggil — lihat controlAuth di lapisan API.
	Files(path string) (FileListing, error)

	// FileContent membaca isi satu file teks.
	FileContent(path string) (FileText, error)

	// Websites adalah seluruh vhost nginx di mesin ini.
	Websites() (WebsiteListing, error)

	// WebsiteSSL keadaan sertifikat satu domain. Dipisah dari daftarnya
	// karena ia MEMBACA berkas sertifikat — satu round-trip lagi per
	// domain, yang akan membuat daftar berisi dua puluh situs jadi dua
	// puluh kali lebih mahal kalau digabung.
	WebsiteSSL(domain string) (WebsiteSSL, error)

	// Containers adalah seluruh container Docker di mesin ini.
	Containers() (ContainerListing, error)

	// ContainerLogs membaca ekor log satu container.
	//
	// Isinya keluaran aplikasi yang BEBAS: kredensial, token, dan data
	// orang lain rutin muncul di sana. Jadi ia lebih dekat ke isi berkas
	// daripada ke daftar container, dan dijaga sekeras itu — lihat
	// controlAuth di lapisan API.
	ContainerLogs(id string, lines int) (string, error)
}

// Controller adalah satu-satunya permukaan yang MENGUBAH mesin.
//
// Dipisah dari Reader dengan sengaja, dan bukan sekadar demi kerapian: Mobile
// Control dimulai sebagai baca-saja, dan satu-satunya cara agar penambahan
// kemampuan mengubah tetap terlihat — di tipe, di baris mux, di review — adalah
// membuatnya mustahil tersembunyi di balik nama "Reader".
//
// Yang ada di sini DAFTAR TERTUTUP, bukan shell. Agent tidak pernah
// menjalankan perintah yang datang dari jaringan; ia hanya memetakan beberapa
// aksi yang sudah dikenalnya ke modul poinhost yang sudah ada.
type Controller interface {
	// ServiceAction menjalankan satu aksi pada satu unit di mesin ini.
	// Aksi yang diizinkan ditentukan pemanggilnya, bukan di sini.
	ServiceAction(name, action string) (ServiceInfo, error)

	// WriteFile menimpa isi satu berkas, membuatnya kalau belum ada.
	//
	// Satu operasi untuk dua hal yang di layar terasa berbeda: menyimpan
	// hasil edit dan mengunggah berkas baru sama-sama "tulis byte ini ke
	// path ini". Memisahkannya jadi dua jalur hanya akan menghasilkan dua
	// tempat yang bisa berbeda perlakuannya terhadap berkas yang sudah ada.
	WriteFile(path string, data []byte) error

	// RenameFile memindahkan satu berkas atau direktori.
	RenameFile(oldPath, newPath string) error

	// DeleteFiles menghapus berkas atau direktori. Direktori dihapus
	// REKURSIF, mengikuti modul Files di app desktop.
	DeleteFiles(paths []string) error

	// SetWebsiteEnabled menyalakan atau mematikan satu vhost.
	SetWebsiteEnabled(domain string, enabled bool) error

	// ContainerAction menjalankan satu aksi pada satu container.
	// Aksi yang diizinkan ditentukan pemanggilnya, bukan di sini.
	ContainerAction(id, action string) error
}
