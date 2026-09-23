package docker

import (
	"fmt"
	"strings"
	"time"
)

// File ini adalah seam khusus modul migrasi Docker antar server
// (internal/modules/dockerxfer): tempat satu-satunya di luar paket ini yang
// boleh menyentuh bentuk internal container (createTemplate, buildCreateArgs,
// resolveAccess/wrap) tanpa menduplikasinya. dockerxfer TIDAK reimplementasi
// logika "docker inspect -> docker create" sendiri — semuanya lewat sini,
// supaya editor recreate satu-server dan migrasi antar-server selalu
// menghasilkan container yang sama untuk konfigurasi yang sama.

// MigrationMount satu titip mount (bind atau named volume) yang harus
// disalin sebagai bagian migrasi container.
type MigrationMount struct {
	// HostSide adalah path host absolut (bind mount) ATAU nama named volume
	// — bentuk yang sama seperti VolumeMount.HostPath. Named dibedakan lewat
	// IsNamedVolume(HostSide), bukan field terpisah yang bisa tidak sinkron.
	HostSide      string
	ContainerPath string
	ReadOnly      bool
}

// Named melaporkan apakah mount ini named volume (true) atau bind mount host (false).
func (m MigrationMount) Named() bool { return IsNamedVolume(m.HostSide) }

// MigrationBlueprint representasi lengkap satu container untuk dibuat ulang
// di SERVER LAIN — supersetnya createTemplate (field internal), diekspor
// dalam bentuk stabil supaya modul migrasi (paket dockerxfer) tidak perlu
// tahu apa pun soal struct docker inspect mentah.
type MigrationBlueprint struct {
	Name          string
	Image         string
	Env           []EnvVar
	Ports         []PortMapping
	Mounts        []MigrationMount
	MemoryBytes   int64
	RestartPolicy string
	Privileged    bool
	WorkingDir    string
	User          string
	Hostname      string
	Labels        map[string]string
	Entrypoint    []string
	HasEntrypoint bool
	Cmd           []string
	ExtraHosts    []string

	// PrimaryNetwork adalah NetworkMode asli container (mis. "bridge",
	// "host", atau nama network custom) beserta alias DNS-nya di network
	// itu — dipetakan ke --network/--network-alias saat create.
	PrimaryNetwork string
	PrimaryAliases []string

	// ExtraNetworks: network TAMBAHAN yang diikuti container di luar
	// PrimaryNetwork (docker create hanya bisa menyambung SATU network,
	// sisanya harus `docker network connect` sesudahnya). Sebelumnya
	// (di homepoin) semua network selain yang utama diam-diam hilang saat
	// migrasi — container yang tersambung ke >1 network custom kehilangan
	// konektivitas ke salah satunya tanpa peringatan apa pun.
	ExtraNetworks []MigrationNetworkAttachment
}

// MigrationNetworkAttachment satu network tambahan yang harus disambungkan
// via `docker network connect` setelah container dibuat.
type MigrationNetworkAttachment struct {
	Name    string
	Aliases []string
}

// attachedNetworkNames pada inspectRaw mengembalikan SEMUA nama network yang
// diikuti container (kunci NetworkSettings.Networks), beserta aliasnya —
// dipakai untuk menyusun PrimaryNetwork+ExtraNetworks pada blueprint.
func attachedNetworks(inv *inspectRaw) map[string][]string {
	out := make(map[string][]string, len(inv.NetworkSettings.Networks))
	shortID := inv.ID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	for name, net := range inv.NetworkSettings.Networks {
		aliases := make([]string, 0, len(net.Aliases))
		for _, a := range net.Aliases {
			a = strings.TrimSpace(a)
			if a == "" || a == shortID {
				continue
			}
			aliases = append(aliases, a)
		}
		out[name] = aliases
	}
	return out
}

// InspectMigrationBlueprint membaca konfigurasi lengkap satu container untuk
// dimigrasikan ke server lain.
func (s *Service) InspectMigrationBlueprint(serverID, rawID string) (*MigrationBlueprint, error) {
	cid, err := normalizeContainerID(rawID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(serverID) == "" {
		return nil, fmt.Errorf("id server wajib diisi")
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf("docker inspect --format %s %s", shellQuote("{{json .}}"), shellQuote(cid))
	res, err := s.runDocker(access, cmd, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = "gagal inspect container"
		}
		return nil, mapDockerError(fmt.Errorf("%s", msg))
	}
	inv, err := parseInspectJSON(res.Stdout)
	if err != nil {
		return nil, err
	}
	_, tpl, err := inspectToDetail(inv)
	if err != nil {
		return nil, err
	}

	mounts := make([]MigrationMount, 0, len(tpl.Volumes))
	for _, v := range tpl.Volumes {
		mounts = append(mounts, MigrationMount{HostSide: v.HostPath, ContainerPath: v.ContainerPath, ReadOnly: v.ReadOnly})
	}

	all := attachedNetworks(inv)
	primaryKey := tpl.NetworkMode
	if _, ok := all[primaryKey]; !ok && primaryKey == "default" {
		primaryKey = "bridge"
	}
	extra := make([]MigrationNetworkAttachment, 0, len(all))
	for name, aliases := range all {
		if name == primaryKey {
			continue
		}
		extra = append(extra, MigrationNetworkAttachment{Name: name, Aliases: aliases})
	}

	return &MigrationBlueprint{
		Name:           tpl.Name,
		Image:          tpl.Image,
		Env:            tpl.Env,
		Ports:          tpl.Ports,
		Mounts:         mounts,
		MemoryBytes:    tpl.MemoryBytes,
		RestartPolicy:  tpl.RestartPolicy,
		Privileged:     tpl.Privileged,
		WorkingDir:     tpl.WorkingDir,
		User:           tpl.User,
		Hostname:       tpl.Hostname,
		Labels:         tpl.Labels,
		Entrypoint:     tpl.Entrypoint,
		HasEntrypoint:  tpl.HasEntrypoint,
		Cmd:            tpl.Cmd,
		ExtraHosts:     tpl.ExtraHosts,
		PrimaryNetwork: tpl.NetworkMode,
		PrimaryAliases: tpl.NetworkAliases,
		ExtraNetworks:  extra,
	}, nil
}

// ContainerNameExists memeriksa apakah nama container sudah dipakai di
// server (dicek sebelum create, supaya kolisi nama gagal cepat dengan pesan
// yang jelas, bukan di tengah proses create).
func (s *Service) ContainerNameExists(serverID, name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, fmt.Errorf("nama container kosong")
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return false, err
	}
	cmd := fmt.Sprintf("docker inspect --format '{{.Id}}' %s >/dev/null 2>&1 && echo yes || echo no", shellQuote(name))
	res, err := s.runDocker(access, cmd, 15*time.Second)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(res.Stdout) == "yes", nil
}

// WrapCommand membungkus skrip docker-adjacent sembarang (mis. tar lewat
// container bantu untuk named volume, save/load image, docker volume
// create) lewat lapisan sudo yang sama dipakai semua perintah docker modul
// ini. Modul migrasi (dockerxfer) memakainya supaya tidak menduplikasi
// logika sudo/akses sendiri untuk perintah yang tidak lewat runDocker biasa
// (mis. dijalankan di sesi SSH khusus untuk streaming, bukan lewat
// executor bersama).
func (s *Service) WrapCommand(serverID, script string) (string, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return "", err
	}
	return access.wrap(script), nil
}

// blueprintToTemplate mengonversi MigrationBlueprint kembali ke createTemplate
// internal supaya bisa dipakai buildCreateArgs — satu-satunya fungsi yang
// tahu cara menyusun `docker create`.
func blueprintToTemplate(bp MigrationBlueprint) *createTemplate {
	volumes := make([]VolumeMount, 0, len(bp.Mounts))
	for _, m := range bp.Mounts {
		volumes = append(volumes, VolumeMount{HostPath: m.HostSide, ContainerPath: m.ContainerPath, ReadOnly: m.ReadOnly})
	}
	return &createTemplate{
		Name:           bp.Name,
		Image:          bp.Image,
		Env:            bp.Env,
		Ports:          bp.Ports,
		Volumes:        volumes,
		MemoryBytes:    bp.MemoryBytes,
		RestartPolicy:  bp.RestartPolicy,
		NetworkMode:    bp.PrimaryNetwork,
		Privileged:     bp.Privileged,
		WorkingDir:     bp.WorkingDir,
		User:           bp.User,
		Hostname:       bp.Hostname,
		Labels:         bp.Labels,
		Entrypoint:     bp.Entrypoint,
		HasEntrypoint:  bp.HasEntrypoint,
		Cmd:            bp.Cmd,
		NetworkAliases: bp.PrimaryAliases,
		ExtraHosts:     bp.ExtraHosts,
	}
}

// CreateFromBlueprint membuat DAN menyalakan container di server tujuan dari
// blueprint hasil InspectMigrationBlueprint di server asal.
//
// Beda dari RecreateContainer (yang mengasumsikan container LAMA ada di
// server yang SAMA dan melakukan stop→rm→create→start): ini server yang
// BERBEDA, jadi tidak ada yang perlu di-stop/rm — murni create→start, plus
// menyambungkan setiap network tambahan (ExtraNetworks) yang tidak bisa
// ikut satu perintah `docker create`.
//
// Named volume pada blueprint diasumsikan SUDAH diisi datanya oleh
// dockerxfer sebelum method ini dipanggil (lihat EnsureNamedVolume) — di
// sini hanya membuat containernya, tidak menyentuh data volume.
func (s *Service) CreateFromBlueprint(serverID string, bp MigrationBlueprint) (*ContainerInfo, error) {
	if strings.TrimSpace(serverID) == "" {
		return nil, fmt.Errorf("id server tujuan wajib diisi")
	}
	tpl := blueprintToTemplate(bp)
	createArgs, err := buildCreateArgs(tpl)
	if err != nil {
		return nil, err
	}
	createCmd := quoteCreateCommand(createArgs)

	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	var script strings.Builder
	script.WriteString("set -e\n")
	// Network custom (non-bawaan) pada sisi tujuan belum tentu ada —
	// dibuat kalau belum, idempotent kalau sudah (sama seperti
	// EnsureNamedVolume di bawah).
	if net := strings.TrimSpace(bp.PrimaryNetwork); net != "" && !IsBuiltinNetworkMode(net) {
		script.WriteString("docker network create " + shellQuote(net) + " >/dev/null 2>&1 || true\n")
	}
	for _, extra := range bp.ExtraNetworks {
		name := strings.TrimSpace(extra.Name)
		if name == "" || IsBuiltinNetworkMode(name) {
			continue
		}
		script.WriteString("docker network create " + shellQuote(name) + " >/dev/null 2>&1 || true\n")
	}
	script.WriteString(createCmd + "\n")
	for _, extra := range bp.ExtraNetworks {
		name := strings.TrimSpace(extra.Name)
		if name == "" || IsBuiltinNetworkMode(name) {
			continue
		}
		connect := []string{"network", "connect"}
		for _, alias := range extra.Aliases {
			connect = append(connect, "--alias", alias)
		}
		connect = append(connect, name, tpl.Name)
		script.WriteString(quoteCreateCommand(connect) + "\n")
	}
	script.WriteString("docker start " + shellQuote(tpl.Name) + "\n")
	script.WriteString("echo \"__poinhost_MIGRATE_OK__$(docker inspect --format '{{.Id}}' " + shellQuote(tpl.Name) + " 2>/dev/null)\"\n")

	res, err := s.runDocker(access, script.String(), 180*time.Second)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = "gagal membuat container di server tujuan"
		}
		return nil, mapDockerError(fmt.Errorf("%s", msg))
	}

	newID := tpl.Name
	for _, line := range strings.Split(res.Stdout+"\n"+res.Stderr, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "__poinhost_MIGRATE_OK__") {
			if id := strings.TrimPrefix(trim, "__poinhost_MIGRATE_OK__"); id != "" {
				newID = id
			}
		}
	}

	s.invalidateListCache(serverID)

	info := ContainerInfo{ID: newID, Name: tpl.Name, Image: tpl.Image, State: "running"}
	return &info, nil
}

// EnsureNamedVolume membuat named volume di server (docker volume create),
// idempotent kalau sudah ada. Dipanggil sebelum menyalurkan data volume ke
// dalamnya, dan sebelum CreateFromBlueprint agar volume yang direferensikan
// container sudah tersedia.
func (s *Service) EnsureNamedVolume(serverID, name string) error {
	name = strings.TrimSpace(name)
	if !IsNamedVolume(name) {
		return fmt.Errorf("nama volume %q tidak valid", name)
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}
	cmd := "docker volume create " + shellQuote(name)
	res, err := s.runDocker(access, cmd, 20*time.Second)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = "gagal membuat volume " + name
		}
		return mapDockerError(fmt.Errorf("%s", msg))
	}
	return nil
}

// ImageExistsOnServer memeriksa apakah image sudah ada di server (dipakai
// untuk melewati pull/stream image yang tidak perlu — server tujuan boleh
// jadi sudah punya image yang sama, mis. dari migrasi container lain
// dengan image yang sama sebelumnya).
func (s *Service) ImageExistsOnServer(serverID, image string) (bool, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return false, fmt.Errorf("nama image kosong")
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return false, err
	}
	cmd := fmt.Sprintf("docker image inspect %s >/dev/null 2>&1 && echo yes || echo no", shellQuote(image))
	res, err := s.runDocker(access, cmd, 20*time.Second)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(res.Stdout) == "yes", nil
}

// ImageHasRegistryDigest melaporkan apakah image punya RepoDigests — tanda
// image itu ditarik dari registry (bukan hasil `docker build` lokal). Kalau
// ya, sisi tujuan cukup `docker pull` image yang sama alih-alih menyalurkan
// isi image lewat save/load — jauh lebih cepat kalau registrynya bisa
// dijangkau dari server tujuan.
func (s *Service) ImageHasRegistryDigest(serverID, image string) (bool, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return false, fmt.Errorf("nama image kosong")
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return false, err
	}
	cmd := fmt.Sprintf("docker image inspect --format %s %s", shellQuote("{{json .RepoDigests}}"), shellQuote(image))
	res, err := s.runDocker(access, cmd, 20*time.Second)
	if err != nil {
		return false, err
	}
	if res.ExitCode != 0 {
		return false, nil
	}
	out := strings.TrimSpace(res.Stdout)
	return out != "" && out != "null" && out != "[]", nil
}

// PullImage menarik image dari registry di server tujuan.
func (s *Service) PullImage(serverID, image string) error {
	image = strings.TrimSpace(image)
	if image == "" {
		return fmt.Errorf("nama image kosong")
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}
	cmd := "docker pull " + shellQuote(image)
	res, err := s.runDocker(access, cmd, 10*time.Minute)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = "docker pull gagal"
		}
		return mapDockerError(fmt.Errorf("%s", msg))
	}
	return nil
}

// ContainerIsRunning memeriksa status container tujuan pasca-migrasi
// (verifikasi ringan — lihat dockerxfer.verifyContainer).
func (s *Service) ContainerIsRunning(serverID, name string) (bool, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return false, err
	}
	cmd := fmt.Sprintf("docker inspect --format %s %s", shellQuote("{{.State.Running}}"), shellQuote(name))
	res, err := s.runDocker(access, cmd, 15*time.Second)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(res.Stdout) == "true", nil
}

// Label compose yang dipakai migrasi untuk menemukan direktori proyek.
const (
	LabelComposeProject     = "com.docker.compose.project"
	LabelComposeService     = "com.docker.compose.service"
	LabelComposeWorkingDir  = "com.docker.compose.project.working_dir"
	LabelComposeConfigFiles = "com.docker.compose.project.config_files"
)

// ComposeMigrationInfo ringkasan proyek compose satu container, dipakai
// panel migrasi SEBELUM migrasi dimulai: apakah opsi "salin direktori
// proyek" bisa dipakai, seberapa besar direktorinya, dan container lain
// dalam proyek yang sama yang TIDAK ikut termigrasi (migrasi Docker
// bekerja per container, bukan per proyek).
type ComposeMigrationInfo struct {
	Managed    bool   `json:"managed"`
	Project    string `json:"project"`
	Service    string `json:"service"`
	WorkingDir string `json:"workingDir"`
	// WorkingDirExists false kalau label menunjuk direktori yang sudah
	// tidak ada di server asal (proyek dipindah/dihapus setelah `up`).
	WorkingDirExists bool  `json:"workingDirExists"`
	WorkingDirBytes  int64 `json:"workingDirBytes"`
	// ConfigFiles compose file yang dipakai saat `up` (dipisah koma oleh
	// compose sendiri) — bisa berada di luar WorkingDir kalau `-f` dipakai.
	ConfigFiles []string `json:"configFiles"`
	// Siblings nama container LAIN dalam proyek compose yang sama.
	Siblings []string `json:"siblings"`
}

func composeMigrationInfoScript(containerID string) string {
	id := shellQuote(containerID)
	return `set +e
lbl() { docker inspect ` + id + ` -f "{{index .Config.Labels \"$1\"}}" 2>/dev/null; }
PROJ=$(lbl ` + LabelComposeProject + `)
if [ -z "$PROJ" ] || [ "$PROJ" = "<no value>" ]; then echo "MANAGED=0"; exit 0; fi
echo "MANAGED=1"
echo "PROJECT=$PROJ"
echo "SERVICE=$(lbl ` + LabelComposeService + `)"
WORKDIR=$(lbl ` + LabelComposeWorkingDir + `)
echo "WORKDIR=$WORKDIR"
echo "FILES=$(lbl ` + LabelComposeConfigFiles + `)"
if [ -n "$WORKDIR" ] && [ -d "$WORKDIR" ]; then
  echo "EXISTS=1"
  echo "BYTES=$(du -sb "$WORKDIR" 2>/dev/null | cut -f1)"
fi
SELF=$(docker inspect ` + id + ` -f '{{.Name}}' 2>/dev/null | sed 's#^/##')
docker ps -a --filter "label=` + LabelComposeProject + `=$PROJ" --format '{{.Names}}' 2>/dev/null | while read -r n; do
  [ -n "$n" ] && [ "$n" != "$SELF" ] && echo "SIBLING=$n"
done
exit 0`
}

// ComposeMigrationInfo membaca info proyek compose satu container.
func (s *Service) ComposeMigrationInfo(serverID, rawID string) (*ComposeMigrationInfo, error) {
	cid, err := normalizeContainerID(rawID)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}
	res, err := s.runDocker(access, composeMigrationInfoScript(cid), 60*time.Second)
	if err != nil {
		return nil, err
	}
	return parseComposeMigrationInfo(res.Stdout), nil
}

// parseComposeMigrationInfo dipisah supaya bisa diuji tanpa SSH.
func parseComposeMigrationInfo(stdout string) *ComposeMigrationInfo {
	info := &ComposeMigrationInfo{ConfigFiles: []string{}, Siblings: []string{}}
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
			for _, f := range strings.Split(val, ",") {
				if f = strings.TrimSpace(f); f != "" {
					info.ConfigFiles = append(info.ConfigFiles, f)
				}
			}
		case "EXISTS":
			info.WorkingDirExists = val == "1"
		case "BYTES":
			fmt.Sscan(val, &info.WorkingDirBytes)
		case "SIBLING":
			info.Siblings = append(info.Siblings, val)
		}
	}
	return info
}
