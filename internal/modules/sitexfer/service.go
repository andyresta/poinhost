package sitexfer

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/andyresta/poinhost/internal/core/sshpool"
	"github.com/andyresta/poinhost/internal/modules/dbxfer"
	"github.com/andyresta/poinhost/internal/modules/servers"
	"github.com/andyresta/poinhost/internal/modules/website"
)

const (
	transferTimeout = 12 * time.Hour
	scanTimeout     = 10 * time.Minute
)

// Service migrasi website (domain/subdomain terpilih) antar server:
// file, vhost, database + user-nya, akun SFTP, dan cron. PHP dan SSL
// SENGAJA tidak ikut — lihat website.migratedVhostOptions.
//
// Situs di server asal TIDAK dihentikan atau diubah sama sekali (beda dari
// migrasi Docker): dua salinan situs tidak saling bertabrakan sampai DNS
// dipindah, dan pengguna butuh situs asal tetap melayani selama memeriksa
// hasil migrasi di tujuan.
type Service struct {
	servers *servers.Service
	website *website.Service
	pool    *sshpool.Pool

	mu        sync.Mutex
	jobs      map[string]*Job
	activeJob string

	emitMu sync.RWMutex
	emit   func(Progress)
}

func NewService(serversSvc *servers.Service, websiteSvc *website.Service, pool *sshpool.Pool) *Service {
	return &Service{servers: serversSvc, website: websiteSvc, pool: pool, jobs: map[string]*Job{}}
}

func (s *Service) SetEmitter(emit func(Progress)) {
	s.emitMu.Lock()
	s.emit = emit
	s.emitMu.Unlock()
}

func (s *Service) publish(p Progress) {
	s.emitMu.RLock()
	emit := s.emit
	s.emitMu.RUnlock()
	if emit != nil {
		emit(p)
	}
}

// planData rencana lengkap, termasuk data internal (hash password, grant)
// yang tidak pernah dikirim ke frontend.
type planData struct {
	plan     Plan
	selected []website.DomainInfo
	dbUsers  []website.DBUserExport
	sftp     []website.SFTPAccountExport
	cron     []website.CronJobInfo
	pgACL    map[string][2]string // db -> {owner, acl}
}

func (pd *planData) block(format string, args ...any) {
	pd.plan.Problems = append(pd.plan.Problems, Problem{Blocking: true, Message: fmt.Sprintf(format, args...)})
}

func (pd *planData) warn(format string, args ...any) {
	pd.plan.Problems = append(pd.plan.Problems, Problem{Message: fmt.Sprintf(format, args...)})
}

func normalizeRequest(req StartRequest) (StartRequest, error) {
	req.SourceServerID = strings.TrimSpace(req.SourceServerID)
	req.DestServerID = strings.TrimSpace(req.DestServerID)
	if req.SourceServerID == "" || req.DestServerID == "" {
		return req, fmt.Errorf("server asal dan server tujuan wajib dipilih")
	}
	if req.SourceServerID == req.DestServerID {
		return req, fmt.Errorf("server asal dan tujuan tidak boleh sama")
	}
	seen := map[string]bool{}
	var domains []string
	for _, d := range req.Domains {
		n, err := website.NormalizeDomain(d)
		if err != nil {
			return req, err
		}
		if !seen[n] {
			seen[n] = true
			domains = append(domains, n)
		}
	}
	if len(domains) == 0 {
		return req, fmt.Errorf("pilih minimal satu domain atau subdomain")
	}
	sort.Strings(domains)
	req.Domains = domains
	return req, nil
}

// Preview menyusun rencana + preflight tanpa mengubah apa pun di server mana pun.
func (s *Service) Preview(req StartRequest) (*Plan, error) {
	req, err := normalizeRequest(req)
	if err != nil {
		return nil, err
	}
	pd, err := s.buildPlan(req)
	if err != nil {
		return nil, err
	}
	return &pd.plan, nil
}

func (s *Service) buildPlan(req StartRequest) (*planData, error) {
	src, dst := req.SourceServerID, req.DestServerID
	if _, err := s.servers.Get(src); err != nil {
		return nil, fmt.Errorf("server asal: %w", err)
	}
	if _, err := s.servers.Get(dst); err != nil {
		return nil, fmt.Errorf("server tujuan: %w", err)
	}
	pd := &planData{pgACL: map[string][2]string{}}
	pd.plan.Problems = []Problem{}

	srcDomains, err := s.website.DomainsOnServer(src)
	if err != nil {
		return nil, fmt.Errorf("baca domain di server asal: %w", err)
	}
	byName := map[string]website.DomainInfo{}
	for _, d := range srcDomains {
		byName[d.Domain] = d
	}
	for _, name := range req.Domains {
		info, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("domain %s tidak ditemukan di server asal", name)
		}
		pd.selected = append(pd.selected, info)
	}
	// Induk dulu, baru subdomain — urutan penulisan vhost di tujuan.
	sort.SliceStable(pd.selected, func(i, j int) bool {
		return !pd.selected[i].IsSubdomain && pd.selected[j].IsSubdomain
	})
	for _, d := range pd.selected {
		pd.plan.Domains = append(pd.plan.Domains, PlanDomain{
			Domain: d.Domain, Parent: d.Parent, IsSubdomain: d.IsSubdomain, Root: d.Root, Enabled: d.Enabled,
			PHPVersion: d.PHPVersion, SSLEnabled: d.SSLEnabled, Proxy: d.ProxyTarget != "" || len(d.ProxyRules) > 0,
		})
	}

	// --- File ---
	pd.plan.Paths = computePaths(pd.selected, srcDomains)
	for i := range pd.plan.Paths {
		p := &pd.plan.Paths[i]
		if err := validateSitePath(p.Path); err != nil {
			pd.block("%v", err)
			continue
		}
		out, err := s.website.RunRootScript(src, statScript(*p), scanTimeout)
		if err != nil {
			pd.warn("Gagal menghitung ukuran %s: %v", p.Path, err)
			continue
		}
		p.Files, p.Bytes = parseStat(out)
		pd.plan.TotalBytes += p.Bytes
	}

	// --- Server tujuan: nginx + bentrok domain ---
	dstList, err := s.website.List(dst)
	if err != nil {
		return nil, fmt.Errorf("baca server tujuan: %w", err)
	}
	if !dstList.Nginx.Installed {
		pd.block("Nginx belum terpasang di server tujuan — pasang dulu dari menu Website server tujuan")
	} else {
		existing := map[string]bool{}
		for _, d := range dstList.Domains {
			existing[d.Domain] = true
		}
		for _, d := range pd.selected {
			if existing[d.Domain] {
				pd.block("Domain %s sudah ada di server tujuan", d.Domain)
			}
		}
	}

	if req.IncludeDatabases {
		if err := s.planDatabases(req, pd); err != nil {
			return nil, err
		}
	}
	if req.IncludeSFTP {
		if err := s.planSFTP(req, pd); err != nil {
			return nil, err
		}
	}
	if req.IncludeCron {
		jobs, err := s.website.ExportCronJobs(src, req.Domains)
		if err != nil {
			pd.warn("Gagal membaca cron di server asal: %v", err)
		} else {
			pd.cron = jobs
			pd.plan.CronJobs = len(jobs)
		}
	}

	pd.plan.CanStart = true
	for _, p := range pd.plan.Problems {
		if p.Blocking {
			pd.plan.CanStart = false
		}
	}
	if pd.plan.Databases == nil {
		pd.plan.Databases = []PlanDatabase{}
	}
	if pd.plan.DBUsers == nil {
		pd.plan.DBUsers = []PlanDBUser{}
	}
	if pd.plan.SFTP == nil {
		pd.plan.SFTP = []PlanSFTP{}
	}
	return pd, nil
}

func (s *Service) planDatabases(req StartRequest, pd *planData) error {
	src, dst := req.SourceServerID, req.DestServerID
	type key struct{ engine, name string }
	dbs := map[key][]string{}
	var order []key
	for _, d := range pd.selected {
		links, err := s.website.ListDomainDatabases(src, d.Domain)
		if err != nil {
			return err
		}
		for _, l := range links {
			k := key{l.Engine, l.Database}
			if _, ok := dbs[k]; !ok {
				order = append(order, k)
			}
			dbs[k] = append(dbs[k], d.Domain)
		}
	}
	byEngine := map[string][]string{}
	for _, k := range order {
		byEngine[k.engine] = append(byEngine[k.engine], k.name)
		pdb := PlanDatabase{Engine: k.engine, Name: k.name, Domains: dbs[k]}
		if k.engine == "postgresql" {
			owner, acl, err := s.website.DBDatabaseACL(src, k.name)
			if err != nil {
				pd.warn("Gagal membaca pemilik database %s: %v", k.name, err)
			} else {
				pdb.Owner = owner
				pd.pgACL[k.name] = [2]string{owner, acl}
			}
		}
		pd.plan.Databases = append(pd.plan.Databases, pdb)
	}

	engines := make([]string, 0, len(byEngine))
	for e := range byEngine {
		engines = append(engines, e)
	}
	sort.Strings(engines)
	for _, engine := range engines {
		names := byEngine[engine]
		st, err := s.website.DBStatus(dst, engine)
		if err != nil || !st.Installed || !st.Active {
			pd.block("%s belum terpasang/aktif di server tujuan — dibutuhkan untuk database %s", engineLabel(engine), strings.Join(names, ", "))
			continue
		}
		for _, n := range names {
			exists, err := s.website.DBDatabaseExists(dst, engine, n)
			if err != nil {
				pd.warn("Gagal memeriksa database %s di tujuan: %v", n, err)
			} else if exists {
				pd.block("Database %s (%s) sudah ada di server tujuan — tidak ditimpa", n, engineLabel(engine))
			}
		}
		planned, exports, warns, err := s.planUsers(src, dst, engine, names)
		if err != nil {
			return err
		}
		for _, w := range warns {
			pd.warn("%s", w)
		}
		pd.dbUsers = append(pd.dbUsers, exports...)
		pd.plan.DBUsers = append(pd.plan.DBUsers, planned...)
	}
	return nil
}

// planUsers menentukan aksi tiap user DB yang terkait dengan database
// `names`. exports hanya berisi user yang akan dibuat (create /
// needs-password), lengkap dengan hash/grant internalnya.
func (s *Service) planUsers(src, dst, engine string, names []string) (planned []PlanDBUser, exports []website.DBUserExport, warns []string, err error) {
	users, err := s.website.ExportDBUsers(src, engine, names)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("baca user database: %w", err)
	}
	dstFlavor := ""
	if engine == "mysql" {
		if dstFlavor, err = s.website.DBMySQLFlavor(dst); err != nil {
			return nil, nil, nil, fmt.Errorf("baca jenis MySQL di tujuan: %w", err)
		}
	}
	for _, u := range users {
		pu := PlanDBUser{Engine: engine, Username: u.Username, Host: u.Host, Databases: u.Databases, Action: DBUserCreate}
		switch {
		case u.SkipReason != "":
			pu.Action, pu.Note = DBUserSkipGlobal, u.SkipReason
		default:
			exists, err := s.website.DBUserHostExists(dst, engine, u.Username, u.Host)
			if err != nil {
				warns = append(warns, fmt.Sprintf("Gagal memeriksa user %s di tujuan: %v", u.Username, err))
			} else if exists {
				pu.Action, pu.Note = DBUserSkipExists, "sudah ada di server tujuan — dilewati (password & grant tidak diubah)"
			} else if !website.MySQLUserPortable(u, dstFlavor) {
				pu.Action = DBUserNeedsPassword
				pu.Note = fmt.Sprintf("hash %s (%s) tidak dikenal %s di server tujuan — isi password aslinya",
					u.Plugin, flavorLabel(u.SourceFlavor), flavorLabel(dstFlavor))
			}
		}
		if pu.Action == DBUserCreate || pu.Action == DBUserNeedsPassword {
			exports = append(exports, u)
		}
		planned = append(planned, pu)
	}
	return planned, exports, warns, nil
}

func flavorLabel(f string) string {
	if f == "mariadb" {
		return "MariaDB"
	}
	return "MySQL"
}

// RepairDBUsers membuat HANYA user database (beserta grant-nya) untuk
// domain yang sudah dimigrasi sebelumnya — kasus nyata: migrasi MySQL 8 →
// MariaDB di mana user tidak bisa dibuat dari hash, sementara file,
// database, dan vhost sudah terpasang dan tidak boleh dimigrasi ulang.
// User yang sudah ada di tujuan tetap dilewati.
func (s *Service) RepairDBUsers(req StartRequest) (*RepairResult, error) {
	req, err := normalizeRequest(req)
	if err != nil {
		return nil, err
	}
	src, dst := req.SourceServerID, req.DestServerID
	byEngine := map[string][]string{}
	seen := map[string]bool{}
	for _, d := range req.Domains {
		links, err := s.website.ListDomainDatabases(src, d)
		if err != nil {
			return nil, err
		}
		for _, l := range links {
			if k := l.Engine + ":" + l.Database; !seen[k] {
				seen[k] = true
				byEngine[l.Engine] = append(byEngine[l.Engine], l.Database)
			}
		}
	}
	res := &RepairResult{Items: []ItemResult{}}
	for engine, names := range byEngine {
		planned, exports, _, err := s.planUsers(src, dst, engine, names)
		if err != nil {
			return nil, err
		}
		for _, pu := range planned {
			if pu.Action == DBUserSkipGlobal || pu.Action == DBUserSkipExists {
				res.Items = append(res.Items, ItemResult{Key: "dbuser:" + DBUserKey(pu.Username, pu.Host), Kind: KindDBUser,
					Label: dbUserLabel(website.DBUserExport{Username: pu.Username, Host: pu.Host}), Status: ItemSkipped, Warning: pu.Note})
			}
		}
		for _, u := range exports {
			item := ItemResult{Key: "dbuser:" + DBUserKey(u.Username, u.Host), Kind: KindDBUser, Label: dbUserLabel(u)}
			existed, err := s.website.ImportDBUser(dst, u, req.DBUserPasswords[DBUserKey(u.Username, u.Host)])
			switch {
			case err != nil:
				item.Status, item.Error = ItemFailed, err.Error()
			case existed:
				item.Status, item.Warning = ItemSkipped, "sudah ada di tujuan — dilewati"
			default:
				item.Status = ItemDone
			}
			res.Items = append(res.Items, item)
		}
	}
	return res, nil
}

func (s *Service) planSFTP(req StartRequest, pd *planData) error {
	src, dst := req.SourceServerID, req.DestServerID
	schemeOK := map[string]string{}
	for _, d := range pd.selected {
		if d.IsSubdomain {
			continue // akun SFTP selalu milik domain induk
		}
		accs, err := s.website.ExportSFTPAccounts(src, d.Domain)
		if err != nil {
			pd.warn("Gagal membaca akun SFTP %s: %v", d.Domain, err)
			continue
		}
		for _, a := range accs {
			ps := PlanSFTP{Username: a.Username, Domain: a.Domain, Enabled: a.Enabled, HashScheme: a.HashScheme}
			exists, err := s.website.LinuxUserExists(dst, a.Username)
			if err != nil {
				pd.warn("Gagal memeriksa user %s di tujuan: %v", a.Username, err)
			} else if exists {
				pd.block("Username SFTP %s sudah dipakai user Linux di server tujuan", a.Username)
			}
			if _, ok := schemeOK[a.HashScheme]; !ok {
				v, _ := s.website.CryptSchemeSupported(dst, a.HashScheme)
				schemeOK[a.HashScheme] = v
			}
			switch schemeOK[a.HashScheme] {
			case "no":
				ps.Note = "server tujuan tidak mengenal hash " + a.HashScheme + " — login akan gagal, set password baru sesudah migrasi"
				pd.warn("Akun SFTP %s: %s", a.Username, ps.Note)
			case "unknown":
				ps.Note = "kompatibilitas hash " + a.HashScheme + " tidak bisa dipastikan (perl tidak ada di tujuan)"
			}
			pd.sftp = append(pd.sftp, a)
			pd.plan.SFTP = append(pd.plan.SFTP, ps)
		}
	}
	return nil
}

func engineLabel(engine string) string {
	if engine == "postgresql" {
		return "PostgreSQL"
	}
	return "MySQL/MariaDB"
}

// Start memulai migrasi di latar belakang. Rencana disusun ULANG di dalam
// job (bukan memakai hasil preview) supaya bentrok yang muncul di antara
// preview dan klik "Mulai" tetap tertangkap.
func (s *Service) Start(req StartRequest) (*Progress, error) {
	req, err := normalizeRequest(req)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if j, ok := s.jobs[s.activeJob]; ok && j != nil && !j.IsTerminal() {
		s.mu.Unlock()
		return nil, fmt.Errorf("masih ada migrasi website berjalan — tunggu selesai atau batalkan dulu")
	}
	parent, parentCancel := context.WithTimeout(context.Background(), transferTimeout)
	job := newJob(parent, req, s.publish)
	s.jobs[job.ID] = job
	s.activeJob = job.ID
	s.mu.Unlock()

	go func() {
		defer parentCancel()
		s.run(job)
	}()
	snap := job.Snapshot()
	return &snap, nil
}

func (s *Service) job(id string) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobs[id]
}

func (s *Service) Status(jobID string) (*Progress, error) {
	j := s.job(jobID)
	if j == nil {
		return nil, fmt.Errorf("job migrasi tidak ditemukan")
	}
	snap := j.Snapshot()
	return &snap, nil
}

func (s *Service) Cancel(jobID string) (*Progress, error) {
	j := s.job(jobID)
	if j == nil {
		return nil, fmt.Errorf("job migrasi tidak ditemukan")
	}
	j.Cancel()
	snap := j.Snapshot()
	return &snap, nil
}

func dbKey(engine, name string) string        { return "db:" + engine + ":" + name }
func dbUserKey(u website.DBUserExport) string { return "dbuser:" + u.Engine + ":" + u.Username + "@" + u.Host }
func dbUserLabel(u website.DBUserExport) string {
	if u.Host != "" {
		return u.Username + "@" + u.Host
	}
	return u.Username
}

func (s *Service) run(job *Job) {
	ctx := job.Context()
	req := job.Req
	src, dst := req.SourceServerID, req.DestServerID

	job.SetStatus(StatusInspecting, "Memeriksa server asal dan tujuan…")
	pd, err := s.buildPlan(req)
	if err != nil {
		job.SetStatus(StatusFailed, err.Error())
		return
	}
	if !pd.plan.CanStart {
		var msgs []string
		for _, p := range pd.plan.Problems {
			if p.Blocking {
				msgs = append(msgs, p.Message)
			}
		}
		job.SetStatus(StatusFailed, "Tidak bisa dimulai: "+strings.Join(msgs, "; "))
		return
	}
	// User yang hash-nya tidak portabel wajib punya password SEBELUM ada
	// yang ditulis — kalau tidak, situs pindah tanpa user DB-nya.
	var missingPw []string
	for _, u := range pd.plan.DBUsers {
		if u.Action == DBUserNeedsPassword && req.DBUserPasswords[DBUserKey(u.Username, u.Host)] == "" {
			missingPw = append(missingPw, DBUserKey(u.Username, u.Host))
		}
	}
	if len(missingPw) > 0 {
		job.SetStatus(StatusFailed, "Isi password untuk user database: "+strings.Join(missingPw, ", "))
		return
	}

	// Daftarkan semua langkah dulu supaya UI langsung menampilkan urutannya.
	for _, u := range pd.dbUsers {
		job.AddItem(dbUserKey(u), KindDBUser, dbUserLabel(u))
	}
	for _, d := range pd.plan.Databases {
		job.AddItem(dbKey(d.Engine, d.Name), KindDatabase, d.Name)
		job.AddTotalBytes(s.website.DBEstimateSize(src, d.Engine, d.Name, nil))
	}
	for _, a := range pd.sftp {
		job.AddItem("sftp:"+a.Username, KindSFTP, a.Username)
	}
	for _, p := range pd.plan.Paths {
		job.AddItem("files:"+p.Path, KindFiles, p.Path)
		job.AddTotalBytes(p.Bytes)
	}
	for _, d := range pd.selected {
		job.AddItem("vhost:"+d.Domain, KindVhost, d.Domain)
	}
	if len(pd.cron) > 0 {
		job.AddItem("cron", KindCron, fmt.Sprintf("%d job", len(pd.cron)))
	}

	job.SetStatus(StatusRunning, "Membuka koneksi…")
	srcHandle, srcClient, err := s.pool.OpenDedicated(ctx, src)
	if err != nil {
		job.SetStatus(StatusFailed, "Gagal terhubung ke server asal: "+err.Error())
		return
	}
	defer s.pool.CloseDedicated(src, srcHandle)
	dstHandle, dstClient, err := s.pool.OpenDedicated(ctx, dst)
	if err != nil {
		job.SetStatus(StatusFailed, "Gagal terhubung ke server tujuan: "+err.Error())
		return
	}
	defer s.pool.CloseDedicated(dst, dstHandle)

	canceled := func() bool {
		if ctx.Err() != nil {
			job.SetStatus(StatusCanceled, "Dibatalkan — langkah yang sudah selesai TIDAK dibatalkan di server tujuan")
			return true
		}
		return false
	}

	// 1. User database — sebelum restore (OWNER TO / DEFINER).
	job.SetStatus(StatusRunning, "Membuat user database…")
	for _, u := range pd.dbUsers {
		if canceled() {
			return
		}
		key := dbUserKey(u)
		job.SetItem(key, ItemRunning, "")
		existed, err := s.website.ImportDBUser(dst, u, req.DBUserPasswords[DBUserKey(u.Username, u.Host)])
		switch {
		case err != nil:
			job.SetItem(key, ItemFailed, err.Error())
		case existed:
			job.SetWarning(key, "sudah ada di tujuan — dilewati")
			job.SetItem(key, ItemSkipped, "")
		default:
			job.SetItem(key, ItemDone, "")
			s.copyVaultCredential(src, dst, u)
		}
	}

	// Password polos tidak dibutuhkan lagi sesudah langkah ini — buang dari
	// memori job (job tetap tersimpan sampai aplikasi ditutup).
	req.DBUserPasswords = nil
	job.Req.DBUserPasswords = nil

	// 2. Database.
	gtid := false
	for _, d := range pd.plan.Databases {
		if d.Engine == "mysql" {
			gtid = dbxfer.MysqldumpSupportsGtidPurged(ctx, srcClient)
			break
		}
	}
	for _, d := range pd.plan.Databases {
		if canceled() {
			return
		}
		key := dbKey(d.Engine, d.Name)
		job.SetStatus(StatusRunning, "Menyalin database "+d.Name+"…")
		job.SetItem(key, ItemRunning, "")
		if err := s.copyDatabase(ctx, job, pd, srcClient, dstClient, d, gtid); err != nil {
			msg := err.Error()
			if ctx.Err() != nil {
				msg = "dibatalkan"
			}
			job.SetItem(key, ItemFailed, msg)
			continue
		}
		job.SetItem(key, ItemDone, "")
		for _, dom := range d.Domains {
			_ = s.website.LinkDomainDatabase(dst, dom, d.Engine, d.Name)
		}
	}

	// 3. Akun SFTP — sebelum file, supaya tar bisa memetakan pemilik file
	// ke user ini berdasarkan nama.
	for _, a := range pd.sftp {
		if canceled() {
			return
		}
		key := "sftp:" + a.Username
		job.SetItem(key, ItemRunning, "")
		if err := s.website.ImportSFTPAccount(dst, a); err != nil {
			job.SetItem(key, ItemFailed, err.Error())
			continue
		}
		if !a.Enabled {
			job.SetDetail(key, "tetap nonaktif (terkunci) seperti di server asal")
		}
		job.SetItem(key, ItemDone, "")
	}

	// 4. File.
	for _, p := range pd.plan.Paths {
		if canceled() {
			return
		}
		key := "files:" + p.Path
		job.SetStatus(StatusRunning, "Menyalin file "+p.Path+"…")
		job.SetItem(key, ItemRunning, "")
		if err := s.copyPath(ctx, job, srcClient, dstClient, p, key); err != nil {
			msg := err.Error()
			if ctx.Err() != nil {
				msg = "dibatalkan"
			}
			job.SetItem(key, ItemFailed, msg)
			continue
		}
		job.SetItem(key, ItemDone, "")
	}

	// 5. Vhost — hanya untuk domain yang file-nya berhasil tersalin.
	job.SetStatus(StatusRunning, "Menulis vhost…")
	vhostOK := map[string]bool{}
	for _, d := range pd.selected {
		if canceled() {
			return
		}
		key := "vhost:" + d.Domain
		if fp := pathFor(pd.plan.Paths, d.Root); fp != "" && job.ItemStatus("files:"+fp) != ItemDone {
			job.SetItem(key, ItemSkipped, "file domain ini gagal disalin — vhost tidak dibuat")
			continue
		}
		job.SetItem(key, ItemRunning, "")
		if err := s.website.ProvisionMigratedVhost(dst, d); err != nil {
			job.SetItem(key, ItemFailed, err.Error())
			continue
		}
		vhostOK[d.Domain] = true
		job.SetItem(key, ItemDone, "")
	}

	// 6. Cron — baris cron `cd` ke document root domain di tujuan.
	if len(pd.cron) > 0 && !canceled() {
		var jobs []website.CronJobInfo
		for _, c := range pd.cron {
			if vhostOK[c.Domain] {
				jobs = append(jobs, c)
			}
		}
		job.SetItem("cron", ItemRunning, "")
		n, err := s.website.ImportCronJobs(dst, jobs)
		switch {
		case err != nil:
			job.SetItem("cron", ItemFailed, err.Error())
		default:
			job.SetDetail("cron", fmt.Sprintf("%d job ditambahkan", n))
			if len(jobs) < len(pd.cron) {
				job.SetWarning("cron", "job milik domain yang vhost-nya gagal dilewati")
			}
			job.SetItem("cron", ItemDone, "")
		}
	} else if ctx.Err() != nil {
		return
	}

	buildReport(job, pd, vhostOK)
	job.doneBytes.Store(job.totalBytes.Load())
	if job.HasFailures() {
		job.SetStatus(StatusPartial, "Selesai dengan sebagian langkah gagal — lihat rincian")
		return
	}
	job.SetStatus(StatusDone, "Migrasi selesai — lanjutkan daftar periksa di bawah")
}

// pathFor folder transfer yang mencakup document root domain.
func pathFor(paths []PlanPath, root string) string {
	for _, p := range paths {
		if within(p.Path, root) {
			return p.Path
		}
	}
	return ""
}

func (s *Service) copyDatabase(ctx context.Context, job *Job, pd *planData, srcClient, dstClient sshClient, d PlanDatabase, gtid bool) error {
	if err := s.website.DBCreateDatabase(website.DBCreateDatabaseRequest{ServerID: job.Req.DestServerID, Engine: d.Engine, Name: d.Name}); err != nil {
		return fmt.Errorf("buat database di tujuan: %w", err)
	}
	dumpCmd, err := s.website.WrapCommand(job.Req.SourceServerID, dbxfer.DumpCommand(d.Engine, d.Name, gtid))
	if err != nil {
		return err
	}
	restoreCmd, err := s.website.WrapStreamingCommand(job.Req.DestServerID, dbxfer.RestoreCommand(d.Engine, d.Name))
	if err != nil {
		return err
	}
	itemCtx, cancel := context.WithTimeout(ctx, transferTimeout)
	defer cancel()
	if err := pipe(itemCtx, srcClient, dstClient, dumpCmd, restoreCmd, job.AddDoneBytes); err != nil {
		return err
	}
	key := dbKey(d.Engine, d.Name)
	if d.Engine == "postgresql" {
		if acl, ok := pd.pgACL[d.Name]; ok {
			missing, err := s.website.DBApplyDatabaseACL(job.Req.DestServerID, d.Name, acl[0], acl[1])
			if err != nil {
				job.SetWarning(key, "pemilik/hak akses database tidak bisa diterapkan: "+err.Error())
			} else if len(missing) > 0 {
				job.SetWarning(key, "role tidak ada di tujuan, hak aksesnya dilewati: "+strings.Join(missing, ", "))
			}
		}
	}
	verify, detail := dbxfer.VerifyDatabase(s.website, job.Req.SourceServerID, job.Req.DestServerID, d.Engine, d.Name)
	if verify == dbxfer.VerifyMismatch {
		job.SetWarning(key, "jumlah baris beda: "+detail)
	} else {
		job.SetDetail(key, detail)
	}
	return nil
}

func (s *Service) copyPath(ctx context.Context, job *Job, srcClient, dstClient sshClient, p PlanPath, key string) error {
	src, dst := job.Req.SourceServerID, job.Req.DestServerID
	ownersOut, err := s.website.RunRootScript(src, ownerScanScript(p), scanTimeout)
	if err != nil {
		return fmt.Errorf("baca pemilik file: %w", err)
	}
	srcCmd, err := s.website.WrapCommand(src, tarCreateCmd(p, job.Req.Compress))
	if err != nil {
		return err
	}
	dstCmd, err := s.website.WrapStreamingCommand(dst, tarExtractCmd(job.Req.Compress))
	if err != nil {
		return err
	}
	itemCtx, cancel := context.WithTimeout(ctx, transferTimeout)
	defer cancel()
	if err := pipe(itemCtx, srcClient, dstClient, srcCmd, dstCmd, job.AddDoneBytes); err != nil {
		return err
	}

	fixOut, err := s.website.RunRootScript(dst, ownerFixScript(p, parseOwners(ownersOut)), scanTimeout)
	if err != nil {
		job.SetWarning(key, "perbaikan pemilik file gagal: "+err.Error())
	} else if fixed := fixedOwners(fixOut); len(fixed) > 0 {
		job.SetWarning(key, "pemilik yang tidak ada di tujuan dialihkan ke user web: "+strings.Join(fixed, ", "))
	}

	srcStat, err1 := s.website.RunRootScript(src, statScript(p), scanTimeout)
	dstStat, err2 := s.website.RunRootScript(dst, statScript(p), scanTimeout)
	if err1 == nil && err2 == nil {
		sf, sb := parseStat(srcStat)
		df, db := parseStat(dstStat)
		if sf == df && sb == db {
			job.SetDetail(key, fmt.Sprintf("cocok: %d file, %s", sf, formatBytes(sb)))
		} else {
			job.SetWarning(key, fmt.Sprintf("beda — asal %d file/%s, tujuan %d file/%s (file berubah selama disalin?)",
				sf, formatBytes(sb), df, formatBytes(db)))
		}
	}
	return nil
}

func fixedOwners(out string) []string {
	var res []string
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "FIXED "); ok {
			res = append(res, rest)
		}
	}
	return res
}

// copyVaultCredential: kalau password user DB ini tersimpan di vault lokal
// untuk server asal (fitur Explore), simpan juga untuk server tujuan —
// passwordnya identik (hash-nya yang dipindah), jadi Explore langsung jalan.
func (s *Service) copyVaultCredential(src, dst string, u website.DBUserExport) {
	host := u.Host
	if u.Engine == "postgresql" {
		host = ""
	}
	pw, found, err := s.website.ExportCredentialPassword(src, u.Engine, u.Username, host)
	if err != nil || !found || pw == "" {
		return
	}
	_, _ = s.website.SaveDBCredential(website.SaveDBCredentialRequest{
		ServerID: dst, Engine: u.Engine, Username: u.Username, Host: u.Host, Password: pw,
	})
}

// buildReport daftar periksa manual sesudah migrasi.
func buildReport(job *Job, pd *planData, vhostOK map[string]bool) {
	for _, d := range pd.selected {
		if !vhostOK[d.Domain] {
			continue
		}
		if d.PHPVersion != "" {
			job.AddReport(fmt.Sprintf("PHP: pasang PHP %s di server tujuan lalu aktifkan untuk %s (tab PHP). Sampai itu, file .php di domain ini ditolak (403).", d.PHPVersion, d.Domain))
		}
		if d.SSLEnabled {
			job.AddReport(fmt.Sprintf("SSL: setelah DNS %s mengarah ke server tujuan, terbitkan sertifikat baru (tab SSL).", d.Domain))
		}
		if !d.Enabled {
			job.AddReport(fmt.Sprintf("%s nonaktif di server asal dan tetap nonaktif di tujuan.", d.Domain))
		}
	}
	for _, u := range pd.plan.DBUsers {
		if u.Action == DBUserSkipGlobal {
			job.AddReport(fmt.Sprintf("User database %s TIDAK dimigrasi (%s) — buat manual kalau dibutuhkan.", u.Username, u.Note))
		}
		if u.Action == DBUserSkipExists {
			job.AddReport(fmt.Sprintf("User database %s sudah ada di tujuan dan dilewati — pastikan password-nya sama dengan yang dipakai aplikasi.", u.Username))
		}
	}
	for _, a := range pd.plan.SFTP {
		if a.Note != "" {
			job.AddReport("SFTP " + a.Username + ": " + a.Note)
		}
	}
	job.AddReport("Situs di server asal tetap berjalan. Uji situs di tujuan (mis. lewat file hosts), lalu pindahkan DNS.")
	job.AddReport("Periksa konfigurasi aplikasi (.env, wp-config.php, dll) yang menunjuk ke IP/host server lama.")
}
