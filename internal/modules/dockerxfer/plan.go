package dockerxfer

import (
	"fmt"
	"path"
	"strings"

	"github.com/andyresta/poinhost/internal/modules/docker"
)

// workdirItemPrefix awalan label item "direktori proyek compose" — sejajar
// dengan "bind:"/"volume:"/"image:" supaya frontend memecahnya dengan cara
// yang sama.
const workdirItemPrefix = "workdir:"

// transferPlan daftar final yang akan disalin untuk satu job.
//
// Kenapa perlu rencana terpisah dari bp.Mounts: saat direktori proyek
// compose ikut disalin, bind mount RELATIF di compose file (mis.
// `./data:/app/data`) sudah di-resolve compose jadi path absolut DI DALAM
// direktori proyek itu. Menyalinnya lagi sebagai mount terpisah berarti
// men-tar data yang sama dua kali (dan menggandakan taksiran ukuran) — jadi
// mount yang tercakup workdir dikeluarkan dari daftar dan dicatat di
// Covered, bukan disalin ulang.
type transferPlan struct {
	// Workdir path absolut direktori proyek compose, kosong kalau opsi salin
	// workdir tidak dipakai.
	Workdir string
	// Mounts mount yang tetap disalin sendiri-sendiri (named volume + bind
	// mount di luar Workdir).
	Mounts []docker.MigrationMount
	// Covered bind mount yang sudah ikut di dalam Workdir.
	Covered []docker.MigrationMount
}

func workdirLabel(dir string) string { return workdirItemPrefix + dir }

// workdirMount membentuk Workdir sebagai bind mount supaya bisa lewat jalur
// tar/verifikasi yang sama persis dengan bind mount biasa.
func (p transferPlan) workdirMount() docker.MigrationMount {
	return docker.MigrationMount{HostSide: p.Workdir}
}

// copyItem satu unit yang di-tar dari asal ke tujuan, beserta kunci item
// progress-nya.
type copyItem struct {
	key   string
	mount docker.MigrationMount
}

// items urutan salin: workdir dulu (supaya bind mount di luar workdir yang
// kebetulan menunjuk subfolder lain tidak tertimpa belakangan), lalu mount.
func (p transferPlan) items() []copyItem {
	out := make([]copyItem, 0, len(p.Mounts)+1)
	if p.Workdir != "" {
		out = append(out, copyItem{key: workdirLabel(p.Workdir), mount: p.workdirMount()})
	}
	for _, m := range p.Mounts {
		out = append(out, copyItem{key: mountLabel(m), mount: m})
	}
	return out
}

// buildPlan menyusun rencana transfer dari blueprint.
func buildPlan(bp *docker.MigrationBlueprint, copyWorkdir bool) (transferPlan, error) {
	plan := transferPlan{Mounts: bp.Mounts}
	if !copyWorkdir {
		return plan, nil
	}

	raw := strings.TrimSpace(bp.Labels[docker.LabelComposeWorkingDir])
	if raw == "" {
		return plan, fmt.Errorf("container %q tidak dibuat lewat docker compose — tidak ada direktori proyek yang bisa disalin", bp.Name)
	}
	dir, err := cleanWorkdir(raw)
	if err != nil {
		return plan, err
	}
	plan.Workdir = dir

	plan.Mounts = make([]docker.MigrationMount, 0, len(bp.Mounts))
	for _, m := range bp.Mounts {
		if !m.Named() && pathWithin(dir, m.HostSide) {
			plan.Covered = append(plan.Covered, m)
			continue
		}
		plan.Mounts = append(plan.Mounts, m)
	}
	return plan, nil
}

// cleanWorkdir menolak path yang berbahaya untuk di-`tar x` di tujuan:
// harus absolut, dan bukan root atau direktori sistem tingkat atas — label
// compose bisa saja berisi "/" kalau `docker compose up` dijalankan dari
// root, dan menyalin seluruh filesystem jelas bukan maksud pengguna.
func cleanWorkdir(raw string) (string, error) {
	if !strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("direktori proyek compose %q bukan path absolut", raw)
	}
	dir := path.Clean(raw)
	switch dir {
	case "/", "/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib64", "/opt",
		"/proc", "/run", "/sbin", "/srv", "/sys", "/tmp", "/usr", "/var":
		return "", fmt.Errorf("direktori proyek compose %q terlalu luas untuk disalin — pindahkan proyek ke subfolder sendiri", dir)
	}
	return dir, nil
}

// pathWithin melaporkan apakah p sama dengan dir atau berada di bawahnya.
func pathWithin(dir, p string) bool {
	if !strings.HasPrefix(p, "/") {
		return false
	}
	p = path.Clean(p)
	return p == dir || strings.HasPrefix(p, dir+"/")
}

// coveredNote catatan untuk item workdir: bind mount mana yang sudah ikut
// tersalin di dalamnya.
func coveredNote(covered []docker.MigrationMount) string {
	if len(covered) == 0 {
		return ""
	}
	paths := make([]string, len(covered))
	for i, m := range covered {
		paths[i] = m.HostSide
	}
	return "sudah mencakup bind mount: " + strings.Join(paths, ", ")
}
