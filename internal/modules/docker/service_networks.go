package docker

import (
	"fmt"
	"strings"
	"time"
)

// ListNetworks mengembalikan daftar network Docker di server.
func (s *Service) ListNetworks(serverID string) (*NetworkListResponse, error) {
	if strings.TrimSpace(serverID) == "" {
		return nil, fmt.Errorf("id server wajib diisi")
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	// docker network ls tidak punya kolom subnet/gateway di --format, jadi langsung inspect
	// semua network sekaligus (jumlahnya kecil per server, beda dengan container yang bisa
	// banyak — {{json .}} di sini tidak masalah, tidak seperti docker ps).
	script := "set +e\n" +
		"IDS=$(docker network ls -q)\n" +
		"if [ -n \"$IDS\" ]; then docker network inspect --format '{{json .}}' $IDS; fi\n" +
		"EC=$?\n" +
		"echo \"__poinhost_DOCKER_EC__${EC}\"\n" +
		"exit 0"

	res, err := s.runDocker(access, script, 20*time.Second)
	if err != nil {
		return nil, err
	}

	out := res.Stdout + "\n" + res.Stderr
	exitCode := res.ExitCode
	var body strings.Builder
	for _, line := range strings.Split(out, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "__poinhost_DOCKER_EC__") {
			fmt.Sscanf(strings.TrimPrefix(trim, "__poinhost_DOCKER_EC__"), "%d", &exitCode)
			continue
		}
		body.WriteString(line)
		body.WriteByte('\n')
	}

	items := parseNetworkList(body.String())
	if exitCode != 0 && len(items) == 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = "gagal memuat daftar network"
		}
		return nil, mapDockerError(fmt.Errorf("%s", msg))
	}

	return &NetworkListResponse{Networks: items}, nil
}

// CreateNetwork membuat network Docker baru di server.
func (s *Service) CreateNetwork(req NetworkCreateRequest) (*NetworkInfo, error) {
	name, err := normalizeNetworkName(req.Name)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.ServerID) == "" {
		return nil, fmt.Errorf("id server wajib diisi")
	}
	if IsBuiltinNetworkMode(name) {
		return nil, fmt.Errorf("nama %q dipakai network bawaan Docker — pakai nama lain", name)
	}
	driver := strings.TrimSpace(req.Driver)
	if driver == "" {
		driver = "bridge"
	}
	subnet := strings.TrimSpace(req.Subnet)
	gateway := strings.TrimSpace(req.Gateway)
	if gateway != "" && subnet == "" {
		return nil, fmt.Errorf("gateway butuh subnet diisi juga")
	}

	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return nil, err
	}

	args := []string{"network", "create", "--driver", driver}
	if subnet != "" {
		args = append(args, "--subnet", subnet)
	}
	if gateway != "" {
		args = append(args, "--gateway", gateway)
	}
	if req.Internal {
		args = append(args, "--internal")
	}
	if req.Attachable {
		args = append(args, "--attachable")
	}
	args = append(args, name)
	cmd := quoteCreateCommand(args)

	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)

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
			msg = "docker network create gagal"
		}
		return nil, mapDockerError(fmt.Errorf("%s", msg))
	}

	inspectCmd := fmt.Sprintf("docker network inspect --format '{{json .}}' %s", shellQuote(name))
	inspectRes, err := s.runDocker(access, inspectCmd, 15*time.Second)
	if err == nil && inspectRes.ExitCode == 0 {
		if items := parseNetworkList(inspectRes.Stdout); len(items) > 0 {
			return &items[0], nil
		}
	}
	// Network sudah berhasil dibuat — inspect gagal bukan alasan untuk melaporkan error,
	// cukup kembalikan info minimal dari request.
	return &NetworkInfo{
		Name:       name,
		Driver:     driver,
		Subnet:     subnet,
		Gateway:    gateway,
		Internal:   req.Internal,
		Attachable: req.Attachable,
	}, nil
}

// RemoveNetwork menghapus network Docker (docker network rm).
func (s *Service) RemoveNetwork(serverID, rawName string) error {
	name, err := normalizeNetworkName(rawName)
	if err != nil {
		return err
	}
	if strings.TrimSpace(serverID) == "" {
		return fmt.Errorf("id server wajib diisi")
	}
	if IsBuiltinNetworkMode(name) {
		return fmt.Errorf("network bawaan Docker %q tidak bisa dihapus", name)
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	cmd := "docker network rm " + shellQuote(name)
	res, err := s.runDocker(access, cmd, 30*time.Second)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = "docker network rm gagal"
		}
		return mapDockerError(fmt.Errorf("%s", msg))
	}
	return nil
}
