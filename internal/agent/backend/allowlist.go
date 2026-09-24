package backend

import (
	"sync"

	"github.com/andyresta/poinhost/internal/agent/agentcfg"
)

// Allowlist adalah daftar user Telegram yang boleh memakai bot, disimpan di
// config.json yang sama dengan sisa konfigurasi agent.
//
// Ditulis kembali ke disk setiap ada penambahan supaya hasil pairing bertahan
// melewati restart — tanpa itu, user harus melakukan pairing ulang setiap kali
// agent di-restart, termasuk setelah update.
type Allowlist struct {
	mu   sync.RWMutex
	path string
	cfg  *agentcfg.Config

	// save dipisah jadi field supaya test bisa menggantinya tanpa menulis
	// ke filesystem.
	save func(path string, cfg *agentcfg.Config) error
}

// NewAllowlist membungkus konfigurasi yang sudah dimuat.
func NewAllowlist(path string, cfg *agentcfg.Config) *Allowlist {
	return &Allowlist{path: path, cfg: cfg, save: agentcfg.Save}
}

// IsAllowed melaporkan apakah user boleh memakai bot.
func (a *Allowlist) IsAllowed(userID int64) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg.Telegram.IsAllowed(userID)
}

// Allow menambahkan user dan langsung menyimpannya.
//
// Kalau penyimpanan gagal, penambahan di memori DIBATALKAN. Membiarkannya
// berarti user itu punya akses sampai restart berikutnya lalu kehilangannya
// tanpa penjelasan — lebih baik pairing-nya gagal terang-terangan sekarang.
func (a *Allowlist) Allow(userID int64, label string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cfg.Telegram.IsAllowed(userID) {
		return nil
	}
	a.cfg.Telegram.AllowedUserIDs = append(a.cfg.Telegram.AllowedUserIDs, userID)
	if err := a.save(a.path, a.cfg); err != nil {
		a.cfg.Telegram.AllowedUserIDs = a.cfg.Telegram.AllowedUserIDs[:len(a.cfg.Telegram.AllowedUserIDs)-1]
		return err
	}
	_ = label // label hanya untuk log pemanggil; config menyimpan ID saja
	return nil
}

// Revoke menghapus user dari daftar.
func (a *Allowlist) Revoke(userID int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	before := a.cfg.Telegram.AllowedUserIDs
	out := before[:0:0]
	for _, id := range before {
		if id != userID {
			out = append(out, id)
		}
	}
	if len(out) == len(before) {
		return nil
	}
	a.cfg.Telegram.AllowedUserIDs = out
	if err := a.save(a.path, a.cfg); err != nil {
		a.cfg.Telegram.AllowedUserIDs = before
		return err
	}
	return nil
}

// Count mengembalikan jumlah user yang diizinkan.
func (a *Allowlist) Count() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.cfg.Telegram.AllowedUserIDs)
}

// IDs mengembalikan salinan daftar user yang diizinkan.
func (a *Allowlist) IDs() []int64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]int64, len(a.cfg.Telegram.AllowedUserIDs))
	copy(out, a.cfg.Telegram.AllowedUserIDs)
	return out
}
