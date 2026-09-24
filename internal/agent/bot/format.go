package bot

import (
	"fmt"
	"strings"
	"time"

	"github.com/andyresta/poinhost/internal/agent/telegram"
)

// Jenis callback tombol inline. Prefiksnya sengaja dua huruf: Telegram
// membatasi callback_data 64 byte, dan ID server berupa UUID sudah memakan 36.
const (
	cbStatus  = "st"
	cbRefresh = "rf"
)

func callbackData(kind, serverID string) string {
	return kind + ":" + serverID
}

func parseCallback(data string) (kind, serverID string, ok bool) {
	k, id, found := strings.Cut(data, ":")
	if !found || id == "" {
		return "", "", false
	}
	if k != cbStatus && k != cbRefresh {
		return "", "", false
	}
	return k, id, true
}

// serverKeyboard membuat satu tombol per server, satu per baris supaya nama
// panjang tetap terbaca di layar HP.
func serverKeyboard(list []ServerInfo, kind string) *telegram.InlineKeyboard {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(list))
	for _, s := range list {
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text:         connectionEmoji(s, Status{}) + " " + s.Name,
			CallbackData: callbackData(kind, s.ID),
		}})
	}
	return &telegram.InlineKeyboard{InlineKeyboard: rows}
}

func msgHelp() string {
	return strings.Join([]string{
		"<b>poinhost agent</b>",
		"",
		"/servers — list managed servers with a quick status",
		"/status — CPU, memory and disk for one server",
		"/help — show this message",
		"",
		"This bot is read-only: it cannot change anything on your servers.",
	}, "\n")
}

func msgNotAuthorized(userID int64) string {
	return fmt.Sprintf(
		"You are not authorised to use this bot.\n\nYour Telegram user ID is <code>%d</code>. "+
			"To get access, open poinhost → Agent Control, generate a pairing code, "+
			"then send <code>/pair &lt;code&gt;</code> here.",
		userID)
}

// formatServerLine adalah satu entri di /servers.
func formatServerLine(srv ServerInfo, st Status) string {
	var sb strings.Builder

	name := esc(srv.Name)
	if srv.IsSelf {
		name += " <i>(this agent)</i>"
	}
	fmt.Fprintf(&sb, "%s <b>%s</b>\n", connectionEmoji(srv, st), name)
	fmt.Fprintf(&sb, "   <code>%s</code>\n", esc(srv.Host))

	if st.Connection != "" && st.Connection != "online" {
		fmt.Fprintf(&sb, "   %s\n", esc(connectionLabel(st.Connection)))
		return sb.String()
	}
	if !hasMetrics(st) {
		sb.WriteString("   no metrics yet\n")
		return sb.String()
	}

	fmt.Fprintf(&sb, "   CPU %s · RAM %s · Disk %s\n",
		pct(st.CPUUsedPct),
		ratioGB(float64(st.MemUsedMB)/1024, float64(st.MemTotalMB)/1024),
		ratioGB(st.DiskUsedGB, st.DiskTotalGB))
	return sb.String()
}

// formatStatusDetail adalah balasan /status untuk satu server.
func formatStatusDetail(srv ServerInfo, st Status) string {
	var sb strings.Builder

	name := esc(srv.Name)
	if srv.IsSelf {
		name += " <i>(this agent)</i>"
	}
	fmt.Fprintf(&sb, "%s <b>%s</b> · <code>%s</code>\n", connectionEmoji(srv, st), name, esc(srv.Host))

	if st.OSName != "" || st.UptimeSeconds > 0 {
		parts := make([]string, 0, 2)
		if st.OSName != "" {
			parts = append(parts, esc(st.OSName))
		}
		if st.UptimeSeconds > 0 {
			parts = append(parts, "up "+uptime(st.UptimeSeconds))
		}
		sb.WriteString(strings.Join(parts, " · ") + "\n")
	}

	if st.Connection != "" && st.Connection != "online" {
		fmt.Fprintf(&sb, "\n%s\n", esc(connectionLabel(st.Connection)))
	}

	if hasMetrics(st) {
		sb.WriteString("\n<pre>")
		fmt.Fprintf(&sb, "CPU   %s %s%s\n", bar(st.CPUUsedPct), pct(st.CPUUsedPct), cores(st.CPUCores))
		fmt.Fprintf(&sb, "RAM   %s %s\n",
			bar(percentOf(float64(st.MemUsedMB), float64(st.MemTotalMB))),
			ratioGB(float64(st.MemUsedMB)/1024, float64(st.MemTotalMB)/1024))
		fmt.Fprintf(&sb, "Disk  %s %s",
			bar(percentOf(st.DiskUsedGB, st.DiskTotalGB)),
			ratioGB(st.DiskUsedGB, st.DiskTotalGB))
		sb.WriteString("</pre>\n")
	} else if st.Connection == "online" {
		sb.WriteString("\nNo metrics collected yet.\n")
	}

	// Error metrik ditampilkan APA ADANYA dan tidak menggantikan angka lama:
	// nilai terakhir yang berhasil tetap berguna, asalkan jelas bahwa itu
	// bukan angka terbaru.
	if st.Error != "" {
		fmt.Fprintf(&sb, "\n⚠️ %s\n", esc(st.Error))
	}

	if !st.CheckedAt.IsZero() {
		suffix := ""
		if st.Stale {
			suffix = " (stale)"
		}
		fmt.Fprintf(&sb, "\n<i>checked %s%s</i>", since(st.CheckedAt), suffix)
	}
	return sb.String()
}

func hasMetrics(st Status) bool {
	return st.MemTotalMB > 0 || st.DiskTotalGB > 0 || st.CPUCores > 0
}

func connectionEmoji(srv ServerInfo, st Status) string {
	switch st.Connection {
	case "online":
		return "🟢"
	case "reconnecting":
		return "🟡"
	case "offline":
		return "🔴"
	default:
		_ = srv
		return "⚪"
	}
}

func connectionLabel(conn string) string {
	switch conn {
	case "offline":
		return "not connected"
	case "reconnecting":
		return "reconnecting…"
	case "":
		return "status unknown"
	default:
		return conn
	}
}

func percentOf(used, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return used / total * 100
}

func pct(v float64) string {
	if v < 0 {
		v = 0
	}
	return fmt.Sprintf("%.0f%%", v)
}

func cores(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf(" (%d core%s)", n, plural(n))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func ratioGB(used, total float64) string {
	if total <= 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f/%.1f GB", used, total)
}

// bar adalah meter sepuluh petak. Dipakai di dalam <pre> supaya lebarnya
// tetap rata di semua klien Telegram.
func bar(pct float64) string {
	const width = 10
	switch {
	case pct < 0:
		pct = 0
	case pct > 100:
		pct = 100
	}
	filled := int(pct/100*width + 0.5)
	return "[" + strings.Repeat("#", filled) + strings.Repeat(".", width-filled) + "]"
}

func uptime(sec int64) string {
	d := time.Duration(sec) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

func since(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

// esc melindungi teks yang berasal dari data user (nama server, host, pesan
// error) supaya tidak merusak parse_mode HTML Telegram — nama server bebas
// diketik user dan bisa saja memuat "<" atau "&".
func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
