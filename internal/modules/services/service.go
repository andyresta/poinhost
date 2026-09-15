// Package services mengelola unit systemd di server remote: melihat
// statusnya, start/stop/restart, dan mengatur autostart saat boot
// (enable/disable).
//
// Semua dijalankan lewat `systemctl` via SSH exec — tidak ada D-Bus/API
// khusus, konsisten dengan modul lain di poinhost (lihat modul docker yang
// juga memakai CLI, bukan socket/API). Init system selain systemd
// (SysV/OpenRC) SENGAJA tidak didukung: dideteksi lalu dilaporkan apa
// adanya ke UI, bukan dipaksa dengan perintah yang setengah jalan.
package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/servers"
)

// ServiceInfo satu unit systemd beserta statusnya.
type ServiceInfo struct {
	Name        string `json:"name"`        // tanpa sufiks ".service"
	Description string `json:"description"` // deskripsi dari unit file
	// ActiveState: "active" | "inactive" | "failed" | "activating" | dst —
	// APAKAH SEDANG BERJALAN sekarang.
	ActiveState string `json:"activeState"`
	// SubState: detail lebih spesifik ("running", "exited", "dead").
	SubState string `json:"subState"`
	// UnitFileState: "enabled" | "disabled" | "static" | "masked" | dst —
	// APAKAH IKUT JALAN SAAT BOOT. Ini dimensi yang BERBEDA dari
	// ActiveState: service bisa aktif sekarang tapi tidak autostart, atau
	// sebaliknya. Keduanya sengaja ditampilkan terpisah di UI.
	UnitFileState string `json:"unitFileState"`
	// Running/Enabled: turunan siap-pakai untuk UI supaya tidak perlu
	// menafsirkan string state di frontend.
	Running bool `json:"running"`
	Enabled bool `json:"enabled"`
	// CanEnable false untuk unit "static"/"masked" — systemd sendiri yang
	// menolak enable/disable-nya, jadi tombolnya dinonaktifkan di UI
	// daripada memunculkan error setelah diklik.
	CanEnable bool `json:"canEnable"`
}

// ListResponse hasil ListServices.
type ListResponse struct {
	Services []ServiceInfo `json:"services"`
	// SystemdAvailable false berarti server ini tidak memakai systemd —
	// seluruh fitur modul ini tidak berlaku di sana.
	SystemdAvailable bool `json:"systemdAvailable"`
}

// Service mengorkestrasi operasi systemd di server remote via SSH.
type Service struct {
	servers  *servers.Service
	executor *sshpool.Executor
	mutex    *sshpool.ServerMutexRegistry
}

// NewService membuat service modul services baru.
func NewService(serversSvc *servers.Service, executor *sshpool.Executor, mutex *sshpool.ServerMutexRegistry) *Service {
	return &Service{servers: serversSvc, executor: executor, mutex: mutex}
}

func (s *Service) run(access *serviceAccess, script string, timeout time.Duration) (*sshpool.ExecResult, error) {
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// destructive=true → tanpa retry: perintah systemctl mengubah state, dan
	// mengulanginya otomatis saat timeout justru bisa menjalankan aksi dua
	// kali tanpa sepengetahuan user.
	res, err := s.executor.Exec(ctx, access.serverID, timeout, access.wrap(script), true)
	if err != nil {
		return res, mapServiceError(err)
	}
	return res, nil
}

// listScript mengambil daftar unit + status dalam SATU round-trip.
//
// `list-units` saja tidak cukup: unit yang ter-install tapi tidak pernah
// dijalankan tidak muncul di sana, padahal justru itu yang ingin di-enable.
// Maka dipakai `list-unit-files` (semua unit yang ADA beserta state
// autostart-nya) lalu di-join dengan `list-units` (yang sedang jalan).
// Pemisahnya byte 0x1F (unit separator) supaya tidak bentrok dengan
// deskripsi unit yang bebas mengandung spasi maupun tab. Ditulis sebagai
// escape OKTAL "\037", bukan heksadesimal "\x1f": oktal itu POSIX awk,
// sedangkan \x cuma ekstensi — dan awk bawaan Debian/Ubuntu adalah mawk,
// bukan gawk.
//
// Variabel penampung kolom SubState dinamai "st", BUKAN "sub": `sub` itu
// nama fungsi bawaan awk, memakainya sebagai variabel = syntax error.
const listScript = `set +e
if ! command -v systemctl >/dev/null 2>&1; then echo "__NO_SYSTEMD__"; exit 0; fi
echo "__UNIT_FILES__"
systemctl list-unit-files --type=service --no-legend --no-pager --plain 2>/dev/null | awk '{print $1 "\037" $2}'
echo "__UNITS__"
systemctl list-units --type=service --all --no-legend --no-pager --plain 2>/dev/null \
  | awk '{name=$1; active=$3; st=$4; $1=$2=$3=$4=""; sub(/^[ \t]+/,""); print name "\037" active "\037" st "\037" $0}'
exit 0`

// ListServices mengembalikan seluruh service systemd di server.
func (s *Service) ListServices(serverID string) (*ListResponse, error) {
	if strings.TrimSpace(serverID) == "" {
		return nil, fmt.Errorf("id server wajib diisi")
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	res, err := s.run(access, listScript, 25*time.Second)
	if err != nil {
		return nil, err
	}
	return parseList(res.Stdout), nil
}

// parseList dipisah dari ListServices supaya bisa diuji tanpa SSH sama
// sekali (lihat service_test.go) — format keluaran systemctl inilah bagian
// yang paling gampang salah baca.
func parseList(stdout string) *ListResponse {
	out := &ListResponse{Services: []ServiceInfo{}, SystemdAvailable: true}
	if strings.Contains(stdout, "__NO_SYSTEMD__") {
		out.SystemdAvailable = false
		return out
	}

	type entry struct {
		fileState   string
		active, sub string
		description string
		seenInUnits bool
	}
	byName := map[string]*entry{}
	order := []string{}

	get := func(name string) *entry {
		if e, ok := byName[name]; ok {
			return e
		}
		e := &entry{}
		byName[name] = e
		order = append(order, name)
		return e
	}

	section := ""
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case "__UNIT_FILES__", "__UNITS__":
			section = trimmed
			continue
		}
		if trimmed == "" {
			continue
		}
		parts := strings.Split(trimmed, "\x1f")
		name := strings.TrimSpace(parts[0])
		// Unit template (mis. "getty@.service") tidak bisa dioperasikan
		// langsung — yang bisa cuma instance-nya ("getty@tty1.service").
		if name == "" || !strings.HasSuffix(name, ".service") || strings.Contains(name, "@.") {
			continue
		}
		switch section {
		case "__UNIT_FILES__":
			if len(parts) >= 2 {
				get(name).fileState = strings.TrimSpace(parts[1])
			}
		case "__UNITS__":
			if len(parts) >= 4 {
				e := get(name)
				e.active = strings.TrimSpace(parts[1])
				e.sub = strings.TrimSpace(parts[2])
				e.description = strings.TrimSpace(parts[3])
				e.seenInUnits = true
			}
		}
	}

	for _, name := range order {
		e := byName[name]
		active := e.active
		if !e.seenInUnits {
			// Ada unit file-nya tapi tidak ter-load sama sekali: bukan
			// "failed", cuma tidak berjalan.
			active = "inactive"
		}
		out.Services = append(out.Services, ServiceInfo{
			Name:          strings.TrimSuffix(name, ".service"),
			Description:   e.description,
			ActiveState:   active,
			SubState:      e.sub,
			UnitFileState: e.fileState,
			Running:       active == "active" || active == "activating" || active == "reloading",
			Enabled:       e.fileState == "enabled" || e.fileState == "enabled-runtime",
			CanEnable:     canToggleEnable(e.fileState),
		})
	}
	return out
}

// canToggleEnable: systemd menolak enable/disable untuk unit static (tidak
// punya bagian [Install]), masked, atau generated. Dicek di sini supaya UI
// bisa menonaktifkan tombolnya lebih dulu, bukan memunculkan error sesudah
// diklik.
func canToggleEnable(fileState string) bool {
	switch fileState {
	case "enabled", "enabled-runtime", "disabled":
		return true
	}
	return false
}

var validActions = map[string]bool{
	"start": true, "stop": true, "restart": true,
	"enable": true, "disable": true,
}

// ServiceAction menjalankan satu aksi systemctl pada satu service.
//
// enable/disable mengubah perilaku SAAT BOOT dan TIDAK menyentuh state
// sekarang (begitu pula sebaliknya) — ini memang perilaku systemd, dan
// sengaja tidak "dibantu" dengan --now supaya satu tombol tidak melakukan
// dua hal yang tidak diminta.
func (s *Service) ServiceAction(serverID, name, action string) (*ServiceInfo, error) {
	if !validActions[action] {
		return nil, fmt.Errorf("aksi tidak dikenal: %s", action)
	}
	unit, err := normalizeUnitName(name)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	script := "set -e\nsystemctl " + action + " " + shellQuote(unit)
	if _, err := s.run(access, script, 30*time.Second); err != nil {
		return nil, err
	}
	return s.statusOf(access, unit)
}

// GetService membaca status satu service (dipakai untuk refresh satu baris
// tanpa menarik ulang seluruh daftar).
func (s *Service) GetService(serverID, name string) (*ServiceInfo, error) {
	unit, err := normalizeUnitName(name)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	return s.statusOf(access, unit)
}

const statusScriptTmpl = `set +e
UNIT=%s
echo "ACTIVE=$(systemctl is-active "$UNIT" 2>/dev/null)"
echo "ENABLED=$(systemctl is-enabled "$UNIT" 2>/dev/null)"
echo "SUB=$(systemctl show "$UNIT" -p SubState --value 2>/dev/null)"
echo "DESC=$(systemctl show "$UNIT" -p Description --value 2>/dev/null)"
exit 0`

func (s *Service) statusOf(access *serviceAccess, unit string) (*ServiceInfo, error) {
	res, err := s.run(access, fmt.Sprintf(statusScriptTmpl, shellQuote(unit)), 20*time.Second)
	if err != nil {
		return nil, err
	}
	info := &ServiceInfo{Name: strings.TrimSuffix(unit, ".service")}
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "ACTIVE="):
			info.ActiveState = strings.TrimPrefix(line, "ACTIVE=")
		case strings.HasPrefix(line, "ENABLED="):
			info.UnitFileState = strings.TrimPrefix(line, "ENABLED=")
		case strings.HasPrefix(line, "SUB="):
			info.SubState = strings.TrimPrefix(line, "SUB=")
		case strings.HasPrefix(line, "DESC="):
			info.Description = strings.TrimPrefix(line, "DESC=")
		}
	}
	info.Running = info.ActiveState == "active" || info.ActiveState == "activating" || info.ActiveState == "reloading"
	info.Enabled = info.UnitFileState == "enabled" || info.UnitFileState == "enabled-runtime"
	info.CanEnable = canToggleEnable(info.UnitFileState)
	return info, nil
}
