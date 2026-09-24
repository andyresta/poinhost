package agentctl

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andyresta/poinhost/internal/core/sshpool"
	"golang.org/x/crypto/ssh"
)

// Agent tidak punya manusia yang bisa menyetujui dialog host key seperti app
// desktop, jadi known_hosts-nya harus sudah terisi saat pengelolaan-diri
// dinyalakan. Tanpa itu, koneksi agent ke 127.0.0.1 gagal dengan
// SSH_HOST_KEY_MISMATCH dan bot melaporkan servernya "not connected".
func TestSeedKnownHostsScript_MenanamKunciDanMenjagaEntriLain(t *testing.T) {
	script := seedKnownHostsScript()

	if !strings.Contains(script, "/etc/ssh/ssh_host_*_key.pub") {
		t.Error("kunci tidak dibaca dari berkas milik mesin itu sendiri")
	}
	// Hanya entri 127.0.0.1 yang dibuang; server LAIN yang sudah didaftarkan ke
	// agent tidak boleh ikut terhapus.
	if !strings.Contains(script, `grep -v '^127\.0\.0\.1 '`) {
		t.Error("entri lama 127.0.0.1 tidak dibersihkan, akan menumpuk duplikat")
	}
	if !strings.Contains(script, "chmod 600 "+DataDir+"/ssh_known_hosts") {
		t.Error("known_hosts agent tidak dibatasi 0600")
	}
	// `grep -c` keluar dengan status 1 saat nol — memakai `|| echo 0` akan
	// mencetak dua baris dan merusak parsing kv.
	if strings.Contains(script, "|| echo 0") {
		t.Error("baris hitungan memakai `|| echo 0`, akan menghasilkan dua nilai")
	}
}

// Format baris yang ditulis skrip harus benar-benar diterima verifier
// known_hosts poinhost. Ini yang membuktikan seluruh rantainya nyambung, bukan
// sekadar berkasnya terisi.
func TestSeedKnownHosts_FormatDiterimaVerifierSSHPool(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}

	// Persis bentuk yang dihasilkan `printf '127.0.0.1 %s\n' "$(cut -d' ' -f1,2 …)"`.
	authorized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	line := fmt.Sprintf("127.0.0.1 %s\n", authorized)

	path := filepath.Join(t.TempDir(), "ssh_known_hosts")
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	cb := sshpool.NewKnownHostsStore(path).Callback()
	// Hostname yang diberikan klien SSH selalu memuat port.
	if err := cb("127.0.0.1:22", nil, sshPub); err != nil {
		t.Fatalf("verifier menolak kunci yang baru saja ditanam: %v", err)
	}
	// Port non-standar juga harus cocok dengan entri yang sama.
	if err := cb("127.0.0.1:2222", nil, sshPub); err != nil {
		t.Errorf("port non-standar ditolak: %v", err)
	}

	// Dan kunci yang BERBEDA tetap harus ditolak — penanaman ini tidak boleh
	// berubah jadi "terima apa saja".
	lain, _, _ := ed25519.GenerateKey(rand.Reader)
	lainPub, _ := ssh.NewPublicKey(lain)
	if err := cb("127.0.0.1:22", nil, lainPub); err == nil {
		t.Error("kunci asing diterima padahal tidak cocok")
	}
}
