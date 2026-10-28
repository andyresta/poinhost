// Package devices menyimpan perangkat yang terdaftar ke satu agent.
//
// Alurnya: HP menukar token pendaftaran dengan token miliknya sendiri, lalu
// MENUNGGU. Sampai seseorang menyetujuinya dari app desktop poinhost, token
// itu tidak membuka apa pun. Jadi token yang ditempel ke HP bukan kredensial
// akses — kalau ia bocor, yang didapat penemunya hanyalah tempat di antrean
// yang terlihat jelas di layar pemiliknya.
//
// Satu perangkat satu token, dan itu yang membuat "cabut HP yang hilang"
// mungkin tanpa memutus HP lain.
package devices

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Status perangkat.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRevoked  = "revoked"
)

// ErrNotFound dikembalikan kalau id perangkat tidak dikenal.
var ErrNotFound = errors.New("perangkat tidak ditemukan")

// Device adalah satu HP yang mendaftar.
//
// TokenHash, bukan tokennya. Berkas ini dibaca root dan ikut tersalin ke
// backup mana pun yang menyentuh direktori data agent; menyimpan token apa
// adanya berarti setiap salinan berkas ini adalah kunci yang masih hidup.
type Device struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform,omitempty"`
	Status   string `json:"status"`

	// CanControl memisahkan "boleh melihat" dari "boleh mengubah".
	//
	// Dua keputusan yang berbeda, jadi dua sakelar yang berbeda. Menyetujui
	// HP supaya bisa memantau tidak sama dengan mengizinkannya mematikan
	// database — dan kalau HP itu hilang, bedanya adalah antara "ada yang
	// melihat metrik saya" dan "ada yang bisa menghentikan layanan saya".
	//
	// Default false, termasuk untuk perangkat yang sudah terdaftar sebelum
	// field ini ada: JSON tanpa field ini terbaca sebagai false, jadi
	// memperbarui agent tidak pernah diam-diam memberi izin baru.
	CanControl bool `json:"canControl,omitempty"`

	TokenHash  string    `json:"tokenHash"`
	CreatedAt  time.Time `json:"createdAt"`
	ApprovedAt time.Time `json:"approvedAt,omitempty"`
}

// Store adalah daftar perangkat yang tersimpan di satu berkas JSON.
//
// Agent adalah SATU-SATUNYA yang menulis berkas ini. App desktop mengubahnya
// lewat socket admin agent, bukan dengan menulis berkasnya sendiri lewat SSH —
// dua penulis atas berkas yang sama berarti persetujuan yang dikirim
// bersamaan dengan pendaftaran bisa saling menimpa tanpa jejak.
type Store struct {
	path string

	mu   sync.Mutex
	list []Device
}

// Open memuat berkas perangkat. Berkas yang belum ada bukan galat — itu
// keadaan normal agent yang belum pernah didaftari HP.
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("baca %s: %w", path, err)
	}
	var berkas struct {
		Devices []Device `json:"devices"`
	}
	if err := json.Unmarshal(b, &berkas); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	s.list = berkas.Devices
	return s, nil
}

// Register mencatat perangkat baru dalam keadaan MENUNGGU, dan mengembalikan
// token miliknya satu kali saja — setelah ini hanya hash-nya yang tersimpan.
func (s *Store) Register(name, platform string) (Device, string, error) {
	token, err := randomHex(32)
	if err != nil {
		return Device{}, "", err
	}
	id, err := randomHex(8)
	if err != nil {
		return Device{}, "", err
	}

	d := Device{
		ID:        id,
		Name:      bersihkan(name, "Perangkat tanpa nama"),
		Platform:  bersihkan(platform, ""),
		Status:    StatusPending,
		TokenHash: hashToken(token),
		CreatedAt: time.Now().UTC(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.list = append(s.list, d)
	if err := s.simpanLocked(); err != nil {
		// Dibatalkan dari memori juga: perangkat yang hanya ada di memori
		// akan hilang saat restart, sementara HP-nya sudah menyimpan token
		// dan menunggu persetujuan yang tidak akan pernah muncul di layar.
		s.list = s.list[:len(s.list)-1]
		return Device{}, "", err
	}
	return d, token, nil
}

// Authenticate mencari perangkat pemilik token.
//
// Mengembalikan perangkat beserta statusnya, termasuk yang MENUNGGU dan yang
// DICABUT — pemanggil yang memutuskan bagaimana membalasnya, supaya HP bisa
// dibedakan antara "tokenmu salah" dan "tunggu disetujui".
func (s *Store) Authenticate(token string) (Device, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Device{}, false
	}
	want := hashToken(token)

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.list {
		if subtle.ConstantTimeCompare([]byte(d.TokenHash), []byte(want)) == 1 {
			return d, true
		}
	}
	return Device{}, false
}

// List mengembalikan seluruh perangkat, yang menunggu lebih dulu.
//
// Urutannya bukan kosmetik: yang menunggu adalah satu-satunya yang menuntut
// tindakan, dan daftar panjang yang menyembunyikannya di bawah membuat
// permintaan baru gampang terlewat.
func (s *Store) List() []Device {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Device, len(s.list))
	copy(out, s.list)
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Status == StatusPending) != (out[j].Status == StatusPending) {
			return out[i].Status == StatusPending
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// SetStatus menyetujui atau mencabut satu perangkat.
// SetControl memberi atau mencabut izin mengubah mesin.
//
// Terpisah dari SetStatus: mencabut akses dan mencabut kendali adalah dua
// tindakan berbeda, dan menggabungkannya berarti satu tombol di panel
// melakukan hal yang tidak diminta.
func (s *Store) SetControl(id string, allowed bool) (Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.list {
		if s.list[i].ID != id {
			continue
		}
		sebelum := s.list[i]
		s.list[i].CanControl = allowed
		if err := s.simpanLocked(); err != nil {
			s.list[i] = sebelum
			return Device{}, err
		}
		return s.list[i], nil
	}
	return Device{}, ErrNotFound
}

func (s *Store) SetStatus(id, status string) (Device, error) {
	if status != StatusApproved && status != StatusRevoked {
		return Device{}, fmt.Errorf("status %q tidak dikenal", status)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.list {
		if s.list[i].ID != id {
			continue
		}
		sebelum := s.list[i]
		s.list[i].Status = status
		if status == StatusApproved {
			s.list[i].ApprovedAt = time.Now().UTC()
		}
		if err := s.simpanLocked(); err != nil {
			s.list[i] = sebelum
			return Device{}, err
		}
		return s.list[i], nil
	}
	return Device{}, ErrNotFound
}

// Delete menghapus perangkat sepenuhnya.
//
// Berbeda dari mencabut: token perangkat yang dicabut masih dikenali dan HP-nya
// bisa diberi tahu bahwa aksesnya dicabut, sementara yang dihapus akan ditolak
// seperti token asing. Keduanya ada karena "HP ini hilang" dan "HP ini sudah
// tidak dipakai" bukan hal yang sama.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.list {
		if s.list[i].ID != id {
			continue
		}
		sebelum := s.list
		s.list = append(append([]Device{}, s.list[:i]...), s.list[i+1:]...)
		if err := s.simpanLocked(); err != nil {
			s.list = sebelum
			return err
		}
		return nil
	}
	return ErrNotFound
}

// simpanLocked menulis berkas lewat berkas sementara, supaya tidak pernah ada
// daftar perangkat setengah tertulis kalau proses mati di tengah.
func (s *Store) simpanLocked() error {
	b, err := json.MarshalIndent(struct {
		Devices []Device `json:"devices"`
	}{Devices: s.list}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func bersihkan(s, fallback string) string {
	s = strings.TrimSpace(s)
	// Nama datang dari HP dan ditampilkan di panel desktop. Dipotong supaya
	// satu perangkat tidak bisa membuat daftarnya tidak terbaca.
	if len(s) > 60 {
		s = s[:60]
	}
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
	if s == "" {
		return fallback
	}
	return s
}
