package filexfer

import "time"

// Status job transfer.
const (
	StatusQueued   = "queued"
	StatusScanning = "scanning"
	StatusRunning  = "running"
	StatusDone     = "done"
	StatusFailed   = "failed"
	StatusCanceled = "canceled"
)

// Status per path sumber.
const (
	PathPending = "pending"
	PathRunning = "running"
	PathDone    = "done"
	PathFailed  = "failed"
)

// DirEntry satu entri folder di server, untuk browser di panel migrasi.
type DirEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

// ListDirResponse isi satu folder remote.
type ListDirResponse struct {
	Path    string     `json:"path"`
	Parent  string     `json:"parent"`
	Entries []DirEntry `json:"entries"`
}

// StartRequest permintaan memulai transfer file antar server.
type StartRequest struct {
	SourceServerID string   `json:"sourceServerId"`
	SourcePaths    []string `json:"sourcePaths"`
	DestServerID   string   `json:"destServerId"`
	DestPath       string   `json:"destPath"`
	Exclude        []string `json:"exclude"`
	Compress       bool     `json:"compress"`
}

// PathResult hasil transfer satu path sumber tingkat atas.
type PathResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Progress snapshot kemajuan satu job, dikirim ke frontend lewat event
// Wails "xfer:progress:<jobId>" dan juga bisa ditarik lewat Status().
type Progress struct {
	JobID          string       `json:"jobId"`
	Status         string       `json:"status"`
	SourceServerID string       `json:"sourceServerId"`
	DestServerID   string       `json:"destServerId"`
	SourcePaths    []string     `json:"sourcePaths"`
	DestPath       string       `json:"destPath"`
	Compress       bool         `json:"compress"`
	TotalBytes     int64        `json:"totalBytes"`
	DoneBytes      int64        `json:"doneBytes"`
	Percent        float64      `json:"percent"`
	CurrentPaths   []string     `json:"currentPaths"`
	Paths          []PathResult `json:"paths"`
	Message        string       `json:"message"`
	StartedAt      time.Time    `json:"startedAt"`
	FinishedAt     *time.Time   `json:"finishedAt,omitempty"`
}
