package docker

import (
	"errors"
	"strings"
)

// dockerAccess konteks eksekusi perintah docker di server remote. Beda dari
// files.fileAccess (yang punya konsep "jalankan sebagai user lain"), docker
// selalu dijalankan sebagai user SSH yang login — sudo di sini murni untuk
// naik ke root kalau user SSH-nya bukan root dan tidak punya akses langsung
// ke socket Docker (lihat Server.UseSudo).
type dockerAccess struct {
	serverID     string
	sshUser      string
	useSudo      bool
	sudoPassword string
}

// resolveAccess menyiapkan akses SSH dan opsi sudo untuk perintah docker.
func (s *Service) resolveAccess(serverID string) (*dockerAccess, error) {
	srv, err := s.servers.Get(serverID)
	if err != nil {
		return nil, err
	}
	access := &dockerAccess{
		serverID: serverID,
		sshUser:  srv.Username,
		useSudo:  srv.UseSudo,
	}
	if srv.UseSudo && srv.Username != "root" && srv.AuthType == "password" {
		// Abaikan error di sini dengan sengaja: kalau password tidak
		// tersedia, wrap() jatuh ke jalur `sudo -n` (NOPASSWD), bukan gagal
		// total di titik resolusi akses.
		pw, _ := s.servers.SudoPassword(serverID)
		access.sudoPassword = pw
	}
	return access, nil
}

// wrap membungkus skrip/perintah docker via bash -c.
// Pakai sudo -n jika tidak ada password agar tidak hang menunggu TTY.
func (a *dockerAccess) wrap(script string) string {
	inner := "bash -c " + shellQuote(script)
	if a.sshUser == "root" || !a.useSudo {
		return inner
	}
	if a.sudoPassword != "" {
		return "echo " + shellQuote(a.sudoPassword) + " | sudo -S -p '' " + inner
	}
	// -n: non-interactive — gagal cepat jika butuh password.
	return "sudo -n " + inner
}

// mapDockerError menerjemahkan error docker/sudo ke pesan yang lebih jelas.
func mapDockerError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "a password is required") ||
		strings.Contains(msg, "terminal is required") ||
		strings.Contains(msg, "incorrect password attempt") ||
		strings.Contains(msg, "sudo: a password is required") ||
		strings.Contains(msg, "sudo: a terminal is required") {
		return errors.New("sudo membutuhkan password — gunakan auth password SSH atau atur NOPASSWD di sudoers")
	}
	if strings.Contains(msg, "permission denied") && (strings.Contains(msg, "docker") || strings.Contains(msg, "socket")) {
		return errors.New("tidak punya akses ke Docker socket — aktifkan sudo di server atau tambahkan user ke grup docker")
	}
	if strings.Contains(msg, "cannot connect to the docker daemon") ||
		strings.Contains(msg, "is the docker daemon running") ||
		strings.Contains(msg, "connection refused") {
		return errors.New("Docker daemon tidak berjalan di server")
	}
	if strings.Contains(msg, "command not found") || strings.Contains(msg, "docker: not found") ||
		(strings.Contains(msg, "no such file") && strings.Contains(msg, "docker")) {
		return errors.New("Docker belum terpasang di server")
	}
	if strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "deadline exceeded") {
		return errors.New("timeout menjalankan perintah Docker — cek daemon Docker atau akses sudo di server")
	}
	if strings.Contains(msg, "already exists") {
		return errors.New("nama sudah dipakai di server ini")
	}
	if strings.Contains(msg, "has active endpoints") {
		return errors.New("network masih dipakai container aktif — stop/lepas container itu dulu sebelum dihapus")
	}
	return err
}
