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
}
