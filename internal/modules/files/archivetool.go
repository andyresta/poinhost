package files

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/core/pkgfail"
	"github.com/andyresta/poinhost/internal/core/sshpool"
)

// Ekstraksi arsip bergantung pada perintah yang TIDAK selalu ada di server.
// `unzip` khususnya absen di hampir semua image minimal — Rocky, AlmaLinux,
// Debian netinst, Alpine — dan dulu kegagalannya baru terlihat SESUDAH
// tombol Ekstrak ditekan, sebagai "exit 127" dengan satu saran yang
// menyebut apt. Saran itu salah di separuh distro yang didukung poinhost,
// dan muncul di saat yang paling tidak berguna: pekerjaannya sudah gagal.
//
// Di sini alatnya diperiksa LEBIH DULU, dan kalau kurang, bisa dipasang
// lewat jalur yang sama persis dengan instalasi nginx: deteksi package
// manager, periksa resolver DNS sebelum memulai, pasang sambil mengalirkan
// keluarannya, lalu terjemahkan kegagalannya lewat pkgfail.

// ArchiveToolInfo adalah hasil pemeriksaan alat ekstraksi untuk satu arsip.
type ArchiveToolInfo struct {
	Format string `json:"format"` // "zip" | "tar.gz"

	// Ready berarti ekstraksi bisa dijalankan sekarang juga, tanpa memasang
	// apa pun. Kalau true, seluruh field di bawah tidak perlu dilihat.
	Ready bool `json:"ready"`

	// Missing adalah perintah yang tidak ditemukan di server.
	Missing []string `json:"missing"`

	// Packages adalah paket yang akan dipasang untuk menutupi Missing.
	Packages []string `json:"packages"`

	DistroID       string `json:"distroId"`
	DistroName     string `json:"distroName"`
	PackageManager string `json:"packageManager"`

	// CanInstall berarti poinhost bisa memasangnya sendiri dari sini.
	CanInstall bool `json:"canInstall"`

	// Blocker adalah alasan poinhost TIDAK bisa memasangnya sendiri.
	Blocker string `json:"blocker,omitempty"`

	// ManualCommand adalah perintah yang bisa dijalankan pemilik server
	// sendiri, sudah disesuaikan dengan package manager distronya — bukan
	// "apt install" yang ditebak untuk semua orang.
	ManualCommand string `json:"manualCommand,omitempty"`
}

// archiveFormat menentukan format arsip dari nama berkasnya.
//
// Dipakai BERSAMA oleh Extract dan ArchiveTool, supaya keduanya tidak
// mungkin berbeda pendapat: pemeriksaan yang mengira sebuah berkas .tgz
// adalah zip akan menawarkan memasang unzip untuk arsip yang sebenarnya
// diekstrak dengan tar.
func archiveFormat(path string) (string, bool) {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return "zip", true
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz", true
	}
	return "", false
}

// alatDetectScript mengumpulkan distro, package manager, dan alat yang ada
// dalam SATU round-trip.
//
// `set +e` dan `exit 0` di ujungnya disengaja: tidak satu pun baris di sini
// boleh menggagalkan skrip. Yang dicari justru perintah yang TIDAK ada, dan
// kode keluar bukan nol dari pemeriksaan itu sendiri akan terbaca sebagai
// "tidak bisa memeriksa".
const alatDetectScript = `set +e
. /etc/os-release 2>/dev/null
echo "ID=${ID:-unknown}"
echo "PRETTY=${PRETTY_NAME:-unknown}"
for c in apt-get dnf yum zypper pacman apk; do
  command -v "$c" >/dev/null 2>&1 && echo "PM=$c"
done
for c in unzip tar gzip python3 python; do
  command -v "$c" >/dev/null 2>&1 && echo "HAVE=$c"
done
exit 0`

// pmPrioritas menentukan pilihan kalau lebih dari satu package manager ada
// — RHEL 8+ punya dnf DAN yum, dan dnf yang dipakai.
var pmPrioritas = []string{"apt", "dnf", "yum", "zypper", "pacman", "apk"}

// parseAlatOutput dipisah supaya bisa diuji tanpa SSH.
func parseAlatOutput(out string) (distroID, distroName, pm string, ada map[string]bool) {
	distroID, distroName = "unknown", "unknown"
	ada = map[string]bool{}
	punya := map[string]bool{}

	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "ID":
			distroID = strings.ToLower(v)
		case "PRETTY":
			distroName = v
		case "PM":
			// Yang dicari binarinya (`apt-get`), yang disebut keluarganya
			// (`apt`) — sama seperti modul lain, supaya pkgfail.HostRepo
			// dan daftar skrip instalasi di bawah memakai nama yang sama.
			if v == "apt-get" {
				v = "apt"
			}
			punya[v] = true
		case "HAVE":
			ada[v] = true
		}
	}

	for _, p := range pmPrioritas {
		if punya[p] {
			pm = p
			break
		}
	}
	return distroID, distroName, pm, ada
}

// siapEkstrak menilai apakah ekstraksi bisa jalan, dan apa yang kurang.
//
// Syaratnya BEDA per format, dan itu bukan detail: zip butuh SALAH SATU
// dari unzip/python, sedangkan tar.gz butuh tar DAN gzip.
func siapEkstrak(format string, ada map[string]bool) (ready bool, kurang []string) {
	switch format {
	case "zip":
		// Urutannya mengikuti buildZipExtractCmd. python memang tidak
		// pernah dipasang oleh fitur ini, tapi kalau sudah ada di server ia
		// benar-benar menutupi kebutuhan unzip — menawarkan pemasangan
		// untuk server yang sudah bisa mengekstrak adalah gangguan.
		if ada["unzip"] || ada["python3"] || ada["python"] {
			return true, nil
		}
		return false, []string{"unzip"}
	case "tar.gz":
		// `tar -xzf` memanggil gzip sebagai proses terpisah di banyak
		// build, jadi tar saja belum cukup.
		for _, c := range []string{"tar", "gzip"} {
			if !ada[c] {
				kurang = append(kurang, c)
			}
		}
		return len(kurang) == 0, kurang
	}
	return false, nil
}

// paketUntuk memetakan nama perintah ke nama paketnya.
//
// Untuk ketiga perintah di sini namanya KEBETULAN sama di keenam package
// manager yang didukung (apt, dnf, yum, zypper, pacman, apk) — dan itulah
// yang membuat fitur ini bisa lintas distro tanpa tabel per-distro. Fungsi
// ini ada supaya kebetulan itu tertulis, bukan tersembunyi: perintah
// keempat belum tentu seberuntung itu.
func paketUntuk(cmd string) string {
	switch cmd {
	case "unzip":
		return "unzip"
	case "tar":
		return "tar"
	case "gzip":
		return "gzip"
	}
	return ""
}

func paketUntukSemua(cmds []string) []string {
	out := make([]string, 0, len(cmds))
	for _, c := range cmds {
		if p := paketUntuk(c); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// aptTungguLock menunggu dpkg/apt lock yang masih dipegang proses lain
// (unattended-upgrades, paling sering) sebelum menyerah.
const aptTungguLock = `for i in $(seq 1 60); do
  if fuser /var/lib/dpkg/lock-frontend >/dev/null 2>&1 || fuser /var/lib/apt/lists/lock >/dev/null 2>&1; then
    [ "$i" = 1 ] && echo ">> Menunggu proses apt lain selesai..."
    sleep 5
  else
    break
  fi
done`

// installPaketScript menyusun skrip pemasangan untuk satu package manager.
//
// Nama paketnya berasal dari paketUntuk, BUKAN dari UI — lihat catatan di
// StreamInstallArchiveTool.
func installPaketScript(pm string, paket []string) (string, bool) {
	if len(paket) == 0 {
		return "", false
	}
	args := strings.Join(paket, " ")
	var inti string
	switch pm {
	case "apt":
		inti = aptTungguLock + `
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ` + args
	case "dnf":
		inti = "dnf install -y " + args
	case "yum":
		inti = "yum install -y " + args
	case "zypper":
		inti = "zypper --non-interactive install " + args
	case "pacman":
		inti = "pacman -Sy --noconfirm --needed " + args
	case "apk":
		inti = "apk add --no-cache " + args
	default:
		return "", false
	}
	return "set -e\necho \">> Memasang " + args + "...\"\n" + inti + "\necho \">> Selesai.\"\n", true
}

// perintahManual adalah bentuk pendek yang enak dibaca dan disalin, untuk
// ditampilkan saat poinhost sendiri tidak bisa memasangnya.
func perintahManual(pm string, paket []string) string {
	if len(paket) == 0 {
		return ""
	}
	args := strings.Join(paket, " ")
	switch pm {
	case "apt":
		return "apt install " + args
	case "dnf":
		return "dnf install " + args
	case "yum":
		return "yum install " + args
	case "zypper":
		return "zypper install " + args
	case "pacman":
		return "pacman -S " + args
	case "apk":
		return "apk add " + args
	}
	return ""
}

// namaServer memilih sebutan yang paling berarti bagi pemilik server:
// PRETTY_NAME kalau ada ("Rocky Linux 8.10"), ID sebagai cadangan, dan
// kalau /etc/os-release pun tidak terbaca, sebut itu apa adanya alih-alih
// mencetak "unknown".
func namaServer(distroName, distroID string) string {
	if n := strings.TrimSpace(distroName); n != "" && n != "unknown" {
		return n
	}
	if id := strings.TrimSpace(distroID); id != "" && id != "unknown" {
		return id
	}
	return "server ini"
}

const blokirButuhRoot = `memasang paket butuh akses root — aktifkan "User ini punya akses sudo" di Edit Server, atau gunakan user root`

// rootAccess adalah konteks eksekusi untuk perintah yang harus jalan
// sebagai ROOT.
//
// Sengaja terpisah dari fileAccess, yang arahnya justru berlawanan: ia
// TURUN ke user lain lewat `sudo -u` supaya operasi berkas memakai hak user
// itu. Memasang paket tidak pernah dilakukan "sebagai" user yang dipilih di
// dropdown panel — yang dipasang menyangkut seluruh server.
type rootAccess struct {
	serverID     string
	sshUser      string
	sudoPassword string
}

// wrap membungkus skrip, naik ke root lewat sudo kalau perlu.
//
// `sh -c`, bukan `bash -c`: skrip di file ini seluruhnya POSIX, dan Alpine
// tidak memasang bash secara bawaan.
//
// `sudo -n` saat tidak ada password tersimpan, supaya kegagalannya cepat
// dan jelas — `sudo` polos akan MENGGANTUNG menunggu prompt password di
// TTY yang tidak ada pada sesi exec SSH.
func (a *rootAccess) wrap(script string) string {
	inner := "sh -c " + shellQuote(script)
	if a.sshUser == "root" {
		return inner
	}
	if a.sudoPassword != "" {
		return "echo " + shellQuote(a.sudoPassword) + " | sudo -S -p '' " + inner
	}
	return "sudo -n " + inner
}

func (s *Service) resolveRoot(serverID string) (*rootAccess, error) {
	srv, err := s.servers.Get(serverID)
	if err != nil {
		return nil, err
	}
	if srv.Username != "root" && !srv.UseSudo {
		return nil, errors.New(blokirButuhRoot)
	}
	access := &rootAccess{serverID: serverID, sshUser: srv.Username}
	if srv.Username != "root" && srv.AuthType == "password" {
		// Error diabaikan dengan sengaja: tanpa password tersimpan,
		// wrap jatuh ke `sudo -n` (jalur NOPASSWD), bukan gagal di sini.
		pw, _ := s.servers.SudoPassword(serverID)
		access.sudoPassword = pw
	}
	return access, nil
}

// ArchiveTool memeriksa apakah arsip ini bisa diekstrak di server, dan
// kalau tidak, apa yang kurang serta bisakah poinhost memasangnya.
func (s *Service) ArchiveTool(ctx context.Context, req ExtractRequest) (*ArchiveToolInfo, error) {
	archive, err := NormalizePath(req.ArchivePath)
	if err != nil {
		return nil, err
	}
	format, ok := archiveFormat(archive)
	if !ok {
		return nil, fmt.Errorf("format arsip tidak dikenali dari nama file: %s", BaseName(archive))
	}
	access, err := s.resolveAccess(ctx, req.ServerID, req.AsUser)
	if err != nil {
		return nil, err
	}

	// Diperiksa sebagai user EFEKTIF — yang sama yang nanti menjalankan
	// ekstraksi. PATH di balik `sudo -u` tidak selalu sama dengan PATH user
	// SSH, dan yang menentukan adalah PATH yang dipakai perintahnya nanti,
	// bukan PATH yang kebetulan dipakai untuk memeriksa.
	res, err := s.execAccess(ctx, access, alatDetectScript, 30*time.Second, false)
	if err != nil {
		return nil, err
	}

	distroID, distroName, pm, ada := parseAlatOutput(res.Stdout + "\n" + res.Stderr)
	ready, kurang := siapEkstrak(format, ada)
	if kurang == nil {
		// Slice nil jadi `null` di JSON, dan sisi React memanggil .join()
		// padanya. Dikosongkan di sini supaya tipe di frontend (string[])
		// benar-benar selalu benar, bukan benar selama kebetulan terisi.
		kurang = []string{}
	}

	info := &ArchiveToolInfo{
		Format:         format,
		Ready:          ready,
		Missing:        kurang,
		Packages:       paketUntukSemua(kurang),
		DistroID:       distroID,
		DistroName:     distroName,
		PackageManager: pm,
	}
	if ready {
		return info, nil
	}

	info.ManualCommand = perintahManual(pm, info.Packages)

	// Ditanyakan ke installPaketScript, bukan disimpulkan dari "ada package
	// manager". Keduanya tidak otomatis sama: deteksi bisa mengenali sebuah
	// package manager yang belum punya cabang skrip di sini, dan kalau
	// CanInstall dinyalakan atas dasar deteksi saja, tombol Pasang muncul
	// lalu gagal saat ditekan — persis kegagalan yang fitur ini ada untuk
	// dihindari.
	if _, bisa := installPaketScript(pm, info.Packages); !bisa {
		info.Blocker = "poinhost belum bisa memasang " + strings.Join(info.Packages, ", ") +
			" sendiri di " + namaServer(distroName, distroID) +
			" — pemasangannya lewat apt, dnf, yum, zypper, pacman, atau apk"
		return info, nil
	}
	if _, err := s.resolveRoot(req.ServerID); err != nil {
		info.Blocker = err.Error()
		return info, nil
	}
	info.CanInstall = true
	return info, nil
}

// StreamInstallArchiveTool memasang alat ekstraksi yang kurang, sambil
// mengalirkan keluarannya ke onLine.
//
// format hanya boleh "zip" atau "tar.gz". Nama paket yang dipasang
// ditentukan DI SINI lewat paketUntuk, bukan dikirim dari UI — supaya panel
// file tidak pernah bisa jadi jalan memasang paket sembarangan di server.
func (s *Service) StreamInstallArchiveTool(ctx context.Context, serverID, format string, onLine func(string) error) error {
	if onLine == nil {
		onLine = func(string) error { return nil }
	}
	if format != "zip" && format != "tar.gz" {
		return fmt.Errorf("format arsip tidak dikenal: %s", format)
	}
	access, err := s.resolveRoot(serverID)
	if err != nil {
		return err
	}

	_ = onLine("Mendeteksi sistem operasi server...")
	// Deteksi ini SEKALIGUS membuktikan jalur root-nya bekerja: skrip
	// berakhir dengan `exit 0`, jadi kode keluar bukan nol di sini hanya
	// bisa datang dari sudo. Itu penting untuk langkah berikutnya —
	// pkgfail.Periksa membaca KODE KELUAR, dan sudo yang menolak akan
	// terbaca olehnya sebagai "resolver mati" kalau tidak disaring di sini.
	res, err := s.executor.Exec(ctx, serverID, 30*time.Second, access.wrap(alatDetectScript), false)
	if err != nil {
		return mapSudoError(err)
	}
	distroID, distroName, pm, ada := parseAlatOutput(res.Stdout + "\n" + res.Stderr)
	_ = onLine("Distro terdeteksi: " + namaServer(distroName, distroID) + " (paket: " + pmLabel(pm) + ")")

	ready, kurang := siapEkstrak(format, ada)
	if ready {
		_ = onLine("Alat ekstraksi sudah tersedia — tidak ada yang perlu dipasang.")
		return nil
	}

	paket := paketUntukSemua(kurang)
	script, ok := installPaketScript(pm, paket)
	if !ok {
		if pm == "" {
			return fmt.Errorf(
				"tidak menemukan package manager yang dikenal di %s — poinhost memasang lewat apt, dnf, yum, zypper, pacman, atau apk",
				namaServer(distroName, distroID),
			)
		}
		return fmt.Errorf("%s belum didukung untuk memasang %s secara otomatis (package manager: %s)",
			namaServer(distroName, distroID), strings.Join(paket, ", "), pm)
	}

	if err := s.lockServer(ctx, serverID, onLine); err != nil {
		return err
	}
	defer s.mutex.Unlock(serverID)

	// Diperiksa SEBELUM "Memulai instalasi...", supaya layar tidak pernah
	// menjanjikan sesuatu yang langsung dibatalkan di baris berikutnya.
	if err := pkgfail.Periksa(pkgfail.HostRepo(pm, distroID), func(skrip string) (*sshpool.ExecResult, error) {
		return s.executor.Exec(ctx, serverID, pkgfail.BatasPeriksa, access.wrap(skrip), false)
	}); err != nil {
		return err
	}

	_ = onLine("Memulai instalasi...")
	return s.runInstallStream(ctx, access, script, onLine)
}

// lockServer menunggu giliran server ini, supaya pemasangan tidak berjalan
// bersamaan dengan operasi lain yang memakai manajer paket yang sama.
func (s *Service) lockServer(ctx context.Context, serverID string, onLine func(string) error) error {
	if s.mutex.TryLock(serverID) {
		return nil
	}
	_ = onLine("Server sedang dipakai operasi lain, menunggu antrian...")
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			if s.mutex.TryLock(serverID) {
				return nil
			}
		}
	}
}

func (s *Service) runInstallStream(ctx context.Context, access *rootAccess, script string, onLine func(string) error) error {
	const sentinel = "__poinhost_INSTALL_DONE__:"
	wrapped := "(\n" + script + "\n) 2>&1\necho \"" + sentinel + "$?\""
	exitCode := -1

	// Ekor keluaran disimpan untuk menebak SEBAB kalau pemasangannya gagal.
	// Dibatasi, bukan seluruhnya: `apt-get update` saja bisa mengeluarkan
	// ratusan baris, dan yang menjelaskan kegagalan selalu ada di dekat
	// akhir.
	const ekorMaks = 80
	var ekor []string

	streamErr := s.executor.ExecStreamPTY(ctx, access.serverID, access.wrap(wrapped), func(line string) error {
		if rest, ok := strings.CutPrefix(line, sentinel); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil {
				exitCode = n
			}
			return nil
		}
		ekor = append(ekor, line)
		if len(ekor) > ekorMaks {
			ekor = ekor[len(ekor)-ekorMaks:]
		}
		return onLine(line)
	})
	if streamErr != nil {
		return streamErr
	}
	if exitCode != 0 {
		return errors.New(pkgfail.Pesan(exitCode, ekor))
	}
	return nil
}

func pmLabel(pm string) string {
	if pm == "" {
		return "tidak diketahui"
	}
	return pm
}
