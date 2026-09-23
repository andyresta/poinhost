package sitexfer

import "time"

// Status job migrasi website — kosakata sama dengan modul xfer lain supaya
// frontend bisa berbagi label; "partial" diambil dari dbxfer: sebagian item
// gagal tapi sisanya sudah terpasang di tujuan.
const (
	StatusQueued     = "queued"
	StatusInspecting = "inspecting"
	StatusRunning    = "running"
	StatusVerifying  = "verifying"
	StatusDone       = "done"
	StatusPartial    = "partial"
	StatusFailed     = "failed"
	StatusCanceled   = "canceled"
)

// Status per item.
const (
	ItemPending = "pending"
	ItemRunning = "running"
	ItemDone    = "done"
	ItemFailed  = "failed"
	ItemSkipped = "skipped"
)

// Jenis item — urutan konstanta ini juga urutan eksekusi (lihat run):
// user DB harus ada SEBELUM restore (pg_dump menulis OWNER TO, MySQL
// menulis DEFINER), user SFTP harus ada SEBELUM file diekstrak (tar
// mencocokkan pemilik file berdasarkan NAMA user), vhost ditulis SESUDAH
// file supaya situs tidak pernah melayani document root setengah jadi,
// dan cron terakhir karena baris cron-nya `cd` ke document root domain.
const (
	KindDBUser   = "dbuser"
	KindDatabase = "database"
	KindSFTP     = "sftp"
	KindFiles    = "files"
	KindVhost    = "vhost"
	KindCron     = "cron"
)

// StartRequest permintaan migrasi (dan preview-nya — bentuknya sama).
type StartRequest struct {
	SourceServerID string `json:"sourceServerId"`
	DestServerID   string `json:"destServerId"`
	// Domains domain dan/atau subdomain yang dipilih, masing-masing berdiri
	// sendiri: memilih domain induk TIDAK otomatis membawa subdomainnya.
	Domains          []string `json:"domains"`
	IncludeDatabases bool     `json:"includeDatabases"`
	IncludeSFTP      bool     `json:"includeSftp"`
	IncludeCron      bool     `json:"includeCron"`
	Compress         bool     `json:"compress"`
	// DBUserPasswords password asli (kunci "user@host") untuk user MySQL
	// yang hash-nya tidak bisa dipasang di server tujuan (Action
	// "needs-password", mis. MySQL 8 → MariaDB). Hanya dipakai untuk CREATE
	// USER lalu dibuang — tidak pernah disimpan atau ikut di Progress.
	DBUserPasswords map[string]string `json:"dbUserPasswords,omitempty"`
}

// PlanDomain satu domain yang akan dipindah.
type PlanDomain struct {
	Domain      string `json:"domain"`
	Parent      string `json:"parent,omitempty"`
	IsSubdomain bool   `json:"isSubdomain"`
	Root        string `json:"root"`
	Enabled     bool   `json:"enabled"`
	PHPVersion  string `json:"phpVersion,omitempty"`
	SSLEnabled  bool   `json:"sslEnabled"`
	Proxy       bool   `json:"proxy"`
}

// PlanPath satu folder yang di-tar dari asal ke path yang sama di tujuan.
type PlanPath struct {
	Path     string   `json:"path"`
	Excludes []string `json:"excludes"`
	Files    int64    `json:"files"`
	Bytes    int64    `json:"bytes"`
}

// PlanDatabase satu database yang tertaut ke domain terpilih.
type PlanDatabase struct {
	Engine  string   `json:"engine"`
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
	Owner   string   `json:"owner,omitempty"`
}

// Aksi user DB di server tujuan.
const (
	DBUserCreate     = "create"
	DBUserSkipExists = "skip-exists"
	DBUserSkipGlobal = "skip-global"
	// DBUserNeedsPassword hash tidak portabel ke server tujuan — dibuat
	// hanya kalau password aslinya diisi (StartRequest.DBUserPasswords).
	DBUserNeedsPassword = "needs-password"
)

// DBUserKey kunci StartRequest.DBUserPasswords untuk satu user.
func DBUserKey(username, host string) string { return username + "@" + host }

// RepairResult hasil RepairDBUsers.
type RepairResult struct {
	Items []ItemResult `json:"items"`
}

// PlanDBUser satu user database yang terkait.
type PlanDBUser struct {
	Engine    string   `json:"engine"`
	Username  string   `json:"username"`
	Host      string   `json:"host,omitempty"`
	Databases []string `json:"databases"`
	Action    string   `json:"action"`
	Note      string   `json:"note,omitempty"`
}

// PlanSFTP satu akun SFTP yang ikut.
type PlanSFTP struct {
	Username   string `json:"username"`
	Domain     string `json:"domain"`
	Enabled    bool   `json:"enabled"`
	HashScheme string `json:"hashScheme"`
	Note       string `json:"note,omitempty"`
}

// Problem temuan preflight. Blocking = migrasi tidak boleh dimulai.
type Problem struct {
	Blocking bool   `json:"blocking"`
	Message  string `json:"message"`
}

// Plan hasil preview: apa saja yang akan dipindah dan apa yang menghalangi.
type Plan struct {
	Domains    []PlanDomain   `json:"domains"`
	Paths      []PlanPath     `json:"paths"`
	Databases  []PlanDatabase `json:"databases"`
	DBUsers    []PlanDBUser   `json:"dbUsers"`
	SFTP       []PlanSFTP     `json:"sftp"`
	CronJobs   int            `json:"cronJobs"`
	TotalBytes int64          `json:"totalBytes"`
	Problems   []Problem      `json:"problems"`
	CanStart   bool           `json:"canStart"`
}

// ItemResult status satu langkah migrasi.
type ItemResult struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
	Warning string `json:"warning,omitempty"`
	// Detail hasil verifikasi atau catatan (mis. "124 file, 3.2 MiB").
	Detail string `json:"detail,omitempty"`
}

// Progress snapshot job, dikirim lewat event "sitexfer:progress:<jobId>".
type Progress struct {
	JobID          string       `json:"jobId"`
	Status         string       `json:"status"`
	SourceServerID string       `json:"sourceServerId"`
	DestServerID   string       `json:"destServerId"`
	Domains        []string     `json:"domains"`
	TotalBytes     int64        `json:"totalBytes"`
	DoneBytes      int64        `json:"doneBytes"`
	Percent        float64      `json:"percent"`
	Items          []ItemResult `json:"items"`
	// Report langkah yang HARUS dilakukan manual sesudahnya (PHP, SSL, DNS)
	// plus hal yang sengaja dilewati — ditampilkan sebagai daftar periksa.
	Report     []string   `json:"report"`
	Message    string     `json:"message"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}
