package website

import (
	"errors"
	"strings"
)

// websiteAccess konteks eksekusi perintah hosting (nginx/php/certbot) di
// server remote. Berbeda dari docker.dockerAccess: modul ini SELALU butuh
// akses root (config nginx/php-fpm/certbot ada di path milik root), jadi
// resolveAccess menolak lebih awal kalau user SSH bukan root DAN server
// tidak punya sudo — bukan diam-diam mencoba lalu gagal di tengah skrip.
type websiteAccess struct {
	serverID     string
	sshUser      string
	useSudo      bool
	sudoPassword string
}

// resolveAccess menyiapkan akses SSH untuk perintah hosting.
func (s *Service) resolveAccess(serverID string) (*websiteAccess, error) {
	srv, err := s.servers.Get(serverID)
	if err != nil {
		return nil, err
	}
	if srv.Username != "root" && !srv.UseSudo {
		return nil, errors.New("mengelola website butuh akses root — aktifkan \"User ini punya akses sudo\" di Edit Server, atau gunakan user root")
	}
	access := &websiteAccess{
		serverID: serverID,
		sshUser:  srv.Username,
		useSudo:  srv.UseSudo,
	}
	if srv.Username != "root" && srv.AuthType == "password" {
		pw, _ := s.servers.SudoPassword(serverID)
		access.sudoPassword = pw
	}
	return access, nil
}

// wrap membungkus skrip via bash -c, naik ke root lewat sudo bila perlu.
// Sengaja pakai `sudo -n` (non-interactive) ketika tidak ada password
// tersimpan — beda dari homepoin yang pakai `sudo bash -c ...` polos di
// jalur ini, yang bisa HANG menunggu prompt password di TTY yang tidak ada
// (SSH exec non-interaktif). Di sini kita lebih suka gagal cepat dengan
// pesan jelas (lihat mapWebsiteError) daripada menggantung selamanya.
func (a *websiteAccess) wrap(script string) string {
	inner := "bash -c " + shellQuote(script)
	if a.sshUser == "root" {
		return inner
	}
	if a.sudoPassword != "" {
		return "echo " + shellQuote(a.sudoPassword) + " | sudo -S -p '' " + inner
	}
	return "sudo -n " + inner
}

// wrapStreaming seperti wrap, tapi untuk perintah yang MEMBACA DATA dari
// stdin sesi SSH (restore dump, `tar x`). wrap biasa menyalurkan password
// sudo lewat pipa `echo pw | sudo -S …` — pipa itu SEKALIGUS jadi stdin
// perintah yang dibungkus, sehingga data yang dikirim ke sesi SSH tidak
// pernah sampai ke tar/psql (yang malah membaca EOF). Di sini password
// dipakai dulu untuk mengisi cache kredensial sudo (`sudo -v`, stdin-nya
// pipa echo tersendiri), lalu perintah sebenarnya dijalankan dengan
// `sudo -n` yang mewarisi stdin sesi SSH apa adanya. Kedua sudo berjalan
// di shell induk yang sama, jadi cache timestamp-nya berlaku; kalau
// sudoers mematikan cache (timestamp_timeout=0), `sudo -n` gagal cepat
// dengan pesan jelas lewat mapWebsiteError, bukan diam-diam kehilangan data.
func (a *websiteAccess) wrapStreaming(script string) string {
	inner := "bash -c " + shellQuote(script)
	if a.sshUser == "root" {
		return inner
	}
	if a.sudoPassword != "" {
		return "echo " + shellQuote(a.sudoPassword) + " | sudo -S -p '' -v 2>/dev/null; sudo -n " + inner
	}
	return "sudo -n " + inner
}

// mapWebsiteError menerjemahkan error sudo/nginx/php/certbot ke pesan yang jelas.
func mapWebsiteError(err error) error {
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
	if strings.Contains(msg, "command not found") || strings.Contains(msg, "not found") {
		if strings.Contains(msg, "nginx") {
			return errors.New("Nginx belum terpasang di server")
		}
		if strings.Contains(msg, "certbot") {
			return errors.New("Certbot belum terpasang di server")
		}
	}
	if strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "deadline exceeded") {
		return errors.New("timeout menjalankan perintah di server — cek koneksi atau beban server")
	}
	return err
}
