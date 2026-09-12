package servers

import "time"

// Server merepresentasikan satu koneksi VPS yang dikelola.
type Server struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	AuthType  string    `json:"authType"` // "key" | "password"
	KeyPath   string    `json:"keyPath,omitempty"`
	Tags      []string  `json:"tags"`
	Color     string    `json:"color"`
	Notes     string    `json:"notes"`
	UseSudo   bool      `json:"useSudo"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SaveServerRequest adalah payload create/update server dari frontend.
// Password mentah (kalau auth_type=password) hanya lewat di request ini —
// tidak pernah dikembalikan ke frontend setelah disimpan (dienkripsi dulu).
type SaveServerRequest struct {
	ID       string   `json:"id,omitempty"`
	Name     string   `json:"name"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Username string   `json:"username"`
	AuthType string   `json:"authType"`
	KeyPath  string   `json:"keyPath,omitempty"`
	Password string   `json:"password,omitempty"`
	Tags     []string `json:"tags"`
	Color    string   `json:"color"`
	Notes    string   `json:"notes"`
	UseSudo  bool     `json:"useSudo"`
}

// ConnectionTestResult adalah hasil uji koneksi SSH ke server.
type ConnectionTestResult struct {
	Status         string `json:"status"` // ok | mismatch | unknown | error
	Latency        string `json:"latency,omitempty"`
	Fingerprint    string `json:"fingerprint,omitempty"`
	OldFingerprint string `json:"oldFingerprint,omitempty"`
	Message        string `json:"message,omitempty"`
}
