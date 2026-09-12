package docker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/servers"
)

// listCacheTTL adalah masa berlaku cache daftar container sebelum di-refresh via SSH.
const listCacheTTL = 8 * time.Second

// listCacheEntry satu entri cache daftar container per server.
type listCacheEntry struct {
	result    *ListResponse
	expiresAt time.Time
}

// Service mengorkestrasi operasi Docker di server remote via SSH (dijalankan
// lewat CLI `docker` via bash -c, bukan Docker API/socket — sama seperti
// homepoin, supaya tidak perlu expose socket Docker apapun ke luar server).
type Service struct {
	servers  *servers.Service
	executor *sshpool.Executor
	mutex    *sshpool.ServerMutexRegistry

	listMu    sync.Mutex
	listCache map[string]listCacheEntry
}

// NewService membuat service modul docker baru.
func NewService(serversSvc *servers.Service, executor *sshpool.Executor, mutex *sshpool.ServerMutexRegistry) *Service {
	return &Service{
		servers:   serversSvc,
		executor:  executor,
		mutex:     mutex,
		listCache: make(map[string]listCacheEntry),
	}
}

func (s *Service) getListCache(serverID string) (*ListResponse, bool) {
	s.listMu.Lock()
	defer s.listMu.Unlock()
	entry, ok := s.listCache[serverID]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.result, true
}

func (s *Service) setListCache(serverID string, result *ListResponse) {
	s.listMu.Lock()
	s.listCache[serverID] = listCacheEntry{result: result, expiresAt: time.Now().Add(listCacheTTL)}
	s.listMu.Unlock()
}

func (s *Service) invalidateListCache(serverID string) {
	s.listMu.Lock()
	delete(s.listCache, serverID)
	s.listMu.Unlock()
}

// runDocker menjalankan satu perintah docker dengan context sendiri (tanpa retry hang).
func (s *Service) runDocker(access *dockerAccess, cmd string, timeout time.Duration) (*sshpool.ExecResult, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// destructive=true → tanpa retry, agar hang sudo/daemon tidak dikalikan RetryMax.
	res, err := s.executor.Exec(ctx, access.serverID, timeout, access.wrap(cmd), true)
	if err != nil {
		return res, mapDockerError(err)
	}
	return res, nil
}

// ListContainers mengembalikan daftar container Docker di server.
func (s *Service) ListContainers(serverID string) (*ListResponse, error) {
	if strings.TrimSpace(serverID) == "" {
		return nil, fmt.Errorf("id server wajib diisi")
	}
	if cached, ok := s.getListCache(serverID); ok {
		return cached, nil
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	// Format TSV field terbatas — hindari {{json .}} yang lambat karena Labels besar.
	tsvFmt := "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.State}}\t{{.Ports}}\t{{.CreatedAt}}"
	script := "set +e\n" +
		"VER=$(docker version --format '{{.Server.Version}}' 2>/dev/null)\n" +
		"echo \"__poinhost_DOCKER_VER__${VER}\"\n" +
		"docker ps -a --format " + shellQuote(tsvFmt) + " </dev/null\n" +
		"EC=$?\n" +
		"echo \"__poinhost_DOCKER_EC__${EC}\"\n" +
		"exit 0"

	res, err := s.runDocker(access, script, 25*time.Second)
	if err != nil {
		return nil, err
	}

	out := res.Stdout + "\n" + res.Stderr
	version := ""
	exitCode := res.ExitCode
	var body strings.Builder
	for _, line := range strings.Split(out, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "__poinhost_DOCKER_VER__") {
			version = strings.TrimPrefix(trim, "__poinhost_DOCKER_VER__")
			continue
		}
		if strings.HasPrefix(trim, "__poinhost_DOCKER_EC__") {
			fmt.Sscanf(strings.TrimPrefix(trim, "__poinhost_DOCKER_EC__"), "%d", &exitCode)
			continue
		}
		if trim == "" {
			continue
		}
		body.WriteString(line)
		body.WriteByte('\n')
	}

	items := parseContainerList(body.String())
	if exitCode != 0 && len(items) == 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		msg = strings.ReplaceAll(msg, "__poinhost_DOCKER_VER__", "")
		msg = strings.TrimSpace(msg)
		if msg == "" {
			if version == "" {
				msg = "Docker belum terpasang atau daemon tidak berjalan"
			} else {
				msg = "gagal memuat daftar container"
			}
		}
		return nil, mapDockerError(fmt.Errorf("%s", msg))
	}

	result := &ListResponse{
		Containers: items,
		DockerOK:   true,
		Version:    version,
	}
	s.setListCache(serverID, result)
	return result, nil
}

// StartContainer menjalankan docker start pada container.
func (s *Service) StartContainer(serverID, containerID string) error {
	return s.runContainerAction(serverID, containerID, "docker start")
}

// StopContainer menghentikan container (docker stop).
func (s *Service) StopContainer(serverID, containerID string) error {
	return s.runContainerAction(serverID, containerID, "docker stop")
}

// RestartContainer merestart container (docker restart).
func (s *Service) RestartContainer(serverID, containerID string) error {
	return s.runContainerAction(serverID, containerID, "docker restart")
}

// RemoveContainer menghapus container (docker rm -f).
func (s *Service) RemoveContainer(serverID, containerID string) error {
	return s.runContainerAction(serverID, containerID, "docker rm -f")
}

// runContainerAction menjalankan aksi docker pada satu container dengan mutex.
func (s *Service) runContainerAction(serverID, rawID, cmdPrefix string) error {
	cid, err := normalizeContainerID(rawID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(serverID) == "" {
		return fmt.Errorf("id server wajib diisi")
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	cmd := fmt.Sprintf("%s %s", cmdPrefix, shellQuote(cid))
	res, err := s.runDocker(access, cmd, 75*time.Second)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = fmt.Sprintf("%s gagal", cmdPrefix)
		}
		return mapDockerError(fmt.Errorf("%s", msg))
	}
	s.invalidateListCache(serverID)
	return nil
}

// ContainerLogs membaca tail log container.
func (s *Service) ContainerLogs(req LogsRequest) (*LogsResponse, error) {
	cid, err := normalizeContainerID(req.ContainerID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.ServerID) == "" {
		return nil, fmt.Errorf("id server wajib diisi")
	}
	lines := req.Lines
	if lines <= 0 {
		lines = 200
	}
	if lines > 2000 {
		lines = 2000
	}

	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf("docker logs --tail %d %s 2>&1", lines, shellQuote(cid))
	res, err := s.runDocker(access, cmd, 30*time.Second)
	if err != nil {
		return nil, err
	}
	content := res.Stdout
	if res.ExitCode != 0 && strings.TrimSpace(content) == "" {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = "gagal membaca log container"
		}
		return nil, mapDockerError(fmt.Errorf("%s", msg))
	}
	return &LogsResponse{
		ContainerID: cid,
		Content:     content,
	}, nil
}

// StreamContainerLogs mengalirkan log container realtime (docker logs -f).
func (s *Service) StreamContainerLogs(ctx context.Context, req LogsRequest, onLine func(line string) error) error {
	cid, err := normalizeContainerID(req.ContainerID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.ServerID) == "" {
		return fmt.Errorf("id server wajib diisi")
	}
	lines := req.Lines
	if lines <= 0 {
		lines = 200
	}
	if lines > 2000 {
		lines = 2000
	}

	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}

	// --tail + -f: kirim histori lalu ikuti baris baru (stdout+stderr digabung).
	cmd := fmt.Sprintf("docker logs --tail %d -f %s 2>&1", lines, shellQuote(cid))
	return s.executor.ExecStreamDedicated(ctx, access.serverID, access.wrap(cmd), onLine)
}

// StreamContainerStats mengalirkan statistik container realtime (docker stats).
func (s *Service) StreamContainerStats(ctx context.Context, serverID, rawID string, onSample func(*StatsResponse) error) error {
	cid, err := normalizeContainerID(rawID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(serverID) == "" {
		return fmt.Errorf("id server wajib diisi")
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}

	// Tanpa --no-stream: satu baris JSON per interval (~1s).
	cmd := fmt.Sprintf(`docker stats --format "{{json .}}" %s`, shellQuote(cid))
	return s.executor.ExecStreamDedicated(ctx, access.serverID, access.wrap(cmd), func(line string) error {
		stats, err := parseContainerStats(line)
		if err != nil {
			return nil // abaikan baris noise / ANSI tanpa JSON
		}
		if stats.ContainerID == "" {
			stats.ContainerID = cid
		}
		return onSample(stats)
	})
}

// ContainerStats membaca statistik resource container (snapshot).
func (s *Service) ContainerStats(serverID, rawID string) (*StatsResponse, error) {
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

	cmd := fmt.Sprintf(`docker stats --no-stream --format "{{json .}}" %s`, shellQuote(cid))
	res, err := s.runDocker(access, cmd, 25*time.Second)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = "gagal membaca statistik container"
		}
		return nil, mapDockerError(fmt.Errorf("%s", msg))
	}
	stats, err := parseContainerStats(res.Stdout)
	if err != nil {
		return nil, err
	}
	if stats.ContainerID == "" {
		stats.ContainerID = cid
	}
	return stats, nil
}
