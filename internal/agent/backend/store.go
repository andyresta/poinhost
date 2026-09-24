package backend

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andyresta/poinhost/internal/agent/transfer"
	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/servers"
)

// LinkedTag menandai server yang ditautkan dari app desktop, supaya bisa
// dibedakan dari server milik agent itu sendiri saat dicabut.
const LinkedTag = "linked"

// Store menerapkan penautan server ke database agent.
type Store struct {
	svc        *servers.Service
	collector  *servers.Collector
	knownHosts *sshpool.KnownHostsStore
	keyDir     string
	selfID     string
}

// NewStore membuat store.
func NewStore(svc *servers.Service, collector *servers.Collector, knownHosts *sshpool.KnownHostsStore, keyDir, selfServerID string) *Store {
	return &Store{svc: svc, collector: collector, knownHosts: knownHosts, keyDir: keyDir, selfID: selfServerID}
}

// Entry adalah server terdaftar seperti dilaporkan API agent.
type Entry struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Host   string `json:"host"`
	IsSelf bool   `json:"isSelf"`

	// Connection dan Error dilaporkan supaya Agent Control bisa menampilkan
	// apakah agent BENAR-BENAR bisa menjangkau server itu. Tanpa ini,
	// satu-satunya cara mengetahuinya adalah membuka Telegram — dan kalau
	// jawabannya "not connected", tidak ada sebab yang bisa dilihat di mana pun.
	Connection string `json:"connection,omitempty"`
	Error      string `json:"error,omitempty"`
}

// List mengembalikan seluruh server yang dikenal agent.
func (s *Store) List() ([]Entry, error) {
	list, err := s.svc.List()
	if err != nil {
		return nil, err
	}
	status := map[string]servers.ServerStatus{}
	if s.collector != nil {
		for _, st := range s.collector.Snapshot() {
			status[st.ID] = st
		}
	}

	out := make([]Entry, 0, len(list))
	for _, srv := range list {
		e := Entry{ID: srv.ID, Name: srv.Name, Host: srv.Host, IsSelf: srv.ID == s.selfID}
		if st, ok := status[srv.ID]; ok {
			e.Connection = st.Connection
			e.Error = st.MetricsError
		}
		out = append(out, e)
	}
	return out, nil
}

// Import menulis server dari bundle ke database agent.
//
// ID asli dipertahankan (UpsertFromBackup, bukan Save): dengan begitu server
// yang sama punya identitas yang sama di desktop dan di agent, sehingga
// mengirim ulang paket memperbarui entri yang ada alih-alih menumpuk duplikat —
// masalah yang sudah pernah terjadi pada entri server lokal.
func (s *Store) Import(b *transfer.Bundle) (int, error) {
	if b == nil {
		return 0, fmt.Errorf("paket kosong")
	}
	if err := os.MkdirAll(s.keyDir, 0o700); err != nil {
		return 0, err
	}

	var n int
	for i := range b.Servers {
		in := b.Servers[i]
		if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.Host) == "" {
			continue
		}

		keyPath := in.KeyPath
		if in.AuthType == "key" && in.KeyFileB64 != "" {
			raw, err := base64.StdEncoding.DecodeString(in.KeyFileB64)
			if err != nil {
				return n, fmt.Errorf("kunci SSH %s rusak: %w", in.Name, err)
			}
			keyPath = filepath.Join(s.keyDir, "server-"+in.ID)
			if err := os.WriteFile(keyPath, raw, 0o600); err != nil {
				return n, fmt.Errorf("simpan kunci SSH %s: %w", in.Name, err)
			}
		}

		// Host key ditanam SEBELUM server didaftarkan, supaya percobaan koneksi
		// pertama — yang terjadi segera setelah pendaftaran — sudah menemukan
		// entri yang sah alih-alih gagal mismatch.
		if len(in.KnownHosts) > 0 {
			if err := s.knownHosts.ReplaceLinesFor(in.Host, in.KnownHosts); err != nil {
				return n, fmt.Errorf("tulis known_hosts untuk %s: %w", in.Host, err)
			}
		}

		tags := in.Tags
		if !hasTag(tags, LinkedTag) {
			tags = append(append([]string{}, tags...), LinkedTag)
		}

		srv := &servers.Server{
			ID: in.ID, Name: in.Name, Host: in.Host, Port: in.Port,
			Username: in.Username, AuthType: in.AuthType, KeyPath: keyPath,
			Tags: tags, Color: in.Color, Notes: in.Notes,
			UseSudo: in.UseSudo, IsActive: true,
		}
		if err := s.svc.UpsertFromBackup(srv, in.Password); err != nil {
			return n, fmt.Errorf("simpan server %s: %w", in.Name, err)
		}
		n++
	}
	return n, nil
}

// Remove mencabut satu server tertaut dari agent.
//
// Server milik agent sendiri TIDAK bisa dicabut lewat jalur ini: mencabutnya
// hanya akan dibuat ulang saat agent start berikutnya, jadi menawarkannya
// cuma membingungkan. Untuk itu ada Copot agent.
func (s *Store) Remove(id string) error {
	if id == "" {
		return fmt.Errorf("id server kosong")
	}
	if id == s.selfID {
		return fmt.Errorf("server tempat agent berjalan tidak bisa dicabut dari sini")
	}

	srv, err := s.svc.Get(id)
	if err != nil {
		return err
	}
	if err := s.svc.Delete(id); err != nil {
		return err
	}
	// Kunci yang ikut dikirim bersama server itu ikut dihapus — kalau tidak,
	// mencabut server meninggalkan kunci privat yang masih bisa dipakai.
	if srv != nil && strings.HasPrefix(srv.KeyPath, filepath.Join(s.keyDir, "server-")) {
		_ = os.Remove(srv.KeyPath)
	}
	return nil
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}
