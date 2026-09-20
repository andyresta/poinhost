package dbxfer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/andyresta/poinhost/internal/modules/website"
)

// verifyDatabase membandingkan jumlah baris SEBENARNYA (COUNT(*) nyata,
// bukan taksiran information_schema/pg_class yang bisa selisih 40-50%
// untuk InnoDB) per tabel antara sumber dan tujuan — keputusan yang dikunci
// eksplisit bareng user: lebih lambat untuk tabel sangat besar, tapi
// hasilnya benar-benar bisa dipercaya. homepoin cuma menaksir dari
// katalog SISI TUJUAN saja, tidak pernah membandingkan ke sumber sama
// sekali — jadi "verifikasi"-nya di sana sebenarnya cuma "database
// tujuan tidak kosong", bukan "data yang sampai memang cocok".
func verifyDatabase(websiteSvc *website.Service, srcServerID, dstServerID, engine, srcDB, dstDB string, tables []string) (verify, detail string) {
	if len(tables) == 0 {
		return VerifySkipped, "tidak ada tabel untuk diverifikasi"
	}

	srcCounts, err := websiteSvc.DBTableRowCounts(srcServerID, engine, srcDB, tables)
	if err != nil {
		return VerifySkipped, "gagal membaca jumlah baris sumber: " + err.Error()
	}
	dstCounts, err := websiteSvc.DBTableRowCounts(dstServerID, engine, dstDB, tables)
	if err != nil {
		return VerifySkipped, "gagal membaca jumlah baris tujuan: " + err.Error()
	}
	return compareRowCounts(tables, srcCounts, dstCounts)
}

// compareRowCounts adalah logika perbandingan murni (tanpa SSH/SQL apa
// pun), dipisah dari verifyDatabase supaya bisa dites langsung.
func compareRowCounts(tables []string, srcCounts, dstCounts map[string]int64) (verify, detail string) {
	sortedTables := append([]string(nil), tables...)
	sort.Strings(sortedTables)

	var mismatches []string
	var totalSrc int64
	for _, t := range sortedTables {
		sc := srcCounts[t]
		dc := dstCounts[t]
		totalSrc += sc
		if sc != dc {
			mismatches = append(mismatches, fmt.Sprintf("%s: sumber=%d tujuan=%d", t, sc, dc))
		}
	}

	if len(mismatches) == 0 {
		return VerifyMatch, fmt.Sprintf("%d tabel, %d baris cocok", len(tables), totalSrc)
	}
	shown := mismatches
	more := ""
	if len(shown) > 3 {
		more = fmt.Sprintf(" (+%d lainnya)", len(shown)-3)
		shown = shown[:3]
	}
	return VerifyMismatch, fmt.Sprintf("%d/%d tabel tidak cocok: %s%s", len(mismatches), len(tables), strings.Join(shown, "; "), more)
}
