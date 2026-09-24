package agentctl

import "io"

// Fase operasi panjang di modul ini. Dilaporkan ke UI supaya user tahu di
// langkah mana ia sedang menunggu — pemasangan dan update melibatkan unggahan
// beberapa megabyte plus restart service, dan spinner tanpa keterangan membuat
// operasi yang berjalan normal terasa menggantung.
const (
	PhasePrepare = "prepare"
	PhaseUpload  = "upload"
	PhaseVerify  = "verify"
	PhaseInstall = "install"
	PhaseHealth  = "health"
	PhaseDone    = "done"
)

// Progress adalah satu pembaruan kemajuan.
type Progress struct {
	ServerID string `json:"serverId"`
	Phase    string `json:"phase"`
	Percent  int    `json:"percent"`
}

// SetEmitter memasang callback yang menerima setiap pembaruan progress.
// app.go menghubungkannya ke runtime.EventsEmit, supaya package ini sendiri
// tidak perlu tahu apa-apa soal Wails — pola yang sama dengan dbxfer dan
// servers.Collector.
func (s *Service) SetEmitter(emit func(Progress)) {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.emit = emit
}

func (s *Service) publish(serverID, phase string, percent int) {
	s.emitMu.RLock()
	emit := s.emit
	s.emitMu.RUnlock()
	if emit == nil {
		return
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	emit(Progress{ServerID: serverID, Phase: phase, Percent: percent})
}

// progressReader melaporkan kemajuan unggahan berdasarkan byte yang benar-benar
// terkirim, bukan tebakan berbasis waktu.
//
// Unggahan binary adalah bagian terlama dari pemasangan dan update, jadi justru
// di sinilah angka yang jujur paling berguna.
type progressReader struct {
	r     io.Reader
	total int64
	read  int64

	from, to int // rentang persen yang diwakili unggahan ini
	last     int
	report   func(percent int)
}

func newProgressReader(r io.Reader, total int64, from, to int, report func(int)) *progressReader {
	return &progressReader{r: r, total: total, from: from, to: to, last: -1, report: report}
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	if p.total > 0 && p.report != nil {
		pct := p.from + int(float64(p.to-p.from)*float64(p.read)/float64(p.total))
		// Hanya lapor saat angkanya berubah: unggahan beberapa megabyte
		// memanggil Read ribuan kali, dan mengirim event untuk tiap panggilan
		// akan membanjiri jembatan IPC tanpa menambah informasi apa pun.
		if pct != p.last {
			p.last = pct
			p.report(pct)
		}
	}
	return n, err
}
