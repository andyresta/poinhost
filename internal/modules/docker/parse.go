package docker

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// normalizeContainerID memvalidasi id atau nama container Docker.
func normalizeContainerID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", errors.New("id container wajib diisi")
	}
	if len(id) > 128 {
		return "", errors.New("id container tidak valid")
	}
	for _, ch := range id {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '_' || ch == '.' {
			continue
		}
		return "", errors.New("id container tidak valid")
	}
	return id, nil
}

// normalizeNetworkName memvalidasi nama network Docker.
func normalizeNetworkName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", errors.New("nama network wajib diisi")
	}
	if len(name) > 128 {
		return "", errors.New("nama network tidak valid")
	}
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '_' || ch == '.' {
			continue
		}
		return "", errors.New("nama network tidak valid (huruf, angka, - _ . saja)")
	}
	return name, nil
}

// shellQuote membungkus argumen shell dengan kutip tunggal aman.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// parseContainerList mem-parsing output docker ps format TSV (7 kolom).
// Format: ID \t Names \t Image \t Status \t State \t Ports \t CreatedAt
// Sengaja tidak memakai {{json .}} karena sangat lambat (Labels besar).
func parseContainerList(stdout string) []ContainerInfo {
	items := make([]ContainerInfo, 0)
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "__poinhost_") {
			continue
		}
		if strings.HasPrefix(line, "{") {
			var row dockerPsLine
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				continue
			}
			items = append(items, containerFromPs(row.ID, row.Names, row.Image, row.Status, row.State, row.Ports, row.CreatedAt))
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 7 {
			continue
		}
		items = append(items, containerFromPs(parts[0], parts[1], parts[2], parts[3], parts[4], parts[5], parts[6]))
	}
	return items
}

// dockerPsLine representasi JSON legacy satu baris docker ps.
type dockerPsLine struct {
	ID        string `json:"ID"`
	Names     string `json:"Names"`
	Image     string `json:"Image"`
	Status    string `json:"Status"`
	State     string `json:"State"`
	Ports     string `json:"Ports"`
	CreatedAt string `json:"CreatedAt"`
}

// containerFromPs membentuk ContainerInfo dari field docker ps.
func containerFromPs(id, names, image, status, state, ports, created string) ContainerInfo {
	name := strings.TrimPrefix(strings.TrimSpace(names), "/")
	if idx := strings.Index(name, ","); idx >= 0 {
		name = name[:idx]
	}
	return ContainerInfo{
		ID:      strings.TrimSpace(id),
		Name:    name,
		Image:   strings.TrimSpace(image),
		Status:  strings.TrimSpace(status),
		State:   strings.ToLower(strings.TrimSpace(state)),
		Ports:   strings.TrimSpace(ports),
		Created: strings.TrimSpace(created),
	}
}

// dockerStatsLine representasi JSON docker stats --no-stream.
type dockerStatsLine struct {
	Container string `json:"Container"`
	Name      string `json:"Name"`
	CPUPerc   string `json:"CPUPerc"`
	MemUsage  string `json:"MemUsage"`
	MemPerc   string `json:"MemPerc"`
	NetIO     string `json:"NetIO"`
	BlockIO   string `json:"BlockIO"`
	PIDs      string `json:"PIDs"`
}

// extractJSONObject mengambil objek JSON pertama dari baris output.
// docker stats streaming sering diawali ANSI clear screen (\x1b[2J\x1b[H).
func extractJSONObject(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) {
				ch := s[i]
				i++
				if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
					break
				}
			}
			i--
			continue
		}
		b.WriteByte(s[i])
	}
	s = strings.TrimSpace(b.String())
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	end := strings.LastIndex(s, "}")
	if end <= start {
		return ""
	}
	return s[start : end+1]
}

// parseContainerStats mem-parsing satu baris JSON docker stats.
func parseContainerStats(stdout string) (*StatsResponse, error) {
	line := strings.TrimSpace(stdout)
	if line == "" {
		return nil, errors.New("statistik container kosong")
	}
	for _, part := range strings.Split(line, "\n") {
		jsonPart := extractJSONObject(part)
		if jsonPart == "" {
			continue
		}
		var row dockerStatsLine
		if err := json.Unmarshal([]byte(jsonPart), &row); err != nil {
			continue
		}
		return &StatsResponse{
			ContainerID: row.Container,
			Name:        strings.TrimPrefix(row.Name, "/"),
			CPUPerc:     row.CPUPerc,
			MemUsage:    row.MemUsage,
			MemPerc:     row.MemPerc,
			NetIO:       row.NetIO,
			BlockIO:     row.BlockIO,
			PIDs:        row.PIDs,
		}, nil
	}
	return nil, errors.New("gagal mem-parsing statistik container")
}

// dockerNetworkInspectLine subset JSON satu baris `docker network inspect --format '{{json .}}'`.
type dockerNetworkInspectLine struct {
	Id         string `json:"Id"`
	Name       string `json:"Name"`
	Driver     string `json:"Driver"`
	Scope      string `json:"Scope"`
	Internal   bool   `json:"Internal"`
	Attachable bool   `json:"Attachable"`
	Created    string `json:"Created"`
	IPAM       struct {
		Config []struct {
			Subnet  string `json:"Subnet"`
			Gateway string `json:"Gateway"`
		} `json:"Config"`
	} `json:"IPAM"`
	Containers map[string]json.RawMessage `json:"Containers"`
	Labels     map[string]string          `json:"Labels"`
}

// parseNetworkList mem-parsing output `docker network inspect --format '{{json .}}'` untuk
// banyak network sekaligus — satu objek JSON per baris (perilaku standar docker inspect saat
// diberi lebih dari satu target).
func parseNetworkList(stdout string) []NetworkInfo {
	items := make([]NetworkInfo, 0)
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var row dockerNetworkInspectLine
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		subnet, gateway := "", ""
		if len(row.IPAM.Config) > 0 {
			subnet = row.IPAM.Config[0].Subnet
			gateway = row.IPAM.Config[0].Gateway
		}
		items = append(items, NetworkInfo{
			ID:             row.Id,
			Name:           row.Name,
			Driver:         row.Driver,
			Scope:          row.Scope,
			Subnet:         subnet,
			Gateway:        gateway,
			Internal:       row.Internal,
			Attachable:     row.Attachable,
			ContainerCount: len(row.Containers),
			Labels:         row.Labels,
			Created:        row.Created,
			Builtin:        IsBuiltinNetworkMode(row.Name),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}
