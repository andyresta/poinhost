package backend

import (
	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/servers"
)

// Store membaca daftar server yang dikenal agent.
//
// Sejak satu agent hanya mengurus mesinnya sendiri, store ini murni baca:
// tidak ada lagi impor paket server dari desktop maupun pencabutannya.
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

	// Connection dan Error dilaporkan supaya Mobile Control bisa menampilkan
	// apakah agent BENAR-BENAR bisa menjangkau mesinnya — bukan sekadar
	// bahwa servicenya hidup.
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
// Remove mencabut satu server tertaut dari agent.
//
func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}
