package sshpool

import (
	"context"
	"io"
	"time"
)

// Progress melaporkan kemajuan satu transfer berkas.
//
// total bernilai -1 kalau ukurannya tidak bisa diketahui lebih dulu. Layar
// yang menerimanya harus menampilkan bar tak tentu, bukan membagi dengan
// nol — dan itu sebabnya dibedakan dari 0, yang berarti berkas kosong.
type Progress func(done, total int64)

// jedaLapor membatasi seberapa sering kemajuan dilaporkan.
//
// io.Copy memanggil Write tiap 32 KiB, jadi satu berkas 1 GB memicu lebih
// dari tiga puluh ribu laporan. Mengirim semuanya ke UI membuat antrean
// event jadi pekerjaan yang lebih berat daripada unduhannya sendiri.
const jedaLapor = 150 * time.Millisecond

// penghitung membungkus writer dan melaporkan kemajuannya.
type penghitung struct {
	w        io.Writer
	total    int64
	done     int64
	lapor    Progress
	terakhir time.Time
}

func (p *penghitung) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if p.lapor == nil {
		return n, err
	}
	// Laporan TERAKHIR dikirim oleh pemanggil, bukan di sini: kalau hanya
	// mengandalkan jeda, unduhan yang selesai tepat sesudah satu laporan
	// akan berhenti di 97% selamanya di layar.
	if time.Since(p.terakhir) >= jedaLapor {
		p.terakhir = time.Now()
		p.lapor(p.done, p.total)
	}
	return n, err
}

// DownloadFileProgress mengunduh berkas remote sambil melaporkan kemajuannya.
//
// Ukurannya dibaca lebih dulu lewat Stat supaya layar bisa menampilkan bar
// yang bergerak ke ujung, bukan sekadar spinner. Stat yang gagal TIDAK
// menggagalkan unduhan — total cukup dilaporkan sebagai tidak diketahui.
func (c *SFTPClient) DownloadFileProgress(ctx context.Context, serverID, path string, w io.Writer, lapor Progress) error {
	client, conn, err := c.open(ctx, serverID)
	if err != nil {
		return err
	}
	defer c.close(serverID, conn, client)

	f, err := client.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var total int64 = -1
	if st, serr := f.Stat(); serr == nil {
		total = st.Size()
	}
	if lapor != nil {
		lapor(0, total)
	}

	pw := &penghitung{w: w, total: total, lapor: lapor, terakhir: time.Now()}
	if _, err := io.Copy(pw, f); err != nil {
		return err
	}
	if lapor != nil {
		lapor(pw.done, total)
	}
	return nil
}

// pembaca membungkus reader dan melaporkan berapa yang sudah terbaca.
//
// Untuk UNGGAHAN yang dihitung adalah sisi BACA, bukan sisi tulis, dan itu
// bukan pilihan sembarangan: pkg/sftp punya File.ReadFrom yang mengirim
// beberapa potongan sekaligus, dan io.Copy hanya memakainya kalau tujuannya
// benar-benar *sftp.File. Membungkus tujuannya akan menyembunyikan jalur
// cepat itu dan membuat unggahan jauh lebih lambat — ditukar dengan sebuah
// bar kemajuan, yang jelas bukan tukar-tambah yang bagus.
//
// Konsekuensinya angka ini sedikit MENDAHULUI apa yang benar-benar sampai
// di server, karena yang terbaca belum tentu sudah terkirim. Selisihnya
// sebatas buffer yang sedang jalan, dan laporan terakhir selalu dikirim
// sesudah penyalinannya benar-benar selesai.
type pembaca struct {
	r        io.Reader
	total    int64
	done     int64
	lapor    Progress
	terakhir time.Time
}

func (p *pembaca) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.lapor == nil {
		return n, err
	}
	if time.Since(p.terakhir) >= jedaLapor {
		p.terakhir = time.Now()
		p.lapor(p.done, p.total)
	}
	return n, err
}

// UploadFileProgress mengunggah satu berkas sambil melaporkan kemajuannya.
//
// total diberikan pemanggil (dari Stat berkas lokalnya), bukan dibaca di
// sini: yang tahu ukurannya adalah sisi yang membuka berkas itu, dan reader
// biasa tidak punya ukuran sama sekali. total -1 berarti tidak diketahui,
// dan layar harus menampilkan bar tak tentu.
func (c *SFTPClient) UploadFileProgress(ctx context.Context, serverID, path string, r io.Reader, total int64, lapor Progress) error {
	client, conn, err := c.open(ctx, serverID)
	if err != nil {
		return err
	}
	defer c.close(serverID, conn, client)

	f, err := client.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if lapor != nil {
		lapor(0, total)
	}

	pr := &pembaca{r: r, total: total, lapor: lapor, terakhir: time.Now()}
	if _, err := io.Copy(f, pr); err != nil {
		return err
	}
	// Laporan TERAKHIR dikirim di sini, bukan diserahkan ke jeda: unggahan
	// yang selesai tepat sesudah satu laporan akan berhenti di 97% selamanya
	// di layar.
	if lapor != nil {
		lapor(pr.done, total)
	}
	return nil
}
