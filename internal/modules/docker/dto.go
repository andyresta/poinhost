package docker

// ContainerRequest permintaan aksi pada satu container.
type ContainerRequest struct {
	ServerID    string `json:"serverId"`
	ContainerID string `json:"containerId"`
}

// LogsRequest permintaan baca log container.
type LogsRequest struct {
	ServerID    string `json:"serverId"`
	ContainerID string `json:"containerId"`
	Lines       int    `json:"lines"`
}

// ContainerInfo satu container Docker di server remote.
type ContainerInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	Status  string `json:"status"`
	State   string `json:"state"`
	Ports   string `json:"ports"`
	Created string `json:"created"`
}

// ListResponse daftar container Docker.
type ListResponse struct {
	Containers []ContainerInfo `json:"containers"`
	DockerOK   bool            `json:"dockerOk"`
	Version    string          `json:"version,omitempty"`
}

// LogsResponse isi log container.
type LogsResponse struct {
	ContainerID string `json:"containerId"`
	Name        string `json:"name,omitempty"`
	Content     string `json:"content"`
}

// StatsResponse statistik resource container.
type StatsResponse struct {
	ContainerID string `json:"containerId"`
	Name        string `json:"name"`
	CPUPerc     string `json:"cpuPerc"`
	MemUsage    string `json:"memUsage"`
	MemPerc     string `json:"memPerc"`
	NetIO       string `json:"netIO"`
	BlockIO     string `json:"blockIO"`
	PIDs        string `json:"pids"`
}

// EngineStatus status instalasi Docker engine di server (untuk wizard).
type EngineStatus struct {
	Installed      bool   `json:"installed"`
	Version        string `json:"version,omitempty"`
	Active         bool   `json:"active"`
	Enabled        bool   `json:"enabled"`
	DistroID       string `json:"distroId,omitempty"`
	DistroName     string `json:"distroName,omitempty"`
	PackageManager string `json:"packageManager,omitempty"`
	CanInstall     bool   `json:"canInstall"`
}

// EnvVar pasangan key/value environment container.
type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PortMapping pemetaan port host → container.
type PortMapping struct {
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"` // tcp | udp
	// HostIP dihitung otomatis dari Scope (lihat portscope.go), bukan
	// input manual — 127.0.0.1 untuk "localhost", kosong (0.0.0.0) untuk
	// "public"/"intranet" (intranet dibatasi lewat firewall, bukan bind).
	HostIP string `json:"hostIp,omitempty"`
	// Scope siapa yang boleh menjangkau port ini dari luar server:
	// "public" (internet, default), "intranet" (jaringan lokal/LAN saja,
	// lewat firewall discoped ke rentang IP privat), "localhost" (cuma
	// dari server itu sendiri, bind 127.0.0.1 — tidak ada mekanisme
	// firewall yang bisa dilewati).
	Scope string `json:"scope,omitempty"`
}

// VolumeMount bind mount host path → container path.
type VolumeMount struct {
	HostPath      string `json:"hostPath"`
	ContainerPath string `json:"containerPath"`
	ReadOnly      bool   `json:"readOnly"`
}

// ContainerInspectResponse konfigurasi container untuk editor UI.
type ContainerInspectResponse struct {
	ContainerID   string        `json:"containerId"`
	Name          string        `json:"name"`
	Image         string        `json:"image"`
	State         string        `json:"state"`
	Env           []EnvVar      `json:"env"`
	Ports         []PortMapping `json:"ports"`
	Volumes       []VolumeMount `json:"volumes"`
	MemoryBytes   int64         `json:"memoryBytes"` // 0 = unlimited
	RestartPolicy string        `json:"restartPolicy,omitempty"`
	NetworkMode   string        `json:"networkMode,omitempty"`
	Cmd           []string      `json:"cmd,omitempty"`
	WorkingDir    string        `json:"workingDir,omitempty"`
	// ExtraHosts entri --add-host ("nama:target"). Ditampilkan dan bisa
	// disunting supaya entri seperti host.docker.internal:host-gateway bisa
	// dipasang kembali dari panel — tanpa ini, container yang kehilangan
	// entrinya hanya bisa diperbaiki lewat shell atau file compose.
	ExtraHosts []string `json:"extraHosts"`
}

// RecreateContainerRequest apply konfigurasi baru lewat recreate container.
type RecreateContainerRequest struct {
	ServerID    string        `json:"serverId"`
	ContainerID string        `json:"containerId"`
	Env         []EnvVar      `json:"env"`
	Ports       []PortMapping `json:"ports"`
	Volumes     []VolumeMount `json:"volumes"`
	MemoryBytes int64         `json:"memoryBytes"` // 0 = unlimited
	ExtraHosts  []string      `json:"extraHosts"`
}

// RecreateContainerResponse hasil recreate (info container baru).
type RecreateContainerResponse struct {
	Container ContainerInfo `json:"container"`
	// Warning: mis. port di-set "intranet" tapi tidak ada firewall aktif
	// terdeteksi di server ini — jadi pembatasannya belum benar-benar
	// berlaku (lihat portscope.go).
	Warning string `json:"warning,omitempty"`
}

// NetworkInfo satu network Docker di server remote.
type NetworkInfo struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Driver         string            `json:"driver"`
	Scope          string            `json:"scope"`
	Subnet         string            `json:"subnet,omitempty"`
	Gateway        string            `json:"gateway,omitempty"`
	Internal       bool              `json:"internal"`
	Attachable     bool              `json:"attachable"`
	ContainerCount int               `json:"containerCount"`
	Labels         map[string]string `json:"labels,omitempty"`
	Created        string            `json:"created,omitempty"`
	// Builtin: true untuk network bawaan Docker (default/bridge/host/none) — tidak bisa dihapus.
	Builtin bool `json:"builtin"`
}

// NetworkListResponse daftar network Docker di server.
type NetworkListResponse struct {
	Networks []NetworkInfo `json:"networks"`
}

// NetworkCreateRequest membuat network Docker baru.
type NetworkCreateRequest struct {
	ServerID   string `json:"serverId"`
	Name       string `json:"name"`
	Driver     string `json:"driver,omitempty"`
	Subnet     string `json:"subnet,omitempty"`
	Gateway    string `json:"gateway,omitempty"`
	Internal   bool   `json:"internal"`
	Attachable bool   `json:"attachable"`
}

// NetworkRequest permintaan aksi pada satu network (mis. hapus).
type NetworkRequest struct {
	ServerID string `json:"serverId"`
	Name     string `json:"name"`
}
