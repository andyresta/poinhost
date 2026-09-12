package files

import (
	"context"
	"errors"
	"strings"
	"time"
)

// fileAccess menyimpan konteks user efektif untuk satu operasi file. Kosong
// (asUser == "") berarti "sebagai user SSH yang login" — jalur cepat SFTP
// langsung, tidak pernah lewat sudo sama sekali.
type fileAccess struct {
	serverID     string
	sshUser      string
	asUser       string
	sudoPassword string
}

// effectiveUser mengembalikan user Linux yang dipakai untuk operasi ini.
func (a *fileAccess) effectiveUser() string {
	if a.asUser != "" {
		return a.asUser
	}
	return a.sshUser
}

// viaSudo mengecek apakah operasi ini harus dijalankan lewat `sudo -u`
// (bukan SFTP langsung sebagai user SSH).
func (a *fileAccess) viaSudo() bool {
	return a.asUser != "" && a.asUser != a.sshUser
}

// wrapSudo membungkus satu perintah shell supaya dijalankan sebagai user
// efektif via sudo. Kalau server auth-nya password, password SSH yang sama
// dipipe ke `sudo -S` (asumsi umum: password sudo = password login) — kalau
// auth-nya key, tidak ada password untuk dipipe, jadi bergantung pada
// NOPASSWD di sudoers server tersebut (lihat mapSudoError untuk pesan
// errornya kalau ternyata sudo minta password).
func (a *fileAccess) wrapSudo(shellCmd string) string {
	if !a.viaSudo() {
		return shellCmd
	}
	userPart := "-u " + shellQuote(a.effectiveUser()) + " -- sh -c " + shellQuote(shellCmd)
	if a.sudoPassword != "" {
		return "echo " + shellQuote(a.sudoPassword) + " | sudo -S -p '' " + userPart
	}
	return "sudo " + userPart
}

// resolveAccess memvalidasi & membangun konteks akses efektif untuk satu
// operasi file. asUser kosong atau sama dengan user SSH server berarti
// jalur normal (tanpa sudo sama sekali).
func (s *Service) resolveAccess(ctx context.Context, serverID, asUser string) (*fileAccess, error) {
	srv, err := s.servers.Get(serverID)
	if err != nil {
		return nil, err
	}

	asUser = strings.TrimSpace(asUser)
	if asUser == "" || asUser == srv.Username {
		return &fileAccess{serverID: serverID, sshUser: srv.Username}, nil
	}
	if !srv.UseSudo {
		return nil, errors.New("ganti user membutuhkan akses sudo pada server (aktifkan \"User ini punya akses sudo\" di Edit Server)")
	}
	if err := s.userExists(ctx, serverID, asUser); err != nil {
		return nil, err
	}

	access := &fileAccess{serverID: serverID, sshUser: srv.Username, asUser: asUser}
	if srv.AuthType == "password" {
		// Abaikan error di sini dengan sengaja — kalau password tidak
		// tersedia, access.sudoPassword tetap kosong dan wrapSudo jatuh ke
		// jalur NOPASSWD, bukan gagal total di titik ini.
		pw, _ := s.servers.SudoPassword(serverID)
		access.sudoPassword = pw
	}
	return access, nil
}

// mapSudoError menerjemahkan error sudo ke pesan yang lebih jelas.
func mapSudoError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "a password is required") ||
		strings.Contains(msg, "terminal is required") ||
		strings.Contains(msg, "incorrect password attempt") {
		return errors.New("sudo membutuhkan password — gunakan auth password SSH (password sama dengan sudo) atau atur NOPASSWD di sudoers")
	}
	return err
}

// userExists memvalidasi bahwa username ada di server remote.
func (s *Service) userExists(ctx context.Context, serverID, username string) error {
	execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := "getent passwd " + shellQuote(username) + " >/dev/null"
	_, err := s.executor.Exec(execCtx, serverID, 15*time.Second, cmd, false)
	if err != nil {
		return errors.New("user " + username + " tidak ditemukan di server")
	}
	return nil
}

// ListSystemUsers mengembalikan user Linux yang bisa dipilih sebagai "jalankan
// sebagai" — root (uid 0) + user biasa (uid 100-65533) yang punya shell
// login (bukan nologin/false). Selalu jalan sebagai user SSH biasa (tidak
// butuh sudo untuk membaca /etc/passwd).
func (s *Service) ListSystemUsers(ctx context.Context, serverID string) ([]SystemUser, error) {
	cmd := `getent passwd | awk -F: 'BEGIN{OFS="\t"} ($3==0){print $1,$3,$6; next} ($3>=100 && $3<65534 && $7 !~ /(nologin|false)$/){print $1,$3,$6}'`
	res, err := s.executor.Exec(ctx, serverID, 30*time.Second, cmd, false)
	if err != nil {
		return nil, err
	}

	users := make([]SystemUser, 0)
	seen := map[string]bool{}
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		if seen[parts[0]] {
			continue
		}
		seen[parts[0]] = true
		users = append(users, SystemUser{Username: parts[0], Home: parts[2]})
	}
	return users, nil
}
