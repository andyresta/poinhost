package services

import (
	"strings"
	"testing"
)

// Keluaran `journalctl -o json` yang ditiru di sini mencakup kasus yang
// bikin parser teks biasa gagal: pesan berisi tab dan tanda kutip, pesan
// non-UTF8 yang dikirim sebagai array angka, PRIORITY yang datang sebagai
// angka (bukan string), entri tanpa _SYSTEMD_UNIT, dan baris non-JSON yang
// kadang ikut terbawa.
const sampleJournal = `-- No entries --
{"__REALTIME_TIMESTAMP":"1757942400000000","PRIORITY":"3","_SYSTEMD_UNIT":"nginx.service","SYSLOG_IDENTIFIER":"nginx","_PID":"812","MESSAGE":"bind() to 0.0.0.0:80 failed\t(98: \"Address already in use\")"}
{"__REALTIME_TIMESTAMP":"1757942401000000","PRIORITY":6,"_SYSTEMD_UNIT":"cron.service","SYSLOG_IDENTIFIER":"CRON","MESSAGE":"pam_unix(cron:session): session opened"}
{"__REALTIME_TIMESTAMP":"1757942402000000","PRIORITY":"4","SYSLOG_IDENTIFIER":"kernel","MESSAGE":[72,105,0,33]}
not json at all
`

func TestParseJournalHandlesAwkwardFields(t *testing.T) {
	out := parseJournal(sampleJournal)
	if !out.SystemdAvailable {
		t.Fatal("journalctl seharusnya terdeteksi ada")
	}
	if len(out.Entries) != 3 {
		t.Fatalf("baris non-JSON harus diabaikan, dapat %d entri: %+v", len(out.Entries), out.Entries)
	}

	// Pesan dengan tab dan tanda kutip harus utuh — inilah alasan memakai
	// -o json, bukan parsing kolom teks.
	first := out.Entries[0]
	if !strings.Contains(first.Message, "\t(98: \"Address already in use\")") {
		t.Fatalf("pesan berisi tab/kutip rusak: %q", first.Message)
	}
	if first.Priority != 3 {
		t.Fatalf("PRIORITY string tidak terbaca: %+v", first)
	}
	// Sufiks ".service" dibuang supaya sama dengan penamaan di daftar unit.
	if first.Unit != "nginx" {
		t.Fatalf("unit seharusnya 'nginx', dapat %q", first.Unit)
	}
	if first.Timestamp == "" {
		t.Fatal("timestamp mikrodetik gagal dikonversi")
	}

	// PRIORITY kadang datang sebagai ANGKA, bukan string.
	if out.Entries[1].Priority != 6 {
		t.Fatalf("PRIORITY numerik tidak terbaca: %+v", out.Entries[1])
	}

	// Pesan non-UTF8 datang sebagai array angka.
	third := out.Entries[2]
	if third.Message != "Hi\x00!" {
		t.Fatalf("pesan array-byte salah decode: %q", third.Message)
	}
	// Tanpa _SYSTEMD_UNIT, identifier dipakai sebagai penanda asal.
	if third.Unit != "kernel" {
		t.Fatalf("fallback ke SYSLOG_IDENTIFIER gagal: %+v", third)
	}
}

func TestParseJournalDetectsMissingJournalctl(t *testing.T) {
	out := parseJournal("__NO_JOURNAL__\n")
	if out.SystemdAvailable {
		t.Fatal("seharusnya melaporkan journalctl tidak tersedia")
	}
	if len(out.Entries) != 0 {
		t.Fatalf("tidak boleh ada entri, dapat %d", len(out.Entries))
	}
}

// Seluruh nilai yang masuk ke baris perintah journalctl berasal dari
// whitelist. Tes ini menjaga agar string bebas dari frontend tidak pernah
// lolos ke shell.
func TestBuildJournalArgsRejectsValuesOutsideWhitelist(t *testing.T) {
	if _, err := buildJournalArgs(JournalRequest{Priority: "err; rm -rf /"}, 100); err == nil {
		t.Fatal("priority di luar whitelist seharusnya ditolak")
	}
	if _, err := buildJournalArgs(JournalRequest{Since: "$(id)"}, 100); err == nil {
		t.Fatal("since di luar whitelist seharusnya ditolak")
	}
	if _, err := buildJournalArgs(JournalRequest{Unit: "nginx && reboot"}, 100); err == nil {
		t.Fatal("nama unit tidak valid seharusnya ditolak")
	}
}

func TestBuildJournalArgs(t *testing.T) {
	// Unit kosong = seluruh sistem: tidak boleh ada flag -u sama sekali.
	args, err := buildJournalArgs(JournalRequest{Since: "1h"}, 300)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(args, "-u ") {
		t.Fatalf("unit kosong tidak boleh menghasilkan -u: %q", args)
	}
	if !strings.Contains(args, "--since '1 hour ago'") || !strings.Contains(args, "-n 300") {
		t.Fatalf("argumen tidak sesuai: %q", args)
	}

	// "boot" memakai flag -b, bukan --since.
	args, err = buildJournalArgs(JournalRequest{Unit: "nginx", Priority: "err", Since: "boot"}, 500)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-u 'nginx.service'", "-p 3", "-b", "-n 500"} {
		if !strings.Contains(args, want) {
			t.Fatalf("argumen %q hilang dari %q", want, args)
		}
	}
	if strings.Contains(args, "--since") {
		t.Fatalf("mode boot tidak boleh memakai --since: %q", args)
	}
}

func TestClampJournalLines(t *testing.T) {
	if got := clampJournalLines(0); got != journalDefaultLines {
		t.Fatalf("0 seharusnya jadi default, dapat %d", got)
	}
	if got := clampJournalLines(5); got != journalMinLines {
		t.Fatalf("nilai terlalu kecil seharusnya dinaikkan, dapat %d", got)
	}
	if got := clampJournalLines(999999); got != journalMaxLines {
		t.Fatalf("nilai terlalu besar seharusnya dibatasi, dapat %d", got)
	}
}
