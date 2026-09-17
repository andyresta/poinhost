package session

import (
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Manager mengelola registry tab yang terbuka di UI dan me-restore layout
// tab dari sesi sebelumnya saat aplikasi dibuka lagi (mirip "restore tabs"
// di browser). Semua akses concurrency-safe karena dipanggil langsung dari
// binding Wails yang bisa dipanggil dari beberapa goroutine JS runtime.
type Manager struct {
	db *sql.DB

	mu   sync.RWMutex
	tabs map[string]*Tab
}

// NewManager membuat session manager baru.
func NewManager(db *sql.DB) *Manager {
	return &Manager{
		db:   db,
		tabs: make(map[string]*Tab),
	}
}

// LoadPersisted memuat ulang tab yang tersimpan dari sesi sebelumnya
// (dipanggil sekali saat startup, sebelum UI pertama kali render).
func (m *Manager) LoadPersisted() ([]*Tab, error) {
	rows, err := m.db.Query(`
		SELECT id, server_id, title, active_module, position, created_at, updated_at
		FROM ui_tabs ORDER BY position ASC`)
	if err != nil {
		return nil, fmt.Errorf("muat ui_tabs: %w", err)
	}
	defer rows.Close()

	m.mu.Lock()
	defer m.mu.Unlock()

	var out []*Tab
	for rows.Next() {
		var t Tab
		var createdAt, updatedAt string
		if err := rows.Scan(&t.ID, &t.ServerID, &t.Title, &t.ActiveModule, &t.Position, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		t.Kind = KindServer
		t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		t.LastActiveAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		m.tabs[t.ID] = &t
		out = append(out, &t)
	}
	return out, rows.Err()
}

// OpenTab membuat tab baru yang menunjuk ke serverID, ditambahkan di akhir
// urutan tab. Tidak membuka koneksi SSH apapun di sini — koneksi (shared
// atau dedicated) baru dibuat lazy saat modul di dalam tab benar-benar
// butuh (files, docker, terminal, dst).
func (m *Manager) OpenTab(serverID, title string) (*Tab, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pos := len(m.tabs)
	now := time.Now()
	t := &Tab{
		ID:           uuid.NewString(),
		Kind:         KindServer,
		ServerID:     serverID,
		Title:        title,
		ActiveModule: "overview",
		Position:     pos,
		CreatedAt:    now,
		LastActiveAt: now,
	}

	_, err := m.db.Exec(`
		INSERT INTO ui_tabs (id, server_id, title, active_module, position)
		VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.ServerID, t.Title, t.ActiveModule, t.Position)
	if err != nil {
		return nil, fmt.Errorf("simpan tab: %w", err)
	}

	m.tabs[t.ID] = t
	return t, nil
}

// OpenMigrationTab membuka tab migrasi. Tab ini TIDAK disimpan ke ui_tabs,
// jadi tidak ikut dipulihkan saat aplikasi dibuka lagi — beda dari tab
// server yang memang layak dipulihkan. Alasannya: isi berguna sebuah tab
// migrasi adalah transfer yang sedang berjalan, dan transfer itu hidup di
// koneksi SSH proses ini — begitu aplikasi ditutup, transfernya berhenti,
// sehingga memulihkan tabnya hanya akan menampilkan formulir kosong yang
// seolah-olah melanjutkan sesuatu.
//
// Konsekuensinya server_id di ui_tabs tetap NOT NULL seperti semula; tidak
// ada baris tab migrasi yang perlu menyimpan server kosong di sana.
func (m *Manager) OpenMigrationTab(title string) (*Tab, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	t := &Tab{
		ID:           uuid.NewString(),
		Kind:         KindMigration,
		Title:        title,
		ActiveModule: "migration-files",
		Position:     len(m.tabs),
		CreatedAt:    now,
		LastActiveAt: now,
	}
	m.tabs[t.ID] = t
	return t, nil
}

// CloseTab menghapus tab dari registry & penyimpanan. Pemanggil (app.go)
// bertanggung jawab menutup koneksi dedicated (terminal) milik tab ini
// LEBIH DULU lewat TerminalRegistry.CloseAllForTab sebelum memanggil ini,
// supaya tidak ada koneksi SSH yang menggantung tanpa pemilik tab.
func (m *Manager) CloseTab(tabID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tabs[tabID]; !ok {
		return fmt.Errorf("tab %s tidak ditemukan", tabID)
	}
	delete(m.tabs, tabID)

	_, err := m.db.Exec(`DELETE FROM ui_tabs WHERE id = ?`, tabID)
	return err
}

// ListTabs mengembalikan semua tab terurut berdasarkan posisi tab bar.
func (m *Manager) ListTabs() []*Tab {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]*Tab, 0, len(m.tabs))
	for _, t := range m.tabs {
		cp := *t
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out
}

// GetTab mengembalikan satu tab berdasarkan ID.
func (m *Manager) GetTab(tabID string) (*Tab, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tabs[tabID]
	if !ok {
		return nil, fmt.Errorf("tab %s tidak ditemukan", tabID)
	}
	cp := *t
	return &cp, nil
}

// SetActiveModule mengganti modul yang sedang ditampilkan di dalam satu tab
// (mis. "files" -> "docker"). Frontend memanggil ini setiap kali user klik
// menu modul DI DALAM tab yang sama — tab & koneksinya tidak berubah, hanya
// state "modul aktif" yang di-swap, sehingga tidak ada reload/reconnect.
func (m *Manager) SetActiveModule(tabID, module string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, ok := m.tabs[tabID]
	if !ok {
		return fmt.Errorf("tab %s tidak ditemukan", tabID)
	}
	t.ActiveModule = module
	t.LastActiveAt = time.Now()

	_, err := m.db.Exec(`UPDATE ui_tabs SET active_module = ?, updated_at = datetime('now') WHERE id = ?`,
		module, tabID)
	return err
}

// Reorder menyimpan urutan baru tab bar (drag & drop).
func (m *Manager) Reorder(tabIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for pos, id := range tabIDs {
		t, ok := m.tabs[id]
		if !ok {
			continue
		}
		t.Position = pos
		if _, err := tx.Exec(`UPDATE ui_tabs SET position = ? WHERE id = ?`, pos, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
