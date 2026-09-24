package agentctl

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/agent/agentcfg"
	"github.com/andyresta/poinhost/internal/agent/dist"
	"golang.org/x/crypto/ssh"
)

// InstallRequest adalah pilihan user di form pemasangan.
type InstallRequest struct {
	ServerID string `json:"serverId"`
	Port     int    `json:"port"`

	// ManageSelf memasang kunci SSH khusus supaya agent bisa mengelola mesin
	// tempat ia berjalan sebagai server terdaftar biasa.
	ManageSelf bool `json:"manageSelf"`
}

// Install memasang agent di server.
//
// Urutannya disusun supaya tidak pernah ada keadaan setengah jadi yang
// menyesatkan: berkas diunggah ke /tmp sebagai user login, DIVERIFIKASI
// checksum-nya, baru dipindahkan ke tempatnya dengan hak root. Service
// dinyalakan paling akhir, setelah config dan unit-nya lengkap.
func (s *Service) Install(ctx context.Context, req InstallRequest) (*Status, error) {
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return nil, err
	}

	s.publish(req.ServerID, PhasePrepare, 2)
	current, err := s.Detect(ctx, req.ServerID)
	if err != nil {
		return nil, err
	}
	if !current.CanInstall() {
		if current.Problem != "" {
			return nil, fmt.Errorf("%s", current.Problem)
		}
		return nil, fmt.Errorf("server ini belum memenuhi syarat pemasangan agent")
	}
	if current.Installed {
		return nil, fmt.Errorf("agent sudah terpasang di server ini — pakai Update kalau ingin mengganti versinya")
	}

	arch, err := dist.ArchFromUname(current.Arch)
	if err != nil {
		return nil, err
	}
	gz, wantSum, err := dist.Compressed(arch)
	if err != nil {
		return nil, err
	}

	port := req.Port
	if port <= 0 {
		port = DefaultPort
	}
	listen := "127.0.0.1:" + strconv.Itoa(port)
	if err := agentcfg.ValidateListen(listen); err != nil {
		return nil, err
	}

	apiToken, err := randomToken()
	if err != nil {
		return nil, err
	}

	acfg := &agentcfg.Config{
		DataDir:  DataDir,
		Listen:   listen,
		APIToken: apiToken,
		Telegram: agentcfg.Telegram{AllowedUserIDs: []int64{}},
	}

	var authorizedKey string
	if req.ManageSelf {
		priv, pub, kerr := generateKeypair()
		if kerr != nil {
			return nil, kerr
		}
		authorizedKey = pub
		acfg.SelfServer = agentcfg.SelfServer{
			Enabled:  true,
			Name:     "This server",
			Host:     "127.0.0.1",
			Port:     access.sshPort,
			Username: "root",
			KeyPath:  KeyPath,
		}
		if uerr := s.uploadTemp(ctx, req.ServerID, tmpKeyPath, []byte(priv)); uerr != nil {
			return nil, uerr
		}
	}

	if err := s.uploadBinary(ctx, req.ServerID, gz, 10, 70); err != nil {
		return nil, err
	}
	s.publish(req.ServerID, PhaseVerify, 72)
	cfgJSON, err := json.MarshalIndent(acfg, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := s.uploadTemp(ctx, req.ServerID, tmpConfigPath, append(cfgJSON, '\n')); err != nil {
		return nil, err
	}
	if err := s.uploadTemp(ctx, req.ServerID, tmpUnitPath, []byte(systemdUnit)); err != nil {
		return nil, err
	}

	s.publish(req.ServerID, PhaseInstall, 78)
	script := installScript(wantSum, req.ManageSelf, authorizedKey)
	if _, err := s.run(ctx, access, script, 120*time.Second); err != nil {
		return nil, err
	}

	return s.waitHealthy(ctx, req.ServerID, 88)
}

// Lokasi sementara di /tmp. SFTP menulis sebagai user login yang belum tentu
// root, jadi berkas mendarat di sini dulu lalu dipindahkan dengan `install`.
const (
	tmpBinaryPath = "/tmp/.poinhost-agent.bin"
	tmpBinaryGz   = "/tmp/.poinhost-agent.bin.gz"
	tmpConfigPath = "/tmp/.poinhost-agent.json"
	tmpUnitPath   = "/tmp/.poinhost-agent.unit"
	tmpKeyPath    = "/tmp/.poinhost-agent.key"
)

// systemdUnit adalah definisi service.
//
// Catatan sengaja TIDAK memakai ProtectHome/ProtectSystem yang ketat: agent
// membaca kunci SSH-nya dari /var/lib/poinhost-agent dan menjalankan perintah
// ke server lain, dan pengerasan yang dipasang tanpa diuji cenderung
// mematahkan hal-hal itu secara diam-diam.
//
// PrivateTmp=yes tetap dipakai, TAPI konsekuensinya harus diingat saat
// menambah fitur: proses agent melihat /tmp privat miliknya sendiri, bukan
// /tmp asli. Berkas yang diserahkan dari app desktop KE PROSES AGENT karena
// itu tidak boleh lewat /tmp — lihat bundleDropPath. Berkas sementara yang
// hanya dibaca perintah shell lewat SSH tidak terpengaruh, karena perintah itu
// berjalan di luar namespace service.
const systemdUnit = `[Unit]
Description=poinhost agent
Documentation=https://github.com/andyresta/poinhost
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/poinhost-agent --config /etc/poinhost-agent/config.json
Restart=always
RestartSec=5
NoNewPrivileges=yes
PrivateTmp=yes

[Install]
WantedBy=multi-user.target
`

func installScript(wantSum string, manageSelf bool, authorizedKey string) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	b.WriteString("install -d -m 0700 " + ConfigDir + " " + DataDir + "\n")
	b.WriteString(gunzipScript())

	// Checksum diperiksa di server, bukan diasumsikan dari "SFTP tidak error".
	b.WriteString("got=$(sha256sum " + tmpBinaryPath + " | cut -d' ' -f1)\n")
	b.WriteString(`if [ "$got" != ` + shellQuote(wantSum) + ` ]; then echo "checksum binary agent tidak cocok" >&2; exit 1; fi` + "\n")

	b.WriteString("install -m 0755 -o root -g root " + tmpBinaryPath + " " + BinaryPath + "\n")
	b.WriteString("install -m 0600 -o root -g root " + tmpConfigPath + " " + ConfigPath + "\n")
	b.WriteString("install -m 0644 -o root -g root " + tmpUnitPath + " " + UnitPath + "\n")

	if manageSelf && authorizedKey != "" {
		b.WriteString(selfKeyScript(authorizedKey))
	}

	b.WriteString("rm -f " + tmpBinaryPath + " " + tmpConfigPath + " " + tmpUnitPath + "\n")
	b.WriteString("systemctl daemon-reload\n")
	b.WriteString("systemctl enable --now " + UnitName + "\n")
	return b.String()
}

// selfKeyScript memasang kunci SSH khusus agent supaya ia bisa menyambung ke
// mesinnya sendiri. Dipakai saat pemasangan maupun saat pengelolaan-diri
// diaktifkan belakangan.
//
// Baris kunci ditulis lewat base64 ke sebuah variabel, bukan langsung di
// command line: apa pun yang ada di command line terlihat di `ps` selama
// perintah berjalan. Penambahannya idempoten supaya dijalankan dua kali tidak
// menumpuk baris duplikat di authorized_keys.
func selfKeyScript(authorizedKey string) string {
	var b strings.Builder
	b.WriteString("install -m 0600 -o root -g root " + tmpKeyPath + " " + KeyPath + "\n")
	b.WriteString("install -d -m 0700 /root/.ssh\n")
	b.WriteString("touch /root/.ssh/authorized_keys && chmod 600 /root/.ssh/authorized_keys\n")
	b.WriteString("key=$(echo " + shellQuote(base64.StdEncoding.EncodeToString([]byte(authorizedKey))) + " | base64 -d)\n")
	// Baris kunci agent SEBELUMNYA dibuang dulu. Tiap pemanggilan membuat
	// keypair baru, jadi tanpa ini setiap pemasangan ulang meninggalkan satu
	// kunci yatim yang masih bisa login sebagai root — dan "copot bersih" jadi
	// bergantung pada penghapusan yang sama di tempat lain.
	b.WriteString("sed -i '/" + authKeyLabel + "$/d' /root/.ssh/authorized_keys 2>/dev/null || true\n")
	b.WriteString(`printf '%s\n' "$key" >> /root/.ssh/authorized_keys` + "\n")
	b.WriteString("rm -f " + tmpKeyPath + "\n")
	b.WriteString(seedKnownHostsScript())
	return b.String()
}

// seedKnownHostsScript menanam host key mesin ini ke known_hosts milik agent.
//
// Tanpa ini agent gagal menyambung ke dirinya sendiri dengan
// SSH_HOST_KEY_MISMATCH: host key-nya belum pernah dipercaya, dan di agent
// tidak ada manusia yang bisa menyetujui dialog seperti di app desktop.
//
// Kuncinya dibaca dari /etc/ssh/ssh_host_*.pub — berkas milik mesin itu
// sendiri — lewat koneksi SSH yang identitasnya SUDAH diverifikasi user saat
// server ditambahkan ke poinhost. Jadi tidak ada keputusan kepercayaan baru di
// sini, hanya meneruskan keputusan yang sudah pernah diambil. Itu juga sebabnya
// `ssh-keyscan` hanya dipakai sebagai cadangan: ia menanyakan kunci lewat
// jaringan alih-alih membacanya dari sumbernya.
func seedKnownHostsScript() string {
	kh := DataDir + "/ssh_known_hosts"
	return strings.Join([]string{
		"umask 077",
		"touch " + kh,
		// Entri 127.0.0.1 lama dibuang lebih dulu supaya pemasangan ulang tidak
		// menumpuk duplikat, sementara entri server LAIN yang sudah didaftarkan
		// ke agent tetap utuh.
		"grep -v '^127\\.0\\.0\\.1 ' " + kh + " > " + kh + ".tmp 2>/dev/null || true",
		"for f in /etc/ssh/ssh_host_*_key.pub; do",
		"  [ -f \"$f\" ] || continue",
		"  printf '127.0.0.1 %s\\n' \"$(cut -d' ' -f1,2 \"$f\")\" >> " + kh + ".tmp",
		"done",
		// Cadangan untuk sistem yang tidak menyimpan berkas .pub.
		"if ! grep -q '^127\\.0\\.0\\.1 ' " + kh + ".tmp 2>/dev/null; then",
		"  ssh-keyscan -T 5 127.0.0.1 2>/dev/null | grep -v '^#' >> " + kh + ".tmp || true",
		"fi",
		"mv " + kh + ".tmp " + kh,
		"chmod 600 " + kh,
		"chown root:root " + kh,
		// `grep -c` sudah mencetak 0 saat tidak ada yang cocok, tapi keluar dengan
		// status 1; `|| true` hanya menelan statusnya. `|| echo 0` justru
		// menghasilkan DUA baris "0" dan merusak parsing kv.
		`echo "known_hosts_entries=$(grep -c '^127\.0\.0\.1 ' ` + kh + ` 2>/dev/null || true)"`,
		"",
	}, "\n")
}

// EnableSelfManagement membuat agent yang sudah terpasang bisa mengelola mesin
// tempat ia berjalan, tanpa perlu dicopot dan dipasang ulang.
//
// Dibutuhkan untuk dua hal: user yang saat memasang tidak mencentang opsinya
// lalu berubah pikiran, dan pemulihan config yang blok selfServer-nya hilang —
// yang bisa terjadi kalau agent versi lama pernah menulis ulang config dengan
// struct yang belum mengenal blok itu (lihat agentcfg.Save).
func (s *Service) EnableSelfManagement(ctx context.Context, serverID string) (*Status, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	current, err := s.Detect(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if !current.Installed {
		return nil, fmt.Errorf("agent belum terpasang di server ini")
	}

	acfg, err := s.readConfig(ctx, access)
	if err != nil {
		return nil, err
	}

	priv, pub, err := generateKeypair()
	if err != nil {
		return nil, err
	}
	if err := s.uploadTemp(ctx, serverID, tmpKeyPath, []byte(priv)); err != nil {
		return nil, err
	}
	if _, err := s.run(ctx, access, "set -e\n"+selfKeyScript(pub), 60*time.Second); err != nil {
		return nil, err
	}

	acfg.SelfServer = agentcfg.SelfServer{
		Enabled:  true,
		Name:     "This server",
		Host:     "127.0.0.1",
		Port:     access.sshPort,
		Username: "root",
		KeyPath:  KeyPath,
	}
	// SelfServerID SENGAJA dipertahankan. Kuncinya memang berganti, tapi entri
	// server menunjuk ke KeyPath yang tetap — berkasnya yang ditimpa, bukan
	// lokasinya. Mengosongkannya membuat agent mendaftarkan entri baru setiap
	// kali aksi ini dijalankan, sehingga daftar server terisi duplikat yang
	// semuanya menunjuk 127.0.0.1.

	if err := s.writeConfig(ctx, serverID, access, acfg); err != nil {
		return nil, err
	}
	if _, err := s.run(ctx, access, "systemctl restart "+UnitName, 45*time.Second); err != nil {
		return nil, err
	}
	return s.waitHealthy(ctx, serverID, 60)
}

// Update mengganti binary agent dengan yang dibawa build desktop ini.
//
// Dijalankan DARI app desktop lewat SSH, bukan oleh agent terhadap dirinya
// sendiri — proses yang menimpa binary-nya sendiri lalu me-restart service-nya
// adalah cara paling mudah kehilangan kendali atas server. Binary lama disimpan
// sebagai .prev dan dikembalikan kalau versi baru gagal sehat.
func (s *Service) Update(ctx context.Context, serverID string) (*Status, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	s.publish(serverID, PhasePrepare, 2)
	current, err := s.Detect(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if !current.Installed {
		return nil, fmt.Errorf("agent belum terpasang di server ini")
	}
	arch, err := dist.ArchFromUname(current.Arch)
	if err != nil {
		return nil, err
	}
	gz, wantSum, err := dist.Compressed(arch)
	if err != nil {
		return nil, err
	}

	if err := s.uploadBinary(ctx, serverID, gz, 8, 72); err != nil {
		return nil, err
	}

	s.publish(serverID, PhaseInstall, 76)
	script := "set -e\n" +
		gunzipScript() +
		"got=$(sha256sum " + tmpBinaryPath + " | cut -d' ' -f1)\n" +
		`if [ "$got" != ` + shellQuote(wantSum) + ` ]; then echo "checksum binary agent tidak cocok" >&2; exit 1; fi` + "\n" +
		"cp -f " + BinaryPath + " " + PrevPath + "\n" +
		"systemctl stop " + UnitName + "\n" +
		"install -m 0755 -o root -g root " + tmpBinaryPath + " " + BinaryPath + "\n" +
		"rm -f " + tmpBinaryPath + "\n" +
		"systemctl start " + UnitName + "\n"
	if _, err := s.run(ctx, access, script, 120*time.Second); err != nil {
		return nil, err
	}

	st, err := s.waitHealthy(ctx, serverID, 86)
	if err == nil {
		return st, nil
	}

	rollback := "set -e\n" +
		"systemctl stop " + UnitName + " || true\n" +
		"[ -x " + PrevPath + " ] && install -m 0755 -o root -g root " + PrevPath + " " + BinaryPath + "\n" +
		"systemctl start " + UnitName + "\n"
	if _, rerr := s.run(ctx, access, rollback, 60*time.Second); rerr != nil {
		return nil, fmt.Errorf("update gagal (%v) DAN rollback juga gagal (%v) — agent mungkin tidak berjalan", err, rerr)
	}
	return nil, fmt.Errorf("update gagal, versi sebelumnya sudah dikembalikan: %w", err)
}

// Uninstall menghapus agent beserta seluruh jejaknya.
func (s *Service) Uninstall(ctx context.Context, serverID string) (*Status, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	// `|| true` di hampir setiap baris disengaja: pencopotan harus
	// menyelesaikan sebanyak mungkin meski sebagian jejak sudah tidak ada,
	// bukan berhenti di langkah pertama dan meninggalkan sisanya.
	script := "systemctl disable --now " + UnitName + " 2>/dev/null || true\n" +
		"rm -f " + UnitPath + "\n" +
		"systemctl daemon-reload 2>/dev/null || true\n" +
		"rm -f " + BinaryPath + " " + PrevPath + "\n" +
		"rm -rf " + ConfigDir + " " + DataDir + "\n" +
		// Kunci agent ikut dicabut dari authorized_keys — tanpa ini "copot
		// bersih" meninggalkan kunci yatim yang masih bisa login sebagai root.
		`if [ -f /root/.ssh/authorized_keys ]; then sed -i '/poinhost-agent/d' /root/.ssh/authorized_keys; fi` + "\n" +
		"echo removed\n"
	if _, err := s.run(ctx, access, script, 60*time.Second); err != nil {
		return nil, err
	}
	return s.Detect(ctx, serverID)
}

// Lifecycle menjalankan start/stop/restart.
func (s *Service) Lifecycle(ctx context.Context, serverID, action string) (*Status, error) {
	switch action {
	case "start", "stop", "restart":
	default:
		return nil, fmt.Errorf("aksi %q tidak dikenal", action)
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	if _, err := s.run(ctx, access, "systemctl "+action+" "+UnitName, 45*time.Second); err != nil {
		return nil, err
	}
	if action == "stop" {
		return s.Detect(ctx, serverID)
	}
	return s.waitHealthy(ctx, serverID, 60)
}

// min1 membatasi rasio di 1 supaya persentase tidak melewati fase berikutnya.
func min1(v float64) float64 {
	if v > 1 {
		return 1
	}
	return v
}

// Logs mengambil ekor jurnal service.
func (s *Service) Logs(ctx context.Context, serverID string, lines int) (string, error) {
	if lines <= 0 || lines > 1000 {
		lines = 200
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return "", err
	}
	script := "journalctl -u " + UnitName + " -n " + strconv.Itoa(lines) + " --no-pager 2>/dev/null || true"
	res, err := s.run(ctx, access, script, 30*time.Second)
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(res.Stdout)
	if out == "" {
		return "(belum ada log untuk " + UnitName + ")", nil
	}
	return out, nil
}

// waitHealthy menunggu agent benar-benar menjawab /healthz.
//
// Kenapa bukan sekadar percaya `systemctl start` yang sukses: systemd
// melaporkan "active" begitu prosesnya ter-fork, jauh sebelum agent selesai
// memuat konfigurasi, membuka database, dan mulai melayani. Melaporkan
// "berhasil" di titik itu berarti user melihat hijau untuk agent yang satu
// detik kemudian mati karena config-nya salah.
func (s *Service) waitHealthy(ctx context.Context, serverID string, fromPercent int) (*Status, error) {
	deadline := time.Now().Add(30 * time.Second)
	start := time.Now()
	var last *Status
	for {
		s.publish(serverID, PhaseHealth, fromPercent+int(float64(100-fromPercent)*min1(time.Since(start).Seconds()/30)))
		st, err := s.Detect(ctx, serverID)
		if err != nil {
			return nil, err
		}
		last = st
		if st.Healthy {
			s.publish(serverID, PhaseDone, 100)
			return st, nil
		}
		if time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if last != nil && last.Installed && !last.Active {
		return last, fmt.Errorf("agent terpasang tapi service-nya tidak berjalan — lihat log untuk sebabnya")
	}
	return last, fmt.Errorf("agent tidak menjawab health check dalam 30 detik — lihat log untuk sebabnya")
}

// uploadTemp menaruh satu berkas di /tmp server lewat SFTP.
//
// Lewat SFTP, bukan `echo ... > file`: isi berkas di sini mencakup binary dan
// konfigurasi yang memuat token bot, dan apa pun yang masuk ke command line
// akan terlihat di `ps` selama perintah berjalan.
func (s *Service) uploadTemp(ctx context.Context, serverID, path string, data []byte) error {
	if err := s.sftp.WriteFile(ctx, serverID, path, data); err != nil {
		return fmt.Errorf("unggah %s: %w", path, err)
	}
	return s.sftp.Chmod(ctx, serverID, path, 0o600)
}

// uploadBinary mengunggah binary agent dalam bentuk terkompresi sambil
// melaporkan kemajuannya per byte, lalu mendekompresinya di server.
func (s *Service) uploadBinary(ctx context.Context, serverID string, gz []byte, from, to int) error {
	s.publish(serverID, PhaseUpload, from)
	r := newProgressReader(bytes.NewReader(gz), int64(len(gz)), from, to, func(pct int) {
		s.publish(serverID, PhaseUpload, pct)
	})
	if err := s.sftp.UploadStream(ctx, serverID, tmpBinaryGz, r); err != nil {
		return fmt.Errorf("unggah binary agent: %w", err)
	}
	return s.sftp.Chmod(ctx, serverID, tmpBinaryGz, 0o600)
}

// gunzipScript mendekompresi binary di server.
func gunzipScript() string {
	return "command -v gzip >/dev/null 2>&1 || { echo \"gzip tidak tersedia di server ini\" >&2; exit 1; }\n" +
		"gzip -dc " + tmpBinaryGz + " > " + tmpBinaryPath + "\n" +
		"rm -f " + tmpBinaryGz + "\n"
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// generateKeypair membuat kunci ed25519 khusus agent.
//
// Kunci TERPISAH, bukan memakai ulang kredensial SSH yang sudah ada di
// poinhost: hanya dengan begitu "copot agent" bisa benar-benar mencabut akses
// (hapus satu baris authorized_keys) tanpa merusak akses app desktop.
func generateKeypair() (privatePEM string, authorizedKey string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	block, err := ssh.MarshalPrivateKey(priv, authKeyLabel)
	if err != nil {
		return "", "", err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", "", err
	}
	authorized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " " + authKeyLabel
	return string(pem.EncodeToMemory(block)), authorized, nil
}

// checksum dipakai test untuk memverifikasi skrip instalasi memakai nilai yang
// sama dengan yang dihitung dari binary.
func checksum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
