package sshpool

import (
	"context"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// ProbeResult menyimpan hasil uji koneksi SSH.
type ProbeResult struct {
	Status         string `json:"status"`
	Latency        string `json:"latency,omitempty"`
	Host           string `json:"host,omitempty"`
	Fingerprint    string `json:"fingerprint,omitempty"`
	OldFingerprint string `json:"oldFingerprint,omitempty"`
	NewFingerprint string `json:"newFingerprint,omitempty"`
}

// ProbeConnection menguji koneksi SSH tanpa mendaftarkan server ke pool.
func (p *Pool) ProbeConnection(ctx context.Context, cfg ServerConfig) (*ProbeResult, error) {
	start := time.Now()
	client, err := p.dial(ctx, cfg)
	if err != nil {
		if hm, ok := IsHostKeyMismatch(err); ok {
			res := &ProbeResult{
				Host:           hm.Host,
				NewFingerprint: hm.NewFingerprint,
				Fingerprint:    hm.NewFingerprint,
			}
			if hm.OldFingerprint == "" {
				res.Status = "unknown"
				return res, nil
			}
			res.Status = "mismatch"
			res.OldFingerprint = hm.OldFingerprint
			return res, nil
		}
		return nil, err
	}
	_ = client.Close()
	return &ProbeResult{
		Status:  "ok",
		Latency: time.Since(start).Round(time.Millisecond).String(),
		Host:    cfg.Host,
	}, nil
}

// TrustHostKey menerima host key server dan menyimpannya ke known_hosts.
func (p *Pool) TrustHostKey(ctx context.Context, cfg ServerConfig) error {
	authMethods, err := buildAuthMethods(cfg)
	if err != nil {
		return err
	}

	addr := formatAddr(cfg.Host, cfg.Port)
	conn, err := dialTCP(ctx, p.cfg.DialTimeout, addr)
	if err != nil {
		return err
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: p.trustHostKeyCallback(),
	}
	client, err := newSSHClient(ctx, conn, addr, sshCfg)
	if err != nil {
		return err
	}
	_ = client.Close()
	return nil
}

func (p *Pool) trustHostKeyCallback() ssh.HostKeyCallback {
	return func(hostname string, _ net.Addr, key ssh.PublicKey) error {
		return p.knownHosts.Trust(hostname, key)
	}
}
