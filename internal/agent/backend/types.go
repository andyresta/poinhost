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
}
