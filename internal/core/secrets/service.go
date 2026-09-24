package secrets

import (
	"errors"
	"fmt"
	"log"
)

// New membuat Vault: coba OS keychain dulu (probe set/delete beneran, bukan
// cuma cek biner ada), fallback ke file lokal terenkripsi kalau OS keychain
// tidak bisa dipakai di mesin ini (mis. Linux tanpa Secret Service jalan).
// dataDir dipakai HANYA untuk fallback file-nya (biasanya ~/.poinhost).
//
// Error dikembalikan, BUKAN ditelan jadi vault in-memory: di mesin tanpa OS
// keychain (server headless, tempat agent nanti berjalan) file vault adalah
// satu-satunya penyimpanan rahasia yang persisten. Diam-diam jatuh ke
// in-memory berarti user menyimpan password, semuanya tampak bekerja sepanjang
// sesi, lalu hilang tanpa jejak saat proses restart — kegagalan yang jauh
// lebih mahal daripada gagal sejak awal dengan pesan yang jelas.
func New(dataDir string) (Vault, error) {
	return newVault(probeKeyring(), dataDir)
}

// newVault dipisah dari New supaya cabang fallback file bisa diuji tanpa
// bergantung pada ada-tidaknya OS keychain di mesin yang menjalankan test.
func newVault(keyringAvailable bool, dataDir string) (Vault, error) {
	if keyringAvailable {
		return keyringVault{}, nil
	}
	log.Printf("secrets: OS keychain tidak tersedia, pakai vault file terenkripsi di %s", dataDir)
	fv, err := newFileVault(dataDir)
	if err != nil {
		return nil, fmt.Errorf("siapkan vault file lokal di %s: %w", dataDir, err)
	}
	return fv, nil
}

// NewEphemeral membuat vault in-memory yang TIDAK persisten. Hanya untuk test
// dan untuk jalur yang memang sengaja tidak ingin menyentuh disk — bukan
// fallback otomatis, supaya kehilangan persistensi tidak pernah terjadi tanpa
// ada yang memilihnya secara sadar.
func NewEphemeral() Vault {
	return newMemVault()
}

// IsKeyUnreadable melaporkan apakah error dari New disebabkan secret.key yang
// ada tapi rusak/tidak terbaca — kasus yang TIDAK boleh "diperbaiki" dengan
// menghapus file, karena secrets.json di sebelahnya jadi tidak bisa dibuka.
func IsKeyUnreadable(err error) bool {
	return errors.Is(err, ErrKeyUnreadable)
}
