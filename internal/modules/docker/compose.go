package docker

// Deteksi "config drift" compose dan penerapan ulangnya.
//
// Masalah yang ditangani di sini nyata dan mahal: container yang BERJALAN bisa
// memakai konfigurasi lama sementara docker-compose.yml di disk sudah berubah.
// Tidak ada tanda apa pun dari luar — container terlihat "Up", port-nya
// ter-publish, tapi setelan yang baru (extra_hosts, environment, volume) tidak
// pernah ikut. Gejalanya baru muncul jauh belakangan sebagai kegagalan yang
// sulit dilacak.
//
// `docker compose up -d` TIDAK cukup untuk memperbaikinya: kalau container-nya
// sudah ada dan dianggap up-to-date, perintah itu hanya men-start-nya. Yang
// benar-benar menerapkan ulang konfigurasi adalah `--force-recreate`.

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ComposeInfo status compose satu container.
type ComposeInfo struct {
	// Managed false untuk container yang dibuat langsung dengan `docker run`
	// — tidak ada compose file yang bisa diterapkan ulang.
	Managed     bool   `json:"managed"`
	Project     string `json:"project"`
	Service     string `json:"service"`
	WorkingDir  string `json:"workingDir"`
	ConfigFiles string `json:"configFiles"`
	// Drift true kalau compose SENDIRI menyatakan container ini perlu dibuat
	// ulang.
	Drift bool `json:"drift"`
	// Verdict putusan mentah dari compose ("Recreate", "Running", dst),
	// ditampilkan apa adanya supaya kesimpulannya bisa ditelusuri.
	Verdict string `json:"verdict,omitempty"`
	// Unknown true kalau drift TIDAK bisa dipastikan (mis. compose file tidak
	// terbaca, atau versi compose tidak mendukung `config --hash`). Sengaja
	// dibedakan dari "tidak ada drift": melaporkan aman padahal tidak tahu
	// justru menyembunyikan masalah yang sedang dicari.
	Unknown bool   `json:"unknown"`
	Message string `json:"message,omitempty"`
}

// composeInfoScript mengumpulkan label compose container lalu menanyakan
// kepada compose sendiri apakah service ini perlu dibuat ulang.
//
// Dipakai `compose --dry-run up`, BUKAN perbandingan `config --hash` dengan
// label `com.docker.compose.config-hash`. Cara hash itu sempat dicoba dan
// TERBUKTI salah: container yang baru saja dibuat ulang dari compose file
// yang sama tetap menghasilkan dua hash berbeda, karena `config --hash` dan
// hash yang ditulis saat `up` tidak dihitung lewat jalur normalisasi yang
// sama. Indikator drift yang keliru lebih buruk daripada tidak ada.
//
// --dry-run adalah mode no-op resmi compose: ia menjalankan seluruh logika
// perbandingannya lalu melaporkan tindakan yang AKAN diambil, tanpa menyentuh
// apa pun.
func composeInfoScript(containerID string) string {
	id := shellQuote(containerID)
	return `set +e
PROJ=$(docker inspect ` + id + ` -f '{{index .Config.Labels "com.docker.compose.project"}}' 2>/dev/null)
if [ -z "$PROJ" ] || [ "$PROJ" = "<no value>" ]; then echo "MANAGED=0"; exit 0; fi
echo "MANAGED=1"
echo "PROJECT=$PROJ"
SVC=$(docker inspect ` + id + ` -f '{{index .Config.Labels "com.docker.compose.service"}}' 2>/dev/null)
WORKDIR=$(docker inspect ` + id + ` -f '{{index .Config.Labels "com.docker.compose.project.working_dir"}}' 2>/dev/null)
FILES=$(docker inspect ` + id + ` -f '{{index .Config.Labels "com.docker.compose.project.config_files"}}' 2>/dev/null)
echo "SERVICE=$SVC"
echo "WORKDIR=$WORKDIR"
echo "FILES=$FILES"
if [ -z "$WORKDIR" ] || [ ! -d "$WORKDIR" ]; then echo "ERR=direktori proyek compose tidak ditemukan di server"; exit 0; fi
cd "$WORKDIR" || { echo "ERR=tidak bisa masuk ke direktori proyek"; exit 0; }
OUT=$(docker compose --dry-run up -d --no-deps "$SVC" 2>&1)
if [ $? -ne 0 ]; then echo "ERR=$(echo "$OUT" | grep -v "^$" | head -1)"; exit 0; fi
echo "$OUT" | sed 's/^/DRY=/'
exit 0`
}

// ComposeStatus membaca status compose satu container.
func (s *Service) ComposeStatus(serverID, containerID string) (*ComposeInfo, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	res, err := s.runDocker(access, composeInfoScript(containerID), 40*time.Second)
	if err != nil {
		return nil, err
	}
	return parseComposeInfo(res.Stdout), nil
}

// parseComposeInfo dipisah supaya bisa diuji tanpa SSH.
func parseComposeInfo(stdout string) *ComposeInfo {
	info := &ComposeInfo{}
	var dry []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "MANAGED":
			info.Managed = val == "1"
		case "PROJECT":
			info.Project = val
		case "SERVICE":
			info.Service = val
		case "WORKDIR":
			info.WorkingDir = val
		case "FILES":
			info.ConfigFiles = val
		case "DRY":
			dry = append(dry, val)
		case "ERR":
			info.Message = val
		}
	}
	if !info.Managed {
		return info
	}
	// Tanpa keluaran dry-run, drift TIDAK bisa disimpulkan. Dilaporkan sebagai
	// "tidak diketahui", BUKAN "aman": melaporkan aman padahal tidak tahu
	// justru menyembunyikan masalah yang sedang dicari.
	if len(dry) == 0 {
		info.Unknown = true
		if info.Message == "" {
			info.Message = "docker compose tidak mendukung --dry-run (versi terlalu lama)"
		}
		return info
	}
	for _, line := range dry {
		low := strings.ToLower(line)
		switch {
		case strings.Contains(low, "recreate"):
			info.Drift = true
			info.Verdict = "Recreate"
		case strings.Contains(low, "create") && info.Verdict == "":
			info.Drift = true
			info.Verdict = "Create"
		case strings.Contains(low, "running") && info.Verdict == "":
			info.Verdict = "Running"
		case strings.Contains(low, "start") && info.Verdict == "":
			info.Verdict = "Start"
		}
	}
	if info.Verdict == "" {
		info.Unknown = true
		info.Message = "keluaran docker compose --dry-run tidak dikenali"
	}
	return info
}

// ApplyCompose membuat ulang container dari compose file yang ada sekarang.
//
// Memakai `--force-recreate` (bukan `up -d` biasa, yang hanya men-start
// container lama) dan `--no-deps` supaya HANYA service ini yang disentuh.
//
// `--remove-orphans` SENGAJA tidak dipakai, walau docker sendiri sering
// menyarankannya di peringatan: banyak proyek memakai nama direktori yang
// sama (mis. beberapa folder bernama "deploy"), sehingga compose menganggap
// container proyek lain sebagai orphan — dan flag itu akan MENGHAPUSNYA.
func (s *Service) ApplyCompose(serverID, containerID string) (*ComposeInfo, error) {
	info, err := s.ComposeStatus(serverID, containerID)
	if err != nil {
		return nil, err
	}
	if !info.Managed {
		return nil, fmt.Errorf("container ini tidak dikelola docker compose, jadi tidak ada konfigurasi yang bisa diterapkan ulang")
	}
	if info.WorkingDir == "" || info.Service == "" {
		return nil, fmt.Errorf("label compose container ini tidak lengkap, lokasi proyeknya tidak bisa dipastikan")
	}

	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	script := "set -e\ncd " + shellQuote(info.WorkingDir) +
		"\ndocker compose up -d --force-recreate --no-deps " + shellQuote(info.Service)

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := s.executor.Exec(ctx, access.serverID, 5*time.Minute, access.wrap(script), true); err != nil {
		return nil, err
	}
	// Status dibaca ULANG sesudahnya supaya hasilnya diverifikasi, bukan
	// diasumsikan berhasil karena perintahnya tidak error.
	return s.ComposeStatus(serverID, containerID)
}
