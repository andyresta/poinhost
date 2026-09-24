// Package transfer mendefinisikan paket data server yang dikirim app desktop
// ke poinhost-agent saat server ditautkan.
//
// Dipakai KEDUA sisi dari satu definisi supaya format kirim dan format terima
// tidak bisa menyimpang — pelajaran dari blok config yang pernah hilang karena
// dua sisi punya pemahaman berbeda tentang isi berkas yang sama.
package transfer

import (
	"encoding/json"
	"fmt"

	"github.com/andyresta/poinhost/internal/core/backup"
)

// Version adalah versi format paket.
const Version = 1

// Bundle adalah isi paket terenkripsi.
type Bundle struct {
	Version int      `json:"version"`
	Servers []Server `json:"servers"`
}

// Server adalah satu server beserta kredensial dan host key-nya.
type Server struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	AuthType string `json:"authType"`

	// KeyPath adalah lokasi kunci DI MESIN AGENT. App desktop mengisinya
	// dengan path tujuan, bukan path miliknya sendiri, karena keduanya hampir
	// pasti berbeda.
	KeyPath    string `json:"keyPath,omitempty"`
	KeyFileB64 string `json:"keyFileB64,omitempty"`
	Password   string `json:"password,omitempty"`

	Tags    []string `json:"tags,omitempty"`
	Color   string   `json:"color,omitempty"`
	Notes   string   `json:"notes,omitempty"`
	UseSudo bool     `json:"useSudo"`

	// KnownHosts adalah baris known_hosts untuk host ini, disalin dari app
	// desktop tempat user SUDAH menyetujui fingerprint-nya.
	//
	// Wajib ikut. Agent tidak punya manusia yang bisa menyetujui dialog host
	// key, jadi tanpa baris ini setiap server yang ditautkan langsung gagal
	// dengan SSH_HOST_KEY_MISMATCH dan bot melaporkannya "not connected".
	KnownHosts []string `json:"knownHosts,omitempty"`
}

// Seal mengenkripsi bundle dengan passphrase.
//
// Memakai ulang format arsip backup poinhost, bukan skema baru: satu
// implementasi kripto yang sama-sama dipakai dua fitur lebih mudah ditinjau
// daripada dua yang mirip.
func Seal(b *Bundle, passphrase string) ([]byte, error) {
	b.Version = Version
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	return backup.Encrypt(raw, passphrase)
}

// Open mendekripsi dan memvalidasi bundle.
func Open(data []byte, passphrase string) (*Bundle, error) {
	raw, err := backup.Decrypt(data, passphrase)
	if err != nil {
		return nil, err
	}
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("paket server tidak bisa dibaca: %w", err)
	}
	if b.Version != Version {
		return nil, fmt.Errorf("versi paket server %d tidak didukung (yang dikenal: %d)", b.Version, Version)
	}
	return &b, nil
}

// sealRaw mengenkripsi bundle TANPA memaksa Version — hanya dipakai test untuk
// membuat paket berversi lain.
func sealRaw(b *Bundle, passphrase string) ([]byte, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	return backup.Encrypt(raw, passphrase)
}
