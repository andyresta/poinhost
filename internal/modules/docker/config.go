package docker

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var envKeyRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// volumeNameRE adalah aturan penamaan volume Docker: diawali huruf atau
// angka, selebihnya boleh huruf, angka, titik, garis bawah, atau strip.
// Garis miring TIDAK termasuk — itulah yang membedakannya dari path.
var volumeNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// isVolumeSource memvalidasi sisi HOST sebuah bind volume.
//
// Docker menerima dua bentuk di posisi ini: path host absolut (bind mount)
// dan nama named volume. Sebelumnya hanya bentuk pertama yang diterima,
// sehingga container yang memakai named volume — bentuk yang justru
// dianjurkan untuk data yang harus bertahan melewati daur hidup container —
// tidak bisa dibuat ulang sama sekali, padahal konfigurasinya sah.
//
// Bentuk ketiga, path relatif seperti `./data`, sengaja TIDAK diterima:
// artinya bergantung pada direktori kerja saat perintah dijalankan, dan
// direktori kerja sesi SSH di sini bukan sesuatu yang user lihat atau
// kendalikan, jadi hasilnya akan mengejutkan.
func isVolumeSource(host string) bool {
	if strings.HasPrefix(host, "/") {
		return true
	}
	return volumeNameRE.MatchString(host)
}

// Batas memory Docker (hard min ~6MB); 0 = unlimited.
const (
	minMemoryBytes int64 = 6 * 1024 * 1024
	maxMemoryBytes int64 = 256 * 1024 * 1024 * 1024 // 256 GiB
)

// inspectRaw representasi subset JSON docker inspect.
type inspectRaw struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Image      string            `json:"Image"`
		Env        []string          `json:"Env"`
		Cmd        []string          `json:"Cmd"`
		Entrypoint json.RawMessage   `json:"Entrypoint"`
		WorkingDir string            `json:"WorkingDir"`
		Labels     map[string]string `json:"Labels"`
		User       string            `json:"User"`
		Hostname   string            `json:"Hostname"`
	} `json:"Config"`
	HostConfig struct {
		PortBindings  map[string][]inspectPortBinding `json:"PortBindings"`
		Binds         []string                        `json:"Binds"`
		Memory        int64                           `json:"Memory"`
		RestartPolicy struct {
			Name              string `json:"Name"`
			MaximumRetryCount int    `json:"MaximumRetryCount"`
		} `json:"RestartPolicy"`
		NetworkMode string   `json:"NetworkMode"`
		Privileged  bool     `json:"Privileged"`
		ExtraHosts  []string `json:"ExtraHosts"`
	} `json:"HostConfig"`
	State struct {
		Status string `json:"Status"`
	} `json:"State"`
	// Mounts top-level docker inspect (bind + named volume).
	Mounts []inspectTopMount `json:"Mounts"`
	// NetworkSettings.Networks berisi alias DNS per-network (mis. docker-compose otomatis
	// alias service "db" ke nama service-nya) — tanpa ini, container lain di network yang
	// sama tidak bisa resolve nama itu lagi setelah container di-recreate.
	NetworkSettings struct {
		Networks map[string]struct {
			Aliases []string `json:"Aliases"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

// inspectTopMount entri Mounts pada output docker inspect.
type inspectTopMount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}

// inspectPortBinding binding port dari inspect.
type inspectPortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

// createTemplate field internal untuk membangun docker create (dari inspect + override UI).
type createTemplate struct {
	Name          string
	Image         string
	Env           []EnvVar
	Ports         []PortMapping
	Volumes       []VolumeMount
	MemoryBytes   int64
	RestartPolicy string
	NetworkMode   string
	Privileged    bool
	WorkingDir    string
	User          string
	Hostname      string
	Labels        map[string]string
	Entrypoint    []string
	HasEntrypoint bool // true jika Entrypoint diinspect ada (termasuk [])
	Cmd           []string
	// NetworkAliases: alias DNS container di NetworkMode-nya (mis. docker-compose ngasih
	// alias "db" ke service mysql) — diteruskan sebagai --network-alias saat create ulang.
	NetworkAliases []string
	// ExtraHosts entri --add-host (mis. "host.docker.internal:host-gateway").
	// Di Linux nama itu TIDAK ada kecuali dipasang begini, jadi menjatuhkannya
	// saat recreate membuat container yang tadinya jalan langsung kehilangan
	// jalur ke host dan masuk restart loop.
	ExtraHosts []string
}

// parseInspectJSON mem-parse output docker inspect (objek atau array 1 elemen).
func parseInspectJSON(raw string) (*inspectRaw, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, fmt.Errorf("inspect container kosong")
	}
	// Ambil JSON utama (abaikan noise di luar)
	startObj := strings.Index(s, "{")
	startArr := strings.Index(s, "[")
	if startArr >= 0 && (startObj < 0 || startArr < startObj) {
		// Array format docker inspect tanpa --format
		var arr []inspectRaw
		if err := json.Unmarshal([]byte(s[startArr:]), &arr); err != nil {
			return nil, fmt.Errorf("gagal parse inspect: %w", err)
		}
		if len(arr) == 0 {
			return nil, fmt.Errorf("inspect container kosong")
		}
		return &arr[0], nil
	}
	if startObj < 0 {
		return nil, fmt.Errorf("inspect container tidak valid")
	}
	end := strings.LastIndex(s, "}")
	if end <= startObj {
		return nil, fmt.Errorf("inspect container tidak valid")
	}
	var inv inspectRaw
	if err := json.Unmarshal([]byte(s[startObj:end+1]), &inv); err != nil {
		return nil, fmt.Errorf("gagal parse inspect: %w", err)
	}
	return &inv, nil
}

// parseEntrypoint menormalisasi field Entrypoint (null | string | []string).
func parseEntrypoint(raw json.RawMessage) (ep []string, present bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var asSlice []string
	if err := json.Unmarshal(raw, &asSlice); err == nil {
		return asSlice, true
	}
	var asStr string
	if err := json.Unmarshal(raw, &asStr); err == nil {
		if asStr == "" {
			return []string{}, true
		}
		return []string{asStr}, true
	}
	return nil, false
}

// networkAliasesFor mengambil alias DNS container di network `netMode` dari
// NetworkSettings.Networks — kuncinya nama network asli, kecuali saat NetworkMode "default"
// yang di NetworkSettings.Networks selalu tercatat sebagai "bridge". Alias 12-hex yang sama
// dengan short ID container difilter karena tidak berarti apa-apa di container baru.
func networkAliasesFor(inv *inspectRaw, netMode string) []string {
	if inv == nil || len(inv.NetworkSettings.Networks) == 0 {
		return nil
	}
	key := netMode
	if _, ok := inv.NetworkSettings.Networks[key]; !ok && key == "default" {
		key = "bridge"
	}
	net, ok := inv.NetworkSettings.Networks[key]
	if !ok {
		return nil
	}
	shortID := inv.ID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	out := make([]string, 0, len(net.Aliases))
	for _, a := range net.Aliases {
		a = strings.TrimSpace(a)
		if a == "" || a == shortID {
			continue
		}
		out = append(out, a)
	}
	return out
}

// inspectToDetail memetakan inspect ke respons UI + template create internal.
func inspectToDetail(inv *inspectRaw) (*ContainerInspectResponse, *createTemplate, error) {
	if inv == nil {
		return nil, nil, fmt.Errorf("inspect kosong")
	}
	name := strings.TrimPrefix(strings.TrimSpace(inv.Name), "/")
	image := strings.TrimSpace(inv.Config.Image)
	if image == "" {
		return nil, nil, fmt.Errorf("image container tidak ditemukan")
	}

	env := parseEnvList(inv.Config.Env)
	ports := parsePortBindings(inv.HostConfig.PortBindings)
	volumes := parseVolumes(inv.HostConfig.Binds, inv.Mounts)
	ep, hasEP := parseEntrypoint(inv.Config.Entrypoint)

	labels := inv.Config.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	// Bind-address mentah tidak bisa membedakan "public" dari "intranet"
	// (keduanya 0.0.0.0) — baca kembali pilihan scope dari label yang kita
	// tulis sendiri waktu recreate (lihat syncPortScopeLabels). Untuk
	// container yang belum pernah di-recreate lewat poinhost (tidak ada
	// label kita), masih tebak dari bind mentahnya: 127.0.0.1 jelas berarti
	// "localhost", selain itu default "public" (status sebelum fitur ini ada).
	for i := range ports {
		if raw, ok := labels[portScopeLabelKey(ports[i].HostPort, ports[i].Protocol)]; ok {
			scope, err := normalizePortScope(raw)
			if err != nil {
				scope = portScopePublic
			}
			ports[i].Scope = scope
			continue
		}
		if ports[i].HostIP == "127.0.0.1" {
			ports[i].Scope = portScopePrivate
		} else {
			ports[i].Scope = portScopePublic
		}
	}

	restart := strings.TrimSpace(inv.HostConfig.RestartPolicy.Name)
	if restart == "" || restart == "no" {
		restart = "no"
	}
	netMode := strings.TrimSpace(inv.HostConfig.NetworkMode)
	if netMode == "" {
		netMode = "default"
	}
	aliases := networkAliasesFor(inv, netMode)

	tpl := &createTemplate{
		Name:           name,
		Image:          image,
		Env:            env,
		Ports:          ports,
		Volumes:        volumes,
		MemoryBytes:    inv.HostConfig.Memory,
		RestartPolicy:  restart,
		NetworkMode:    netMode,
		Privileged:     inv.HostConfig.Privileged,
		WorkingDir:     strings.TrimSpace(inv.Config.WorkingDir),
		User:           strings.TrimSpace(inv.Config.User),
		Hostname:       strings.TrimSpace(inv.Config.Hostname),
		Labels:         labels,
		Entrypoint:     ep,
		HasEntrypoint:  hasEP,
		Cmd:            inv.Config.Cmd,
		NetworkAliases: aliases,
		ExtraHosts:     inv.HostConfig.ExtraHosts,
	}

	detail := &ContainerInspectResponse{
		ContainerID:   inv.ID,
		Name:          name,
		Image:         image,
		State:         strings.ToLower(strings.TrimSpace(inv.State.Status)),
		Env:           env,
		Ports:         ports,
		Volumes:       volumes,
		MemoryBytes:   inv.HostConfig.Memory,
		RestartPolicy: restart,
		NetworkMode:   netMode,
		Cmd:           inv.Config.Cmd,
		WorkingDir:    tpl.WorkingDir,
		ExtraHosts:    tpl.ExtraHosts,
	}
	return detail, tpl, nil
}

// parseEnvList memecah []"KEY=value" ke EnvVar (skip PATH noise tetap dikirim agar recreate akurat).
func parseEnvList(lines []string) []EnvVar {
	out := make([]EnvVar, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			key = line
			val = ""
		}
		out = append(out, EnvVar{Key: key, Value: val})
	}
	return out
}

// parsePortBindings memetakan HostConfig.PortBindings ke PortMapping.
func parsePortBindings(pb map[string][]inspectPortBinding) []PortMapping {
	if len(pb) == 0 {
		return []PortMapping{}
	}
	out := make([]PortMapping, 0)
	for contSpec, binds := range pb {
		// contSpec = "80/tcp"
		proto := "tcp"
		portStr := contSpec
		if i := strings.LastIndex(contSpec, "/"); i >= 0 {
			portStr = contSpec[:i]
			p := strings.ToLower(contSpec[i+1:])
			if p == "udp" || p == "tcp" {
				proto = p
			}
		}
		cport, err := strconv.Atoi(portStr)
		if err != nil || cport <= 0 {
			continue
		}
		if len(binds) == 0 {
			out = append(out, PortMapping{ContainerPort: cport, Protocol: proto})
			continue
		}
		for _, b := range binds {
			hport := 0
			if b.HostPort != "" {
				hport, _ = strconv.Atoi(b.HostPort)
			}
			out = append(out, PortMapping{
				HostPort:      hport,
				ContainerPort: cport,
				Protocol:      proto,
				HostIP:        b.HostIP,
			})
		}
	}
	return out
}

// parseVolumes menggabungkan Binds (format lawas) dan Mounts top-level
// (bind + named volume).
//
// Named volume SEBELUMNYA difilter habis di sini ("if !EqualFold(Type,
// bind) { continue }"), padahal itu justru cara yang dianjurkan Docker untuk
// data yang harus bertahan melewati daur hidup container — akibatnya
// ContainerInspectResponse.Volumes diam-diam kosong untuk container yang
// memakainya, dan recreate/migrasi jadi tidak pernah tahu data itu ada.
//
// Untuk mount bertipe "volume", sisi host memakai NAMA volume (m.Name),
// bukan m.Source — m.Source menunjuk direktori overlay internal Docker di
// host (mis. /var/lib/docker/volumes/xxx/_data), detail implementasi yang
// tidak portable antar server dan tidak sah sebagai argumen "docker create
// -v". Nama volume itulah yang sudah diterima sisi create (lihat
// isVolumeSource) sebagai bentuk host yang sah selain path absolut.
func parseVolumes(binds []string, mounts []inspectTopMount) []VolumeMount {
	seen := map[string]bool{}
	out := make([]VolumeMount, 0)

	add := func(host, cont string, ro bool) {
		host = strings.TrimSpace(host)
		cont = strings.TrimSpace(cont)
		if host == "" || cont == "" {
			return
		}
		key := host + "\x00" + cont
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, VolumeMount{HostPath: host, ContainerPath: cont, ReadOnly: ro})
	}

	for _, b := range binds {
		host, cont, ro := splitBind(b)
		add(host, cont, ro)
	}
	for _, m := range mounts {
		switch strings.ToLower(strings.TrimSpace(m.Type)) {
		case "bind":
			add(m.Source, m.Destination, !m.RW)
		case "volume":
			add(m.Name, m.Destination, !m.RW)
		}
	}
	return out
}

// IsNamedVolume membedakan sisi host VolumeMount: named volume (nama Docker
// volume) vs bind mount (path host absolut). Dipakai modul lain (mis. mesin
// migrasi Docker) yang perlu menangani keduanya secara berbeda — bind mount
// bisa di-tar langsung dari path host, named volume harus dibaca lewat
// container bantu karena lokasi filesystem-nya adalah detail internal
// Docker yang tidak boleh diasumsikan.
func IsNamedVolume(hostSide string) bool {
	hostSide = strings.TrimSpace(hostSide)
	return hostSide != "" && !strings.HasPrefix(hostSide, "/") && volumeNameRE.MatchString(hostSide)
}

// splitBind memecah string docker bind "host:container[:ro|rw]".
func splitBind(raw string) (host, cont string, ro bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	parts := strings.Split(raw, ":")
	if len(parts) < 2 {
		return "", "", false
	}
	if len(parts) == 2 {
		return parts[0], parts[1], false
	}
	last := strings.ToLower(parts[len(parts)-1])
	if last == "ro" || last == "rw" || last == "z" || last == "Z" {
		mode := last
		if len(parts) == 3 {
			return parts[0], parts[1], mode == "ro"
		}
		host = strings.Join(parts[:len(parts)-2], ":")
		cont = parts[len(parts)-2]
		return host, cont, mode == "ro"
	}
	host = parts[0]
	cont = strings.Join(parts[1:], ":")
	return host, cont, false
}

// validateRecreateOverrides memvalidasi payload UI recreate.
func validateRecreateOverrides(env []EnvVar, ports []PortMapping, volumes []VolumeMount, mem int64, extraHosts []string) error {
	for _, e := range env {
		k := strings.TrimSpace(e.Key)
		if k == "" {
			continue
		}
		if !envKeyRE.MatchString(k) {
			return fmt.Errorf("nama env %q tidak valid", k)
		}
		if strings.ContainsAny(e.Value, "\x00") {
			return fmt.Errorf("nilai env %q tidak valid", k)
		}
	}
	for _, p := range ports {
		proto := strings.ToLower(strings.TrimSpace(p.Protocol))
		if proto == "" {
			proto = "tcp"
		}
		if proto != "tcp" && proto != "udp" {
			return fmt.Errorf("protocol port tidak valid")
		}
		if p.ContainerPort < 1 || p.ContainerPort > 65535 {
			return fmt.Errorf("port container tidak valid")
		}
		if p.HostPort < 0 || p.HostPort > 65535 {
			return fmt.Errorf("port host tidak valid")
		}
		if p.HostPort == 0 {
			return fmt.Errorf("port host wajib 1–65535")
		}
		if _, err := normalizePortScope(p.Scope); err != nil {
			return err
		}
	}
	for _, v := range volumes {
		host := strings.TrimSpace(v.HostPath)
		cont := strings.TrimSpace(v.ContainerPath)
		if host == "" && cont == "" {
			continue
		}
		if !isVolumeSource(host) {
			return fmt.Errorf("sumber volume %q tidak valid: harus path absolut atau nama named volume", host)
		}
		if !strings.HasPrefix(cont, "/") {
			return fmt.Errorf("path container volume harus absolut")
		}
		if strings.Contains(host, "\x00") || strings.Contains(cont, "\x00") {
			return fmt.Errorf("path volume tidak valid")
		}
	}
	if mem < 0 {
		return fmt.Errorf("memory limit tidak valid")
	}
	if mem > 0 && mem < minMemoryBytes {
		return fmt.Errorf("memory limit minimal 6 MB")
	}
	if mem > maxMemoryBytes {
		return fmt.Errorf("memory limit terlalu besar")
	}
	if _, err := normalizeExtraHosts(extraHosts); err != nil {
		return err
	}
	return nil
}

// normalizeOverrides membersihkan list env/ports/volumes untuk create.
func normalizeOverrides(env []EnvVar, ports []PortMapping, volumes []VolumeMount) (outEnv []EnvVar, outPorts []PortMapping, outVols []VolumeMount) {
	for _, e := range env {
		k := strings.TrimSpace(e.Key)
		if k == "" {
			continue
		}
		outEnv = append(outEnv, EnvVar{Key: k, Value: e.Value})
	}
	for _, p := range ports {
		if p.ContainerPort <= 0 && p.HostPort <= 0 {
			continue
		}
		proto := strings.ToLower(strings.TrimSpace(p.Protocol))
		if proto == "" {
			proto = "tcp"
		}
		scope, err := normalizePortScope(p.Scope)
		if err != nil {
			scope = portScopePublic
		}
		outPorts = append(outPorts, PortMapping{
			HostPort:      p.HostPort,
			ContainerPort: p.ContainerPort,
			Protocol:      proto,
			HostIP:        hostIPForScope(scope),
			Scope:         scope,
		})
	}
	for _, v := range volumes {
		host := strings.TrimSpace(v.HostPath)
		cont := strings.TrimSpace(v.ContainerPath)
		if host == "" || cont == "" {
			continue
		}
		outVols = append(outVols, VolumeMount{HostPath: host, ContainerPath: cont, ReadOnly: v.ReadOnly})
	}
	return outEnv, outPorts, outVols
}

// applyOverrides ke template create dari payload UI.
func applyOverrides(tpl *createTemplate, env []EnvVar, ports []PortMapping, volumes []VolumeMount, mem int64, extraHosts []string) {
	e, p, v := normalizeOverrides(env, ports, volumes)
	tpl.Env = e
	tpl.Ports = p
	tpl.Volumes = v
	tpl.MemoryBytes = mem
	// Error di sini sudah ditolak validateRecreateOverrides sebelum sampai
	// ke titik ini, jadi hasilnya dipakai apa adanya.
	hosts, _ := normalizeExtraHosts(extraHosts)
	tpl.ExtraHosts = hosts
	// Docker sendiri tidak bisa membedakan "public" vs "intranet" dari
	// bind-address (keduanya 0.0.0.0) — simpan pilihan scope di label
	// supaya kebuka lagi RecreateContainerModal nanti menampilkan pilihan
	// yang sama, bukan diam-diam reset ke "public".
	tpl.Labels = syncPortScopeLabels(tpl.Labels, tpl.Ports)
}

// IsBuiltinNetworkMode true untuk network mode bawaan Docker yang selalu ada di server
// manapun (tidak perlu/tidak bisa di-"docker network create") — selain itu dianggap network
// custom (mis. buatan docker-compose) yang harus dipastikan ada dulu sebelum docker create.
func IsBuiltinNetworkMode(net string) bool {
	switch strings.ToLower(strings.TrimSpace(net)) {
	case "default", "bridge", "host", "none":
		return true
	}
	return strings.HasPrefix(net, "container:")
}

// buildCreateArgs menyusun argumen `docker create` (tanpa prefix perintah).
func buildCreateArgs(tpl *createTemplate) ([]string, error) {
	if tpl == nil {
		return nil, fmt.Errorf("template create kosong")
	}
	name := strings.TrimSpace(tpl.Name)
	if name == "" {
		return nil, fmt.Errorf("nama container wajib")
	}
	image := strings.TrimSpace(tpl.Image)
	if image == "" {
		return nil, fmt.Errorf("image container wajib")
	}

	cmd := append([]string{}, tpl.Cmd...)

	args := []string{"create", "--name", name}

	restart := strings.TrimSpace(tpl.RestartPolicy)
	if restart != "" && restart != "no" {
		args = append(args, "--restart", restart)
	}

	// --add-host harus ikut ditulis ulang. `host.docker.internal` di Linux
	// bukan nama yang ada dengan sendirinya: ia hanya berarti sesuatu kalau
	// container dibuat dengan entri host-gateway ini. Menjatuhkannya saat
	// recreate membuat container yang tadinya sehat kehilangan jalur ke
	// host — dan gagalnya baru terlihat sebagai restart loop setelahnya.
	for _, h := range tpl.ExtraHosts {
		h = strings.TrimSpace(h)
		if h == "" || !strings.Contains(h, ":") {
			continue
		}
		args = append(args, "--add-host", h)
	}

	net := strings.TrimSpace(tpl.NetworkMode)
	if net != "" && net != "default" {
		if strings.HasPrefix(net, "container:") {
			args = append(args, "--network", "bridge")
		} else {
			args = append(args, "--network", net)
			for _, alias := range tpl.NetworkAliases {
				args = append(args, "--network-alias", alias)
			}
		}
	}

	if tpl.Privileged {
		args = append(args, "--privileged")
	}

	if wd := strings.TrimSpace(tpl.WorkingDir); wd != "" {
		args = append(args, "--workdir", wd)
	}
	if user := strings.TrimSpace(tpl.User); user != "" {
		args = append(args, "--user", user)
	}
	if host := strings.TrimSpace(tpl.Hostname); host != "" {
		args = append(args, "--hostname", host)
	}

	for k, v := range tpl.Labels {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		args = append(args, "--label", k+"="+v)
	}

	for _, e := range tpl.Env {
		args = append(args, "-e", e.Key+"="+e.Value)
	}

	for _, p := range tpl.Ports {
		spec := fmt.Sprintf("%d:%d/%s", p.HostPort, p.ContainerPort, p.Protocol)
		if ip := strings.TrimSpace(p.HostIP); ip != "" && ip != "0.0.0.0" {
			spec = fmt.Sprintf("%s:%d:%d/%s", ip, p.HostPort, p.ContainerPort, p.Protocol)
		}
		args = append(args, "-p", spec)
	}

	for _, v := range tpl.Volumes {
		bind := v.HostPath + ":" + v.ContainerPath
		if v.ReadOnly {
			bind += ":ro"
		}
		args = append(args, "-v", bind)
	}

	if tpl.MemoryBytes > 0 {
		args = append(args, "--memory", strconv.FormatInt(tpl.MemoryBytes, 10))
	}

	if tpl.HasEntrypoint {
		if len(tpl.Entrypoint) == 0 {
			args = append(args, "--entrypoint", "")
		} else if len(tpl.Entrypoint) == 1 {
			args = append(args, "--entrypoint", tpl.Entrypoint[0])
		} else {
			args = append(args, "--entrypoint", tpl.Entrypoint[0])
			cmd = append(append([]string{}, tpl.Entrypoint[1:]...), cmd...)
		}
	}

	args = append(args, image)
	args = append(args, cmd...)
	return args, nil
}

// quoteCreateCommand membentuk shell command: docker create ... (setiap arg di-quote).
func quoteCreateCommand(args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, "docker")
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// extraHostNameRE membatasi sisi KIRI entri --add-host ke bentuk hostname
// yang wajar. Tanpa batas ini, string apa pun bisa menyelinap ke argumen
// docker dan gagalnya baru terlihat sebagai error create yang tidak jelas.
var extraHostNameRE = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)

// normalizeExtraHosts membersihkan dan memvalidasi daftar entri --add-host.
//
// Bentuknya "nama:target", dengan target berupa alamat IP atau kata kunci
// `host-gateway` (yang diterjemahkan Docker menjadi alamat host). Entri
// kosong dibuang diam-diam supaya baris kosong di form tidak jadi error,
// tapi entri yang terisi separuh dilaporkan — itu hampir selalu salah ketik,
// dan membuangnya tanpa bilang akan terasa seperti setelan yang "tidak mau
// tersimpan".
func normalizeExtraHosts(entries []string) ([]string, error) {
	out := make([]string, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, raw := range entries {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		name, target, ok := strings.Cut(raw, ":")
		name = strings.TrimSpace(name)
		target = strings.TrimSpace(target)
		if !ok || name == "" || target == "" {
			return nil, fmt.Errorf("entri host tambahan %q tidak valid: gunakan bentuk nama:target", raw)
		}
		if !extraHostNameRE.MatchString(name) {
			return nil, fmt.Errorf("nama host %q tidak valid", name)
		}
		if strings.ContainsAny(target, " \t") {
			return nil, fmt.Errorf("target host %q tidak valid", target)
		}
		entry := name + ":" + target
		if seen[entry] {
			continue
		}
		seen[entry] = true
		out = append(out, entry)
	}
	return out, nil
}
