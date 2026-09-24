package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// keyLen adalah panjang kunci AES-256 dalam byte.
const keyLen = 32

// ErrKeyUnreadable menandai secret.key yang ADA tapi tidak bisa dipakai
// (tidak terbaca, atau ukurannya salah). Ini sengaja dibedakan dari "belum
// ada": yang pertama tidak boleh ditimpa kunci baru, yang kedua boleh.
var ErrKeyUnreadable = errors.New("secrets: kunci vault lokal tidak bisa dibaca")

// fileVault adalah fallback kalau OS keychain tidak tersedia (mis. Linux
// tanpa Secret Service jalan): satu file JSON di ~/.poinhost/secrets.json
// berisi map key->ciphertext base64, dienkripsi AES-256-GCM dengan kunci
// 32-byte acak yang dibuat sekali dan disimpan terpisah di
// ~/.poinhost/secret.key (permission 0600) — beda dari homepoin yang
// menurunkan kunci dari master password yang diketik user (poinhost tidak
// punya konsep master password, lihat ARCHITECTURE.md §7), jadi kuncinya
// murni acak & spesifik-mesin-ini.
type fileVault struct {
	mu       sync.Mutex
	keyPath  string
	dataPath string

	// key di-cache setelah pembacaan pertama yang berhasil. Selain menghemat
	// satu syscall per operasi vault, ini juga membuat kunci yang dipakai
	// selama satu proses hidup TIDAK bisa berubah diam-diam di tengah jalan
	// kalau file di disk tersentuh pihak lain.
	key []byte
}

func newFileVault(dir string) (*fileVault, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	fv := &fileVault{
		keyPath:  filepath.Join(dir, "secret.key"),
		dataPath: filepath.Join(dir, "secrets.json"),
	}
	if _, err := fv.loadOrCreateKey(); err != nil {
		return nil, err
	}
	return fv, nil
}

// loadOrCreateKey membaca secret.key, dan HANYA membuat kunci baru kalau
// filenya memang belum ada. Pemanggil harus memegang v.mu (atau memanggil
// sebelum fileVault dibagikan ke goroutine lain).
//
// Kunci baru SENGAJA tidak dibuat untuk kasus error baca apa pun selain
// "file tidak ada", dan tidak juga untuk file berukuran salah: secrets.json
// di sebelahnya masih terenkripsi dengan kunci LAMA, jadi menimpa secret.key
// berarti menghapus seluruh isinya secara permanen tanpa bisa dipulihkan.
// Lebih baik gagal keras dan membiarkan user memperbaiki/memulihkan filenya
// daripada "berhasil" jalan di atas vault yang isinya sudah tidak terbaca.
func (v *fileVault) loadOrCreateKey() ([]byte, error) {
	if v.key != nil {
		return v.key, nil
	}

	b, err := os.ReadFile(v.keyPath)
	switch {
	case err == nil && len(b) == keyLen:
		v.key = b
		return v.key, nil

	case err == nil:
		return nil, fmt.Errorf(
			"%w: %s berukuran %d byte, seharusnya %d — jangan hapus file ini, "+
				"pulihkan dari backup kalau ada (menimpanya membuat semua rahasia tersimpan tidak bisa dibuka lagi)",
			ErrKeyUnreadable, v.keyPath, len(b), keyLen)

	case !os.IsNotExist(err):
		return nil, fmt.Errorf(
			"%w: %s ada tapi tidak bisa dibaca (%v) — periksa permission/kepemilikan file",
			ErrKeyUnreadable, v.keyPath, err)
	}

	key := make([]byte, keyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	// O_EXCL: kalau ada proses lain yang menang balapan membuat file ini,
	// kita pakai punya dia alih-alih menimpanya.
	f, err := os.OpenFile(v.keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return v.rereadKey()
		}
		return nil, err
	}
	if _, err := f.Write(key); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	v.key = key
	return v.key, nil
}

// rereadKey dipakai saat pembuatan kalah balapan dengan proses lain.
func (v *fileVault) rereadKey() ([]byte, error) {
	b, err := os.ReadFile(v.keyPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %s (%v)", ErrKeyUnreadable, v.keyPath, err)
	}
	if len(b) != keyLen {
		return nil, fmt.Errorf("%w: %s berukuran %d byte, seharusnya %d",
			ErrKeyUnreadable, v.keyPath, len(b), keyLen)
	}
	v.key = b
	return v.key, nil
}

func (v *fileVault) gcm() (cipher.AEAD, error) {
	key, err := v.loadOrCreateKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (v *fileVault) load() (map[string]string, error) {
	m := map[string]string{}
	b, err := os.ReadFile(v.dataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (v *fileVault) save(m map[string]string) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := v.dataPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, v.dataPath)
}

func (v *fileVault) Get(key string) (string, bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	m, err := v.load()
	if err != nil {
		return "", false, err
	}
	enc, ok := m[key]
	if !ok {
		return "", false, nil
	}
	gcm, err := v.gcm()
	if err != nil {
		return "", false, err
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", false, err
	}
	if len(raw) < gcm.NonceSize() {
		return "", false, fmt.Errorf("data rahasia korup")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", false, err
	}
	return string(plain), true, nil
}

func (v *fileVault) Set(key, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	gcm, err := v.gcm()
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(value), nil)
	m, err := v.load()
	if err != nil {
		return err
	}
	m[key] = base64.StdEncoding.EncodeToString(ciphertext)
	return v.save(m)
}

func (v *fileVault) Delete(key string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	m, err := v.load()
	if err != nil {
		return err
	}
	delete(m, key)
	return v.save(m)
}

// memVault adalah jaring pengaman TERAKHIR (in-memory, tidak persisten) —
// dipakai hanya kalau fileVault pun gagal dibuat (mis. direktori data tidak
// bisa ditulis sama sekali), supaya aplikasi tetap bisa jalan alih-alih panik.
type memVault struct {
	mu   sync.Mutex
	data map[string]string
}

func newMemVault() *memVault {
	return &memVault{data: make(map[string]string)}
}

func (v *memVault) Get(key string) (string, bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	val, ok := v.data[key]
	return val, ok, nil
}

func (v *memVault) Set(key, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.data[key] = value
	return nil
}

func (v *memVault) Delete(key string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.data, key)
	return nil
}
