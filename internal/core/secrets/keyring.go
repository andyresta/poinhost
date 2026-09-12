package secrets

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// keyringService adalah nama "service" yang dipakai untuk mengelompokkan
// seluruh entry poinhost di Keychain (macOS) / Credential Manager (Windows)
// / Secret Service (Linux, lewat libsecret/D-Bus) — satu namespace, banyak
// key (satu per kredensial database).
const keyringService = "poinhost"

// keyringVault mendelegasikan penyimpanan ke credential manager bawaan OS —
// pilihan PALING optimal untuk app desktop: tidak perlu kelola kunci enkripsi
// sendiri, dan sudah terlindungi oleh login OS user.
type keyringVault struct{}

func (keyringVault) Get(key string) (string, bool, error) {
	v, err := keyring.Get(keyringService, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return v, true, nil
}

func (keyringVault) Set(key, value string) error {
	return keyring.Set(keyringService, key, value)
}

func (keyringVault) Delete(key string) error {
	err := keyring.Delete(keyringService, key)
	if err != nil && errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// probeKeyring mengecek apakah OS keychain benar-benar bisa dipakai di mesin
// ini (mis. Linux tanpa Secret Service/D-Bus session — umum di beberapa
// distro minimal atau lingkungan headless — akan gagal di sini).
func probeKeyring() bool {
	const probeKey = "__poinhost_probe__"
	if err := keyring.Set(keyringService, probeKey, "ok"); err != nil {
		return false
	}
	_ = keyring.Delete(keyringService, probeKey)
	return true
}
