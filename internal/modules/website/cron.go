package website

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CronHeader satu header HTTP custom untuk job tipe "http".
type CronHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CronJobInfo satu job cron milik satu domain.
type CronJobInfo struct {
	ID            string       `json:"id"`
	Domain        string       `json:"domain"`
	TaskType      string       `json:"taskType"` // "command" | "http"
	Schedule      string       `json:"schedule"`
	ScheduleText  string       `json:"scheduleText"`
	Command       string       `json:"command,omitempty"`
	URL           string       `json:"url,omitempty"`
	Method        string       `json:"method,omitempty"`
	Payload       string       `json:"payload,omitempty"`
	Headers       []CronHeader `json:"headers,omitempty"`
	Description   string       `json:"description,omitempty"`
	Enabled       bool         `json:"enabled"`
	ActionSummary string       `json:"actionSummary"`
	LogPath       string       `json:"logPath"`
}

// CronListResponse daftar job cron milik satu domain.
type CronListResponse struct {
	Domain     string        `json:"domain"`
	DomainRoot string        `json:"domainRoot"`
	Jobs       []CronJobInfo `json:"jobs"`
	Active     int           `json:"active"`
	Inactive   int           `json:"inactive"`
}

// CronJobRequest field gabungan untuk Create/Update job cron — ID kosong
// berarti Create, diisi berarti Update.
type CronJobRequest struct {
	ServerID    string       `json:"serverId"`
	Domain      string       `json:"domain"`
	ID          string       `json:"id,omitempty"`
	TaskType    string       `json:"taskType"`
	Schedule    string       `json:"schedule"`
	Command     string       `json:"command,omitempty"`
	URL         string       `json:"url,omitempty"`
	Method      string       `json:"method,omitempty"`
	Payload     string       `json:"payload,omitempty"`
	Headers     []CronHeader `json:"headers,omitempty"`
	Description string       `json:"description,omitempty"`
	Enabled     bool         `json:"enabled"`
}

// CronToggleRequest mengaktifkan/menonaktifkan satu job tanpa mengubah field lain.
type CronToggleRequest struct {
	ServerID string `json:"serverId"`
	Domain   string `json:"domain"`
	ID       string `json:"id"`
	Enabled  bool   `json:"enabled"`
}

// CronLogRequest permintaan baca log satu job.
type CronLogRequest struct {
	ServerID string `json:"serverId"`
	Domain   string `json:"domain"`
	ID       string `json:"id"`
	Lines    int    `json:"lines"`
}

// CronLogResponse isi log satu job cron.
type CronLogResponse struct {
	ID      string `json:"id"`
	Domain  string `json:"domain"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Exists  bool   `json:"exists"`
}

const cronFilePath = "/etc/cron.d/poinhost"

// Setiap job cron poinhost disimpan sebagai TIGA baris tetap: satu (atau
// dua, untuk tipe http) baris komentar metadata berisi SEMUA field
// terenkode base64 (jadi tidak ada ambiguitas parsing sama sekali — bukan
// meng-infer command/schedule dari baris cron mentahnya balik, yang rawan
// pecah kalau ada spasi/karakter aneh), lalu SATU baris cron sesungguhnya
// yang SELALU ditulis ulang dari metadata (bukan dipertahankan dari file
// lama) sehingga baris ke-3 murni untuk dieksekusi cron, tidak pernah
// dibaca balik oleh parser ini.
const cronMetaPrefix = "# poinhost:job "
const cronCommandMetaPrefix = "# poinhost:command "
const cronHTTPMetaPrefix = "# poinhost:http "

var cronFieldRE = regexp.MustCompile(`^[0-9*,\-/]+$`)

// validateCronSchedule memvalidasi 5 field cron secara ringan (format token,
// bukan validasi rentang cron penuh — cron sendiri yang akan menolak kalau
// isinya benar-benar tidak masuk akal saat dieksekusi).
func validateCronSchedule(schedule string) error {
	fields := strings.Fields(strings.TrimSpace(schedule))
	if len(fields) != 5 {
		return errFmt("jadwal cron harus 5 field (menit jam tanggal bulan hari)")
	}
	for _, f := range fields {
		if !cronFieldRE.MatchString(f) {
			return errFmt("field jadwal %q tidak valid", f)
		}
	}
	return nil
}

// describeSchedule menghasilkan deskripsi ringkas dalam Bahasa Indonesia
// untuk pola jadwal umum — fallback ke jadwal mentah untuk pola lain
// (versi sederhana, bukan replikasi penuh preset-builder homepoin).
func describeSchedule(schedule string) string {
	switch schedule {
	case "* * * * *":
		return "Setiap menit"
	case "*/5 * * * *":
		return "Setiap 5 menit"
	case "*/15 * * * *":
		return "Setiap 15 menit"
	case "*/30 * * * *":
		return "Setiap 30 menit"
	case "0 * * * *":
		return "Setiap jam"
	}
	fields := strings.Fields(schedule)
	if len(fields) == 5 && fields[2] == "*" && fields[3] == "*" && fields[4] == "*" {
		return fmt.Sprintf("Harian jam %s:%s", pad2(fields[1]), pad2(fields[0]))
	}
	return "Kustom: " + schedule
}

func pad2(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func cronLogPath(domain, jobID string) string {
	return "/var/log/poinhost-cron/" + strings.ReplaceAll(domain, "/", "_") + "-" + jobID + ".log"
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
func unb64(s string) string {
	dec, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(dec)
}

func headersToB64(headers []CronHeader) string {
	var b strings.Builder
	for i, h := range headers {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(b64(h.Name) + "=" + b64(h.Value))
	}
	return b64(b.String())
}

func headersFromB64(encoded string) []CronHeader {
	raw := unb64(encoded)
	if raw == "" {
		return nil
	}
	var out []CronHeader
	for _, pair := range strings.Split(raw, ";") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		out = append(out, CronHeader{Name: unb64(kv[0]), Value: unb64(kv[1])})
	}
	return out
}

// buildCronLine menyusun baris cron sesungguhnya yang dieksekusi crond —
// dikomentari (diawali "# ") kalau job sedang nonaktif.
func buildCronLine(job CronJobInfo, domainRoot string) string {
	var inner string
	if job.TaskType == "http" {
		method := job.Method
		if method == "" {
			method = "GET"
		}
		parts := []string{"curl", "-sS", "-f", "-X", method}
		for _, h := range job.Headers {
			parts = append(parts, "-H", shellQuote(h.Name+": "+h.Value))
		}
		if method == "POST" && job.Payload != "" {
			parts = append(parts, "-d", shellQuote(job.Payload))
		}
		parts = append(parts, shellQuote(job.URL))
		inner = strings.Join(parts, " ") + " >> " + shellQuote(job.LogPath) + " 2>&1"
	} else {
		inner = "cd " + shellQuote(domainRoot) + " && /bin/bash -lc " + shellQuote(job.Command) + " >> " + shellQuote(job.LogPath) + " 2>&1"
	}
	line := job.Schedule + " root " + inner
	if !job.Enabled {
		line = "# " + line
	}
	return line
}

// parseCronFile mem-parsing file /etc/cron.d/poinhost jadi daftar job +
// baris "asing" (bukan buatan poinhost, mis. ditambahkan admin manual di
// server) yang dipertahankan apa adanya supaya tidak hilang saat ditulis ulang.
func parseCronFile(content string) (jobs []CronJobInfo, foreign []string) {
	lines := strings.Split(content, "\n")
	i := 0
	for i < len(lines) {
		line := lines[i]
		if !strings.HasPrefix(line, cronMetaPrefix) {
			if strings.TrimSpace(line) != "" {
				foreign = append(foreign, line)
			}
			i++
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, cronMetaPrefix))
		job := CronJobInfo{}
		for _, f := range fields {
			switch {
			case strings.HasPrefix(f, "id="):
				job.ID = strings.TrimPrefix(f, "id=")
			case strings.HasPrefix(f, "domain="):
				job.Domain = strings.TrimPrefix(f, "domain=")
			case strings.HasPrefix(f, "enabled="):
				job.Enabled = strings.TrimPrefix(f, "enabled=") == "1"
			case strings.HasPrefix(f, "type="):
				job.TaskType = strings.TrimPrefix(f, "type=")
			case strings.HasPrefix(f, "schedule_b64="):
				job.Schedule = unb64(strings.TrimPrefix(f, "schedule_b64="))
			case strings.HasPrefix(f, "desc_b64="):
				job.Description = unb64(strings.TrimPrefix(f, "desc_b64="))
			}
		}
		i++

		if job.TaskType == "command" && i < len(lines) && strings.HasPrefix(lines[i], cronCommandMetaPrefix) {
			for _, f := range strings.Fields(strings.TrimPrefix(lines[i], cronCommandMetaPrefix)) {
				if v, ok := strings.CutPrefix(f, "command_b64="); ok {
					job.Command = unb64(v)
				}
			}
			i++
		} else if job.TaskType == "http" && i < len(lines) && strings.HasPrefix(lines[i], cronHTTPMetaPrefix) {
			for _, f := range strings.Fields(strings.TrimPrefix(lines[i], cronHTTPMetaPrefix)) {
				switch {
				case strings.HasPrefix(f, "url_b64="):
					job.URL = unb64(strings.TrimPrefix(f, "url_b64="))
				case strings.HasPrefix(f, "method="):
					job.Method = strings.TrimPrefix(f, "method=")
				case strings.HasPrefix(f, "payload_b64="):
					job.Payload = unb64(strings.TrimPrefix(f, "payload_b64="))
				case strings.HasPrefix(f, "headers_b64="):
					job.Headers = headersFromB64(strings.TrimPrefix(f, "headers_b64="))
				}
			}
			i++
		}
		// Baris cron sesungguhnya (baris ke-3) SELALU ditulis ulang dari
		// metadata di atas saat serialisasi — dilewati begitu saja di sini.
		if i < len(lines) {
			i++
		}

		job.LogPath = cronLogPath(job.Domain, job.ID)
		job.ScheduleText = describeSchedule(job.Schedule)
		if job.TaskType == "http" {
			job.ActionSummary = job.Method + " " + job.URL
		} else {
			job.ActionSummary = job.Command
		}
		if job.ID != "" {
			jobs = append(jobs, job)
		}
	}
	return jobs, foreign
}

// renderCronFile menyusun ulang seluruh isi file dari daftar job + baris asing.
func renderCronFile(jobs []CronJobInfo, foreign []string, domainRoots map[string]string) string {
	var b strings.Builder
	b.WriteString("# Dikelola poinhost — jangan edit manual, perubahan akan tertimpa\n")
	b.WriteString("# untuk job yang dibuat lewat aplikasi (baris lain di luar itu aman).\n")
	for _, f := range foreign {
		b.WriteString(f)
		b.WriteByte('\n')
	}
	for _, job := range jobs {
		enabledFlag := "0"
		if job.Enabled {
			enabledFlag = "1"
		}
		fmt.Fprintf(&b, "%sid=%s domain=%s enabled=%s type=%s schedule_b64=%s desc_b64=%s\n",
			cronMetaPrefix, job.ID, job.Domain, enabledFlag, job.TaskType, b64(job.Schedule), b64(job.Description))
		if job.TaskType == "http" {
			fmt.Fprintf(&b, "%surl_b64=%s method=%s payload_b64=%s headers_b64=%s\n",
				cronHTTPMetaPrefix, b64(job.URL), job.Method, b64(job.Payload), headersToB64(job.Headers))
		} else {
			fmt.Fprintf(&b, "%scommand_b64=%s\n", cronCommandMetaPrefix, b64(job.Command))
		}
		root := domainRoots[job.Domain]
		b.WriteString(buildCronLine(job, root))
		b.WriteByte('\n')
	}
	return b.String()
}

func (s *Service) readCronFile(access *websiteAccess) (string, []CronJobInfo, []string, error) {
	res, err := s.run(access, "cat "+cronFilePath+" 2>/dev/null || true", 15*time.Second)
	if err != nil {
		return "", nil, nil, err
	}
	jobs, foreign := parseCronFile(res.Stdout)
	return res.Stdout, jobs, foreign, nil
}

func (s *Service) writeCronFile(access *websiteAccess, jobs []CronJobInfo, foreign []string, domainRoots map[string]string) error {
	content := renderCronFile(jobs, foreign, domainRoots)
	script := `set -e
echo ` + shellQuote(b64(content)) + ` | base64 -d > ` + cronFilePath + `.tmp.$$
chmod 0644 ` + cronFilePath + `.tmp.$$
mv ` + cronFilePath + `.tmp.$$ ` + cronFilePath
	_, err := s.run(access, script, 15*time.Second)
	return err
}

// domainRootsFor membangun map domain->root dari SATU listDomains (sudah
// ter-cache) — dipakai supaya buildCronLine tiap job punya `cd <root>`
// yang benar TANPA memanggil getDomain berulang per job seperti homepoin's
// collectDomainRoots (yang bahkan re-list SEMUA vhost per domain berbeda
// di file cron — N+1 di atas N+1).
func (s *Service) domainRootsFor(serverID string, jobs []CronJobInfo) (map[string]string, error) {
	all, err := s.listDomains(serverID)
	if err != nil {
		return nil, err
	}
	roots := make(map[string]string, len(all))
	for _, d := range all {
		roots[d.Domain] = d.Root
	}
	return roots, nil
}

// ListCronJobs mengembalikan semua job cron milik satu domain.
func (s *Service) ListCronJobs(serverID, domain string) (*CronListResponse, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	info, err := s.getDomain(serverID, domain)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return nil, err
	}

	_, allJobs, _, err := s.readCronFile(access)
	if err != nil {
		return nil, err
	}

	jobs := make([]CronJobInfo, 0)
	active, inactive := 0, 0
	for _, j := range allJobs {
		if j.Domain != domain {
			continue
		}
		jobs = append(jobs, j)
		if j.Enabled {
			active++
		} else {
			inactive++
		}
	}
	return &CronListResponse{Domain: domain, DomainRoot: info.Root, Jobs: jobs, Active: active, Inactive: inactive}, nil
}

func validateCronJobRequest(req CronJobRequest) error {
	if err := validateCronSchedule(req.Schedule); err != nil {
		return err
	}
	switch req.TaskType {
	case "command":
		if strings.TrimSpace(req.Command) == "" {
			return errFmt("perintah wajib diisi")
		}
	case "http":
		if strings.TrimSpace(req.URL) == "" {
			return errFmt("URL wajib diisi")
		}
		if req.Method != "GET" && req.Method != "POST" {
			return errFmt("method HTTP harus GET atau POST")
		}
	default:
		return errFmt("tipe job tidak dikenal: %s (pakai command atau http)", req.TaskType)
	}
	return nil
}

// upsertCronJob melakukan Create (req.ID kosong) atau Update (req.ID diisi)
// — baca file (1 round-trip) -> ubah in-memory -> tulis file (1 round-trip).
func (s *Service) upsertCronJob(req CronJobRequest) (*CronJobInfo, error) {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return nil, err
	}
	if err := validateCronJobRequest(req); err != nil {
		return nil, err
	}
	if _, err := s.getDomain(req.ServerID, domain); err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return nil, err
	}

	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)

	_, jobs, foreign, err := s.readCronFile(access)
	if err != nil {
		return nil, err
	}

	newJob := CronJobInfo{
		ID:          req.ID,
		Domain:      domain,
		TaskType:    req.TaskType,
		Schedule:    strings.TrimSpace(req.Schedule),
		Command:     req.Command,
		URL:         req.URL,
		Method:      req.Method,
		Payload:     req.Payload,
		Headers:     req.Headers,
		Description: req.Description,
		Enabled:     req.Enabled,
	}
	if newJob.ID == "" {
		newJob.ID = uuid.NewString()[:8]
	}
	newJob.LogPath = cronLogPath(domain, newJob.ID)

	found := false
	for i, j := range jobs {
		if j.ID == newJob.ID {
			jobs[i] = newJob
			found = true
			break
		}
	}
	if !found {
		jobs = append(jobs, newJob)
	}

	roots, err := s.domainRootsFor(req.ServerID, jobs)
	if err != nil {
		return nil, err
	}
	if err := s.writeCronFile(access, jobs, foreign, roots); err != nil {
		return nil, err
	}
	newJob.ScheduleText = describeSchedule(newJob.Schedule)
	return &newJob, nil
}

// CreateCronJob membuat job cron baru untuk satu domain.
func (s *Service) CreateCronJob(req CronJobRequest) (*CronJobInfo, error) {
	req.ID = ""
	return s.upsertCronJob(req)
}

// UpdateCronJob memperbarui job cron yang sudah ada.
func (s *Service) UpdateCronJob(req CronJobRequest) (*CronJobInfo, error) {
	if req.ID == "" {
		return nil, errFmt("id job wajib diisi untuk update")
	}
	return s.upsertCronJob(req)
}

// ToggleCronJob mengaktifkan/menonaktifkan satu job tanpa mengubah field lain.
func (s *Service) ToggleCronJob(req CronToggleRequest) error {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return err
	}

	s.mutex.Lock(req.ServerID)
	defer s.mutex.Unlock(req.ServerID)

	_, jobs, foreign, err := s.readCronFile(access)
	if err != nil {
		return err
	}
	found := false
	for i, j := range jobs {
		if j.ID == req.ID && j.Domain == domain {
			jobs[i].Enabled = req.Enabled
			found = true
			break
		}
	}
	if !found {
		return errFmt("job %s tidak ditemukan", req.ID)
	}
	roots, err := s.domainRootsFor(req.ServerID, jobs)
	if err != nil {
		return err
	}
	return s.writeCronFile(access, jobs, foreign, roots)
}

// DeleteCronJob menghapus satu job cron (log file yang sudah ada TIDAK ikut
// dihapus, supaya histori log tetap bisa diaudit).
func (s *Service) DeleteCronJob(serverID, domain, jobID string) error {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return err
	}
	access, err := s.resolveAccess(serverID)
	if err != nil {
		return err
	}

	s.mutex.Lock(serverID)
	defer s.mutex.Unlock(serverID)

	_, jobs, foreign, err := s.readCronFile(access)
	if err != nil {
		return err
	}
	kept := make([]CronJobInfo, 0, len(jobs))
	found := false
	for _, j := range jobs {
		if j.ID == jobID && j.Domain == domain {
			found = true
			continue
		}
		kept = append(kept, j)
	}
	if !found {
		return errFmt("job %s tidak ditemukan", jobID)
	}
	roots, err := s.domainRootsFor(serverID, kept)
	if err != nil {
		return err
	}
	return s.writeCronFile(access, kept, foreign, roots)
}

// ReadCronLog membaca log satu job cron.
func (s *Service) ReadCronLog(req CronLogRequest) (*CronLogResponse, error) {
	domain, err := NormalizeDomain(req.Domain)
	if err != nil {
		return nil, err
	}
	access, err := s.resolveAccess(req.ServerID)
	if err != nil {
		return nil, err
	}
	path := cronLogPath(domain, req.ID)
	lines := clampLogLines(req.Lines)

	script := `if [ ! -f ` + shellQuote(path) + ` ]; then echo "__poinhost_NOT_FOUND__"; exit 0; fi
tail -n ` + strconv.Itoa(lines) + ` ` + shellQuote(path) + ` 2>/dev/null || true`
	res, err := s.run(access, script, 15*time.Second)
	if err != nil {
		return nil, err
	}
	if strings.Contains(res.Stdout, "__poinhost_NOT_FOUND__") {
		return &CronLogResponse{ID: req.ID, Domain: domain, Path: path, Exists: false}, nil
	}
	return &CronLogResponse{ID: req.ID, Domain: domain, Path: path, Content: res.Stdout, Exists: true}, nil
}
