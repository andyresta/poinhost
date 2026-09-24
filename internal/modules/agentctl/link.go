package agentctl

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/agent/transfer"
)

// agentKeyDir adalah tempat agent menyimpan kunci SSH server tertaut.
const agentKeyDir = DataDir + "/keys"

// bundleDropPath adalah lokasi serah-terima paket server di mesin agent.
//
// SENGAJA di dalam direktori data agent, BUKAN di /tmp seperti berkas
// sementara lain di modul ini. Bedanya menentukan: berkas lain dibaca oleh
// perintah shell yang kita jalankan lewat SSH, yang melihat /tmp asli —
// sedangkan berkas ini dibaca oleh PROSES AGENT, yang berjalan dengan
// PrivateTmp=yes sehingga /tmp-nya adalah namespace privat yang sama sekali
// berbeda. Ditaruh di /tmp, agent tidak akan pernah menemukannya.
const bundleDropPath = DataDir + "/.import.bundle"

// LinkedServer adalah server yang terdaftar di agent.
type LinkedServer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Host   string `json:"host"`
	IsSelf bool   `json:"isSelf"`

	// Connection adalah status koneksi SEBAGAIMANA DILIHAT AGENT — bukan dari
	// app desktop. Keduanya bisa berbeda, dan justru pandangan agent yang
	// menentukan apa yang dilaporkan bot.
	Connection string `json:"connection,omitempty"`
	Error      string `json:"error,omitempty"`
}

// LinkedServers mengembalikan daftar server yang benar-benar terdaftar di
// agent — dibaca dari agent, bukan dari catatan sisi desktop, supaya layar
// tidak pernah menampilkan keadaan yang berbeda dari kenyataan.
func (s *Service) LinkedServers(ctx context.Context, serverID string) ([]LinkedServer, error) {
	out, err := s.callAgent(ctx, serverID, "GET", "/v1/servers")
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Servers []LinkedServer `json:"servers"`
	}
	if json.Unmarshal([]byte(out), &parsed) != nil {
		return nil, fmt.Errorf("agent tidak mengembalikan daftar server: %s", trimForError(out))
	}
	if parsed.Servers == nil {
		parsed.Servers = []LinkedServer{}
	}
	return parsed.Servers, nil
}

// LinkRequest adalah pilihan server yang mau ditautkan.
type LinkRequest struct {
	ServerID string   `json:"serverId"`
	TargetID []string `json:"targetIds"`
}

// LinkServers mengirim server pilihan user beserta kredensialnya ke agent.
//
// Kredensial dikirim sebagai berkas TERENKRIPSI lewat SFTP, dan passphrase-nya
// menyusul lewat API loopback yang terautentikasi. Jadi kredensial tidak pernah
// ada dalam bentuk terbuka di disk server, dan tidak pernah jadi bagian dari
// command line yang terlihat di `ps` — pola yang sama dengan pengiriman token
// bot.
func (s *Service) LinkServers(ctx context.Context, req LinkRequest) ([]LinkedServer, error) {
	if len(req.TargetID) == 0 {
		return nil, fmt.Errorf("belum ada server yang dipilih")
	}

	bundle, err := s.buildBundle(req.ServerID, req.TargetID)
	if err != nil {
		return nil, err
	}

	passphrase, err := randomToken()
	if err != nil {
		return nil, err
	}
	sealed, err := transfer.Seal(bundle, passphrase)
	if err != nil {
		return nil, err
	}
	if err := s.uploadTemp(ctx, req.ServerID, bundleDropPath, sealed); err != nil {
		return nil, err
	}

	body, err := json.Marshal(map[string]string{"path": bundleDropPath, "passphrase": passphrase})
	if err != nil {
		return nil, err
	}
	if _, err := s.callAgentBody(ctx, req.ServerID, "POST", "/v1/servers/import", string(body)); err != nil {
		return nil, err
	}
	return s.LinkedServers(ctx, req.ServerID)
}

// UnlinkServer mencabut satu server dari agent.
func (s *Service) UnlinkServer(ctx context.Context, serverID, targetID string) ([]LinkedServer, error) {
	if _, err := s.callAgent(ctx, serverID, "DELETE", "/v1/servers/"+targetID); err != nil {
		return nil, err
	}
	return s.LinkedServers(ctx, serverID)
}

// buildBundle menyusun paket dari data server di app desktop.
func (s *Service) buildBundle(agentServerID string, targetIDs []string) (*transfer.Bundle, error) {
	bundle := &transfer.Bundle{}

	for _, id := range targetIDs {
		if id == agentServerID {
			// Mesin agent sendiri sudah dikelola lewat jalur pengelolaan-diri,
			// dengan kunci khusus. Mengirimnya lagi sebagai server tertaut
			// hanya akan menimpa entri itu dengan kredensial yang berbeda.
			continue
		}
		srv, err := s.servers.Get(id)
		if err != nil || srv == nil {
			return nil, fmt.Errorf("server %s tidak ditemukan", id)
		}

		entry := transfer.Server{
			ID: srv.ID, Name: srv.Name, Host: srv.Host, Port: srv.Port,
			Username: srv.Username, AuthType: srv.AuthType,
			Tags: srv.Tags, Color: srv.Color, Notes: srv.Notes, UseSudo: srv.UseSudo,
			KnownHosts: s.knownHostLines(srv.Host),
		}

		if srv.AuthType == "key" {
			path := srv.KeyPath
			if path == "" {
				path = defaultKeyPath()
			}
			raw, rerr := os.ReadFile(path)
			if rerr != nil {
				return nil, fmt.Errorf("baca kunci SSH %s (%s): %w", srv.Name, path, rerr)
			}
			entry.KeyFileB64 = base64.StdEncoding.EncodeToString(raw)
			// Path TUJUAN, bukan path di mesin ini: agent menyimpan kunci di
			// direktori datanya sendiri.
			entry.KeyPath = agentKeyDir + "/server-" + srv.ID
		} else {
			pw, _ := s.servers.SudoPassword(srv.ID)
			if pw == "" {
				return nil, fmt.Errorf("password SSH untuk %s tidak tersimpan — buka form server lalu simpan ulang passwordnya", srv.Name)
			}
			entry.Password = pw
		}

		if len(entry.KnownHosts) == 0 {
			// Tanpa host key, agent pasti gagal menyambung dan bot akan
			// melaporkan server itu "not connected" tanpa penjelasan. Lebih
			// baik ditolak sekarang, dengan sebab yang jelas.
			return nil, fmt.Errorf("host key untuk %s belum tersimpan di poinhost — buka server itu sekali lalu coba lagi", srv.Name)
		}
		bundle.Servers = append(bundle.Servers, entry)
	}

	if len(bundle.Servers) == 0 {
		return nil, fmt.Errorf("tidak ada server yang bisa ditautkan dari pilihan itu")
	}
	return bundle, nil
}

func (s *Service) knownHostLines(host string) []string {
	if s.knownHosts == nil {
		return nil
	}
	return s.knownHosts.LinesFor(host)
}

func defaultKeyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "id_ed25519")
}

// callAgentBody sama dengan callAgent tapi mengirim body JSON.
func (s *Service) callAgentBody(ctx context.Context, serverID, method, path, body string) (string, error) {
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return "", err
	}
	acfg, err := s.readConfig(ctx, access)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(acfg.APIToken) == "" {
		return "", fmt.Errorf("config agent belum punya apiToken — pasang ulang agent")
	}

	// Token DAN body sama-sama lewat variabel yang diisi dari base64: body
	// memuat passphrase paket kredensial, dan apa pun di command line terlihat
	// di `ps` selama perintah berjalan.
	script := "t=$(echo " + shellQuote(base64.StdEncoding.EncodeToString([]byte(acfg.APIToken))) + " | base64 -d)\n" +
		"b=$(echo " + shellQuote(base64.StdEncoding.EncodeToString([]byte(body))) + " | base64 -d)\n" +
		`curl -fsS --max-time 30 -X ` + method +
		` -H "Authorization: Bearer $t" -H 'Content-Type: application/json' --data-binary "$b" ` +
		`http://127.0.0.1:` + strconv.Itoa(portOf(acfg.Listen)) + path + "\n"

	res, err := s.run(ctx, access, script, 60*time.Second)
	if err != nil {
		return "", fmt.Errorf("agent menolak paket server: %w", err)
	}
	return strings.TrimSpace(res.Stdout), nil
}
