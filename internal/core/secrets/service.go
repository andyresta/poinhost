package secrets

import "log"

// New membuat Vault: coba OS keychain dulu (probe set/delete beneran, bukan
// cuma cek biner ada), fallback ke file lokal terenkripsi kalau OS keychain
// tidak bisa dipakai di mesin ini (mis. Linux tanpa Secret Service jalan).
// dataDir dipakai HANYA untuk fallback file-nya (biasanya ~/.poinhost).
func New(dataDir string) Vault {
	if probeKeyring() {
		return keyringVault{}
	}
	fv, err := newFileVault(dataDir)
	if err != nil {
		log.Printf("secrets: gagal siapkan vault file lokal (%v), pakai in-memory (TIDAK persisten)", err)
		return newMemVault()
	}
	return fv
}
