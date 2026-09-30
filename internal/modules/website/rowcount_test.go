package website

import (
	"strings"
	"testing"
)

// Semua tabel dihitung dalam SATU kueri. Sebelumnya tiap tabel berarti satu
// round-trip SSH tersendiri, sehingga skema dengan puluhan tabel membuat
// verifikasi (dan sekarang juga daftar tabel di panel migrasi) terasa
// menggantung lama tanpa alasan yang terlihat.
func TestRowCountSQL_SingleQueryForAllTables(t *testing.T) {
	sql := rowCountSQL("mysql", "shop", []string{"orders", "users"})
	if n := strings.Count(sql, "UNION ALL"); n != 1 {
		t.Errorf("dua tabel seharusnya digabung satu UNION ALL, dapat %d: %s", n, sql)
	}
	if !strings.Contains(sql, "FROM `shop`.`orders`") || !strings.Contains(sql, "FROM `shop`.`users`") {
		t.Errorf("tabel tidak lengkap: %s", sql)
	}
	// Label literal wajib ada: urutan baris hasil UNION ALL tidak dijamin,
	// jadi angka dipetakan lewat nama, bukan posisi.
	if !strings.Contains(sql, "SELECT 'orders'") {
		t.Errorf("label nama tabel hilang: %s", sql)
	}
}

func TestRowCountSQL_PostgresQuotesIdentifiers(t *testing.T) {
	sql := rowCountSQL("postgres", "shop", []string{"orders"})
	if !strings.Contains(sql, `FROM "orders"`) {
		t.Errorf("identifier tidak dikutip: %s", sql)
	}
	if strings.Contains(sql, "`") {
		t.Errorf("backtick MySQL bocor ke PostgreSQL: %s", sql)
	}
}

func TestParseRowCounts_MapsByNameNotPosition(t *testing.T) {
	// Sengaja terbalik urutannya terhadap daftar tabel yang diminta.
	out, err := parseRowCounts("users\t120\norders\t8\n", []string{"orders", "users"})
	if err != nil {
		t.Fatalf("parseRowCounts: %v", err)
	}
	if out["orders"] != 8 || out["users"] != 120 {
		t.Fatalf("hasil = %v, mau orders=8 users=120", out)
	}
}

// Nol adalah jawaban yang sah. Tabel yang TIDAK muncul di hasil berarti
// kueri-nya bermasalah, dan mengarang nol dari baris yang hilang akan
// membuat verifikasi melaporkan "tidak cocok" atas sesuatu yang sebenarnya
// tidak pernah terbaca.
func TestParseRowCounts_MissingTableIsAnError(t *testing.T) {
	if _, err := parseRowCounts("orders\t8\n", []string{"orders", "users"}); err == nil {
		t.Fatal("tabel yang hilang dari hasil seharusnya jadi error")
	}
	out, err := parseRowCounts("orders\t0\n", []string{"orders"})
	if err != nil {
		t.Fatalf("nol seharusnya sah: %v", err)
	}
	if out["orders"] != 0 {
		t.Fatalf("hasil = %v, mau orders=0", out)
	}
}

func TestParseRowCounts_ToleratesCarriageReturn(t *testing.T) {
	out, err := parseRowCounts("orders\t8\r\n", []string{"orders"})
	if err != nil {
		t.Fatalf("parseRowCounts: %v", err)
	}
	if out["orders"] != 8 {
		t.Fatalf("hasil = %v, mau orders=8", out)
	}
}
