package bot

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andyresta/poinhost/internal/agent/telegram"
)

// --- test double ---

type fakeAPI struct {
	mu        sync.Mutex
	sent      []string
	edited    []string
	answered  []string
	keyboards []*telegram.InlineKeyboard
}

func (f *fakeAPI) GetMe(context.Context) (*telegram.User, error) {
	return &telegram.User{ID: 1, IsBot: true, Username: "uji_bot"}, nil
}
func (f *fakeAPI) GetUpdates(context.Context, int64, int) ([]telegram.Update, error) { return nil, nil }
func (f *fakeAPI) SetMyCommands(context.Context, []telegram.BotCommand) error        { return nil }

func (f *fakeAPI) SendMessage(_ context.Context, _ int64, text string, kb *telegram.InlineKeyboard) (*telegram.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, text)
	f.keyboards = append(f.keyboards, kb)
	return &telegram.Message{MessageID: int64(len(f.sent))}, nil
}

func (f *fakeAPI) EditMessageText(_ context.Context, _, _ int64, text string, _ *telegram.InlineKeyboard) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.edited = append(f.edited, text)
	return nil
}

func (f *fakeAPI) AnswerCallbackQuery(_ context.Context, _, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answered = append(f.answered, text)
	return nil
}

func (f *fakeAPI) lastSent() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 {
		return ""
	}
	return f.sent[len(f.sent)-1]
}

type fakeBackend struct {
	servers    []ServerInfo
	statuses   map[string]Status
	refreshed  []string
	refreshErr error
}

func (b *fakeBackend) ListServers() ([]ServerInfo, error) { return b.servers, nil }

func (b *fakeBackend) Server(id string) (ServerInfo, bool) {
	for _, s := range b.servers {
		if s.ID == id {
			return s, true
		}
	}
	return ServerInfo{}, false
}

func (b *fakeBackend) Status(id string) (Status, bool) {
	st, ok := b.statuses[id]
	return st, ok
}

func (b *fakeBackend) Refresh(_ context.Context, id string) (Status, error) {
	b.refreshed = append(b.refreshed, id)
	if b.refreshErr != nil {
		return Status{}, b.refreshErr
	}
	return b.statuses[id], nil
}

type fakeAllow struct {
	ids   map[int64]bool
	added []int64
	err   error
}

func (a *fakeAllow) IsAllowed(id int64) bool { return a.ids[id] }
func (a *fakeAllow) Count() int              { return len(a.ids) }
func (a *fakeAllow) Allow(id int64, _ string) error {
	if a.err != nil {
		return a.err
	}
	a.ids[id] = true
	a.added = append(a.added, id)
	return nil
}

func newTestBot(allowed ...int64) (*Bot, *fakeAPI, *fakeBackend, *fakeAllow) {
	api := &fakeAPI{}
	be := &fakeBackend{
		servers: []ServerInfo{
			{ID: "id-lokal", Name: "srv-lokal", Host: "127.0.0.1", IsSelf: true},
			{ID: "id-backup", Name: "Backup Devpoin", Host: "169.58.122.135"},
			{ID: "id-staging", Name: "staging-jkt", Host: "10.0.0.7"},
		},
		statuses: map[string]Status{
			"id-lokal": {
				Connection: "online", OSName: "Ubuntu 24.04", UptimeSeconds: 93600,
				CPUCores: 4, CPUUsedPct: 7, MemUsedMB: 1126, MemTotalMB: 4096,
				DiskUsedGB: 12, DiskTotalGB: 40, CheckedAt: time.Now().Add(-20 * time.Second),
			},
			"id-backup": {
				Connection: "online", OSName: "Debian 12", UptimeSeconds: 1123200,
				CPUCores: 8, CPUUsedPct: 12, MemUsedMB: 3277, MemTotalMB: 8192,
				DiskUsedGB: 42, DiskTotalGB: 80, CheckedAt: time.Now().Add(-40 * time.Second),
			},
			"id-staging": {Connection: "offline"},
		},
	}
	allow := &fakeAllow{ids: map[int64]bool{}}
	for _, id := range allowed {
		allow.ids[id] = true
	}
	return New(api, be, allow, NewPairing()), api, be, allow
}

func privateMsg(userID int64, text string) *telegram.Message {
	return &telegram.Message{
		MessageID: 1,
		From:      &telegram.User{ID: userID, FirstName: "Andy"},
		Chat:      &telegram.Chat{ID: userID, Type: "private"},
		Text:      text,
	}
}

// --- otorisasi ---

// Perintah dari user yang belum diizinkan tidak boleh dijalankan, dan
// balasannya harus memberi tahu jalan keluarnya (pairing) — kalau tidak, user
// yang sah pun buntu tanpa tahu harus berbuat apa.
func TestBot_MenolakUserTidakDiizinkan(t *testing.T) {
	b, api, _, _ := newTestBot()

	b.handleMessage(context.Background(), privateMsg(42, "/servers"))

	got := api.lastSent()
	if !strings.Contains(got, "not authorised") {
		t.Errorf("balasan = %q, mau penolakan", got)
	}
	if !strings.Contains(got, "42") {
		t.Error("balasan tidak menyebut user ID, user tidak bisa melakukan pairing manual")
	}
	if strings.Contains(got, "Backup Devpoin") {
		t.Error("daftar server bocor ke user yang tidak diizinkan")
	}
}

// Grup diabaikan TOTAL — tanpa balasan sama sekali. Di grup, chat_id adalah
// grupnya, jadi siapa pun anggota grup bisa memicu perintah; membalas juga
// mengungkap keberadaan bot infrastruktur ini ke seluruh anggota.
func TestBot_MengabaikanChatGrup(t *testing.T) {
	b, api, _, _ := newTestBot(7)

	m := privateMsg(7, "/servers")
	m.Chat = &telegram.Chat{ID: -100123, Type: "group"}
	b.handleMessage(context.Background(), m)

	if len(api.sent) != 0 {
		t.Errorf("bot membalas di grup: %q", api.sent)
	}
}

func TestBot_MengabaikanPesanDariBotLain(t *testing.T) {
	b, api, _, _ := newTestBot(7)

	m := privateMsg(7, "/servers")
	m.From.IsBot = true
	b.handleMessage(context.Background(), m)

	if len(api.sent) != 0 {
		t.Errorf("bot membalas bot lain: %q", api.sent)
	}
}

// --- /servers ---

func TestBot_ServersMenampilkanSemuaServerDanStatus(t *testing.T) {
	b, api, _, _ := newTestBot(7)

	b.handleMessage(context.Background(), privateMsg(7, "/servers"))

	got := api.lastSent()
	for _, mau := range []string{"Backup Devpoin", "srv-lokal", "staging-jkt", "(3)"} {
		if !strings.Contains(got, mau) {
			t.Errorf("keluaran tidak memuat %q:\n%s", mau, got)
		}
	}
	if !strings.Contains(got, "this agent") {
		t.Error("server tempat agent berjalan tidak ditandai")
	}
	if !strings.Contains(got, "not connected") {
		t.Error("server offline tidak ditandai sebagai tidak terhubung")
	}
	// Server offline tidak punya metrik; jangan tampilkan angka palsu.
	if strings.Contains(got, "0.0/0.0 GB") {
		t.Error("metrik kosong ditampilkan sebagai angka nol")
	}

	kb := api.keyboards[len(api.keyboards)-1]
	if kb == nil || len(kb.InlineKeyboard) != 3 {
		t.Fatalf("mau 3 tombol server, dapat %+v", kb)
	}
}

// --- /status ---

// Tanpa argumen, mengetik nama server di HP itu menyiksa — jadi yang muncul
// harus tombol, bukan permintaan mengetik.
func TestBot_StatusTanpaArgumenMenawarkanTombol(t *testing.T) {
	b, api, _, _ := newTestBot(7)

	b.handleMessage(context.Background(), privateMsg(7, "/status"))

	kb := api.keyboards[len(api.keyboards)-1]
	if kb == nil || len(kb.InlineKeyboard) != 3 {
		t.Fatalf("mau tombol pilihan server, dapat %+v", kb)
	}
	for _, row := range kb.InlineKeyboard {
		if len(row[0].CallbackData) > 64 {
			t.Errorf("callback_data %d byte, batas Telegram 64", len(row[0].CallbackData))
		}
	}
}

func TestBot_StatusDenganNamaMenampilkanDetail(t *testing.T) {
	b, api, _, _ := newTestBot(7)

	b.handleMessage(context.Background(), privateMsg(7, "/status Backup Devpoin"))

	got := api.lastSent()
	for _, mau := range []string{"Backup Devpoin", "Debian 12", "CPU", "RAM", "Disk", "3.2/8.0 GB", "42.0/80.0 GB"} {
		if !strings.Contains(got, mau) {
			t.Errorf("detail tidak memuat %q:\n%s", mau, got)
		}
	}
}

// Nama server boleh disebut sebagian, karena mengetik nama panjang di HP itu
// menyebalkan — tapi hanya kalau tidak ambigu.
func TestBot_StatusMenerimaNamaSebagian(t *testing.T) {
	b, api, _, _ := newTestBot(7)

	b.handleMessage(context.Background(), privateMsg(7, "/status backup"))

	if !strings.Contains(api.lastSent(), "Debian 12") {
		t.Errorf("pencocokan sebagian gagal:\n%s", api.lastSent())
	}
}

func TestBot_StatusNamaTidakDikenalMenawarkanPilihan(t *testing.T) {
	b, api, _, _ := newTestBot(7)

	b.handleMessage(context.Background(), privateMsg(7, "/status tidak-ada"))

	got := api.lastSent()
	if !strings.Contains(got, "No server named") {
		t.Errorf("balasan = %q", got)
	}
	if kb := api.keyboards[len(api.keyboards)-1]; kb == nil {
		t.Error("tidak menawarkan tombol pilihan setelah nama salah")
	}
}

// --- tombol inline ---

func TestBot_TombolRefreshMemaksaAmbilUlangDanMengeditPesan(t *testing.T) {
	b, api, be, _ := newTestBot(7)

	b.handleCallback(context.Background(), &telegram.CallbackQuery{
		ID:      "cb1",
		From:    &telegram.User{ID: 7},
		Message: &telegram.Message{MessageID: 9, Chat: &telegram.Chat{ID: 7, Type: "private"}},
		Data:    callbackData(cbRefresh, "id-backup"),
	})

	if len(be.refreshed) != 1 || be.refreshed[0] != "id-backup" {
		t.Errorf("Refresh dipanggil untuk %v, mau [id-backup]", be.refreshed)
	}
	if len(api.edited) != 1 {
		t.Fatalf("mau 1 editMessageText, dapat %d", len(api.edited))
	}
	if !strings.Contains(api.edited[0], "Backup Devpoin") {
		t.Errorf("hasil edit tidak memuat nama server:\n%s", api.edited[0])
	}
}

// Tombol status hanya membaca cache — tidak boleh memicu SSH.
func TestBot_TombolStatusTidakMemaksaRefresh(t *testing.T) {
	b, _, be, _ := newTestBot(7)

	b.handleCallback(context.Background(), &telegram.CallbackQuery{
		ID:      "cb2",
		From:    &telegram.User{ID: 7},
		Message: &telegram.Message{MessageID: 9, Chat: &telegram.Chat{ID: 7, Type: "private"}},
		Data:    callbackData(cbStatus, "id-backup"),
	})

	if len(be.refreshed) != 0 {
		t.Errorf("tombol status memicu Refresh: %v", be.refreshed)
	}
}

func TestBot_TombolDariUserTidakDiizinkanDitolak(t *testing.T) {
	b, api, be, _ := newTestBot()

	b.handleCallback(context.Background(), &telegram.CallbackQuery{
		ID:      "cb3",
		From:    &telegram.User{ID: 999},
		Message: &telegram.Message{MessageID: 9, Chat: &telegram.Chat{ID: 999, Type: "private"}},
		Data:    callbackData(cbRefresh, "id-backup"),
	})

	if len(be.refreshed) != 0 {
		t.Error("user tidak diizinkan berhasil memicu Refresh")
	}
	if len(api.edited) != 0 {
		t.Error("pesan status dikirim ke user tidak diizinkan")
	}
}

// --- pairing ---

func TestBot_PairingMemasukkanUserKeAllowlist(t *testing.T) {
	b, api, _, allow := newTestBot()
	code, _, err := b.pairing.Issue()
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	b.handleMessage(context.Background(), privateMsg(42, "/pair "+FormatCode(code)))

	if !allow.ids[42] {
		t.Fatalf("user tidak masuk allowlist; balasan: %q", api.lastSent())
	}
	if !strings.Contains(api.lastSent(), "Paired") {
		t.Errorf("balasan = %q", api.lastSent())
	}
}

// Satu kode hanya boleh memasukkan satu orang.
func TestBot_KodePairingHanyaSekaliPakai(t *testing.T) {
	b, _, _, allow := newTestBot()
	code, _, _ := b.pairing.Issue()

	b.handleMessage(context.Background(), privateMsg(42, "/pair "+code))
	b.handleMessage(context.Background(), privateMsg(43, "/pair "+code))

	if allow.ids[43] {
		t.Error("kode pairing yang sama berhasil dipakai dua kali")
	}
}

func TestBot_PairingKodeSalahTidakMemberiAkses(t *testing.T) {
	b, api, _, allow := newTestBot()
	_, _, _ = b.pairing.Issue()

	b.handleMessage(context.Background(), privateMsg(42, "/pair PH-AAAA-BBBB"))

	if allow.ids[42] {
		t.Fatal("kode salah memberi akses")
	}
	if !strings.Contains(api.lastSent(), "not valid") {
		t.Errorf("balasan = %q", api.lastSent())
	}
}

// Kalau penyimpanan allowlist gagal, user TIDAK boleh dianggap sudah pairing:
// aksesnya akan hilang diam-diam saat agent restart.
func TestBot_PairingGagalSimpanDilaporkan(t *testing.T) {
	b, api, _, allow := newTestBot()
	allow.err = errors.New("disk penuh")
	code, _, _ := b.pairing.Issue()

	b.handleMessage(context.Background(), privateMsg(42, "/pair "+code))

	if !strings.Contains(api.lastSent(), "saving the change failed") {
		t.Errorf("balasan = %q, mau memberi tahu penyimpanan gagal", api.lastSent())
	}
}

// --- parsing ---

func TestParseCommand(t *testing.T) {
	cases := []struct{ in, cmd, arg string }{
		{"/servers", "servers", ""},
		{"/status  Backup Devpoin ", "status", "Backup Devpoin"},
		{"/status@poinhost_bot vps1", "status", "vps1"},
		{"/STATUS vps1", "status", "vps1"},
		{"halo", "", "halo"},
	}
	for _, c := range cases {
		cmd, arg := parseCommand(c.in)
		if cmd != c.cmd || arg != c.arg {
			t.Errorf("parseCommand(%q) = (%q,%q), mau (%q,%q)", c.in, cmd, arg, c.cmd, c.arg)
		}
	}
}

// Nama server diketik bebas oleh user dan bisa memuat karakter yang merusak
// parse_mode HTML Telegram — yang bukan cuma bikin jelek, tapi membuat
// Telegram menolak seluruh pesannya.
func TestFormat_MeloloskanKarakterHTMLDiNamaServer(t *testing.T) {
	srv := ServerInfo{ID: "x", Name: "prod <b>& staging", Host: "10.0.0.1"}
	got := formatStatusDetail(srv, Status{Connection: "online"})

	if strings.Contains(got, "<b>& staging") {
		t.Errorf("nama server tidak di-escape:\n%s", got)
	}
	if !strings.Contains(got, "&lt;b&gt;&amp; staging") {
		t.Errorf("hasil escape tidak sesuai:\n%s", got)
	}
}
