package sshpool

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/andyresta/poinhost/internal/core/config"
	"golang.org/x/crypto/ssh"
)

// ErrorKind mengklasifikasikan jenis error SSH.
type ErrorKind int

const (
	ErrTransient ErrorKind = iota
	ErrPermanent
)

// ExecResult menyimpan hasil eksekusi perintah SSH.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Executor mengeksekusi perintah remote via SSH dengan timeout dan retry.
type Executor struct {
	pool  *Pool
	cfg   config.ExecutorConfig
	mutex *ServerMutexRegistry
}

// NewExecutor membuat executor SSH baru.
func NewExecutor(pool *Pool, mutex *ServerMutexRegistry, cfg config.ExecutorConfig) *Executor {
	return &Executor{pool: pool, mutex: mutex, cfg: cfg}
}

// Exec menjalankan perintah dengan kategori timeout dan opsi retry, memakai
// slot shared (multiplexed, dipakai bersama semua tab ke server yang sama).
func (e *Executor) Exec(ctx context.Context, serverID string, timeout time.Duration, cmd string, destructive bool) (*ExecResult, error) {
	return e.exec(ctx, serverID, SlotShared, timeout, cmd, destructive)
}

// ExecMetrics menjalankan perintah pengambilan metrik (CPU/RAM/disk/dst) pada
// slot dedicated `SlotMetrics` — SENGAJA terpisah dari slot shared, supaya
// heartbeat status server tidak pernah ikut antre di belakang operasi berat
// modul lain (files/docker/dbmanager) ke server yang sama, dan sebaliknya.
func (e *Executor) ExecMetrics(ctx context.Context, serverID string, timeout time.Duration, cmd string) (*ExecResult, error) {
	return e.exec(ctx, serverID, SlotMetrics, timeout, cmd, false)
}

func (e *Executor) exec(ctx context.Context, serverID string, slot SlotType, timeout time.Duration, cmd string, destructive bool) (*ExecResult, error) {
	var lastErr error
	attempts := 1
	if !destructive {
		attempts = e.cfg.RetryMax + 1
	}

	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			backoff := e.retryBackoff(attempt - 1)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		conn, err := e.pool.Acquire(ctx, serverID, slot)
		if err != nil {
			lastErr = err
			if !destructive && ClassifyError(err) == ErrTransient {
				continue
			}
			return nil, err
		}

		result, execErr := e.execOnce(ctx, conn, timeout, cmd)
		e.pool.Release(serverID, slot, conn)

		if execErr == nil {
			return result, nil
		}
		lastErr = execErr
		if destructive || ClassifyError(execErr) == ErrPermanent {
			return nil, execErr
		}
	}

	return nil, lastErr
}

// ExecStreamDedicated menjalankan perintah streaming pada koneksi SSH khusus
// (tidak memakai kuota shared) — cocok untuk proses jangka panjang seperti
// `tail -f` agar tidak menghabiskan pool koneksi bersama, dan agar beberapa
// tab bisa menjalankan stream berbeda ke server yang sama secara bersamaan.
func (e *Executor) ExecStreamDedicated(ctx context.Context, serverID string, cmd string, onLine func(line string) error) error {
	handle, conn, err := e.pool.OpenDedicated(ctx, serverID)
	if err != nil {
		return err
	}
	defer e.pool.CloseDedicated(serverID, handle)
	return runStream(ctx, conn, cmd, onLine)
}

// ExecStreamPTY menjalankan perintah streaming dengan PTY (pseudo-terminal)
// pada koneksi dedicated. Dengan PTY, perintah seperti dnf/apt/certbot
// menganggap dirinya berjalan di terminal sehingga menampilkan progress
// secara real-time, mirip terminal asli.
func (e *Executor) ExecStreamPTY(ctx context.Context, serverID string, cmd string, onLine func(line string) error) error {
	handle, conn, err := e.pool.OpenDedicated(ctx, serverID)
	if err != nil {
		return err
	}
	defer e.pool.CloseDedicated(serverID, handle)
	return runStreamPTY(ctx, conn, cmd, onLine)
}

// ProgressLinePrefix menandai baris output yang berasal dari pembaruan
// progress (carriage return), agar konsumen (mis. handler WS/event instalasi)
// bisa menampilkannya seolah menimpa baris sebelumnya seperti di terminal asli.
const ProgressLinePrefix = "\x00\x01poinhostPROG\x01\x00"

func runStreamPTY(ctx context.Context, conn *ssh.Client, cmd string, onLine func(line string) error) error {
	sess, err := conn.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", 40, 160, modes); err != nil {
		return err
	}

	stdout, err := sess.StdoutPipe()
	if err != nil {
		return err
	}

	if err := sess.Start(cmd); err != nil {
		return err
	}

	emit := func(line string) {
		if err := onLine(line); err != nil {
			_ = sess.Close()
		}
	}

	doneScan := make(chan struct{})
	go func() {
		defer close(doneScan)
		reader := bufio.NewReader(stdout)
		var sb []byte
		flush := func(progress bool) {
			line := strings.TrimRight(string(sb), "\r\n")
			sb = sb[:0]
			if line == "" {
				return
			}
			if progress {
				emit(ProgressLinePrefix + line)
			} else {
				emit(line)
			}
		}
		for {
			b, rerr := reader.ReadByte()
			if rerr != nil {
				flush(false)
				return
			}
			switch b {
			case '\n':
				flush(false)
			case '\r':
				if nb, _ := reader.Peek(1); len(nb) == 1 && nb[0] == '\n' {
					_, _ = reader.ReadByte()
					flush(false)
				} else {
					flush(true)
				}
			default:
				sb = append(sb, b)
			}
		}
	}()

	go func() {
		<-ctx.Done()
		_ = sess.Signal(ssh.SIGTERM)
		_ = sess.Close()
	}()

	done := make(chan error, 1)
	go func() {
		<-doneScan
		done <- sess.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err == nil {
			return nil
		}
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			switch exitErr.ExitStatus() {
			case 0, 143, 137, -1:
				return nil
			}
		}
		return err
	}
}

// runStream menjalankan perintah pada koneksi SSH, mengalirkan stdout DAN
// stderr baris per baris. stdin di-set EOF agar sudo/bash tidak menggantung.
func runStream(ctx context.Context, conn *ssh.Client, cmd string, onLine func(line string) error) error {
	sess, err := conn.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()

	sess.Stdin = strings.NewReader("")

	stdout, err := sess.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		return err
	}

	if err := sess.Start(cmd); err != nil {
		return err
	}

	var onLineMu sync.Mutex
	emit := func(line string) {
		onLineMu.Lock()
		defer onLineMu.Unlock()
		if err := onLine(line); err != nil {
			_ = sess.Close()
		}
	}

	var wg sync.WaitGroup
	scan := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			emit(scanner.Text())
		}
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)

	go func() {
		<-ctx.Done()
		_ = sess.Signal(ssh.SIGTERM)
		_ = sess.Close()
	}()

	done := make(chan error, 1)
	go func() {
		wg.Wait()
		done <- sess.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err == nil {
			return nil
		}
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			switch exitErr.ExitStatus() {
			case 0, 143, 137, -1:
				return nil
			}
		}
		return err
	}
}

func (e *Executor) execOnce(ctx context.Context, conn *ssh.Client, timeout time.Duration, cmd string) (*ExecResult, error) {
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sess, err := conn.NewSession()
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	var stdout, stderr strings.Builder
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- sess.Run(cmd)
	}()

	select {
	case <-execCtx.Done():
		_ = sess.Signal(ssh.SIGTERM)
		return nil, execCtx.Err()
	case err := <-done:
		result := &ExecResult{
			Stdout: stdout.String(),
			Stderr: stderr.String(),
		}
		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				result.ExitCode = exitErr.ExitStatus()
				msg := strings.TrimSpace(result.Stderr)
				if msg == "" {
					msg = strings.TrimSpace(result.Stdout)
				}
				return result, fmt.Errorf("perintah gagal (exit %d): %s", result.ExitCode, msg)
			}
			return nil, err
		}
		result.ExitCode = 0
		return result, nil
	}
}

// WrapNohup membungkus perintah panjang dengan nohup agar tetap jalan jika sesi putus.
func WrapNohup(originalCmd, jobID string) string {
	return fmt.Sprintf("nohup %s > /tmp/poinhost-%s.log 2>&1 & echo $!", originalCmd, jobID)
}

// ClassifyError mengklasifikasikan error sebagai transient atau permanent.
func ClassifyError(err error) ErrorKind {
	if err == nil {
		return ErrPermanent
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrTransient
	}
	if errors.Is(err, io.EOF) || errors.Is(err, ErrPoolExhausted) {
		return ErrTransient
	}
	msg := strings.ToLower(err.Error())
	transientKeywords := []string{
		"connection reset",
		"broken pipe",
		"eof",
		"timeout",
		"i/o timeout",
		"use of closed network connection",
		"connection refused",
	}
	for _, kw := range transientKeywords {
		if strings.Contains(msg, kw) {
			return ErrTransient
		}
	}
	permanentKeywords := []string{
		"permission denied",
		"command not found",
		"no such file",
		"authentication failed",
	}
	for _, kw := range permanentKeywords {
		if strings.Contains(msg, kw) {
			return ErrPermanent
		}
	}
	return ErrPermanent
}

func (e *Executor) retryBackoff(index int) time.Duration {
	if index < len(e.cfg.RetryBackoff) {
		return e.cfg.RetryBackoff[index]
	}
	if len(e.cfg.RetryBackoff) > 0 {
		return e.cfg.RetryBackoff[len(e.cfg.RetryBackoff)-1]
	}
	return time.Second
}

// TimeoutForCategory mengembalikan durasi timeout berdasarkan kategori.
func TimeoutForCategory(cfg config.ExecutorConfig, category string) time.Duration {
	switch category {
	case "fast":
		return cfg.TimeoutFast
	case "medium":
		return cfg.TimeoutMedium
	case "long":
		return cfg.TimeoutLong
	case "very_long":
		return cfg.TimeoutVeryLong
	default:
		return cfg.TimeoutFast
	}
}
