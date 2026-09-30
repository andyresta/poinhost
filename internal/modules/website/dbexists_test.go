package website

import (
	"strings"
	"testing"
)

// Keberadaan database ditanyakan ke katalog engine, bukan disimpulkan dari
// teks error, dan pembandingnya PERSIS.
//
// Kasus yang memicu ini: migrasi database ke nama yang sudah ada selalu
// gagal. Pemanggilnya mencoba mentoleransi kondisi itu dengan mencari
// potongan "already exists" di pesan error — padahal MySQL berkata
//
//	ERROR 1007 (HY000): Can't create database 'alkana'; database exists
//
// yang tidak memuat potongan itu sama sekali. Teks error server bukan API:
// beda engine, beda versi, dan ikut berubah kalau bahasa server diganti.
func TestDBExistsQuery_AsksTheCatalog(t *testing.T) {
	mysql := dbExistsQuery("mysql", "alkana")
	if !strings.Contains(mysql, "information_schema.SCHEMATA") {
		t.Errorf("MySQL tidak menanyakan katalog: %s", mysql)
	}
	if !strings.Contains(mysql, "'alkana'") {
		t.Errorf("nama database tidak masuk kueri: %s", mysql)
	}

	pg := dbExistsQuery("postgres", "alkana")
	if !strings.Contains(pg, "pg_database") {
		t.Errorf("PostgreSQL tidak menanyakan katalog: %s", pg)
	}
	if !strings.Contains(pg, "'alkana'") {
		t.Errorf("nama database tidak masuk kueri: %s", pg)
	}
}

// `SHOW DATABASES LIKE 'app_data'` juga cocok dengan "appxdata", karena di
// LIKE garis bawah adalah wildcard satu karakter — dan nama database dengan
// garis bawah sangat lazim. Pembandingnya harus "=" atas katalog, bukan LIKE.
func TestDBExistsQuery_MySQLDoesNotUseLike(t *testing.T) {
	got := dbExistsQuery("mysql", "app_data")
	if strings.Contains(strings.ToUpper(got), "LIKE") {
		t.Errorf("masih memakai LIKE (garis bawah jadi wildcard): %s", got)
	}
	if !strings.Contains(got, "SCHEMA_NAME = 'app_data'") {
		t.Errorf("bukan pembandingan persis: %s", got)
	}
}

// Kutip tunggal di nama database harus di-escape, bukan menutup literal dan
// menyambung jadi SQL tambahan.
func TestDBExistsQuery_EscapesQuotes(t *testing.T) {
	for _, engine := range []string{"mysql", "postgres"} {
		got := dbExistsQuery(engine, "it's")
		if !strings.Contains(got, "'it''s'") {
			t.Errorf("%s: kutip tidak di-escape: %s", engine, got)
		}
	}
}
