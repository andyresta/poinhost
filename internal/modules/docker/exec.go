package docker

import "fmt"

// ExecCommand hasil pembangunan perintah `docker exec` interaktif siap
// dijalankan lewat PTY (lihat terminal.Service.OpenCommand). SudoPassword
// tidak kosong hanya kalau server perlu naik ke root lewat sudo interaktif
// (auth password) — pemanggil (App binding) yang bertanggung jawab
// menuliskannya ke stdin PTY setelah sesi terbuka.
type ExecCommand struct {
	Command      string
	SudoPassword string
}

// normalizeExecShell memvalidasi pilihan shell di dalam container.
// Nilai: auto (default), bash, sh.
func normalizeExecShell(raw string) (string, error) {
	switch raw {
	case "", "auto":
		return "auto", nil
	case "bash", "sh":
		return raw, nil
	default:
		return "", fmt.Errorf("shell tidak didukung (pakai auto, bash, atau sh)")
	}
}

// buildExecInner menyusun argumen docker exec setelah id container tervalidasi.
func buildExecInner(containerID, shell string) string {
	cidQ := shellQuote(containerID)
	switch shell {
	case "bash":
		return "docker exec -it " + cidQ + " bash"
	case "sh":
		return "docker exec -it " + cidQ + " sh"
	default:
		// Auto: prefer bash bila ada, fallback sh (image Alpine/minimal).
		inner := "command -v bash >/dev/null 2>&1 && exec bash || exec sh"
		return "docker exec -it " + cidQ + " sh -c " + shellQuote(inner)
	}
}

// BuildExecCommand menyusun perintah PTY: docker exec -it ke dalam container,
// dibungkus sudo bila server butuh elevasi. Password sudo (kalau perlu)
// dikembalikan terpisah supaya App binding bisa mengirimkannya lewat stdin
// PTY setelah sesi dibuka, sama seperti pola sudo di modul files.
func (s *Service) BuildExecCommand(serverID, containerID, shell string) (*ExecCommand, error) {
	cid, err := normalizeContainerID(containerID)
	if err != nil {
		return nil, err
	}
	shell, err = normalizeExecShell(shell)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	execCmd := buildExecInner(cid, shell)
	if access.sshUser == "root" || !access.useSudo {
		return &ExecCommand{Command: execCmd}, nil
	}
	if access.sudoPassword == "" {
		return &ExecCommand{Command: "sudo -n " + execCmd}, nil
	}
	// -v: validasi kredensial sudo dulu (di sinilah password dibutuhkan di stdin),
	// baru jalankan exec sesungguhnya non-interaktif (-n) — supaya sesi docker
	// exec itu sendiri tidak pernah menerima password sebagai bagian dari inputnya.
	cmd := "sudo -S -p '' -v && stty echo && sudo -n " + execCmd
	return &ExecCommand{Command: cmd, SudoPassword: access.sudoPassword}, nil
}
