// Package telegram adalah klien Bot API seperlunya untuk poinhost-agent:
// long-polling getUpdates plus perintah kirim/ubah pesan.
//
// Ditulis tangan alih-alih memakai library pihak ketiga karena permukaan yang
// dipakai sangat sempit (enam metode), sedangkan agent adalah binary yang
// dikirim ke server produksi user — makin sedikit dependensi yang ikut, makin
// sedikit yang harus diikuti kalau ada advisory keamanan.
//
// Long-polling dipilih supaya TIDAK ada port masuk yang perlu dibuka: agent
// yang menelepon keluar ke api.telegram.org, bukan sebaliknya.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// APIBase adalah endpoint Bot API resmi.
const APIBase = "https://api.telegram.org"

// Client memanggil Bot API untuk satu token.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// New membuat klien dengan timeout yang cocok untuk long-polling.
func New(token string) *Client {
	return NewWithBase(token, APIBase)
}

// NewWithBase memungkinkan mengarahkan klien ke server lain — dipakai test
// untuk menjalankan Bot API tiruan tanpa menyentuh jaringan.
func NewWithBase(token, baseURL string) *Client {
	return &Client{
		token:   token,
		baseURL: strings.TrimRight(baseURL, "/"),
		// Timeout harus lebih longgar dari timeout long-poll terpanjang,
		// kalau tidak setiap getUpdates yang sehat justru diputus klien.
		http: &http.Client{Timeout: 90 * time.Second},
	}
}

// User adalah akun Telegram (bot maupun manusia).
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// DisplayName mengembalikan nama yang enak dibaca manusia.
func (u User) DisplayName() string {
	if u.Username != "" {
		return "@" + u.Username
	}
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name != "" {
		return name
	}
	return fmt.Sprintf("user %d", u.ID)
}

// Chat adalah percakapan. Type "private" adalah satu-satunya yang dilayani
// agent — lihat agentcfg.Telegram.AllowedUserIDs.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// Message adalah satu pesan masuk.
type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from"`
	Chat      *Chat  `json:"chat"`
	Text      string `json:"text"`
}

// CallbackQuery adalah penekanan tombol inline.
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *User    `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

// Update adalah satu entri dari getUpdates.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

// InlineKeyboardButton adalah satu tombol di bawah pesan.
type InlineKeyboardButton struct {
	Text string `json:"text"`
	// CallbackData dibatasi 64 byte oleh Telegram. Pemanggil bertanggung
	// jawab menjaga batas itu; lihat bot.serverButton.
	CallbackData string `json:"callback_data,omitempty"`
}

// InlineKeyboard adalah susunan tombol per baris.
type InlineKeyboard struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// BotCommand adalah satu entri menu perintah.
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

// Error adalah kegagalan yang dilaporkan Bot API sendiri (bukan jaringan).
type Error struct {
	Code        int
	Description string
	Method      string
}

func (e *Error) Error() string {
	return fmt.Sprintf("telegram %s: %d %s", e.Method, e.Code, e.Description)
}

// IsUnauthorized melaporkan apakah error berarti tokennya salah/dicabut.
func IsUnauthorized(err error) bool {
	var te *Error
	if !errors.As(err, &te) {
		return false
	}
	return te.Code == 401
}

func (c *Client) call(ctx context.Context, method string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}

	url := fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("telegram %s: baca respons: %w", method, err)
	}

	var api apiResponse
	if err := json.Unmarshal(raw, &api); err != nil {
		return fmt.Errorf("telegram %s: respons tidak dikenali (HTTP %d)", method, resp.StatusCode)
	}
	if !api.OK {
		return &Error{Code: api.ErrorCode, Description: api.Description, Method: method}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(api.Result, out)
}

// GetMe memverifikasi token sekaligus mengembalikan identitas bot.
func (c *Client) GetMe(ctx context.Context) (*User, error) {
	var u User
	if err := c.call(ctx, "getMe", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUpdates menunggu update baru sampai timeoutSeconds (long-polling).
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	payload := map[string]any{
		"offset":  offset,
		"timeout": timeoutSeconds,
		// Hanya dua jenis update yang dipakai. Membatasinya di sisi server
		// Telegram menghemat lalu lintas dan membuat agent tidak perlu
		// mengabaikan jenis lain satu per satu.
		"allowed_updates": []string{"message", "callback_query"},
	}
	var out []Update
	if err := c.call(ctx, "getUpdates", payload, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SendMessage mengirim pesan teks, opsional dengan tombol inline.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, kb *InlineKeyboard) (*Message, error) {
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
		// Status server sering memuat host/IP; pratinjau tautan hanya
		// menambah keramaian di chat.
		"link_preview_options": map[string]any{"is_disabled": true},
	}
	if kb != nil {
		payload["reply_markup"] = kb
	}
	var m Message
	if err := c.call(ctx, "sendMessage", payload, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// EditMessageText mengganti isi pesan di tempat — dipakai tombol Refresh
// supaya chat tidak dipenuhi salinan status yang sama.
func (c *Client) EditMessageText(ctx context.Context, chatID, messageID int64, text string, kb *InlineKeyboard) error {
	payload := map[string]any{
		"chat_id":              chatID,
		"message_id":           messageID,
		"text":                 text,
		"parse_mode":           "HTML",
		"link_preview_options": map[string]any{"is_disabled": true},
	}
	if kb != nil {
		payload["reply_markup"] = kb
	}
	err := c.call(ctx, "editMessageText", payload, nil)
	if isNotModified(err) {
		// Telegram menolak edit yang isinya persis sama. Itu bukan kegagalan
		// dari sudut pandang user — tombol Refresh yang ditekan dua kali
		// dalam satu detik memang wajar menghasilkan teks identik.
		return nil
	}
	return err
}

// AnswerCallbackQuery menghentikan indikator loading pada tombol inline.
func (c *Client) AnswerCallbackQuery(ctx context.Context, id, text string) error {
	payload := map[string]any{"callback_query_id": id}
	if text != "" {
		payload["text"] = text
	}
	return c.call(ctx, "answerCallbackQuery", payload, nil)
}

// SetMyCommands mendaftarkan menu perintah, yang membuat Telegram
// memunculkan daftar lengkap dengan autocomplete begitu user mengetik "/".
func (c *Client) SetMyCommands(ctx context.Context, cmds []BotCommand) error {
	return c.call(ctx, "setMyCommands", map[string]any{"commands": cmds}, nil)
}

func isNotModified(err error) bool {
	var te *Error
	if !errors.As(err, &te) {
		return false
	}
	return strings.Contains(strings.ToLower(te.Description), "message is not modified")
}
