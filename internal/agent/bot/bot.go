// Package bot menjalankan konektor Telegram poinhost-agent.
//
// Balasan bot berbahasa Inggris (keputusan produk), sedangkan komentar kode
// tetap Bahasa Indonesia seperti sisa repo ini.
//
// Batas yang ditegakkan package ini, dan alasannya:
//
//   - HANYA chat privat. Di grup, chat_id adalah grupnya: siapa pun yang
//     ditambahkan ke grup itu akan ikut bisa menjalankan perintah tanpa pernah
//     disetujui siapa pun.
//   - Otorisasi berdasarkan user_id, BUKAN chat_id, karena itulah identitas
//     orangnya.
//   - Hanya fungsi baca. Belum ada satu pun perintah yang mengubah server,
//     jadi belum ada mesin konfirmasi yang perlu dipercaya.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/agent/telegram"
)

// pollTimeout adalah lama satu long-poll getUpdates.
const pollTimeout = 30

// ServerInfo adalah server terdaftar, seperti yang dilihat bot.
type ServerInfo struct {
	ID     string
	Name   string
	Host   string
	IsSelf bool // server tempat agent ini berjalan
}

// Status adalah snapshot kesehatan satu server.
type Status struct {
	Connection    string
	OSName        string
	Hostname      string
	UptimeSeconds int64
	CPUCores      int
	CPUUsedPct    float64
	MemUsedMB     int64
	MemTotalMB    int64
	DiskUsedGB    float64
	DiskTotalGB   float64
	CheckedAt     time.Time
	Stale         bool
	Error         string
}

// Backend adalah satu-satunya jalan bot menyentuh dunia poinhost.
//
// Sengaja berupa interface sempit, bukan *servers.Service langsung: itu
// membuat seluruh perilaku perintah bisa diuji tanpa SSH, database, maupun
// jaringan — dan menegaskan bahwa bot tidak punya kemampuan lain selain yang
// tercantum di sini.
type Backend interface {
	ListServers() ([]ServerInfo, error)
	Server(id string) (ServerInfo, bool)
	Status(id string) (Status, bool)
	Refresh(ctx context.Context, id string) (Status, error)
}

// Allowlist mengatur siapa yang boleh memakai bot.
type Allowlist interface {
	IsAllowed(userID int64) bool
	Allow(userID int64, label string) error
	Count() int
}

// API adalah bagian klien Telegram yang dipakai bot.
type API interface {
	GetMe(ctx context.Context) (*telegram.User, error)
	GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]telegram.Update, error)
	SendMessage(ctx context.Context, chatID int64, text string, kb *telegram.InlineKeyboard) (*telegram.Message, error)
	EditMessageText(ctx context.Context, chatID, messageID int64, text string, kb *telegram.InlineKeyboard) error
	AnswerCallbackQuery(ctx context.Context, id, text string) error
	SetMyCommands(ctx context.Context, cmds []telegram.BotCommand) error
}

// Bot menerjemahkan pesan Telegram menjadi panggilan Backend.
type Bot struct {
	api     API
	backend Backend
	allow   Allowlist
	pairing *Pairing

	offset int64
}

// New membuat bot.
func New(api API, backend Backend, allow Allowlist, pairing *Pairing) *Bot {
	return &Bot{api: api, backend: backend, allow: allow, pairing: pairing}
}

// Commands adalah menu yang didaftarkan ke Telegram. Dari sinilah "/" di
// aplikasi Telegram memunculkan daftar berikut autocomplete, sehingga user
// tidak perlu menghafal apa pun.
func Commands() []telegram.BotCommand {
	return []telegram.BotCommand{
		{Command: "servers", Description: "List managed servers with a quick status"},
		{Command: "status", Description: "CPU, memory and disk for one server"},
		{Command: "help", Description: "Show available commands"},
	}
}

// Run melakukan long-poll sampai ctx dibatalkan.
func (b *Bot) Run(ctx context.Context) error {
	me, err := b.api.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("verifikasi token bot: %w", err)
	}
	log.Printf("telegram: terhubung sebagai %s", me.DisplayName())

	if err := b.api.SetMyCommands(ctx, Commands()); err != nil {
		// Menu perintah cuma kenyamanan; bot tetap berguna tanpanya.
		log.Printf("telegram: gagal mendaftarkan menu perintah: %v", err)
	}

	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		updates, err := b.api.GetUpdates(ctx, b.offset, pollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if telegram.IsUnauthorized(err) {
				// Token dicabut atau salah: mengulang selamanya hanya
				// menghasilkan derau. Berhenti dan biarkan operator
				// memperbaikinya dari Agent Control.
				return fmt.Errorf("token bot ditolak Telegram: %w", err)
			}
			log.Printf("telegram: getUpdates gagal (%v), coba lagi dalam %s", err, backoff)
			if !sleepCtx(ctx, backoff) {
				return ctx.Err()
			}
			if backoff < 60*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second

		for _, u := range updates {
			if u.UpdateID >= b.offset {
				b.offset = u.UpdateID + 1
			}
			b.handle(ctx, u)
		}
	}
}

func (b *Bot) handle(ctx context.Context, u telegram.Update) {
	switch {
	case u.Message != nil:
		b.handleMessage(ctx, u.Message)
	case u.CallbackQuery != nil:
		b.handleCallback(ctx, u.CallbackQuery)
	}
}

func (b *Bot) handleMessage(ctx context.Context, m *telegram.Message) {
	if m.From == nil || m.Chat == nil || m.From.IsBot {
		return
	}
	// Grup diabaikan diam-diam, tanpa balasan apa pun: membalas di grup
	// justru memberi tahu isi grup bahwa ada bot infrastruktur di sana.
	if m.Chat.Type != "private" {
		return
	}

	cmd, arg := parseCommand(m.Text)

	// /pair adalah SATU-SATUNYA perintah yang boleh dari user yang belum
	// diizinkan — memang itu gunanya.
	if cmd == "pair" {
		b.handlePair(ctx, m, arg)
		return
	}

	if !b.allow.IsAllowed(m.From.ID) {
		log.Printf("telegram: perintah %q dari user tidak diizinkan (%d)", cmd, m.From.ID)
		b.reply(ctx, m.Chat.ID, msgNotAuthorized(m.From.ID))
		return
	}

	switch cmd {
	case "start", "help":
		b.reply(ctx, m.Chat.ID, msgHelp())
	case "servers":
		b.sendServerList(ctx, m.Chat.ID)
	case "status":
		b.sendStatusFor(ctx, m.Chat.ID, arg)
	case "":
		b.reply(ctx, m.Chat.ID, msgHelp())
	default:
		b.reply(ctx, m.Chat.ID, fmt.Sprintf("Unknown command /%s.\n\n%s", esc(cmd), msgHelp()))
	}
}

func (b *Bot) handlePair(ctx context.Context, m *telegram.Message, arg string) {
	if b.allow.IsAllowed(m.From.ID) {
		b.reply(ctx, m.Chat.ID, "You are already paired with this agent.")
		return
	}
	if strings.TrimSpace(arg) == "" {
		b.reply(ctx, m.Chat.ID, "Send <code>/pair &lt;code&gt;</code> using the pairing code shown in poinhost → Agent Control.")
		return
	}
	if err := b.pairing.Redeem(arg); err != nil {
		log.Printf("telegram: pairing gagal dari user %d", m.From.ID)
		b.reply(ctx, m.Chat.ID, "That pairing code is not valid. Generate a new one in poinhost → Agent Control.")
		return
	}
	if err := b.allow.Allow(m.From.ID, m.From.DisplayName()); err != nil {
		log.Printf("telegram: gagal menyimpan allowlist: %v", err)
		b.reply(ctx, m.Chat.ID, "Paired, but saving the change failed. Check the agent logs.")
		return
	}
	log.Printf("telegram: user %d (%s) ditambahkan lewat pairing", m.From.ID, m.From.DisplayName())
	b.reply(ctx, m.Chat.ID, "Paired. You can now use this bot.\n\n"+msgHelp())
}

func (b *Bot) handleCallback(ctx context.Context, q *telegram.CallbackQuery) {
	if q.From == nil {
		return
	}
	if !b.allow.IsAllowed(q.From.ID) {
		_ = b.api.AnswerCallbackQuery(ctx, q.ID, "Not authorized")
		return
	}

	kind, id, ok := parseCallback(q.Data)
	if !ok {
		_ = b.api.AnswerCallbackQuery(ctx, q.ID, "")
		return
	}

	var note string
	if kind == cbRefresh {
		// Refresh memaksa satu round-trip SSH; sisanya dilayani dari cache
		// Collector yang memang selalu hangat.
		if _, err := b.backend.Refresh(ctx, id); err != nil {
			note = "Refresh failed"
		} else {
			note = "Refreshed"
		}
	}
	_ = b.api.AnswerCallbackQuery(ctx, q.ID, note)

	text, kb := b.statusMessage(id)
	if q.Message == nil || q.Message.Chat == nil {
		return
	}
	if err := b.api.EditMessageText(ctx, q.Message.Chat.ID, q.Message.MessageID, text, kb); err != nil {
		log.Printf("telegram: gagal memperbarui pesan status: %v", err)
	}
}

func (b *Bot) sendServerList(ctx context.Context, chatID int64) {
	list, err := b.backend.ListServers()
	if err != nil {
		b.reply(ctx, chatID, "Could not read the server list: "+esc(err.Error()))
		return
	}
	if len(list) == 0 {
		b.reply(ctx, chatID, "No servers are registered with this agent yet.\n\nAdd them from poinhost → Agent Control.")
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "<b>Managed servers (%d)</b>\n", len(list))
	for _, srv := range list {
		st, _ := b.backend.Status(srv.ID)
		sb.WriteString("\n")
		sb.WriteString(formatServerLine(srv, st))
	}
	sb.WriteString("\n\nUse /status for details.")

	b.replyKB(ctx, chatID, sb.String(), serverKeyboard(list, cbStatus))
}

func (b *Bot) sendStatusFor(ctx context.Context, chatID int64, arg string) {
	list, err := b.backend.ListServers()
	if err != nil {
		b.reply(ctx, chatID, "Could not read the server list: "+esc(err.Error()))
		return
	}
	if len(list) == 0 {
		b.reply(ctx, chatID, "No servers are registered with this agent yet.")
		return
	}

	if strings.TrimSpace(arg) == "" {
		b.replyKB(ctx, chatID, "Pick a server:", serverKeyboard(list, cbStatus))
		return
	}

	match, err := resolveServer(list, arg)
	if err != nil {
		b.replyKB(ctx, chatID, esc(err.Error())+"\n\nPick one:", serverKeyboard(list, cbStatus))
		return
	}

	text, kb := b.statusMessage(match.ID)
	b.replyKB(ctx, chatID, text, kb)
}

func (b *Bot) statusMessage(serverID string) (string, *telegram.InlineKeyboard) {
	srv, ok := b.backend.Server(serverID)
	if !ok {
		return "That server is no longer registered with this agent.", nil
	}
	st, _ := b.backend.Status(serverID)
	kb := &telegram.InlineKeyboard{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
		{Text: "🔄 Refresh", CallbackData: callbackData(cbRefresh, serverID)},
	}}}
	return formatStatusDetail(srv, st), kb
}

func (b *Bot) reply(ctx context.Context, chatID int64, text string) {
	b.replyKB(ctx, chatID, text, nil)
}

func (b *Bot) replyKB(ctx context.Context, chatID int64, text string, kb *telegram.InlineKeyboard) {
	if _, err := b.api.SendMessage(ctx, chatID, text, kb); err != nil {
		log.Printf("telegram: gagal mengirim pesan: %v", err)
	}
}

// parseCommand memecah "/status vps-1@BotName" menjadi ("status", "vps-1").
// Sufiks @NamaBot dipakai Telegram di grup, tapi klien juga menyisipkannya di
// chat privat pada beberapa alur, jadi tetap dibuang di sini.
func parseCommand(text string) (cmd, arg string) {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(t, "/") {
		return "", t
	}
	t = strings.TrimPrefix(t, "/")
	head, rest, _ := strings.Cut(t, " ")
	if at := strings.IndexByte(head, '@'); at >= 0 {
		head = head[:at]
	}
	return strings.ToLower(strings.TrimSpace(head)), strings.TrimSpace(rest)
}

// resolveServer mencocokkan argumen user ke satu server: cocok persis dulu
// (tanpa peduli besar-kecil huruf), baru cocok sebagian.
func resolveServer(list []ServerInfo, arg string) (ServerInfo, error) {
	needle := strings.ToLower(strings.TrimSpace(arg))

	for _, s := range list {
		if strings.EqualFold(s.Name, needle) || s.ID == arg || strings.EqualFold(s.Host, needle) {
			return s, nil
		}
	}

	var partial []ServerInfo
	for _, s := range list {
		if strings.Contains(strings.ToLower(s.Name), needle) {
			partial = append(partial, s)
		}
	}
	switch len(partial) {
	case 1:
		return partial[0], nil
	case 0:
		return ServerInfo{}, fmt.Errorf("No server named %q.", arg)
	default:
		return ServerInfo{}, fmt.Errorf("%q matches %d servers.", arg, len(partial))
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// ErrNoBackend dipakai backend tiruan/awal yang belum punya data.
var ErrNoBackend = errors.New("backend belum siap")
