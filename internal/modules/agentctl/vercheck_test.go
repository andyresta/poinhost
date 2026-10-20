package agentctl

import "testing"

// Agent 0.2.2 di server tidak punya socket admin, jadi persetujuan perangkat
// baru bisa dipakai setelah di-update. Kalau versi yang dibundel tidak
// terbaca lebih baru, tombol Update tidak pernah muncul dan user terjebak
// dengan panel yang melaporkan galat tanpa jalan keluar.
func TestVersi030MenawarkanUpdateDari022(t *testing.T) {
	if got := decideVersionAction("0.2.2", "0.3.0"); got != versionUpdate {
		t.Errorf("= %v, mau versionUpdate", got)
	}
	// Versi bundel yang bukan semver (mis. "dev") membuat perbandingan
	// menyerah diam-diam — dan itulah yang terjadi kalau binary dibangun
	// tanpa VERSION.
	if got := decideVersionAction("0.2.2", "dev-local"); got == versionUpdate {
		t.Error("versi bundel tak terbaca ikut ditawarkan sebagai update")
	}
}
