package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"

	"golang.org/x/crypto/pbkdf2"
)

// magic menandai file sebagai arsip poinhost yang valid — dicek sebelum
// mencoba dekripsi, supaya file salah (mis. bukan arsip poinhost sama
// sekali) dapat pesan error yang jelas, bukan "passphrase salah".
var magic = []byte("POINHOSTBKP1")

const (
	saltLen          = 16
	nonceLen         = 12
	pbkdf2Iterations = 200_000
)

// deriveKey menurunkan kunci AES-256 dari PASSPHRASE yang diingat user
// (PBKDF2-HMAC-SHA256) — beda mendasar dari vault lokal
// (internal/core/secrets) yang kuncinya acak & spesifik satu mesin: arsip
// ini akan dibawa ke mesin LAIN, jadi kuncinya justru harus bisa
// direproduksi dari sesuatu yang diingat user (bukan disimpan di mesin
// mana pun), yaitu passphrase itu sendiri + salt acak yang ikut ditulis
// di file (salt bukan rahasia, cuma mencegah rainbow table).
func deriveKey(passphrase string, salt []byte) []byte {
	return pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, 32, sha256.New)
}

// Encrypt mengemas plaintext (payload JSON) jadi satu file biner:
// magic || salt || nonce || ciphertext(AES-256-GCM).
func Encrypt(plaintext []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := deriveKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 0, len(magic)+len(salt)+len(nonce)+len(ciphertext))
	out = append(out, magic...)
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

// Decrypt membalik Encrypt — mengembalikan error yang jelas beda antara
// "bukan file arsip poinhost" dan "passphrase salah/file rusak".
func Decrypt(data []byte, passphrase string) ([]byte, error) {
	if len(data) < len(magic)+saltLen+nonceLen {
		return nil, errors.New("file bukan arsip poinhost yang valid (terlalu pendek)")
	}
	if string(data[:len(magic)]) != string(magic) {
		return nil, errors.New("file bukan arsip poinhost yang valid")
	}
	rest := data[len(magic):]
	salt := rest[:saltLen]
	nonce := rest[saltLen : saltLen+nonceLen]
	ciphertext := rest[saltLen+nonceLen:]

	key := deriveKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("passphrase salah, atau file rusak")
	}
	return plaintext, nil
}
