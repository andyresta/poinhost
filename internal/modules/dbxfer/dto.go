package dbxfer

import "time"

// Status job migrasi database.
const (
	StatusQueued     = "queued"
	StatusInspecting = "inspecting"
	StatusRunning    = "running"
	StatusVerifying  = "verifying"
	StatusDone       = "done"
	// StatusPartial: sebagian database berhasil, sebagian gagal — homepoin
	// TIDAK punya status ini sama sekali, job dengan item gagal tetap
	// dilaporkan "done" di level atas (konsumen yang hanya membaca field
	// status, bukan mengiterasi items[], salah mengira migrasi sukses
	// penuh). Di sini keduanya dibedakan secara eksplisit.
	StatusPartial  = "partial"
	StatusFailed   = "failed"
	StatusCanceled = "canceled"
)

// Status per item (satu database).
const (
	ItemPending = "pending"
	ItemRunning = "running"
	ItemDone    = "done"
	ItemFailed  = "failed"
)

// Hasil verifikasi ringan per item: COUNT(*) NYATA (bukan taksiran
// information_schema/pg_class) dibandingkan sumber vs tujuan — lihat
// verify.go untuk alasannya.
const (
	VerifyMatch    = "match"
	VerifyMismatch = "mismatch"
	VerifySkipped  = "skipped"
)

// ItemSelection satu database (dan opsional subset tabelnya) yang dipilih untuk dimigrasikan.
type ItemSelection struct {
	Database string `json:"database"`
	// AllTables true berarti seluruh tabel di database ini disalin — Tables
	// diabaikan. false berarti hanya tabel yang disebut di Tables.
	AllTables bool     `json:"allTables"`
	Tables    []string `json:"tables,omitempty"`
	// DestDatabase: nama database di tujuan, kalau ingin berbeda dari nama
	// sumber. Kosong = nama sama. Hanya diperbolehkan diisi kalau job ini
	// cuma memilih SATU database (lihat validasi di service.go) — kalau
	// lebih dari satu, nama tujuan harus mengikuti nama sumber supaya tidak
	// ambigu.
	DestDatabase string `json:"destDatabase,omitempty"`
}

// StartRequest permintaan memulai migrasi database antar server.
type StartRequest struct {
	SourceServerID string `json:"sourceServerId"`
	DestServerID   string `json:"destServerId"`
	// Engine: "mysql" | "postgresql" — berlaku untuk KEDUA sisi. Tidak ada
	// migrasi lintas engine (MySQL -> PostgreSQL atau sebaliknya) — sama
	// seperti homepoin, ini keterbatasan yang disengaja (perlu penerjemah
	// skema, di luar cakupan fitur ini) bukan yang belum sempat dikerjakan.
	Engine string          `json:"engine"`
	Items  []ItemSelection `json:"items"`
}

// ItemResult status migrasi satu database.
type ItemResult struct {
	Database     string `json:"database"`
	DestDatabase string `json:"destDatabase"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	// Warning: database tujuan sudah ada sebelum restore dimulai — restore
	// akan MENIMPA tabel bernama sama (lihat catatan --add-drop-table/
	// --clean di engine.go), bukan gagal atau menimpa bersih semuanya.
	Warning string `json:"warning,omitempty"`
	// TableCount: jumlah tabel yang dipilih untuk disalin (AllTables
	// diselesaikan ke daftar nyata sebelum transfer dimulai).
	TableCount   int    `json:"tableCount"`
	Verify       string `json:"verify,omitempty"`
	VerifyDetail string `json:"verifyDetail,omitempty"`
}

// Progress snapshot kemajuan satu job migrasi, dikirim ke frontend lewat
// event Wails "dbxfer:progress:<jobId>".
type Progress struct {
	JobID          string       `json:"jobId"`
	Status         string       `json:"status"`
	SourceServerID string       `json:"sourceServerId"`
	DestServerID   string       `json:"destServerId"`
	Engine         string       `json:"engine"`
	TotalBytes     int64        `json:"totalBytes"`
	DoneBytes      int64        `json:"doneBytes"`
	Percent        float64      `json:"percent"`
	CurrentItems   []string     `json:"currentItems"`
	Items          []ItemResult `json:"items"`
	Message        string       `json:"message"`
	StartedAt      time.Time    `json:"startedAt"`
	FinishedAt     *time.Time   `json:"finishedAt,omitempty"`
}

// TableListResponse daftar tabel satu database, untuk picker frontend.
type TableListResponse struct {
	Tables []string `json:"tables"`
}
