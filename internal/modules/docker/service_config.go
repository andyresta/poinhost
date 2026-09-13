package docker

import (
	"fmt"
	"strings"
	"time"
)

// InspectContainer membaca konfigurasi container lewat docker inspect.
func (s *Service) InspectContainer(serverID, rawID string) (*ContainerInspectResponse, error) {
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

	// --format object tunggal (lebih ringan dari array default).
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
		inv, err = parseInspectJSON(res.Stdout + "\n" + res.Stderr)
		if err != nil {
			return nil, err
		}
	}
	detail, _, err := inspectToDetail(inv)
	if err != nil {
		return nil, err
	}
	if detail.ContainerID == "" {
		detail.ContainerID = cid
	}
	return detail, nil
}

// RecreateContainer menerapkan env/port/volume/memory lewat stop → rm → create → start.
func (s *Service) RecreateContainer(req RecreateContainerRequest) (*RecreateContainerResponse, error) {
	serverID := strings.TrimSpace(req.ServerID)
	cid, err := normalizeContainerID(req.ContainerID)
	if err != nil {
		return nil, err
	}
	if serverID == "" {
		return nil, fmt.Errorf("id server wajib diisi")
	}
	if err := validateRecreateOverrides(req.Env, req.Ports, req.Volumes, req.MemoryBytes); err != nil {
		return nil, err
	}

	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	// 1) Inspect sumber
	inspectCmd := fmt.Sprintf("docker inspect --format %s %s", shellQuote("{{json .}}"), shellQuote(cid))
	res, err := s.runDocker(access, inspectCmd, 30*time.Second)
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

	// 2) Override UI
	applyOverrides(tpl, req.Env, req.Ports, req.Volumes, req.MemoryBytes)

	// 3) Build create
	createArgs, err := buildCreateArgs(tpl)
	if err != nil {
		return nil, err
	}
	createCmd := quoteCreateCommand(createArgs)

	// 4) stop → rm → create → start (nama container dipertahankan), lalu
	// terapkan firewall untuk scope port ("intranet"/"public") — satu
	// round-trip SSH yang sama, bukan panggilan terpisah.
	script := "set -e\n" +
		"CID=" + shellQuote(cid) + "\n" +
		"NAME=" + shellQuote(tpl.Name) + "\n" +
		"docker stop \"$CID\" 2>/dev/null || true\n" +
		"docker rm \"$CID\"\n" +
		createCmd + "\n" +
		"docker start \"$NAME\"\n" +
		"echo \"__poinhost_RECREATE_OK__$(docker inspect --format '{{.Id}}' \"$NAME\" 2>/dev/null)\"\n" +
		portScopeApplyScript(tpl.Ports)

	runRes, err := s.runDocker(access, script, 120*time.Second)
	if err != nil {
		return nil, err
	}
	if runRes.ExitCode != 0 {
		msg := strings.TrimSpace(runRes.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(runRes.Stdout)
		}
		if msg == "" {
			msg = "gagal recreate container"
		}
		return nil, mapDockerError(fmt.Errorf("%s", msg))
	}

	newID := ""
	for _, line := range strings.Split(runRes.Stdout+"\n"+runRes.Stderr, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "__poinhost_RECREATE_OK__") {
			newID = strings.TrimPrefix(trim, "__poinhost_RECREATE_OK__")
		}
	}
	if newID == "" {
		newID = tpl.Name
	}

	s.invalidateListCache(serverID)

	info := ContainerInfo{
		ID:    newID,
		Name:  tpl.Name,
		Image: tpl.Image,
		State: "running",
	}
	if len(tpl.Ports) > 0 {
		parts := make([]string, 0, len(tpl.Ports))
		for _, p := range tpl.Ports {
			parts = append(parts, fmt.Sprintf("%d:%d/%s", p.HostPort, p.ContainerPort, p.Protocol))
		}
		info.Ports = strings.Join(parts, ", ")
	}

	firewallDetected := "none"
	for _, line := range strings.Split(runRes.Stdout+"\n"+runRes.Stderr, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "FIREWALL="); ok {
			firewallDetected = v
		}
	}

	return &RecreateContainerResponse{Container: info, Warning: portScopeWarning(tpl.Ports, firewallDetected)}, nil
}
