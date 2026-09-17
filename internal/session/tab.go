// Package session mengelola konsep "tab" di UI — SATU tab merepresentasikan
// satu jendela kerja yang menunjuk ke satu server, mirip tab browser.
//
// Ini sengaja dipisah dari sshpool.Pool: Pool mengelola KONEKSI per server
// (dipakai bersama oleh semua tab yang menunjuk server yang sama — buka 3 tab
// ke server X tetap 1 pool koneksi shared untuk X, bukan 3x lipat), sedangkan
// Manager di sini murni mengelola state TAMPILAN (tab mana yang terbuka,
// modul apa yang sedang aktif di tab itu, urutan tab) dan siklus hidup sesi
// yang benar-benar unik per tab (terutama terminal PTY — lihat tab_terminal.go).
//
// Pemisahan ini yang tidak ada di homepoin (di sana konsep "tab" tidak
// eksis — satu halaman browser = satu context server, dan slot terminal
// dedicated di pool cuma satu per server sehingga tab terminal kedua akan
// memutus tab terminal pertama).
package session

import "time"

// Jenis tab. Tab "server" menunjuk ke satu server dan isinya modul-modul
// pengelolaan server; tab "migration" tidak menunjuk ke server manapun —
// server asal dan tujuannya dipilih di dalam panelnya sendiri.
const (
	KindServer    = "server"
	KindMigration = "migration"
)

// Tab merepresentasikan satu tab yang terbuka di UI.
type Tab struct {
	ID string `json:"id"`
	// Kind membedakan tab server dari tab migrasi. Kosong dianggap
	// "server" demi tab lama yang tersimpan sebelum kolom ini ada.
	Kind string `json:"kind"`
	// ServerID kosong untuk tab migrasi.
	ServerID     string    `json:"serverId"`
	Title        string    `json:"title"`
	ActiveModule string    `json:"activeModule"`
	Position     int       `json:"position"`
	CreatedAt    time.Time `json:"createdAt"`
	LastActiveAt time.Time `json:"lastActiveAt"`
}
