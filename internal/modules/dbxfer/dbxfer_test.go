package dbxfer

import (
	"context"
	"strings"
	"testing"
)

func TestCompareRowCounts_Match(t *testing.T) {
	tables := []string{"users", "orders"}
	src := map[string]int64{"users": 10, "orders": 20}
	dst := map[string]int64{"users": 10, "orders": 20}
	verify, detail, _ := compareRowCounts(tables, src, dst)
	if verify != VerifyMatch {
		t.Fatalf("verify = %q, mau %q (%s)", verify, VerifyMatch, detail)
	}
	if !strings.Contains(detail, "30 baris") {
		t.Errorf("detail tidak menyebut total baris yang benar: %s", detail)
	}
}

// Verifikasi HARUS membandingkan ke sumber, bukan cuma memastikan tujuan
// tidak kosong (itu yang dilakukan homepoin) — kasus ini menangkap data
// yang hilang sebagian di satu tabel, sesuatu yang lolos dari pengecekan
// "0 tabel = gagal" milik homepoin.
func TestCompareRowCounts_Mismatch(t *testing.T) {
	tables := []string{"users", "orders"}
	src := map[string]int64{"users": 10, "orders": 20}
	dst := map[string]int64{"users": 10, "orders": 18} // 2 baris hilang saat restore
	verify, detail, _ := compareRowCounts(tables, src, dst)
	if verify != VerifyMismatch {
		t.Fatalf("verify = %q, mau %q", verify, VerifyMismatch)
	}
	if !strings.Contains(detail, "orders") || !strings.Contains(detail, "sumber=20") || !strings.Contains(detail, "tujuan=18") {
		t.Errorf("detail tidak menyebut tabel & angka yang bentrok: %s", detail)
	}
}

func TestCompareRowCounts_ManyMismatchesTruncated(t *testing.T) {
	tables := []string{"a", "b", "c", "d", "e"}
	src := map[string]int64{"a": 1, "b": 1, "c": 1, "d": 1, "e": 1}
	dst := map[string]int64{"a": 2, "b": 2, "c": 2, "d": 2, "e": 2}
	verify, detail, _ := compareRowCounts(tables, src, dst)
	if verify != VerifyMismatch {
		t.Fatalf("verify = %q, mau %q", verify, VerifyMismatch)
	}
	if !strings.Contains(detail, "5/5 tabel") || !strings.Contains(detail, "+2 lainnya") {
		t.Errorf("ringkasan mismatch banyak tabel salah: %s", detail)
	}
}

// Job dengan sebagian item gagal harus bisa dibedakan dari yang sukses
// penuh — homepoin melaporkan KEDUANYA sebagai status "done" di level atas,
// sehingga konsumen yang hanya membaca field status (bukan items[]) salah
// mengira migrasi sukses total walau sebagian database gagal.
func TestJob_PartialStatusIsTerminal(t *testing.T) {
	j := newJob(context.Background(), StartRequest{}, []ItemResult{
		{Database: "a", Status: ItemPending},
		{Database: "b", Status: ItemPending},
	}, nil)
	j.SetStatus(StatusPartial, "1 database berhasil, 1 gagal")
	if !j.IsTerminal() {
		t.Fatal("StatusPartial harus dianggap terminal")
	}
	snap := j.Snapshot()
	if snap.Status != StatusPartial {
		t.Fatalf("snap.Status = %q, mau %q", snap.Status, StatusPartial)
	}
	if snap.FinishedAt == nil {
		t.Error("FinishedAt harus terisi begitu status terminal")
	}
}

func TestJob_ItemOrderPreserved(t *testing.T) {
	j := newJob(context.Background(), StartRequest{}, []ItemResult{
		{Database: "zeta", Status: ItemPending},
		{Database: "alpha", Status: ItemPending},
	}, nil)
	snap := j.Snapshot()
	if len(snap.Items) != 2 || snap.Items[0].Database != "zeta" || snap.Items[1].Database != "alpha" {
		t.Fatalf("urutan item berubah: %+v", snap.Items)
	}
}

// Rincian per tabel harus ikut keluar, bukan cuma ringkasannya. Tanpa ini
// user hanya melihat kata "cocok" dan harus mempercayainya — sementara yang
// dia butuhkan justru bukti bahwa tiap tabel benar-benar sampai.
func TestCompareRowCounts_ReturnsPerTableRows(t *testing.T) {
	tables := []string{"orders", "users"}
	src := map[string]int64{"users": 120, "orders": 8}
	dst := map[string]int64{"users": 120, "orders": 3}

	_, _, rows := compareRowCounts(tables, src, dst)
	if len(rows) != 2 {
		t.Fatalf("jumlah baris rincian = %d, mau 2", len(rows))
	}
	// Diurutkan menurut nama tabel, supaya daftarnya stabil antar pemanggilan
	// dan tidak berpindah-pindah urutan di layar.
	if rows[0].Table != "orders" || rows[1].Table != "users" {
		t.Fatalf("urutan tabel tidak stabil: %+v", rows)
	}
	if rows[0].SourceRows != 8 || rows[0].DestRows != 3 || rows[0].Match {
		t.Errorf("orders seharusnya tidak cocok: %+v", rows[0])
	}
	if rows[1].SourceRows != 120 || rows[1].DestRows != 120 || !rows[1].Match {
		t.Errorf("users seharusnya cocok: %+v", rows[1])
	}
}

// Tabel yang ada di sumber tapi tidak terbaca di tujuan tercatat 0, bukan
// hilang dari daftar — justru baris itulah yang paling perlu terlihat.
func TestCompareRowCounts_MissingDestTableShowsZero(t *testing.T) {
	_, _, rows := compareRowCounts([]string{"invoices"}, map[string]int64{"invoices": 42}, map[string]int64{})
	if len(rows) != 1 {
		t.Fatalf("rincian = %+v", rows)
	}
	if rows[0].SourceRows != 42 || rows[0].DestRows != 0 || rows[0].Match {
		t.Errorf("baris = %+v, mau sumber=42 tujuan=0 tidak cocok", rows[0])
	}
}
