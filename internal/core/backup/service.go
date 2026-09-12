package backup

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/andyresta/poinhost/internal/modules/servers"
	"github.com/andyresta/poinhost/internal/modules/website"
)

// Service mengorkestrasi export/import arsip data poinhost, mengumpulkan
// dari (dan menulis balik ke) beberapa modul sekaligus: servers (koneksi
// SSH) dan website (kredensial database + tautan domain<->database).
type Service struct {
	servers *servers.Service
	website *website.Service
}

// NewService membuat backup.Service baru.
func NewService(serversSvc *servers.Service, websiteSvc *website.Service) *Service {
	return &Service{servers: serversSvc, website: websiteSvc}
}

// BuildPayload mengumpulkan seluruh data yang akan diekspor. includeKeyFiles
// SENGAJA opsional (default off dari sisi UI) — membaca & menyertakan isi
// file private key SSH ke dalam arsip adalah keputusan yang harus disadari
// user, bukan otomatis.
func (s *Service) BuildPayload(includeKeyFiles bool) (*Payload, error) {
	list, err := s.servers.List()
	if err != nil {
		return nil, err
	}

	payload := &Payload{Version: 1, ExportedAt: time.Now().UTC().Format(time.RFC3339)}

	for _, srv := range list {
		password, _ := s.servers.SudoPassword(srv.ID)
		se := ServerExport{
			ID: srv.ID, Name: srv.Name, Host: srv.Host, Port: srv.Port,
			Username: srv.Username, AuthType: srv.AuthType, KeyPath: srv.KeyPath,
			Password: password, Tags: srv.Tags, Color: srv.Color, Notes: srv.Notes,
			UseSudo: srv.UseSudo,
		}
		if includeKeyFiles && srv.AuthType == "key" && srv.KeyPath != "" {
			if data, err := os.ReadFile(srv.KeyPath); err == nil {
				se.KeyFileB64 = base64.StdEncoding.EncodeToString(data)
			}
		}
		payload.Servers = append(payload.Servers, se)
	}

	creds, err := s.website.ListAllDBCredentials()
	if err == nil {
		for _, c := range creds {
			pw, ok, err := s.website.ExportCredentialPassword(c.ServerID, c.Engine, c.Username, c.Host)
			if err != nil || !ok {
				continue
			}
			payload.DBCredentials = append(payload.DBCredentials, DBCredentialExport{
				ServerID: c.ServerID, Engine: c.Engine, Username: c.Username, Host: c.Host, Password: pw,
			})
		}
	}

	links, err := s.website.ListAllDomainDatabases()
	if err == nil {
		for _, l := range links {
			payload.DomainDatabases = append(payload.DomainDatabases, DomainDatabaseExport{
				ServerID: l.ServerID, Domain: l.Domain, Engine: l.Engine, Database: l.Database,
			})
		}
	}

	return payload, nil
}

// Export membangun payload lalu mengenkripsinya dengan passphrase — hasilnya
// siap ditulis apa adanya sebagai file (lihat app.go: ExportBackup).
func (s *Service) Export(passphrase string, includeKeyFiles bool) ([]byte, error) {
	if passphrase == "" {
		return nil, fmt.Errorf("passphrase wajib diisi (dipakai lagi saat import di perangkat lain)")
	}
	payload, err := s.BuildPayload(includeKeyFiles)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return Encrypt(raw, passphrase)
}

// Import mendekripsi arsip lalu menuliskan isinya ke database lokal +
// vault lokal perangkat ini — ID server DIPERTAHANKAN dari arsip (lihat
// servers.Service.UpsertFromBackup), sehingga aman dijalankan berulang
// kali (idempotent) untuk arsip yang sama.
func (s *Service) Import(data []byte, passphrase string) (*ImportSummary, error) {
	raw, err := Decrypt(data, passphrase)
	if err != nil {
		return nil, err
	}
	var payload Payload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("format arsip tidak dikenal: %w", err)
	}

	summary := &ImportSummary{}

	for _, se := range payload.Servers {
		keyPath := se.KeyPath
		if se.KeyFileB64 != "" {
			if p, err := saveImportedKeyFile(se.ID, se.KeyPath, se.KeyFileB64); err == nil {
				keyPath = p
			}
		}
		srv := &servers.Server{
			ID: se.ID, Name: se.Name, Host: se.Host, Port: se.Port, Username: se.Username,
			AuthType: se.AuthType, KeyPath: keyPath, Tags: se.Tags, Color: se.Color,
			Notes: se.Notes, UseSudo: se.UseSudo, IsActive: true,
		}
		if err := s.servers.UpsertFromBackup(srv, se.Password); err != nil {
			return summary, fmt.Errorf("impor server %q: %w", se.Name, err)
		}
		summary.ServersImported++
	}

	for _, c := range payload.DBCredentials {
		_, err := s.website.SaveDBCredential(website.SaveDBCredentialRequest{
			ServerID: c.ServerID, Engine: c.Engine, Username: c.Username, Host: c.Host,
			Password: c.Password, Verify: false,
		})
		if err == nil {
			summary.DBCredentialsImported++
		}
	}

	for _, l := range payload.DomainDatabases {
		if err := s.website.LinkDomainDatabase(l.ServerID, l.Domain, l.Engine, l.Database); err == nil {
			summary.DomainLinksImported++
		}
	}

	return summary, nil
}

// saveImportedKeyFile menulis isi private key yang dibawa arsip ke folder
// khusus lokal (bukan menimpa lokasi asalnya di mesin sumber, yang mungkin
// sudah tidak relevan/ada di mesin ini) — permission 0600, mengikuti
// konvensi file kunci SSH biasa.
func saveImportedKeyFile(serverID, originalPath, b64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".poinhost", "imported_keys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := filepath.Base(originalPath)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "id_key"
	}
	path := filepath.Join(dir, serverID+"_"+name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
