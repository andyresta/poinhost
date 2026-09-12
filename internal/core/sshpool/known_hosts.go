package sshpool

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// KnownHostsStore mengelola verifikasi dan penyimpanan host key SSH.
type KnownHostsStore struct {
	path string
	mu   sync.RWMutex
}

// HostKeyMismatch menyimpan detail ketidakcocokan fingerprint.
type HostKeyMismatch struct {
	OldFingerprint string
	NewFingerprint string
	Host           string
}

func (e *HostKeyMismatch) Error() string {
	return ErrHostKeyMismatch.Error()
}

// NewKnownHostsStore membuat store known_hosts di path tertentu.
func NewKnownHostsStore(path string) *KnownHostsStore {
	return &KnownHostsStore{path: path}
}

// Callback mengembalikan HostKeyCallback untuk klien SSH.
func (s *KnownHostsStore) Callback() ssh.HostKeyCallback {
	return func(hostname string, _ net.Addr, key ssh.PublicKey) error {
		return s.verify(hostname, key)
	}
}

func (s *KnownHostsStore) verify(hostname string, key ssh.PublicKey) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	host := normalizeHost(hostname)
	newFP := FingerprintSHA256(key)

	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		return &HostKeyMismatch{Host: host, NewFingerprint: newFP}
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if !hostMatches(fields[0], host) {
			continue
		}
		storedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			continue
		}
		oldFP := FingerprintSHA256(storedKey)
		if oldFP != newFP {
			return &HostKeyMismatch{Host: host, OldFingerprint: oldFP, NewFingerprint: newFP}
		}
		return nil
	}

	return &HostKeyMismatch{Host: host, NewFingerprint: newFP}
}

// Trust menyimpan host key baru ke file known_hosts.
func (s *KnownHostsStore) Trust(hostname string, key ssh.PublicKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	host := normalizeHost(hostname)
	line := knownhosts.Line([]string{host}, key)

	if err := os.MkdirAll(dirOf(s.path), 0o700); err != nil {
		return err
	}

	var existing []byte
	if data, err := os.ReadFile(s.path); err == nil {
		existing = data
	}

	filtered := removeHostEntries(string(existing), host)
	content := strings.TrimSpace(filtered)
	if content != "" {
		content += "\n"
	}
	content += line + "\n"

	return os.WriteFile(s.path, []byte(content), 0o600)
}

// FingerprintSHA256 menghitung fingerprint SHA256 host key.
func FingerprintSHA256(key ssh.PublicKey) string {
	hash := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.StdEncoding.EncodeToString(hash[:])
}

func normalizeHost(host string) string {
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		return host[:idx]
	}
	return host
}

func hostMatches(entry, host string) bool {
	entry = strings.TrimPrefix(entry, "|1|")
	if entry == host || strings.HasPrefix(entry, host+",") {
		return true
	}
	for _, part := range strings.Split(entry, ",") {
		if part == host {
			return true
		}
	}
	return false
}

func removeHostEntries(content, host string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && hostMatches(fields[0], host) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, string(os.PathSeparator)); i >= 0 {
		return path[:i]
	}
	return "."
}

// IsHostKeyMismatch mengecek apakah error adalah mismatch host key.
func IsHostKeyMismatch(err error) (*HostKeyMismatch, bool) {
	var hm *HostKeyMismatch
	if errors.As(err, &hm) {
		return hm, true
	}
	return nil, false
}

// IsHostKeyUnknown mengecek apakah host key belum pernah dipercaya.
func IsHostKeyUnknown(err error) bool {
	var hm *HostKeyMismatch
	if errors.As(err, &hm) {
		return hm.OldFingerprint == ""
	}
	return false
}

// FormatMismatchError membuat pesan error mismatch yang informatif.
func FormatMismatchError(hm *HostKeyMismatch) string {
	if hm.OldFingerprint == "" {
		return fmt.Sprintf("host key baru untuk %s: %s", hm.Host, hm.NewFingerprint)
	}
	return fmt.Sprintf("fingerprint lama %s, baru %s", hm.OldFingerprint, hm.NewFingerprint)
}
