package services

// Pembacaan journald (`journalctl`) untuk modul Services.
//
// Alasan fitur ini menempel di modul Services, bukan berdiri sendiri: daftar
// unit bisa menunjukkan sebuah service "failed", tapi tanpa lognya user tidak
// punya cara tahu SEBABNYA selain membuka Terminal manual. Jadi log-nya
// sengaja sedekat mungkin dengan daftar service-nya.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// JournalEntry satu baris log journald yang sudah terstruktur.
type JournalEntry struct {
	// Timestamp RFC3339 hasil konversi __REALTIME_TIMESTAMP (mikrodetik
	// sejak epoch). Dikirim sebagai string, bukan time.Time, supaya tidak
	// ada penafsiran ulang zona waktu di sisi frontend.
	Timestamp  string `json:"timestamp"`
	Priority   int    `json:"priority"`
	Unit       string `json:"unit"`
	Identifier string `json:"identifier"`
	PID        string `json:"pid"`
	Message    string `json:"message"`
}

// JournalRequest parameter pembacaan log. Semua field yang masuk ke perintah
// shell divalidasi lewat whitelist, bukan di-escape saja.
type JournalRequest struct {
	ServerID string `json:"serverId"`
	// Unit kosong = seluruh sistem (bukan satu service).
	Unit string `json:"unit"`
	// Priority: "" (semua) atau salah satu kunci journalPriorities.
	Priority string `json:"priority"`
	// Since: salah satu kunci journalSince.
	Since string `json:"since"`
	Lines int    `json:"lines"`
}

// JournalResponse hasil ReadJournal.
type JournalResponse struct {
	Entries          []JournalEntry `json:"entries"`
	SystemdAvailable bool           `json:"systemdAvailable"`
	// Truncated true kalau jumlah baris yang kembali sama dengan batas yang
	// diminta — artinya kemungkinan besar masih ada yang lebih lama lagi.
	Truncated bool `json:"truncated"`
}

// journalPriorities memetakan nama level ke angka syslog. Dipakai sebagai
// WHITELIST: nilai di luar peta ini ditolak, jadi tidak ada string bebas dari
// frontend yang sampai ke baris perintah.
var journalPriorities = map[string]string{
	"":        "",
	"emerg":   "0",
	"alert":   "1",
	"crit":    "2",
	"err":     "3",
	"warning": "4",
	"notice":  "5",
	"info":    "6",
	"debug":   "7",
}

// journalSince juga whitelist. Nilainya memakai bentuk kalimat yang memang
// didokumentasikan journalctl ("1 hour ago"), bukan singkatan seperti "-1h"
// yang dukungannya berbeda antar versi systemd.
var journalSince = map[string]string{
	"boot": "", // ditangani khusus dengan flag -b
	"15m":  "15 min ago",
	"1h":   "1 hour ago",
	"6h":   "6 hours ago",
	"24h":  "1 day ago",
	"7d":   "7 days ago",
}

const (
	journalMinLines     = 50
	journalMaxLines     = 2000
	journalDefaultLines = 200
)

func clampJournalLines(n int) int {
	if n <= 0 {
		return journalDefaultLines
	}
	if n < journalMinLines {
		return journalMinLines
	}
	if n > journalMaxLines {
		return journalMaxLines
	}
	return n
}

// buildJournalArgs menyusun potongan argumen journalctl dari request yang
// SUDAH divalidasi. Dipisah dari pemanggilan SSH supaya bisa diuji langsung —
// di sinilah salah kutip/salah flag paling mungkin terjadi.
func buildJournalArgs(req JournalRequest, lines int) (string, error) {
	var args []string

	if strings.TrimSpace(req.Unit) != "" {
		unit, err := normalizeUnitName(req.Unit)
		if err != nil {
			return "", err
		}
		args = append(args, "-u "+shellQuote(unit))
	}

	prio, ok := journalPriorities[req.Priority]
	if !ok {
		return "", fmt.Errorf("level log tidak dikenal: %s", req.Priority)
	}
	if prio != "" {
		// -p N = level N DAN yang lebih gawat, sesuai konvensi journalctl.
		args = append(args, "-p "+prio)
	}

	since := req.Since
	if since == "" {
		since = "1h"
	}
	sinceVal, ok := journalSince[since]
	if !ok {
		return "", fmt.Errorf("rentang waktu tidak dikenal: %s", since)
	}
	if since == "boot" {
		args = append(args, "-b")
	} else {
		args = append(args, "--since "+shellQuote(sinceVal))
	}

	args = append(args, "-n "+strconv.Itoa(lines))
	return strings.Join(args, " "), nil
}

// journalFields membatasi field yang dikirim journald.
//
// Ini BUKAN optimasi spekulatif: `-o json` polos mengirim SELURUH metadata
// tiap entri (_CMDLINE, _EXE, _SYSTEMD_CGROUP, _CAP_EFFECTIVE, dan puluhan
// lainnya), sehingga 500 baris bisa lebih dari 1 MB dan permintaannya timeout
// di server yang sibuk. Dengan dibatasi, payload-nya tinggal sekitar
// sepersepuluh. (journalctl tetap selalu menyertakan __CURSOR dan
// __REALTIME_TIMESTAMP di luar daftar ini.)
const journalFields = "__REALTIME_TIMESTAMP,PRIORITY,_SYSTEMD_UNIT,SYSLOG_IDENTIFIER,_PID,MESSAGE"

// journalScript: cek ketersediaan dulu, baru baca. Formatnya `-o json` (satu
// objek JSON per baris) — SENGAJA bukan format teks, karena pesan log bebas
// mengandung spasi, tab, bahkan newline, sehingga parsing teks kolom selalu
// rapuh.
//
// --output-fields baru ada sejak systemd 236, jadi dukungannya diprobe dulu
// daripada mengasumsi versi systemd di server orang lain. Probe-nya memakai
// `-n 1`, BUKAN `-n 0`: nol tidak diterima semua versi, dan probe yang gagal
// akan diam-diam mematikan pembatasan field di SEMUA server — persis masalah
// yang sedang diperbaiki di sini.
func journalScript(args string) string {
	return `set +e
if ! command -v journalctl >/dev/null 2>&1; then echo "__NO_JOURNAL__"; exit 0; fi
FIELDS=""
if journalctl --output-fields=MESSAGE --no-pager -n 1 >/dev/null 2>&1; then
  FIELDS="--output-fields=` + journalFields + `"
fi
journalctl --no-pager -o json $FIELDS ` + args + ` 2>/dev/null
exit 0`
}

// ReadJournal membaca log journald sesuai filter.
func (s *Service) ReadJournal(req JournalRequest) (*JournalResponse, error) {
	if strings.TrimSpace(req.ServerID) == "" {
		return nil, fmt.Errorf("id server wajib diisi")
	}
	lines := clampJournalLines(req.Lines)
	args, err := buildJournalArgs(req, lines)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return nil, err
	}
	res, err := s.run(access, journalScript(args), 45*time.Second)
	if err != nil {
		return nil, err
	}
	out := parseJournal(res.Stdout)
	out.Truncated = len(out.Entries) >= lines
	return out, nil
}

// StreamJournal mengikuti log baru secara realtime (`journalctl -f`).
//
// onEntry dipanggil per baris yang berhasil di-parse; baris yang tidak
// berbentuk JSON (mis. catatan "-- No entries --") diabaikan diam-diam
// daripada memunculkan error palsu ke UI.
func (s *Service) StreamJournal(ctx context.Context, req JournalRequest, onEntry func(JournalEntry) error) error {
	if strings.TrimSpace(req.ServerID) == "" {
		return fmt.Errorf("id server wajib diisi")
	}
	// Follow selalu mulai dari 0 baris riwayat: snapshot-nya sudah diambil
	// lebih dulu lewat ReadJournal, jadi mengulangnya di sini akan membuat
	// baris yang sama muncul dua kali di layar.
	args, err := buildJournalArgs(req, 0)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}
	// Follow memakai --output-fields tanpa probe: stream hanya dipakai dari UI
	// yang snapshot-nya sudah berhasil lebih dulu, dan kalau flag-nya tidak
	// didukung journalctl gagal cepat sehingga UI menampilkan errornya.
	cmd := "journalctl --no-pager -o json --output-fields=" + journalFields + " -f " + args + " 2>/dev/null"
	return s.executor.ExecStreamDedicated(ctx, access.serverID, access.wrap(cmd), func(line string) error {
		entry, ok := parseJournalLine(line)
		if !ok {
			return nil
		}
		return onEntry(entry)
	})
}

// parseJournal dipisah supaya bisa diuji tanpa SSH.
func parseJournal(stdout string) *JournalResponse {
	out := &JournalResponse{Entries: []JournalEntry{}, SystemdAvailable: true}
	if strings.Contains(stdout, "__NO_JOURNAL__") {
		out.SystemdAvailable = false
		return out
	}
	for _, line := range strings.Split(stdout, "\n") {
		if entry, ok := parseJournalLine(line); ok {
			out.Entries = append(out.Entries, entry)
		}
	}
	return out
}

func parseJournalLine(line string) (JournalEntry, bool) {
	line = strings.TrimSpace(strings.TrimRight(line, "\r"))
	if line == "" || !strings.HasPrefix(line, "{") {
		return JournalEntry{}, false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return JournalEntry{}, false
	}

	entry := JournalEntry{
		Message:    journalField(raw, "MESSAGE"),
		Unit:       strings.TrimSuffix(journalField(raw, "_SYSTEMD_UNIT"), ".service"),
		Identifier: journalField(raw, "SYSLOG_IDENTIFIER"),
		PID:        journalField(raw, "_PID"),
		Priority:   6, // default "info" kalau field-nya tidak ada
	}
	if p := journalField(raw, "PRIORITY"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n >= 0 && n <= 7 {
			entry.Priority = n
		}
	}
	if ts := journalField(raw, "__REALTIME_TIMESTAMP"); ts != "" {
		if usec, err := strconv.ParseInt(ts, 10, 64); err == nil {
			entry.Timestamp = time.UnixMicro(usec).Format(time.RFC3339)
		}
	}
	if entry.Unit == "" {
		entry.Unit = entry.Identifier
	}
	return entry, true
}

// journalField membaca satu field journald yang tipenya TIDAK konsisten:
// biasanya string, kadang angka (PRIORITY), dan untuk pesan non-UTF8
// journald mengirimnya sebagai array byte. Ketiganya ditangani di sini
// supaya pemanggilnya tidak perlu tahu.
func journalField(raw map[string]json.RawMessage, key string) string {
	v, ok := raw[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(v, &n); err == nil {
		return n.String()
	}
	// Pesan non-UTF8 dikirim journald sebagai ARRAY ANGKA (mis.
	// [72,105,0,33]), bukan string base64 — jadi harus lewat []int, karena
	// unmarshal ke []byte hanya menerima base64.
	var nums []int
	if err := json.Unmarshal(v, &nums); err == nil {
		b := make([]byte, 0, len(nums))
		for _, x := range nums {
			if x >= 0 && x <= 255 {
				b = append(b, byte(x))
			}
		}
		return string(b)
	}
	return ""
}
