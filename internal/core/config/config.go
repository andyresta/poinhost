// Package config menyimpan parameter aplikasi poinhost.
//
// Skeleton awal ini sengaja lebih ramping dari config.go homepoin — hanya
// bagian yang dipakai modul yang sudah di-porting (SSH pool, executor,
// database). Bagian lain (MySQL pool, migration, filexfer, dbxfer, dst)
// ditambahkan kembali saat modul terkait di-porting.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config menyimpan seluruh parameter runtime aplikasi.
type Config struct {
	DataDir      string
	DatabasePath string
	LogDir       string
	LogLevel     string

	SSH      SSHConfig
	Executor ExecutorConfig
}

// SSHConfig menyimpan parameter koneksi pool SSH.
type SSHConfig struct {
	SharedMuxConns          int
	PoolWaitTimeout         time.Duration
	KeepaliveInterval       time.Duration
	KeepaliveTimeout        time.Duration
	HealthCheckTimeout      time.Duration
	HealthCheckSkipAfterUse time.Duration
	DialTimeout             time.Duration
	ReconnectBackoff        []time.Duration
}

// ExecutorConfig menyimpan parameter eksekusi perintah SSH.
type ExecutorConfig struct {
	TimeoutFast     time.Duration
	TimeoutMedium   time.Duration
	TimeoutLong     time.Duration
	TimeoutVeryLong time.Duration
	RetryMax        int
	RetryBackoff    []time.Duration
}

// Default mengembalikan konfigurasi default, dioverride oleh environment
// variable bila di-set (dipakai untuk dev/testing, bukan .env seperti homepoin
// karena poinhost adalah aplikasi desktop, bukan server yang di-deploy).
func Default() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dataDir := filepath.Join(home, ".poinhost")

	cfg := &Config{
		DataDir:  dataDir,
		LogDir:   filepath.Join(dataDir, "logs"),
		LogLevel: getEnv("POINHOST_LOG_LEVEL", "info"),
		SSH: SSHConfig{
			SharedMuxConns:          clampInt(getEnvInt("SSH_SHARED_MUX_CONNS", 2), 1, 4),
			PoolWaitTimeout:         time.Duration(getEnvInt("SSH_POOL_WAIT_TIMEOUT_SECONDS", 10)) * time.Second,
			KeepaliveInterval:       time.Duration(getEnvInt("SSH_KEEPALIVE_INTERVAL_SECONDS", 30)) * time.Second,
			KeepaliveTimeout:        time.Duration(getEnvInt("SSH_KEEPALIVE_TIMEOUT_SECONDS", 10)) * time.Second,
			HealthCheckTimeout:      time.Duration(getEnvInt("SSH_HEALTH_CHECK_TIMEOUT_SECONDS", 3)) * time.Second,
			HealthCheckSkipAfterUse: time.Duration(getEnvInt("SSH_HEALTH_CHECK_SKIP_SECONDS", 30)) * time.Second,
			DialTimeout:             time.Duration(getEnvInt("SSH_DIAL_TIMEOUT_SECONDS", 15)) * time.Second,
			ReconnectBackoff:        parseDurationList(getEnv("SSH_RECONNECT_BACKOFF_SECONDS", "2,4,8,30")),
		},
		Executor: ExecutorConfig{
			TimeoutFast:     time.Duration(getEnvInt("SSH_TIMEOUT_FAST_SECONDS", 10)) * time.Second,
			TimeoutMedium:   time.Duration(getEnvInt("SSH_TIMEOUT_MEDIUM_SECONDS", 60)) * time.Second,
			TimeoutLong:     time.Duration(getEnvInt("SSH_TIMEOUT_LONG_SECONDS", 600)) * time.Second,
			TimeoutVeryLong: time.Duration(getEnvInt("SSH_TIMEOUT_VERY_LONG_SECONDS", 1800)) * time.Second,
			RetryMax:        getEnvInt("SSH_RETRY_MAX", 3),
			RetryBackoff:    parseDurationList(getEnv("SSH_RETRY_BACKOFF_SECONDS", "1,2,4")),
		},
	}
	return cfg, nil
}

// EnsureDirs membuat folder data & log jika belum ada.
func (c *Config) EnsureDirs() error {
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return err
	}
	return os.MkdirAll(c.LogDir, 0o700)
}

// DBPath mengembalikan path file database SQLite.
func (c *Config) DBPath() string {
	if c.DatabasePath != "" {
		return c.DatabasePath
	}
	return filepath.Join(c.DataDir, "poinhost.db")
}

// KnownHostsPath mengembalikan path file known_hosts SSH.
func (c *Config) KnownHostsPath() string {
	return filepath.Join(c.DataDir, "ssh_known_hosts")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func parseDurationList(raw string) []time.Duration {
	parts := strings.Split(raw, ",")
	out := make([]time.Duration, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		sec, err := strconv.Atoi(p)
		if err != nil {
			continue
		}
		out = append(out, time.Duration(sec)*time.Second)
	}
	if len(out) == 0 {
		return []time.Duration{time.Second}
	}
	return out
}
