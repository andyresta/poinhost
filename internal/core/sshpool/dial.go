package sshpool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

// ProbeTimeout mengembalikan batas waktu uji koneksi SSH (dial + handshake).
func ProbeTimeout(dialTimeout time.Duration) time.Duration {
	timeout := dialTimeout + 15*time.Second
	if timeout < 30*time.Second {
		return 30 * time.Second
	}
	return timeout
}

// NewProbeContext membuat context dengan batas waktu uji koneksi SSH.
func NewProbeContext(dialTimeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), ProbeTimeout(dialTimeout))
}

// FriendlyConnectionError mengubah error koneksi SSH menjadi pesan yang mudah dipahami.
func FriendlyConnectionError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("koneksi SSH timeout — periksa host, port, dan firewall")
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connection refused"):
		return errors.New("koneksi ditolak — periksa apakah SSH aktif di server")
	case strings.Contains(msg, "no route to host"),
		strings.Contains(msg, "network is unreachable"):
		return errors.New("host tidak dapat dijangkau — periksa alamat dan jaringan")
	case strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "context deadline exceeded"),
		strings.Contains(msg, "timeout"):
		return errors.New("koneksi SSH timeout — periksa host, port, dan firewall")
	case strings.Contains(msg, "unable to authenticate"),
		strings.Contains(msg, "permission denied"):
		return errors.New("autentikasi SSH gagal — periksa username, password, atau kunci")
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Timeout() {
			return errors.New("koneksi SSH timeout — periksa host, port, dan firewall")
		}
		if errors.Is(opErr.Err, syscall.ECONNREFUSED) {
			return errors.New("koneksi ditolak — periksa apakah SSH aktif di server")
		}
	}

	return err
}

func dialTCP(ctx context.Context, dialTimeout time.Duration, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, FriendlyConnectionError(err)
	}
	return conn, nil
}

func newSSHClient(ctx context.Context, conn net.Conn, addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
		defer func() { _ = conn.SetDeadline(time.Time{}) }()
	}

	type handshakeResult struct {
		sshConn ssh.Conn
		chans   <-chan ssh.NewChannel
		reqs    <-chan *ssh.Request
		err     error
	}

	resultCh := make(chan handshakeResult, 1)
	go func() {
		sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
		resultCh <- handshakeResult{sshConn, chans, reqs, err}
	}()

	select {
	case <-ctx.Done():
		_ = conn.Close()
		res := <-resultCh
		if res.err == nil && res.sshConn != nil {
			_ = res.sshConn.Close()
		}
		return nil, FriendlyConnectionError(ctx.Err())
	case res := <-resultCh:
		if res.err != nil {
			return nil, FriendlyConnectionError(res.err)
		}
		return ssh.NewClient(res.sshConn, res.chans, res.reqs), nil
	}
}

func formatAddr(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}
