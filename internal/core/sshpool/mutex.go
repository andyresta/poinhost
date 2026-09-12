package sshpool

import "sync"

// ServerMutexRegistry mengelola mutex per server untuk mencegah operasi
// konflik (mis. dua klik "restart nginx" bersamaan, atau edit vhost
// bersamaan dari dua tab yang menunjuk server yang sama).
type ServerMutexRegistry struct {
	mu      sync.Mutex
	mutexes map[string]*sync.Mutex
}

// NewServerMutexRegistry membuat registry mutex baru.
func NewServerMutexRegistry() *ServerMutexRegistry {
	return &ServerMutexRegistry{
		mutexes: make(map[string]*sync.Mutex),
	}
}

// Lock mengunci operasi untuk server tertentu.
func (r *ServerMutexRegistry) Lock(serverID string) {
	r.mu.Lock()
	m, ok := r.mutexes[serverID]
	if !ok {
		m = &sync.Mutex{}
		r.mutexes[serverID] = m
	}
	r.mu.Unlock()
	m.Lock()
}

// Unlock melepas kunci operasi server.
func (r *ServerMutexRegistry) Unlock(serverID string) {
	r.mu.Lock()
	m, ok := r.mutexes[serverID]
	r.mu.Unlock()
	if ok {
		m.Unlock()
	}
}

// TryLock mencoba mengunci tanpa menunggu; false jika sedang sibuk.
func (r *ServerMutexRegistry) TryLock(serverID string) bool {
	r.mu.Lock()
	m, ok := r.mutexes[serverID]
	if !ok {
		m = &sync.Mutex{}
		r.mutexes[serverID] = m
	}
	r.mu.Unlock()
	return m.TryLock()
}
