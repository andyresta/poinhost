package sshpool

import (
	"context"
	"errors"
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

// ErrFingerprintRequired dikembalikan kalau TrustHostKey dipanggil tanpa
// fingerprint yang disetujui user.
var ErrFingerprintRequired = errors.New("SSH_FINGERPRINT_REQUIRED")

// TrustHostKey menyimpan host key server ke known_hosts — TAPI hanya kalau
// fingerprint-nya sama persis dengan `expectedFingerprint`, yaitu fingerprint
// yang tadi ditampilkan ProbeConnection dan disetujui user di layar.
//
// Kenapa fingerprint harus dibawa masuk ke sini, bukan "percaya saja apa yang
// disodorkan koneksi ini": probe dan trust adalah DUA koneksi terpisah yang
// dijembatani waktu reaksi manusia. Tanpa pembanding, yang tersimpan adalah
// host key milik koneksi kedua — yang belum tentu host key yang dilihat user
// pada koneksi pertama. Itu membuka dua kegagalan sekaligus: penyerang di
// jalur bisa menyodorkan key jinak saat probe lalu key miliknya saat trust
// (dan karena Trust() menghapus entri lama, pin yang salah itu tidak pernah
// memicu peringatan lagi), dan tanpa penyerang pun host di belakang
// round-robin DNS / load balancer bisa menyimpan key mesin yang berbeda dari
// yang disetujui.
//
// Koneksi ini juga SENGAJA tidak membawa metode autentikasi apa pun. Host key
// callback berjalan saat key exchange, SEBELUM autentikasi — jadi key-nya
// sudah didapat tanpa perlu mengirim password/kunci SSH ke host yang identitasnya
// justru belum terverifikasi. Handshake memang lalu gagal di tahap auth, dan
// itu bukan kegagalan operasi ini (bandingkan `ssh-keyscan`).
func (p *Pool) TrustHostKey(ctx context.Context, cfg ServerConfig, expectedFingerprint string) error {
	if expectedFingerprint == "" {
		return ErrFingerprintRequired
	}

	addr := formatAddr(cfg.Host, cfg.Port)
	conn, err := dialTCP(ctx, p.cfg.DialTimeout, addr)
	if err != nil {
		return err
	}

	var (
		seenFP   string
		trusted  bool
		storeErr error
	)

	sshCfg := &ssh.ClientConfig{
		User: cfg.Username,
		Auth: nil, // lihat komentar di atas: jangan kirim kredensial ke host tak terverifikasi
		HostKeyCallback: func(hostname string, _ net.Addr, key ssh.PublicKey) error {
			seenFP = FingerprintSHA256(key)
			if seenFP != expectedFingerprint {
				return &HostKeyMismatch{
					Host:           normalizeHost(hostname),
					OldFingerprint: expectedFingerprint,
					NewFingerprint: seenFP,
				}
			}
			storeErr = p.knownHosts.Trust(hostname, key)
			trusted = storeErr == nil
			return storeErr
		},
	}

	client, dialErr := newSSHClient(ctx, conn, addr, sshCfg)
	if client != nil {
		// Server yang mengizinkan auth "none" bisa saja tetap memberi klien
		// utuh; kita tidak membutuhkannya, cukup host key-nya.
		_ = client.Close()
	}

	switch {
	case trusted:
		// dialErr di sini hampir pasti "unable to authenticate" — diabaikan
		// dengan sengaja, host key sudah tersimpan dan itulah tujuannya.
		return nil
	case storeErr != nil:
		return storeErr
	case seenFP != "" && seenFP != expectedFingerprint:
		return &HostKeyMismatch{
			Host:           normalizeHost(addr),
			OldFingerprint: expectedFingerprint,
			NewFingerprint: seenFP,
		}
	case dialErr != nil:
		return dialErr
	default:
		return errors.New("server tidak mengirim host key")
	}
}
