package sshpool

import (
	"strings"
	"testing"
)

// Perintah yang gagal sering menaruh sebab yang bisa ditindaklanjuti di
// stdout dan hanya kalimat umum di stderr — certbot adalah contoh yang
// paling sering terlihat user. Dulu stdout cuma dibaca kalau stderr
// kebetulan kosong, sehingga justru bagian pentingnya yang hilang.
func TestFailureDetail_KeepsStdoutEvenWhenStderrPresent(t *testing.T) {
	res := &ExecResult{
		Stdout: "Domain: api.example.com\nDetail: Invalid response ... 404",
		Stderr: "Some challenges have failed.",
	}
	got := FailureDetail(res)
	if !strings.Contains(got, "Detail: Invalid response") {
		t.Errorf("keterangan stdout hilang: %q", got)
	}
	if !strings.Contains(got, "Some challenges have failed.") {
		t.Errorf("keterangan stderr hilang: %q", got)
	}
}

// Banyak perintah menulis pesan yang sama ke kedua aliran; menggabungkannya
// mentah-mentah akan menampilkan kalimat yang sama dua kali.
func TestFailureDetail_DoesNotRepeatDuplicatedOutput(t *testing.T) {
	res := &ExecResult{Stdout: "gagal: izin ditolak", Stderr: "gagal: izin ditolak"}
	if got := FailureDetail(res); got != "gagal: izin ditolak" {
		t.Fatalf("FailureDetail = %q, mau tanpa pengulangan", got)
	}
}

func TestFailureDetail_SingleStreamAndEmpty(t *testing.T) {
	if got := FailureDetail(&ExecResult{Stderr: "  hanya stderr  "}); got != "hanya stderr" {
		t.Errorf("stderr saja: %q", got)
	}
	if got := FailureDetail(&ExecResult{Stdout: "hanya stdout"}); got != "hanya stdout" {
		t.Errorf("stdout saja: %q", got)
	}
	if got := FailureDetail(&ExecResult{}); got != "tidak ada keluaran" {
		t.Errorf("kosong: %q", got)
	}
	if got := FailureDetail(nil); got != "tidak ada keluaran" {
		t.Errorf("nil: %q", got)
	}
}
