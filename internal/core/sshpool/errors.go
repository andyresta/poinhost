package sshpool

import "errors"

// ErrPoolExhausted dikembalikan saat pool penuh dan timeout antrian habis.
var ErrPoolExhausted = errors.New("SSH_POOL_EXHAUSTED")

// ErrHostKeyMismatch dikembalikan saat fingerprint host key tidak cocok.
var ErrHostKeyMismatch = errors.New("SSH_HOST_KEY_MISMATCH")

// ErrHostKeyUnknown dikembalikan saat host key belum pernah dipercaya.
var ErrHostKeyUnknown = errors.New("SSH_HOST_KEY_UNKNOWN")
