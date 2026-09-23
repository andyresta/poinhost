package dockerxfer

import "time"

// Status job migrasi Docker. Sengaja memakai kosakata yang sama dengan
// filexfer (queued/running/done/failed/canceled) supaya frontend bisa
// berbagi tabel label status — hanya "verifying" yang baru, mewakili tahap
// pasca-transfer yang TIDAK ADA di homepoin (lihat verify.go).
const (
	StatusQueued     = "queued"
	StatusInspecting = "inspecting"
	StatusRunning    = "running"
	StatusVerifying  = "verifying"
	StatusDone       = "done"
	StatusFailed     = "failed"
	StatusCanceled   = "canceled"
)

// Status per item (satu mount, atau satu entri sintetis "image").
const (
	ItemPending = "pending"
	ItemRunning = "running"
	ItemDone    = "done"
	ItemFailed  = "failed"
	// ItemSkipped: image yang sudah ada di server tujuan tidak perlu
	// ditarik/disalurkan ulang.
	ItemSkipped = "skipped"
)

// Hasil verifikasi ringan per mount (jumlah file + ukuran total, bukan
// checksum per-file — lihat catatan di verify.go soal alasan pilihan ini).
const (
	VerifyMatch    = "match"
	VerifyMismatch = "mismatch"
	VerifySkipped  = "skipped"
)

// imageItemLabel adalah label item sintetis yang mewakili transfer konten
// image (bukan mount) di dalam daftar Items yang sama dengan mount — lihat
// catatan desain di job.go soal kenapa image tidak punya progress bar
// terpisah seperti di homepoin.
const imageItemLabel = "__image__"

// StartRequest permintaan memulai migrasi satu container Docker ke server lain.
type StartRequest struct {
	SourceServerID string `json:"sourceServerId"`
	ContainerID    string `json:"containerId"`
	DestServerID   string `json:"destServerId"`
	// Compress: dipakai KONSISTEN untuk data mount ATAUPUN image (beda dari
	// homepoin yang selalu meng-gzip image tapi tidak pernah meng-gzip
	// volume/bind, tanpa opsi apa pun untuk menyamakannya).
	Compress bool `json:"compress"`
	// CopyWorkdir: ikut salin direktori proyek compose (label
	// com.docker.compose.project.working_dir) ke path yang SAMA di tujuan —
	// Dockerfile, source, docker-compose.yml, .env — supaya
	// `docker compose` tetap bisa dipakai di server tujuan. Tanpa ini hanya
	// mount container yang disalin.
	CopyWorkdir bool `json:"copyWorkdir"`
}

// ItemResult status satu item transfer: satu mount (bind atau named volume)
// atau entri sintetis untuk image.
type ItemResult struct {
	// Label bentuk yang ditampilkan di UI, mis. "bind:/var/www",
	// "volume:app_data", atau "image:nginx:1.25".
	Label  string `json:"label"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// Warning: catatan non-fatal, mis. "tujuan sudah berisi data — akan
	// digabung" saat sebuah mount/volume di tujuan ternyata tidak kosong.
	// homepoin diam-diam menggabung/menimpa dalam kasus ini tanpa memberi
	// tahu apa pun; di sini pengguna setidaknya melihatnya sebelum transfer
	// berjalan.
	Warning      string `json:"warning,omitempty"`
	Verify       string `json:"verify,omitempty"`
	VerifyDetail string `json:"verifyDetail,omitempty"`
}

// Progress snapshot kemajuan satu job migrasi, dikirim ke frontend lewat
// event Wails "dockerxfer:progress:<jobId>".
type Progress struct {
	JobID          string       `json:"jobId"`
	Status         string       `json:"status"`
	SourceServerID string       `json:"sourceServerId"`
	DestServerID   string       `json:"destServerId"`
	ContainerID    string       `json:"containerId"`
	ContainerName  string       `json:"containerName"`
	Compress       bool         `json:"compress"`
	CopyWorkdir    bool         `json:"copyWorkdir"`
	TotalBytes     int64        `json:"totalBytes"`
	DoneBytes      int64        `json:"doneBytes"`
	Percent        float64      `json:"percent"`
	CurrentItems   []string     `json:"currentItems"`
	Items          []ItemResult `json:"items"`
	// ContainerRunning: hasil verifikasi "container tujuan benar-benar
	// menyala" — homepoin tidak punya pengecekan ini sama sekali, sukses di
	// sana murni berarti perintah SSH-nya tidak error.
	ContainerRunning *bool      `json:"containerRunning,omitempty"`
	Message          string     `json:"message"`
	StartedAt        time.Time  `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
}
