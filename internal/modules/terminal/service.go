// Package terminal membungkus koneksi dedicated dari session.TerminalRegistry
// menjadi sesi shell PTY interaktif sungguhan (RequestPty + Shell), dan
// menjembataninya ke frontend lewat event Wails (bukan WebSocket custom
// seperti homepoin — lihat ARCHITECTURE.md).
package terminal

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/andyresta/poinhost/internal/session"
	"golang.org/x/crypto/ssh"
)

// ptySession menyimpan semua yang dibutuhkan untuk satu sesi shell interaktif.
type ptySession struct {
	tabID   string
	session *ssh.Session
	stdin   io.WriteCloser
}

// Service mengelola siklus hidup sesi PTY. Satu instance dipakai untuk
// SELURUH aplikasi (bukan per-tab) — identitas per-sesi ada di sessionID
// yang dikembalikan Open, sama seperti yang dipakai session.TerminalRegistry
// di baliknya, supaya cuma ada SATU sistem ID untuk konsep "sesi terminal".
type Service struct {
	registry *session.TerminalRegistry

	mu       sync.Mutex
	sessions map[string]*ptySession

	onData func(sessionID string, data []byte)
	onExit func(sessionID string, reason string)
}

// NewService membuat terminal.Service baru.
func NewService(registry *session.TerminalRegistry) *Service {
	return &Service{
		registry: registry,
		sessions: make(map[string]*ptySession),
	}
}

// SetEmitters mendaftarkan callback output & exit. app.go menghubungkan ini
// ke runtime.EventsEmit Wails setelah context siap — package ini sendiri
// tidak bergantung pada Wails sama sekali (gampang diuji, gampang dipindah
// kalau nanti transportnya diganti).
func (s *Service) SetEmitters(onData func(sessionID string, data []byte), onExit func(sessionID, reason string)) {
	s.mu.Lock()
	s.onData = onData
	s.onExit = onExit
	s.mu.Unlock()
}

// Open membuka sesi shell interaktif baru untuk tab tertentu. Ukuran awal
// PTY sengaja default kecil (nanti langsung di-resize oleh frontend
// begitu FitAddon menghitung ukuran kontainer sebenarnya — lihat
// TerminalPanel.tsx), bukan angka final.
func (s *Service) Open(ctx context.Context, tabID, serverID string) (string, error) {
	sessionID, client, err := s.registry.Open(ctx, tabID, serverID)
	if err != nil {
		return "", err
	}

	sshSess, err := client.NewSession()
	if err != nil {
		s.registry.Close(tabID, sessionID)
		return "", fmt.Errorf("buka sesi SSH: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sshSess.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		_ = sshSess.Close()
		s.registry.Close(tabID, sessionID)
		return "", fmt.Errorf("minta PTY: %w", err)
	}

	stdin, err := sshSess.StdinPipe()
	if err != nil {
		_ = sshSess.Close()
		s.registry.Close(tabID, sessionID)
		return "", fmt.Errorf("buka stdin: %w", err)
	}
	stdout, err := sshSess.StdoutPipe()
	if err != nil {
		_ = sshSess.Close()
		s.registry.Close(tabID, sessionID)
		return "", fmt.Errorf("buka stdout: %w", err)
	}

	if err := sshSess.Shell(); err != nil {
		_ = sshSess.Close()
		s.registry.Close(tabID, sessionID)
		return "", fmt.Errorf("mulai shell: %w", err)
	}

	s.mu.Lock()
	s.sessions[sessionID] = &ptySession{tabID: tabID, session: sshSess, stdin: stdin}
	s.mu.Unlock()

	go s.readLoop(sessionID, stdout)

	return sessionID, nil
}

// readLoop mengalirkan output PTY (stdout tergabung, karena remote shell
// menulis ke device pty yang sama) ke callback onData sampai sesi berakhir.
func (s *Service) readLoop(sessionID string, stdout io.Reader) {
	buf := make([]byte, 8192)
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.mu.Lock()
			onData := s.onData
			s.mu.Unlock()
			if onData != nil {
				onData(sessionID, chunk)
			}
		}
		if err != nil {
			s.mu.Lock()
			onExit := s.onExit
			s.mu.Unlock()
			if onExit != nil {
				onExit(sessionID, err.Error())
			}
			return
		}
	}
}

// Write mengirim input (keystroke) ke shell remote.
func (s *Service) Write(sessionID string, data []byte) error {
	s.mu.Lock()
	ps, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("sesi terminal %s tidak ditemukan", sessionID)
	}
	_, err := ps.stdin.Write(data)
	return err
}

// Resize memberi tahu ukuran PTY yang baru ke shell remote (dipanggil tiap
// kontainer xterm.js di frontend berubah ukuran).
func (s *Service) Resize(sessionID string, cols, rows int) error {
	s.mu.Lock()
	ps, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("sesi terminal %s tidak ditemukan", sessionID)
	}
	return ps.session.WindowChange(rows, cols)
}

// Close menutup satu sesi terminal: sesi SSH shell-nya, lalu koneksi
// dedicated di baliknya lewat session.TerminalRegistry.
func (s *Service) Close(sessionID string) {
	s.mu.Lock()
	ps, ok := s.sessions[sessionID]
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	if !ok {
		return
	}
	_ = ps.session.Close()
	s.registry.Close(ps.tabID, sessionID)
}

// CloseAllForTab menutup semua sesi terminal milik satu tab — dipanggil
// saat tab ditutup, SEBELUM session.TerminalRegistry.CloseAllForTab (yang
// dipertahankan sebagai jaring pengaman untuk koneksi dedicated apa pun
// yang mungkin dibuka modul lain di masa depan untuk tab yang sama).
func (s *Service) CloseAllForTab(tabID string) {
	s.mu.Lock()
	var ids []string
	for id, ps := range s.sessions {
		if ps.tabID == tabID {
			ids = append(ids, id)
		}
	}
	s.mu.Unlock()

	for _, id := range ids {
		s.Close(id)
	}
}
