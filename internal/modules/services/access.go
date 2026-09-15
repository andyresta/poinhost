package services

import (
	"errors"
	"regexp"
	"strings"
)

// serviceAccess konteks eksekusi systemctl di server remote — polanya sama
// dengan dockerAccess: sudo di sini murni untuk naik ke root kalau user SSH
// bukan root (systemctl start/stop/enable butuh root).
type serviceAccess struct {
	serverID     string
	sshUser      string
	useSudo      bool
	sudoPassword string
}

func (s *Service) resolveAccess(serverID string) (*serviceAccess, error) {
	srv, err := s.servers.Get(serverID)
	if err != nil {
		return nil, err
	}
	access := &serviceAccess{
		serverID: serverID,
		sshUser:  srv.Username,
		useSudo:  srv.UseSudo,
	}
	if srv.UseSudo && srv.Username != "root" && srv.AuthType == "password" {
		// Error diabaikan dengan sengaja: kalau password tidak tersedia,
		// wrap() jatuh ke `sudo -n` (NOPASSWD) — sama seperti modul docker.
		pw, _ := s.servers.SudoPassword(serverID)
		access.sudoPassword = pw
	}
	return access, nil
}

func (a *serviceAccess) wrap(script string) string {
	inner := "bash -c " + shellQuote(script)
	if a.sshUser == "root" || !a.useSudo {
		return inner
	}
	if a.sudoPassword != "" {
		return "echo " + shellQuote(a.sudoPassword) + " | sudo -S -p '' " + inner
	}
	// -n: non-interactive — gagal cepat, tidak menggantung menunggu TTY.
	return "sudo -n " + inner
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Nama unit systemd yang sah. Divalidasi KETAT sebelum masuk ke perintah
// shell — nama unit datang dari frontend, dan walaupun sudah lewat
// shellQuote, membatasi ke karakter yang memang dipakai systemd menutup
// seluruh celah injeksi sejak awal, bukan mengandalkan satu lapis quoting.
var unitNameRE = regexp.MustCompile(`^[A-Za-z0-9_.@:\\-]{1,128}$`)

func normalizeUnitName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", errors.New("nama service wajib diisi")
	}
	if !strings.HasSuffix(name, ".service") {
		name += ".service"
	}
	if !unitNameRE.MatchString(name) {
		return "", errors.New("nama service tidak valid")
	}
	return name, nil
}

// mapServiceError menerjemahkan kegagalan yang sering terjadi ke pesan yang
// bisa ditindaklanjuti — bukan melempar stderr systemd mentah ke UI.
func mapServiceError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "a password is required"),
		strings.Contains(msg, "a terminal is required"),
		strings.Contains(msg, "incorrect password attempt"):
		return errors.New("sudo membutuhkan password — gunakan auth password SSH atau atur NOPASSWD di sudoers")
	case strings.Contains(msg, "access denied"), strings.Contains(msg, "permission denied"):
		return errors.New("tidak punya izin menjalankan systemctl — aktifkan sudo untuk server ini")
	case strings.Contains(msg, "command not found"), strings.Contains(msg, "systemctl: not found"):
		return errors.New("server ini tidak memakai systemd, jadi modul Services belum bisa dipakai")
	case strings.Contains(msg, "deadline exceeded"):
		return errors.New("timeout menjalankan systemctl — service mungkin menggantung saat start/stop")
	}
	return err
}
