// Package backup mengimplementasikan export/import data poinhost sebagai
// SATU file terenkripsi passphrase — mekanisme pindah data antar perangkat
// yang paling sederhana: TANPA akun, TANPA server, TANPA layanan pihak
// ketiga. User yang memegang sendiri file & passphrase-nya, lewat kanal
// apa pun yang mereka percaya (USB, Drive pribadi, email, dst).
//
// Ini SENGAJA bukan sinkronisasi otomatis/real-time — hanya mekanisme
// export sekali → import sekali, konsisten dengan sikap poinhost yang
// tanpa login/server (lihat ARCHITECTURE.md §7 & §14): begitu data
// (termasuk kredensial SSH & database) berpindah lewat jaringan/akun pihak
// ketiga, itu menambah permukaan risiko yang sebelumnya sengaja dihindari.
package backup

// Payload adalah seluruh isi arsip — representasi stabil dari data yang
// disimpan LOKAL poinhost, dikumpulkan dari beberapa modul (servers,
// website), BUKAN dump mentah tabel SQLite supaya format ini tidak ikut
// berubah kalau skema internal berubah. SENGAJA TIDAK menyertakan
// activity_logs (audit trail, bisa besar & tidak relevan dipindah) atau
// ui_tabs (state UI device-specific, tidak masuk akal direstore ke
// perangkat lain).
type Payload struct {
	Version    int    `json:"version"`
	ExportedAt string `json:"exportedAt"`

	Servers         []ServerExport         `json:"servers"`
	DBCredentials   []DBCredentialExport   `json:"dbCredentials,omitempty"`
	DomainDatabases []DomainDatabaseExport `json:"domainDatabases,omitempty"`
}

// ServerExport satu server + kredensial SSH-nya. KeyFileB64 opsional
// (dicentang eksplisit oleh user saat export) — isi private key file yang
// ditunjuk KeyPath, base64, supaya server auth key-based langsung jalan di
// perangkat baru tanpa perlu user menyalin manual kunci SSH-nya sendiri.
type ServerExport struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Host       string   `json:"host"`
	Port       int      `json:"port"`
	Username   string   `json:"username"`
	AuthType   string   `json:"authType"`
	KeyPath    string   `json:"keyPath,omitempty"`
	KeyFileB64 string   `json:"keyFileB64,omitempty"`
	Password   string   `json:"password,omitempty"`
	Tags       []string `json:"tags"`
	Color      string   `json:"color"`
	Notes      string   `json:"notes"`
	UseSudo    bool     `json:"useSudo"`
}

// DBCredentialExport satu kredensial database MySQL tersimpan (lihat
// internal/modules/website/dbcreds.go) — Password diambil dari vault lokal
// mesin sumber, akan disimpan lagi ke vault lokal mesin tujuan saat import.
type DBCredentialExport struct {
	ServerID string `json:"serverId"`
	Engine   string `json:"engine"`
	Username string `json:"username"`
	Host     string `json:"host"`
	Password string `json:"password"`
}

// DomainDatabaseExport satu tautan domain<->database (kurasi lokal, lihat
// internal/modules/website/domaindb.go).
type DomainDatabaseExport struct {
	ServerID string `json:"serverId"`
	Domain   string `json:"domain"`
	Engine   string `json:"engine"`
	Database string `json:"database"`
}

// ImportSummary ringkasan hasil satu operasi import — ditampilkan ke user
// supaya jelas apa yang benar-benar berubah, bukan cuma "berhasil".
type ImportSummary struct {
	ServersImported       int `json:"serversImported"`
	DBCredentialsImported int `json:"dbCredentialsImported"`
	DomainLinksImported   int `json:"domainLinksImported"`
}
