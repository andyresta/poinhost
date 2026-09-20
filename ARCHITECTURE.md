# Arsitektur poinhost

poinhost adalah rewrite [homepoin](https://github.com/andyresta/homepoin) (Self
Host Manager berbasis Go + browser tab) menjadi **aplikasi desktop native
cross-platform** (Windows/macOS/Linux) dengan skema koneksi & multi-tab yang
lebih matang.

## 1. Tech stack & alasannya

| Bagian | Pilihan | Kenapa |
|---|---|---|
| Backend | Go (reuse ~90% logic homepoin) | SSH pool, executor, auth, jobs sudah battle-tested di homepoin — rewrite ke Rust berarti membangun ulang ekosistem SSH/PTY yang jauh lebih tipis di Rust, tanpa manfaat performa nyata (operasinya I/O-bound, bukan CPU-bound) |
| Shell desktop | [Wails v2](https://wails.io) | "Tauri versi Go" — native WebView OS (WebView2/WebKit/GTK), binding Go↔JS langsung (bukan HTTP roundtrip ke localhost seperti homepoin), instalasi native (tray, installer per-OS) |
| Frontend | React + TypeScript + Vite | SPA — pindah tab/modul tidak pernah full page reload (beda dari homepoin yang server-rendered `templ`) |
| State | Zustand | Satu store kecil untuk tabs+servers, tanpa boilerplate Redux |
| DB lokal | SQLite (modernc.org/sqlite, no-CGO) | Sama seperti homepoin — tetap butuh data relasional (servers, tabs, kredensial), bukan sekadar config file |

Dibahas & diputuskan bersama sebelum implementasi (lihat riwayat percakapan):
Wails dipilih di atas Tauri+Rust (rewrite total, ekosistem SSH Rust tipis) dan
Tauri+Go-sidecar (dua proses terpisah, IPC lebih kasar).

## 2. Masalah di homepoin yang jadi pemicu desain ulang ini

Dari sesi debugging performa sebelumnya, dua akar masalah nyata di homepoin:

1. **`WarmConnection` on-demand per-klik**, bukan untuk semua server saat
   start — klik pertama ke server yang belum pernah disentuh terasa seperti
   "handshake dari nol".
2. **Slot terminal tunggal per server** (`sshpool.OpenTerminal` di homepoin)
   — membuka tab terminal kedua ke server yang sama **memutus** tab terminal
   pertama. Ini blocker keras untuk skema multi-tab yang diminta.
3. **Full page reload** tiap pindah modul (server-rendered, bukan SPA) —
   layout & data di-render ulang dari nol setiap navigasi walau koneksi SSH
   di baliknya sebenarnya sudah tersambung.

poinhost memperbaiki ketiganya secara struktural (bukan tambal sulam):

- `servers.Service.Bootstrap()` memanaskan **semua** server terdaftar saat
  aplikasi start (lihat `internal/modules/servers/service.go`), bukan lazy
  per-klik.
- `sshpool.Pool` tidak lagi punya slot `terminal` tunggal — diganti
  `OpenDedicated`/`CloseDedicated` yang berbasis **handle**, sehingga N tab
  boleh punya sesi dedicated (terminal, log stream) ke server yang sama
  secara bersamaan tanpa saling menggantikan.
- Frontend SPA: `TabContent.tsx` me-mount SEMUA tab yang terbuka sekaligus
  dan hanya toggle `hidden` — pindah tab/modul tidak pernah unmount/refetch.

## 3. Model inti: Connection vs Tab

Pemisahan yang tidak eksis secara eksplisit di homepoin:

```
┌─────────────────────────────────────────────────────────────────┐
│  internal/session  — "tab" = konsep UI, BUKAN koneksi            │
│                                                                   │
│   Tab { id, serverId, activeModule, position }                   │
│   Manager  : registry tab + persist layout (tabel ui_tabs)       │
│   TerminalRegistry : ikat sesi terminal dedicated -> tab pemilik  │
└───────────────────────────┬───────────────────────────────────────┘
                            │ tab.serverId
┌───────────────────────────▼───────────────────────────────────────┐
│  internal/core/sshpool  — "koneksi" = per SERVER, bukan per tab   │
│                                                                    │
│   serverPool {                                                    │
│     metrics    : 1 koneksi dedicated (heartbeat/monitoring)        │
│     shared     : N koneksi di-multiplex (files/docker/dbmanager)   │
│     dedicated  : map[handle]*ssh.Client (terminal, log stream)     │
│   }                                                                │
└─────────────────────────────────────────────────────────────────┘
```

Konsekuensi praktis:

- **3 tab ke server yang SAMA** → tetap 1 entry `serverPool`, shared
  connection dipakai bersama oleh ketiganya untuk operasi ringan (files,
  docker list, dst) — tidak ada penggandaan koneksi TCP hanya karena tab-nya
  banyak.
- **1 tab dengan 2 sub-sesi terminal** (mis. split pane, fitur masa depan) →
  2 entry di `serverPool.dedicated`, masing-masing independen.
- **Menutup tab** → `TerminalRegistry.CloseAllForTab` menutup semua koneksi
  dedicated milik tab itu SEBELUM tab dihapus dari `Manager` — tidak ada
  koneksi SSH menggantung tanpa pemilik.
- **Server dihapus** → `Pool.UnregisterServer` menutup seluruh
  `serverPool` (shared + metrics + dedicated) sekaligus, apa pun jumlah tab
  yang tadinya menunjuk ke sana.

## 4. Struktur folder

```
poinhost/
├── main.go                     # wails.Run(...), embed frontend/dist
├── app.go                      # App struct = SEMUA method yang di-bind ke frontend
├── embed.go                    # embed migrations/
├── wails.json
│
├── internal/
│   ├── core/                   # infra lintas-modul (setara internal/shared/ homepoin)
│   │   ├── backup/             # export/import arsip terenkripsi passphrase (§15)
│   │   ├── config/             # parameter runtime (trimmed — lihat §6 roadmap)
│   │   ├── database/           # buka SQLite WAL, migration runner
│   │   ├── secrets/            # vault password LOKAL: OS keychain + fallback file AES-GCM (§14)
│   │   └── sshpool/            # pool koneksi (lihat §3), executor, sftp, known_hosts, tunnel (§14)
│   │
│   ├── session/                 # BARU — tidak ada padanannya di homepoin
│   │   ├── tab.go               # struct Tab
│   │   ├── manager.go           # registry tab + persist ui_tabs
│   │   └── terminal.go          # TerminalRegistry (tab -> koneksi dedicated)
│   │
│   └── modules/                 # vertical slice per fitur, sama filosofi homepoin
│       ├── servers/              # CRUD server + status/metrik (lihat §8)
│       │   ├── dto.go
│       │   ├── repository.go     # CRUD tabel `servers`
│       │   ├── service.go        # CRUD + register/warm ke sshpool.Pool
│       │   ├── status.go         # ServerStatus DTO + fetchMetrics
│       │   ├── metrics.go        # script SSH + parser (paritas homepoin)
│       │   └── collector.go      # scheduler status/metrik (lihat §8)
│       ├── terminal/             # PTY interaktif (lihat §9) + exec-into-container (§11)
│       │   └── service.go        # bungkus koneksi dedicated jadi shell PTY (Open/OpenCommand)
│       ├── files/                 # file manager SFTP (lihat §10)
│       │   ├── dto.go
│       │   ├── pathutil.go        # NormalizePath/JoinPath/dst (traversal-safe)
│       │   ├── archive_cmd.go     # command zip/tar.gz + fallback python3
│       │   └── service.go         # list/mkdir/upload/download/delete/compress
│       ├── docker/                # Containers + Networks via CLI docker (lihat §11)
│       │   ├── dto.go
│       │   ├── access.go          # dockerAccess/resolveAccess/wrap (sudo ke root)
│       │   ├── parse.go           # parsing TSV docker ps + JSON inspect/stats/network
│       │   ├── config.go          # inspect -> createTemplate -> buildCreateArgs (recreate)
│       │   ├── service.go         # ListContainers + start/stop/restart/remove + logs/stats
│       │   ├── service_networks.go
│       │   ├── service_config.go  # InspectContainer/RecreateContainer
│       │   ├── install.go         # detect distro + install/start Docker engine
│       │   └── exec.go            # BuildExecCommand (dipakai terminal.OpenCommand)
│       └── website/               # domain/vhost Nginx + PHP-FPM + SSL + 7 tab (§12-§13)
│           ├── dto.go
│           ├── access.go          # websiteAccess/resolveAccess/wrap (selalu butuh root)
│           ├── parse.go           # NormalizeDomain, shellQuote, util kecil
│           ├── service.go         # cache status Nginx/domain + StreamInstall generik
│           ├── nginx.go           # DetectStatus gabungan+cache, TestReload gabungan
│           ├── vhost.go           # builder vhost gabungan (static/PHP/proxy) + webroot
│           ├── domains.go         # List SATU round-trip, Create/Delete/SetEnabled/CreateWebsite
│           ├── php.go             # status/install repo+versi/switch per domain
│           ├── ssl.go             # status/issue/enable/disable/renew via certbot
│           ├── dns.go             # preview/export zona BIND (tanpa exec SSH)
│           ├── logs.go            # snapshot + stream access/error log per domain
│           ├── proxy.go           # edit ProxyTarget/ProxyRules vhost (whole-domain + per-path)
│           ├── sftp.go            # akun Linux ter-chroot per domain (useradd + sshd_config.d)
│           ├── cron.go            # job command/HTTP per domain, /etc/cron.d/poinhost
│           ├── database.go        # provisioning MySQL/PostgreSQL (bukan browser tabel)
│           ├── dbcreds.go         # simpan/lupa/list kredensial DB via vault lokal (§14)
│           ├── dbconnpool.go      # cache koneksi Explore generik, dipakai MySQL & PostgreSQL (§14)
│           ├── mysqlexplore.go    # Explore MySQL: koneksi driver asli tunneled (§14)
│           ├── pgexplore.go       # Explore PostgreSQL: satu koneksi per database (§14)
│           └── domaindb.go        # tautan domain<->database, kurasi lokal (§14)
│
├── migrations/
│   └── 001_core.sql             # servers, app_settings, activity_logs, ui_tabs,
│                                 # website_db_credentials, website_domain_databases (§14)
│
└── frontend/
    ├── src/
    │   ├── store/tabs.ts         # Zustand: servers + tabs + statuses + activeTabId
    │   ├── features/
    │   │   ├── servers/          # ServersPage, ServerFormModal, ServerWorkspace,
    │   │   │                     # OverviewPanel, TerminalPanel (xterm.js),
    │   │   │                     # FilesPanel, StatusDot, PromptModal, CompressModal,
    │   │   │                     # DockerPanel, NetworksPanel, EngineInstallWizard,
    │   │   │                     # ContainerLogsModal/StatsModal/ExecModal, RecreateContainerModal,
    │   │   │                     # WebsitePanel, WebsiteEngineWizard, CreateWebsiteModal,
    │   │   │                     # SubdomainModal, DomainDetailPanel (accordion inline,
    │   │   │                     # bukan modal — §18: PHP/SSL/Files/Logs/Proxy/DNS/SFTP/
    │   │   │                     # Cron/Database), MySQLExplorer, PGExplorer (§14, §18),
    │   │   │                     # BackupModal (§15)
    │   │   └── tabs/              # TabBar & TabContent (keep-alive per tab)
    │   └── App.tsx
    └── wailsjs/                  # auto-generated binding Go<->TS (`wails generate module`)
```

Konvensi modul vertical-slice dipertahankan dari homepoin (`dto.go`,
`repository.go`, `service.go`, `routes.go`/binding, `page`/komponen React) —
`servers` di atas adalah contoh polanya untuk modul berikutnya.

## 5. Alur data satu operasi (contoh: buka tab, lihat file)

1. User klik server di `ServersPage` → `openOrFocus()` cek apakah sudah ada
   tab untuk `serverId` itu; kalau belum, `openTab()` memanggil binding
   `OpenServerTab` → `session.Manager.OpenTab` → insert baris `ui_tabs` →
   tab baru masuk store Zustand, langsung jadi tab aktif.
2. `TabContent` me-render `ServerWorkspace` untuk tab itu (dan tetap
   me-render semua tab lain yang sudah terbuka, cuma `hidden`).
3. User klik modul "Files" di `ServerWorkspace` → `setModule()` — optimistic
   update Zustand dulu (instan), lalu `SetTabActiveModule` menyimpan ke
   `ui_tabs.active_module` di background.
4. (Setelah modul `files` di-porting) komponen Files akan memanggil binding
   yang di baliknya jalan lewat `sshpool.Pool.Acquire(serverID, SlotShared)`
   — kalau tab lain ke server yang sama sudah pernah membuka Files/Docker
   sebelumnya, koneksi shared itu KEMUNGKINAN BESAR sudah hangat (warmed
   saat startup + health-check-skip 30 detik), jadi tidak ada dial ulang.

## 6. Tambah/Edit Server: beda UX dari homepoin

homepoin: satu form modal datar (semua field satu kolom), tombol Simpan
**disembunyikan** sampai Tes Koneksi lolos — berlaku sama untuk tambah server
baru MAUPUN edit server yang sudah ada (ganti nama/catatan sekalipun tetap
wajib tes ulang koneksi SSH). Tidak ada indikator status koneksi selain teks
polos, dan tidak ada tags/warna yang benar-benar terpakai di form meski ada
di schema.

poinhost (`ServerFormModal.tsx`, `ServersPage.tsx`):

- **Form dikelompokkan per section** (Koneksi / Autentikasi / Tampilan &
  catatan) alih-alih satu kolom panjang — lebih mudah dipindai.
- **Toggle key/password pakai segmented control**, bukan `<select>` — dan
  field yang relevan (path kunci vs password) langsung berubah tanpa reload.
- **Status koneksi berupa chip berwarna** (abu "menguji…", hijau "Terhubung ·
  42ms", merah pesan error) — bukan sekadar teks.
- **Trust host-key terintegrasi di alur yang sama**: kalau status `unknown`/
  `mismatch`, chip berubah jadi banner kuning menampilkan fingerprint
  lama-vs-baru + tombol "Percayai Host Key" — dan ini jalan bahkan untuk
  server yang BELUM disimpan — `servers.Service.TrustHostKey` menerima
  `SaveServerRequest` penuh, bukan cuma ID, sebuah perbaikan dari desain awal
  skeleton ini yang keliru mengasumsikan server harus sudah tersimpan dulu.
- **Simpan hanya digembok tes-koneksi untuk server BARU** (`mode==='create'`).
  Edit server yang sudah ada & terbukti jalan (ganti warna, tag, catatan,
  nama) tidak perlu tes ulang SSH — keputusan sadar untuk mengurangi friksi,
  beda dari homepoin yang menggembok Simpan di kedua mode tanpa pandang bulu.
  Mengubah field koneksi (host/port/username/authType/keyPath/password)
  otomatis me-reset status tes ke `idle` supaya user tidak bisa diam-diam
  menyimpan kredensial baru yang belum pernah dicoba.
- **Tags & warna dipakai nyata**: warna jadi accent kiri kartu server + titik
  di tab bar (bukan cuma kolom schema yang tak terlihat), tags jadi chip yang
  bisa ditambah/dihapus dan tampil di kartu (berguna untuk mengelompokkan
  banyak server — mis. per lingkungan/lokasi — sesuai tujuan multi-server).
- **Server list jadi kartu**, bukan `<li>` polos: accent warna, tag pill,
  indikator jumlah tab yang sedang terbuka ke server itu, aksi Edit/Hapus
  muncul saat hover (bukan selalu tampil, supaya daftar tetap ringkas).
- **Hapus server** dulu menutup semua tab yang menunjuk ke sana (lewat
  `closeTab` yang sudah membersihkan `TerminalRegistry` per tab) SEBELUM
  memanggil `DeleteServer` — supaya tidak ada bookkeeping sesi terminal yang
  nyangkut setelah `Pool.UnregisterServer` menutup koneksinya di level pool.

### Password SSH server: disimpan lewat `secrets.Vault`, bukan kolom SQLite apa adanya

Sebelumnya password SSH server (kalau auth-nya `password`, bukan key) hanya
disimpan apa adanya di kolom `servers.password_enc` — nama kolomnya
menjanjikan enkripsi tapi isinya benar-benar plaintext (lihat TODO yang dulu
ada di `repository.go`). Kalau file `poinhost.db` di-copy/dicuri, semua
password SSH ikut bocor. Sekarang dipindah ke `internal/core/secrets.Vault`
— SATU instance yang sama persis dipakai kredensial database (MySQL/
PostgreSQL Explore, §14): OS keychain dulu, fallback file lokal AES-256-GCM
kalau OS keychain tidak tersedia (mis. Linux tanpa Secret Service jalan).
`app.go` membuat vault-nya SEKALI lalu meneruskan instance yang sama ke
`servers.NewService` maupun `website.NewService` — bukan dua vault terpisah
untuk hal yang secara prinsip sama.

`servers.Repository.Create`/`Update` sekarang sama sekali tidak menyentuh
kolom `password_enc` lagi (metadata saja) — password dibaca/ditulis
`servers.Service` lewat `vault.Get`/`vault.Set` dengan key deterministik
`serverpass:<id>`, pola yang sama persis dengan `vaultKeyForDBCredential` di
§14. Semantik "password kosong di form edit berarti jangan ubah" (frontend
tidak pernah menampilkan/mengirim ulang password lama) tetap dipertahankan
di layer Service, cuma sumbernya pindah dari kolom SQL ke vault.

**Migrasi data lama**: karena kolom `password_enc` sudah lama ada dan
mungkin sudah terisi plaintext dari instalasi poinhost sebelum vault ini
ditambahkan, `servers.Service.Bootstrap` menjalankan
`migrateLegacyPasswords()` SEKALI di setiap startup, sebelum server
didaftarkan ke pool: baca semua baris dengan `password_enc` terisi, pindahkan
tiap satu ke vault (`vault.Set`), lalu kosongkan kolomnya
(`ClearLegacyPassword`) — supaya tidak ada plaintext yang tertinggal di
SQLite. Best-effort per server (satu server gagal dipindah tidak
menghalangi server lain ikut termigrasi, dan password_enc-nya SENGAJA
dibiarkan terisi sampai berhasil dipindah di startup berikutnya — bukan
dihapus begitu saja sebelum vault-nya benar-benar tersimpan). Idempotent:
begitu kolomnya kosong, `ListLegacyPasswords` tidak mengembalikan apa-apa
lagi, jadi aman dijalankan tiap startup selamanya.

## 7. Login/register homepoin: DIHAPUS, bukan sekadar belum di-porting

homepoin butuh master password + TOTP karena dia jalan sebagai **web server**
(`127.0.0.1:7070`) — siapa pun yang bisa mengirim request ke port itu (proses
lain di mesin yang sama, atau siapa pun yang tunneling ke port itu) harus
melewati login dulu. Itu batas keamanan yang masuk akal untuk model
client-server.

poinhost **bukan** web server — dia aplikasi desktop native (Wails: WebView
di window sendiri, tidak bind ke port yang bisa diakses proses lain). Batas
aksesnya sudah dijamin di layer OS: siapa pun yang bisa membuka aplikasi ini
sudah harus login ke akun desktop (Windows/macOS/Linux) di mesin itu duluan.
Menambah login/TOTP di atasnya cuma menambah friksi tanpa menutup celah
keamanan baru — makanya **keputusan desainnya adalah TIDAK mengimplementasikan
mekanisme register/login sama sekali**, bukan "belum sempat di-porting".
Konsekuensinya:

- Tidak ada `internal/core/auth`, tidak ada halaman `/setup` atau `/login`,
  tidak ada session cookie/JWT, tidak ada TOTP.
- App langsung menampilkan daftar server begitu dibuka (lihat `app.go` —
  `startup()` langsung `Bootstrap()` servers, tidak ada auth gate).
- Kalau nanti ada kebutuhan "banyak orang pakai satu instal poinhost yang
  sama" (multi-user di satu mesin/akun OS yang sama), itu kasus yang beda
  dari homepoin punya — akan dibahas ulang saat kebutuhannya benar-benar ada,
  bukan diasumsikan dari awal.

## 8. Status Server: arsitektur untuk skala banyak server

### Cara homepoin

Satu goroutine (`Monitor.runOnce`) jalan tiap `HeartbeatInterval` (default
30 detik) dan memeriksa **SEMUA** server aktif **serentak** dalam satu
"gelombang" — dibatasi worker pool (`HeartbeatWorkers`, default 8), tapi
tidak ada stagger sama sekali: kalau ada 100 server, tiap 30 detik persis
ada lonjakan ~100 percobaan SSH yang mengantre di 8 worker itu. Status
"up/down" juga digabung dengan pengambilan metrik: satu-satunya cara
homepoin tahu server "up" adalah kalau script metrik (yang menjalankan
`top -bn1`, perlu ~1 detik sampling) berhasil — jadi mengetahui "hidup atau
tidak" selalu ikut membayar biaya penuh pengambilan metrik.

### Cara poinhost — dua tingkat, terpisah total

**Tingkat 1 — status koneksi (`ServerStatus.Connection`): GRATIS.**
`Service.ConnectionStatus` (`status.go`) cuma baca `sshpool.Pool.Status(id)`
— nilai yang SUDAH dijaga hidup oleh mekanisme keepalive pool (§3) yang
berjalan independen dari fitur status ini. Tidak ada SSH round-trip
tambahan sama sekali. `Collector.connectionLoop` (`collector.go`) membaca
ini untuk semua server tiap `DefaultConnectionPoll` (4 detik) dan HANYA
emit event kalau nilainya berubah. Biayanya O(N) pembacaan map per tick —
bahkan untuk ribuan server ini masih dalam orde microsecond, jauh di bawah
biaya satu SSH round-trip.

**Tingkat 2 — metrik (CPU/RAM/disk/OS/hostname/uptime): worker pool +
stagger + prioritas.** `Collector.metricsSchedulerLoop` + `metricsWorker`:

- **Worker pool dibatasi** (`DefaultMetricsWorkers = 6`) — jumlah percobaan
  SSH metrik yang berjalan bersamaan tidak pernah lebih dari ini, berapa
  pun jumlah server.
- **Di-stagger**: kunjungan pertama tiap server dijadwalkan pada waktu
  ACAK di dalam satu window interval (`enqueueDue`), bukan semua di t=0 —
  menghindari lonjakan periodik yang persis sama tiap tick.
- **Prioritas berbasis perhatian** (`Subscribe`/`Unsubscribe`): server
  dengan tab terbuka dicek tiap `DefaultActiveInterval` (15 detik); server
  yang cuma nongkrong di sidebar tanpa tab dicek tiap `DefaultIdleInterval`
  (90 detik). `App.OpenServerTab`/`CloseServerTab` (app.go) memanggil ini
  otomatis — bukan sesuatu yang perlu diatur manual dari UI. Tab yang
  direstore saat startup ikut di-subscribe (lihat `startup()`), supaya
  server yang "biasa dikelola" langsung dapat prioritas tanpa perlu diklik
  ulang satu-satu.
- **Skip kalau sudah tahu offline**: `enqueueDue` cek cache Tingkat 1 dulu
  — kalau statusnya `offline`, jadwal metrik cuma digeser mundur, TIDAK
  mencoba `top -bn1` yang sudah pasti timeout (menghemat waktu worker dan
  file descriptor untuk server yang memang sedang mati).
- **Non-blocking**: kalau worker pool penuh saat suatu server jatuh tempo,
  server itu dilewati tick ini (dicoba lagi tick berikutnya), TIDAK
  menumpuk di channel — mencegah antrean membengkak tanpa batas kalau
  jumlah server jauh melebihi kapasitas worker.
- **Cache-first + refresh manual**: nilai terakhir yang berhasil selalu
  disimpan (`Collector.cache`) dan langsung ditampilkan; kalau percobaan
  berikutnya gagal, `MetricsStale=true` + `MetricsError` diisi TAPI angka
  lama tetap ditampilkan (tidak tiba-tiba kosong). `RefreshServerStatus`
  (binding) memanggil `RefreshNow` yang melewati jadwal sepenuhnya — dipakai
  tombol "↻ Refresh" di panel Overview.
- **Slot SSH terpisah**: metrik jalan di `SlotMetrics` (dedicated per
  server), bukan `SlotShared` — supaya heartbeat tidak pernah mengantre di
  belakang operasi berat modul lain (files/docker) ke server yang sama,
  dan sebaliknya operasi modul lain tidak menunggu heartbeat selesai dulu.

**Push, bukan poll, ke frontend**: `Collector.SetEmitter` dihubungkan ke
`runtime.EventsEmit` Wails di `startup()`. Frontend pasang SATU listener
`EventsOn('server:status', …)` di root (`App.tsx`) yang update Zustand
store — tidak ada frontend yang polling binding berulang-ulang, dan tidak
perlu WebSocket broadcaster custom + reconnect-backoff seperti homepoin
(`ServersStatusBC`), karena Wails cuma punya satu window untuk di-fan-out,
bukan banyak client browser.

**Kenapa ini "teroptimal" untuk banyak server**: biaya Tingkat 1 (status
hidup/mati, yang paling sering dibutuhkan untuk titik di sidebar) TIDAK
bertambah sama sekali seiring N membesar — murni pembacaan memori. Biaya
Tingkat 2 (metrik berat) dibatasi di angka tetap (worker pool) berapa pun N,
dan makin kecil per-server rata-rata seiring N membesar karena mayoritas
server (yang tidak punya tab terbuka) otomatis turun ke interval yang 6x
lebih jarang.

**Belum dikerjakan (kalau nanti fleet-nya ratusan+)**: saat ini metrik
Tingkat 2 tetap dijadwalkan untuk SEMUA server aktif, bukan hanya yang
sedang terlihat di sidebar (yang bisa di-scroll/dipaging) — untuk instalasi
puluhan server ini tidak masalah, tapi kalau nanti benar-benar sampai
ratusan/ribuan, langkah lanjut yang wajar adalah menambah "visible in
viewport" sebagai syarat subscribe tambahan (mirip virtualized list),
bukan cuma "punya tab terbuka".

## 9. Terminal: PTY interaktif via xterm.js

### Cara homepoin

WebSocket binary (`/ws/servers/{id}/terminal`, frame `0x00=STDIN,
0x01=STDOUT, 0x02=RESIZE, ...`) menjembatani xterm.js di browser ke satu
sesi PTY di server, lewat `sshpool.OpenTerminal` — SATU slot dedicated per
server yang otomatis MENGGANTIKAN sesi lama begitu ada yang baru dibuka
(lihat catatan `OpenTerminal` di kode lama: "Sesi lama … otomatis
digantikan"). Buka 2 tab terminal ke server yang sama = tab pertama putus.

### Cara poinhost

**Transport**: bukan WebSocket custom, tapi event Wails yang sudah dipakai
untuk status server (§8) — `runtime.EventsEmit`/`EventsOn`. Tidak perlu
protokol frame biner sendiri (0x00/0x01/dst) karena Wails cuma satu window,
bukan banyak client browser yang perlu di-multipleks.

- **Encoding**: output PTY di-**base64**-kan sebelum lewat event
  (`app.go` startup, emitter `terminalSvc.SetEmitters`) — byte mentah dari
  shell remote tidak dijamin UTF-8 valid (karakter multi-byte bisa
  terpotong pas di batas satu `Read()`), dan base64 menghindari masalah itu
  tanpa perlu peduli soal encoding sama sekali. Frontend decode balik ke
  `Uint8Array` dan serahkan ke `term.write()` — xterm.js terima raw bytes,
  bukan string JS, supaya tidak ada mangling di lapisan UTF-16 JS.
- **Event per-sesi**: `terminal:output:<sessionId>` dan
  `terminal:exit:<sessionId>`, bukan satu event global — tiap `TerminalPanel`
  di frontend cuma dengar output miliknya sendiri, tidak perlu filter
  payload sisi klien.
- **Input** (keystroke dari xterm.js `onData`) dikirim apa adanya sebagai
  string lewat binding `WriteTerminal` — arah ini tidak butuh base64 karena
  keystroke pengguna praktis selalu valid UTF-8.

**Arsitektur backend** (`internal/modules/terminal/service.go`, di atas
`session.TerminalRegistry` yang sudah dibangun sejak §3):

1. `Service.Open(ctx, tabID, serverID)` — minta koneksi dedicated dari
   `TerminalRegistry.Open` (yang mendial via `sshpool.Pool.OpenDedicated`),
   lalu `RequestPty` + `Shell()` di atasnya, simpan `(tabID, *ssh.Session,
   stdin)` di map sendiri dikunci `sessionID` yang SAMA dengan yang
   dikembalikan `TerminalRegistry` — sengaja satu sistem ID, bukan dua.
2. Goroutine `readLoop` per sesi membaca stdout terus-menerus dan
   memanggil callback `onData` (dihubungkan ke `EventsEmit` di `app.go`)
   sampai sesi berakhir, lalu panggil `onExit`.
3. **Tidak ada slot tunggal per server** seperti homepoin — `Open` boleh
   dipanggil berkali-kali untuk server yang sama (dari tab berbeda) dan
   masing-masing dapat `*ssh.Session` + koneksi dedicated sendiri, karena
   `sshpool.OpenDedicated` (§3) sudah didesain ulang untuk itu sejak awal.

**Siklus hidup terikat ke TAB, bukan ke modul yang sedang aktif**:
`ServerWorkspace.tsx` me-mount `TerminalPanel` SEKALI saat modul "Terminal"
pertama kali dikunjungi dalam satu tab, lalu menjaganya tetap mounted
(`hidden`, bukan unmount) selama tab itu terbuka — sama seperti `TabContent`
menjaga semua TAB tetap mounted (§lihat kode `ServerWorkspace`, set
`STATEFUL_MODULES`). Jadi pindah ke Files/Docker lalu balik ke Terminal
TIDAK memutus shell atau menghilangkan scrollback. Sesi baru benar-benar
ditutup hanya saat TAB-nya ditutup: `App.CloseServerTab` memanggil
`terminalSvc.CloseAllForTab(tabID)` (menutup tiap `*ssh.Session` + koneksi
dedicated-nya) SEBELUM `TerminalRegistry.CloseAllForTab(tabID)` (jaring
pengaman kalau ada koneksi dedicated lain untuk tab itu di masa depan yang
bukan dari modul terminal).

**Ukuran PTY**: `FitAddon` xterm.js menghitung cols/rows dari ukuran
kontainer sungguhan, lalu `ResizeTerminal` mengirimkannya ke
`ssh.Session.WindowChange`. `ResizeObserver` di kontainer memicu ini tiap
window/panel di-resize; efek terpisah men-trigger `fit()` lagi saat panel
yang tadinya `hidden` ditampilkan kembali (elemen `display:none` punya
ukuran 0, jadi ResizeObserver tidak berguna selama disembunyikan).

## 10. Files: file manager via SFTP

### Cara homepoin

Upload lewat `<input type=file>` browser (multipart form POST ke
`/api/.../files/upload`), download lewat `<a href="/api/.../files/download">`
yang memicu download manager browser. Wajar untuk aplikasi web — tapi
poinhost bukan aplikasi web.

### Cara poinhost — dialog OS native, bukan trik ala-browser

Upload & download di poinhost SENGAJA memakai **dialog file native OS**
(`runtime.OpenMultipleFilesDialog` / `runtime.SaveFileDialog` dari Wails),
bukan `<input type=file>` + base64 encode ke JSON ala aplikasi web:

- **Upload**: `App.UploadFilesToServer(serverId, remoteDir)` membuka dialog
  "Buka File" native (boleh pilih banyak sekaligus), lalu tiap file dibaca
  langsung dari disk lokal (`os.Open`) dan di-stream ke SFTP
  (`sftp.UploadStream`) — isi file TIDAK PERNAH ditampung penuh di memori
  JS maupun sebagai string base64 yang membengkak ~33%, berapa pun besar
  filenya.
- **Download**: `App.DownloadFileFromServer(serverId, remotePath)` membuka
  dialog "Simpan" native (default nama file dari `BaseName`), lalu SFTP
  di-stream langsung ke file lokal yang dipilih. Tidak ada Blob/objectURL/
  `<a download>` yang perilakunya tidak konsisten di dalam WebView.
- `files.Service` sendiri tidak tahu apa-apa soal dialog — dia cuma
  menerima PATH LOKAL (`UploadFromLocalPath`/`DownloadToLocalPath`), dialog
  dipanggil di `app.go` (lapisan yang memang boleh bergantung ke Wails
  runtime, sama seperti event emit di §8/§9) supaya `internal/modules/files`
  sendiri tetap portable & gampang diuji.

### Fitur

List (direktori dulu, lalu file, terurut nama), buat folder, buat file
kosong, upload, download, rename, hapus (rekursif untuk direktori berisi —
coba SFTP `Remove` dulu, fallback `rm -rf` via exec kalau direktori tidak
kosong), kompres (`.zip` dengan fallback Python3 kalau binary `zip`/`unzip`
tidak terpasang — dipertahankan dari `archive_cmd.go` homepoin yang sudah
teruji; `.tar.gz` via `tar`), ekstrak (deteksi format dari ekstensi nama
arsip, tujuan default = direktori yang sedang dibuka), **edit isi file teks**
dan **ubah permission (chmod)**.

Konsisten dengan §3: operasi file lewat `sshpool.SFTPClient`/`Executor.Exec`
yang keduanya jalan di slot `SlotShared` — pindah dari tab Files ke tab
Terminal (atau ke server lain) TIDAK memicu dial SSH baru, koneksi shared
yang sama (sudah dipanaskan sejak startup) dipakai bersama.

### Edit file teks — CodeMirror 6, bukan Monaco

Tombol 📝 (muncul untuk file ≤512KB) membuka `EditFileModal`, isi file
diambil `ReadFileContent` (backend menolak file >2MB, sama seperti batas
homepoin — mencegah UI membeku menampung file besar) dan disimpan lewat
`WriteFileContent` (overwrite penuh, bukan patch).

**CodeMirror 6** (`@uiw/react-codemirror`) dipilih di atas Monaco Editor:

- Monaco butuh web worker terpisah per bahasa (setup lebih rumit di Vite,
  dan startup app jadi lebih berat) — CodeMirror jalan di satu thread,
  lebih cocok untuk aplikasi desktop yang harus terasa instan dibuka.
- `@uiw/codemirror-extensions-langs` memetakan puluhan bahasa **langsung
  dari ekstensi file** (`langs.sh`, `langs.yaml`, `langs.json`, dst) — tidak
  perlu tabel mapping manual per bahasa seperti biasanya dibutuhkan Monaco;
  `detectLanguage()` di `EditFileModal.tsx` cuma mengecek apakah ekstensi
  file ada di `langNames`, dengan alias kecil untuk beberapa ekstensi umum
  yang tidak match langsung (`.conf`→cfg, `.env`→sh, `.yaml`→yml).
- Trade-off yang jujur: paket ini memuat SEMUA bahasa sekaligus (bukan
  lazy per-bahasa), jadi bundle JS naik cukup besar (~600KB → ~2.2MB
  minified). Untuk aplikasi desktop dengan aset ter-embed di binary (dimuat
  sekali dari disk lokal, bukan di-fetch tiap kali seperti web), ini bukan
  masalah nyata — cuma dicatat di sini kalau nanti mau dioptimalkan
  (code-splitting per-bahasa via `import()` dinamis).
- Tutup modal saat ada perubahan belum disimpan meminta konfirmasi
  (`confirm()`, pola yang sama dipakai `handleDelete` di `FilesPanel`).

### Chmod — grid checkbox, bukan kotak teks oktal

`ChmodModal.tsx` mem-parsing string mode Unix dari `FileEntry.mode` (mis.
`"-rw-r--r--"`) jadi grid checkbox Read/Write/Execute × Owner/Group/Other,
menampilkan angka oktal hasilnya secara live — lebih enak dipakai daripada
kotak teks oktal polos (yang tetap ditampilkan sebagai output, untuk yang
sudah hafal angka seperti "755"/"644"). `Service.Chmod` mem-parsing oktal
ini balik jadi `os.FileMode` lalu panggil `sftp.Chmod`.

### Copy & Search

`Copy` (salin, sumber tetap ada — beda dari `Rename` yang memindahkan) dan
`Search` (cari nama file/direktori rekursif dari satu direktori, dibatasi
200 hasil) SELALU lewat `execAccess` (`cp -r`, `find … -iname`) — SFTP tidak
punya primitif copy maupun search sama sekali, jadi tidak ada "jalur cepat"
SFTP untuk dua operasi ini seperti operasi lain (beda dari List/Mkdir/dst
yang punya jalur SFTP kalau bukan sudo). `SearchModal.tsx` menampilkan hasil
sebagai daftar terpisah (bukan menimpa tabel utama, karena hit bisa datang
dari sub-direktori mana pun) — klik "Buka" memindahkan `FilesPanel` ke
direktori tempat file itu berada.

### Elevasi sudo (`asUser`) — dipertahankan dari homepoin, disesuaikan

Prinsipnya: SFTP subsystem SSH SELALU jalan sebagai user yang login —
tidak bisa "SFTP sebagai user lain" tanpa re-autentikasi penuh. Satu-
satunya cara operasi file sebagai user LAIN adalah lewat shell command yang
dibungkus `sudo -u <user>`. `files/access.go` (`fileAccess`/`resolveAccess`/
`wrapSudo`) dan `files/sudoops.go` (`listDirSudo`/`readFileSudo`/dst)
diporting nyaris apa adanya dari `access.go`/`sudoops.go` homepoin — logikanya
sudah teruji, cuma sumber passwordnya beda:

- Homepoin: password SSH sesi login di-cache di server per sesi HTTP
  browser (`SSHPasswordForSession`), dipipe ke `sudo -S` kalau perlu.
- Poinhost: **tidak ada sesi/login sama sekali** (§7) — password server
  (kalau auth-nya password) sudah tersimpan di SQLite sejak Tambah Server,
  jadi `servers.Service.SudoPassword(id)` tinggal baca langsung dari
  situ, tanpa perlu cache sesi apa pun. Lebih sederhana justru KARENA
  poinhost tidak punya login.
- Server dengan auth SSH key tidak punya "password" untuk dipipe ke
  `sudo -S` — jalur itu bergantung pada `NOPASSWD` di `/etc/sudoers`
  server target; kalau sudo tetap minta password, error diterjemahkan
  jadi pesan yang jelas (`mapSudoError`).

Setiap operasi FILE (Mkdir/CreateFile/Rename/Delete/Compress/Extract/
Read/Write/Chmod/Upload/Download, plus Copy/Search yang memang selalu lewat
shell) menerima `asUser` opsional dan `resolveAccess` memvalidasi: kosong
atau sama dengan user SSH → jalur SFTP normal tanpa sudo; beda user →
WAJIB `server.useSudo == true` (dicentang di Edit Server, §6) dan user itu
harus benar-benar ada di server (`getent passwd`), baru diizinkan.

**Batasan yang diterima** (sama seperti homepoin): upload/download SEBAGAI
user lain TIDAK benar-benar streaming — isi file dibaca penuh ke memori
dulu (upload: file lokal dibaca penuh lalu `cp` lewat file sementara;
download: `cat` dieksekusi dan outputnya ditangkap penuh sebagai satu
string) karena jalurnya lewat shell command, bukan pipe SFTP mentah. Untuk
file besar yang dioperasikan SEBAGAI user lain, ini lebih lambat & lebih
boros memori dibanding jalur SFTP biasa — batasan yang sama persis dengan
homepoin, bukan regresi baru.

Frontend: dropdown "Jalankan sebagai" di toolbar `FilesPanel` HANYA muncul
kalau `server.useSudo` true, terisi dari `ListSystemUsers` (getent passwd,
difilter ke user login-capable + root). Ganti pilihan langsung memuat ulang
direktori yang sama sebagai user baru — bisa kelihatan berbeda isinya kalau
permission direktori membatasi siapa boleh lihat apa.

Frontend (`FilesPanel.tsx`) di-keep-alive per tab sama seperti Overview &
Terminal (§9) — direktori yang sedang dibuka & seleksi file tidak hilang
saat pindah ke modul lain lalu balik lagi. Modal kecil (`PromptModal`,
`CompressModal`, `ChmodModal`, `EditFileModal`, `SearchModal`) dipakai untuk
semua input folder/file/rename/kompres/chmod/edit/cari, bukan
`window.prompt()` (dukungannya tidak konsisten lintas WebView platform).
Interaksi per-baris pakai tombol yang muncul saat hover (pola yang sama
dengan `ServersPage`), bukan context-menu klik-kanan kustom — pilihan sadar
untuk mengurangi kompleksitas UI di porting awal ini.

## 11. Docker: containers, networks, exec, install wizard

### Scope — mengikuti homepoin APA ADANYA, bukan menambah cakupan baru

Sebelum porting, homepoin dicek langsung (`routes.go` + template
`docker/images.html`/`volumes.html`/`compose.html`): dari lima submenu
Docker di homepoin, cuma **Containers** dan **Networks** yang benar-benar
punya implementasi backend. **Images, Volumes, dan Compose di homepoin
sendiri cuma halaman placeholder "coming soon"** — tidak ada satu route
API pun untuk ketiganya. Jadi porting ke poinhost sengaja dibatasi sama:
Containers + Networks dibangun **penuh** (termasuk fitur yang di homepoin
ada di balik service.go/service_config.go/install.go — bukan cuma daftar
sederhana), sementara tiga submenu lain diberi placeholder yang JUJUR
mengatakan itu ("belum ada implementasinya di homepoin sendiri"), bukan
pura-pura sudah porting padahal cuma tampilan kosong.

### Eksekusi tetap lewat CLI `docker` via SSH, bukan Docker API/socket

Sama seperti homepoin: tidak ada dependency ke Docker Engine API atau akses
langsung ke `/var/run/docker.sock` dari poinhost. Semua operasi adalah
perintah `docker ...` yang dieksekusi lewat `sshpool.Executor` (slot
shared untuk perintah cepat, koneksi dedicated untuk yang streaming) —
konsisten dengan §3: pindah dari tab Files/Terminal ke tab Docker (atau
sebaliknya) tidak pernah memicu dial SSH baru.

`internal/modules/docker/access.go` (`dockerAccess`/`resolveAccess`/`wrap`)
polanya sama dengan `files/access.go`, tapi lebih sederhana — Docker tidak
punya konsep "jalankan sebagai user lain" seperti Files; cuma ada dua
keadaan: jalan langsung (user SSH sudah `root` atau `server.useSudo`
false) atau dibungkus `sudo` (naik ke root, pakai password tersimpan kalau
auth-nya password, atau `sudo -n` mengandalkan NOPASSWD kalau auth-nya
key). `mapDockerError` diporting apa adanya dari homepoin — menerjemahkan
pesan sudo/socket/daemon-not-running mentah jadi pesan yang jelas.

### Containers

`ListContainers` mem-parsing `docker ps -a --format` TSV (bukan
`{{json .}}` — jauh lebih cepat kalau container punya banyak Label besar,
teknik yang sama dipertahankan dari homepoin), di-cache 8 detik per server
di dalam `docker.Service` sendiri (cache in-memory sederhana, bukan paket
generik terpisah seperti homepoin — dipakainya cuma di sini) supaya
polling ringan dari `DockerPanel` (setiap 10 detik selagi sub-tab
Containers aktif) tidak berarti `docker ps` baru tiap panggilan.
Start/Stop/Restart/Remove dikunci per-server lewat
`sshpool.ServerMutexRegistry` yang sama dipakai modul lain — dua aksi
Docker dari dua tab yang menunjuk server yang sama tidak saling
tabrakan.

Log & statistik container punya dua mode, sama seperti homepoin:
snapshot (`DockerContainerLogs`/`DockerContainerStats`, sekali panggil)
dan **stream realtime** (`docker logs -f` / `docker stats`) lewat
`Executor.ExecStreamDedicated` — jalan di koneksi SSH KHUSUS, bukan slot
shared, supaya `tail -f` yang berjalan lama tidak ikut mengantre di
belakang operasi Docker lain, dan beberapa modal log/stats bisa terbuka
bersamaan ke container berbeda tanpa saling memblokir.

### Streaming lewat event Wails, bukan WebSocket custom

Homepoin memakai WebSocket khusus (`WSHandler.HandleLogsWS`/
`HandleStatsWS`/`HandleInstallWS`) untuk mengalirkan log/stats/progress
instalasi ke browser. Poinhost tidak punya server HTTP sama sekali (§1),
jadi pola ini diganti dengan **event Wails per-stream**, mengikuti pola
yang sama dipakai Terminal (§9) dan status server (§8):

- `App.StreamDockerContainerLogs`/`StreamDockerContainerStats`/
  `StreamDockerEngineInstall` masing-masing membuat `streamID` (UUID) +
  `context.CancelFunc` yang didaftarkan ke `App.streams` (map kecil di
  `app.go`, dianalogikan dengan `handler_ws.go` homepoin tapi untuk event,
  bukan koneksi WS), lalu menjalankan stream di goroutine terpisah dan
  mem-`runtime.EventsEmit` tiap baris/sampel ke event bernama
  `docker:logs:<streamID>` / `docker:stats:<streamID>` /
  `docker:install:<streamID>`.
- `App.StopDockerStream(streamID)` membatalkan context-nya — dipanggil
  frontend saat modal log/stats ditutup, atau otomatis dibersihkan sendiri
  begitu stream berakhir wajar (container di-stop, instalasi selesai/gagal).
- Semua stream yang masih terdaftar dibatalkan paksa di `shutdown()`
  supaya tidak ada goroutine tersisa nyoba jalan setelah SSH pool/DB
  ditutup.

### Inspect & Recreate — bukan "edit container in place"

Docker tidak punya cara mengubah env/port/volume/memory container yang
sudah berjalan begitu saja — satu-satunya jalan adalah siklus **stop → rm
→ create (dengan konfigurasi baru) → start**, dengan nama container
dipertahankan. `internal/modules/docker/config.go` (diporting verbatim
dari homepoin, fungsi murni tanpa dependency sesi/DB) mem-parsing output
`docker inspect` jadi `createTemplate`, menerapkan override dari form UI
(`applyOverrides`/`validateRecreateOverrides` — validasi port 1-65535,
path volume harus absolut, memory limit minimal 6MB), lalu membangun ulang
argumen `docker create` (`buildCreateArgs`) termasuk melestarikan alias
DNS network dari `docker-compose` (`networkAliasesFor` — kalau tidak
di-preserve, container lain di network yang sama gagal resolve nama
service itu lagi setelah recreate). `RecreateContainerModal.tsx` memuat
konfigurasi SEKARANG lewat `InspectDockerContainer` sebagai starting
point, dan meminta konfirmasi eksplisit sebelum apply (container akan
berhenti sesaat).

### Networks

`ListNetworks`/`CreateNetwork`/`RemoveNetwork` diporting nyaris apa adanya
— network bawaan Docker (`default`/`bridge`/`host`/`none`, dicek lewat
`IsBuiltinNetworkMode`) tidak bisa dihapus maupun dipakai sebagai nama
network baru, dijaga baik di backend (source of truth) maupun disembunyikan
tombol hapusnya di `NetworksPanel.tsx`.

### Engine install wizard

VPS baru sering belum punya Docker terpasang sama sekali. `install.go`
mendeteksi distro (`/etc/os-release` + package manager yang tersedia) dan
status service (`DetectEngineStatus`), lalu:

- Kalau sudah terpasang tapi service-nya mati → `StartEngine`
  (`systemctl start/enable`, atau `rc-service` untuk Alpine/OpenRC).
- Kalau belum terpasang → `StreamInstallEngine` menjalankan skrip resmi
  `get.docker.com` (didukung: apt/dnf/yum/apk) lewat `ExecStreamPTY` (PTY
  supaya output progress terasa seperti terminal asli), progresnya
  mengalir ke `EngineInstallWizard.tsx` sebagai log baris-per-baris lewat
  event di atas.

Beda dari homepoin: skrip apt/dnf/yum homepoin memakai helper
`nginx.AptPrelude`/`DockerPMWrapperAPT` dkk dari modul `hosting/nginx`
(wrapper lock-wait yang dipakai bersama modul Nginx homepoin) — poinhost
belum punya modul hosting apa pun, jadi bagian tunggu-lock apt (`fuser
/var/lib/dpkg/lock-frontend`) ditulis inline & self-contained di
`docker/install.go` sendiri, bukan diimpor dari modul yang tidak ada.
Kalau nanti ada modul kedua yang butuh helper serupa (mis. modul Nginx
di-porting), baru diekstrak jadi shared package — sama seperti keputusan
"belum ada abstraksi `access` lintas modul" di §11 lama (sekarang §12).

### Exec ke dalam container — memakai ulang `terminal.Service`, bukan jalur baru

Homepoin punya jalur PTY terpisah untuk "docker exec" (`terminal/
docker_exec_cmd.go` + `handler_docker_exec_ws.go`, WebSocket sendiri).
Poinhost TIDAK membuat jalur duplikat — `terminal.Service.Open` di-refactor
jadi `terminal.Service.open(ctx, tabID, serverID, command)` (dipakai lewat
dua nama publik: `Open` untuk shell login biasa, `OpenCommand` untuk
menjalankan SATU perintah tertentu lewat PTY). `docker.Service
.BuildExecCommand` cuma menyusun string perintah `docker exec -it <id>
<shell>` (dibungkus sudo bila perlu, port dari `buildDockerExecCommand`
homepoin) — `App.OpenDockerExec` memanggil `terminalSvc.OpenCommand` dengan
perintah itu, lalu (kalau perlu password sudo) mengirimkannya lewat
`terminalSvc.Write` yang SAMA dipakai keystroke terminal biasa.

Konsekuensinya: sesi exec container otomatis dapat SEMUA infrastruktur PTY
yang sudah ada — event `terminal:output:<sessionId>`/`terminal:exit:<sessionId>`,
base64 encoding output, `WriteTerminal`/`ResizeTerminal`/`CloseTerminal` —
tanpa satu baris kode baru di lapisan itu. `ContainerExecModal.tsx` memakai
`xterm.js` dengan pola render yang sama persis dengan `TerminalPanel.tsx`,
bedanya SENGAJA tidak di-keep-alive: sesi ditutup begitu modal ditutup,
karena "masuk sebentar ke satu container" adalah tindakan sesaat, beda
dari sesi Terminal VPS yang memang dirancang bertahan lintas perpindahan
modul (§9).

### Port container: publik (internet) vs intranet (LAN) vs localhost

Setiap port yang di-publish container (`-p host:container`) otomatis bisa
dijangkau siapa pun yang bisa menjangkau server ini, termasuk dari
internet — terlepas dari tujuan sebenarnya (mis. database internal yang
cuma dipakai app lain di server yang sama, atau exporter metrik yang
harusnya cuma dibaca dari LAN kantor). Selector "Scope" per port di
`RecreateContainerModal` (`internal/modules/docker/portscope.go`) memberi
kontrol eksplisit, tiga tingkat:

- **Publik (internet)** — bisa diakses dari mana saja. Default, sama
  seperti perilaku sebelum fitur ini ada.
- **Intranet (LAN saja)** — HANYA dari rentang IP privat (RFC1918:
  `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`), lewat aturan firewall
  yang dibatasi. Docker sendiri tidak bisa membatasi "hanya LAN" waktu
  bind (cuma bisa bind ke satu IP tertentu, bukan rentang), jadi port
  tetap bind ke semua interface (`0.0.0.0`) dan firewall-lah yang
  menyaring sumbernya — sama seperti pendekatan akses Docker→database di
  §14, tapi di sini arahnya kebalik: yang dibatasi itu SUMBER dari luar,
  bukan tujuan bridge Docker.
- **Localhost saja** — bind ke `127.0.0.1`. Satu-satunya mode yang
  proteksinya independen dari firewall: tidak terjangkau dari luar server
  ini apa pun kondisi firewall-nya.

Kalau scope "Intranet" dipilih tapi TIDAK ada firewall (ufw/firewalld)
aktif terdeteksi di server, pembatasannya tidak benar-benar berlaku (sama
terbukanya dengan "Publik") — dilaporkan eksplisit lewat
`RecreateContainerResponse.Warning`, bukan diam-diam gagal membatasi
(pola yang sama dengan peringatan MySQL tanpa firewall di §14). Fitur ini
TIDAK PERNAH memasang/mengaktifkan firewall baru, cuma menambah/menghapus
aturan sempit ke firewall yang memang sudah aktif.

Docker sendiri tidak bisa membedakan "public" dari "intranet" dari
bind-address mentah (keduanya sama-sama `0.0.0.0`) — jadi pilihan scope
disimpan sebagai container label (`poinhost.portscope.<port>.<proto>`),
ditulis ulang tiap recreate (`syncPortScopeLabels`) dan dibaca kembali
waktu inspect, supaya modal recreate berikutnya menampilkan pilihan yang
SAMA seperti terakhir diset — bukan diam-diam reset ke "Publik" tiap kali
container di-recreate untuk alasan lain (ganti env, misalnya).

Saat scope BERUBAH (mis. dari Publik ke Intranet), skrip firewall
(`portScopeApplyScript`) selalu MENGHAPUS dulu semua kemungkinan aturan
poinhost sebelumnya (allow-dari-mana-saja maupun tiap CIDR intranet)
sebelum menambah yang baru — kalau tidak, aturan lama yang lebih longgar
bisa diam-diam tetap berlaku berdampingan dengan yang baru lebih ketat
(ufw/firewalld meng-OR semua allow rule yang cocok, jadi satu aturan
longgar yang tertinggal saja cukup membuat pembatasan baru tidak
berarti). Diterapkan dalam SATU skrip untuk semua port sekaligus (satu
round-trip SSH, dijalankan langsung setelah recreate berhasil), bukan
satu panggilan terpisah per port.

## 12. Website: domain/vhost Nginx + PHP-FPM + SSL Let's Encrypt

### Scope tahap ini — "inti" menu Website, sisanya menyusul

Menu "Website" homepoin (link sidebar-nya ke `/servers/{id}/hosting/domains`)
sebenarnya adalah SEMBILAN modul terpisah di baliknya:
`domains`/`nginx`/`php`/`ssl` (tahap ini) + `domainfiles`/`logs`/`proxy`/
`database`(per-domain)/`cron`/`dns`/`sftp` (menyusul). Beda dari Docker
§11 (yang batas scope-nya ditentukan homepoin sendiri — Images/Volumes/
Compose memang belum pernah diimplementasikan di sana), SEMUA sembilan
bagian Website ini sudah production-ready di homepoin — pembatasan scope
di sini murni soal urutan pengerjaan (dikonfirmasi ke user), bukan karena
sisanya tidak ada. Tab Files/Logs/Proxy/Database/Cron/DNS/SFTP di
`DomainDetailModal.tsx` diberi placeholder yang jujur menyebutkan itu:
"sudah berjalan di homepoin, belum di-porting ke poinhost" — beda kalimat
dari placeholder Docker yang bilang "belum ada di homepoin sendiri".

### Kenapa homepoin terasa lambat pindah-pindah menu di sini (bukan re-handshake SSH)

Sebelum porting, dilakukan riset kode homepoin secara langsung untuk
menjawab pertanyaan user "katanya sudah pooling, kok masih lambat?".
Hasilnya: **SSH memang tidak pernah re-handshake** di homepoin (ada
connection pool) — masalahnya adalah SETIAP tampilan halaman menjalankan
BANYAK perintah SSH berurutan yang saling menunggu (round-trip), bukan
satu:

- `domains.List()` — 1 `ls` untuk daftar file vhost, LALU **satu `cat`
  terpisah per file vhost** (pola N+1: 10 domain = 11 round-trip HANYA
  untuk daftar domain), ditambah `nginx.DetectStatus` (2 round-trip lagi)
  yang dipanggil DI DALAM `List()` walau frontend sendiri SUDAH memanggil
  status Nginx secara terpisah sebelumnya — deteksi yang sama diulang.
- `php.Status`/`ssl.Status` masing-masing mendeteksi ULANG distro +
  status Nginx dari nol (tidak ada cache sama sekali di request
  sebelumnya), dan `ssl.Status` bahkan memanggil ULANG seluruh
  `domains.List()` (N+1 round-trip lagi) hanya untuk menghitung daftar
  SAN satu domain.
- Hasilnya: membuka tab SSL untuk server dengan 10 domain bisa memicu
  **~20 round-trip SSH berurutan** sebelum halaman selesai render — dan
  klik "Aktifkan SSL" bisa mencapai **60-80 round-trip** (Status
  dipanggil ulang 2-3 kali di dalam satu alur Issue/Enable). Di koneksi
  SSH dengan latency 50-100ms per round-trip, itu detik-demi-detik nyata,
  padahal connection pool-nya sendiri sama sekali tidak bermasalah.

### Perbaikan di poinhost — gabungkan perintah, cache pendek, bukan ubah pool

Pool SSH poinhost (`sshpool.Executor`, §3) sudah otomatis tidak pernah
re-handshake sejak awal — jadi perbaikan di sini murni soal **mengurangi
JUMLAH round-trip per tampilan**, bukan menyentuh lapisan koneksi:

1. **`listDomains()` — SATU round-trip, bukan N+1.** Satu skrip shell
   me-`for`-loop semua file vhost DAN `cat` isinya sekaligus, dipisahkan
   marker (`===POINHOST_VHOST_START===`/`END===`), diparsing di sisi Go
   (`domains.go:parseVhostBlocks`). Berapa pun banyak domainnya, tetap 1
   round-trip SSH.
2. **`getNginxStatus()` — distro-detect + status Nginx digabung jadi SATU
   skrip** (`nginx.go:combinedStatusScript`, gabungan
   `distroDetectScript` + cek binary/versi/service Nginx), lalu
   di-**cache 5 detik per server** (`statusCacheTTL`). `domains.List`/
   `php.Status`/`ssl.Status` semua memanggil fungsi cache ini — begitu
   salah satu memicu deteksi, yang lain dalam jendela 5 detik yang sama
   dapat jawabannya GRATIS, tanpa SSH sama sekali.
3. **`listDomains()` JUGA di-cache 5 detik** — jadi `ssl.Status` yang di
   homepoin memanggil ulang `domains.List()` (N+1 round-trip) di poinhost
   tinggal baca dari cache yang sama (nyaris selalu 0 round-trip
   tambahan), bukan karena logikanya diubah jadi "lebih ringan", tapi
   karena `listDomains()` itu sendiri SUDAH murah (poin 1) dan di-cache.
4. **`testReload()` — `nginx -t` + reload digabung jadi SATU Exec**
   (`nginx.go:testReload`, pakai `set -e` supaya reload TIDAK pernah
   jalan kalau test config gagal), bukan dua panggilan terpisah seperti
   `nginx.TestReload` homepoin.
5. **Operasi tulis (Create/Delete/SetEnabled/rewriteVhost) masing-masing
   SATU Exec besar**, bukan rangkaian panggilan kecil — mis.
   `provisionDomain` (dipakai Create/CreateSubdomain) menggabungkan cek
   duplikat + siapkan webroot + tulis index.html + tulis vhost + `nginx
   -t` + reload dalam SATU skrip, dengan `trap ... ERR` untuk rollback
   (hapus vhost yang baru ditulis) kalau test config gagal — bandingkan
   dengan homepoin's `provisionDomain` yang ~8 round-trip terpisah untuk
   hal yang sama.

Cache 5 detik dipilih supaya user yang klik-klik pindah tab
domain/PHP/SSL dalam satu sesi kerja tetap terasa instan, tapi perubahan
nyata di server (mis. install Nginx lewat wizard) tetap kelihatan dalam
hitungan detik — bukan basi berjam-jam. Setiap operasi tulis yang
mengubah keadaan (`Create`, `Delete`, `SetEnabled`, `rewriteVhost`,
install) memanggil `invalidateDomainCache`/`invalidateNginxCache` supaya
tidak perlu menunggu TTL habis untuk melihat hasilnya sendiri.

### Model vhost — satu builder, bukan dua tingkat legacy+options

Homepoin punya DUA fungsi pembangun vhost berbeda (`BuildVhostConfig`
"legacy" untuk static/PHP, `BuildVhostConfigWithOptions` terpisah untuk
varian proxy) yang bisa saling drift. poinhost cuma punya SATU
(`vhost.go:buildVhostConfig` + `vhostOptions`) yang menangani
static/PHP/proxy-per-path/proxy-whole-domain DAN blok SSL sekaligus — dan
inilah yang dipakai bahkan untuk domain paling sederhana sekalipun (bukan
jalur khusus). Metadata rekonstruksi (`# poinhost-managed domain: ... php=
... ssl=on ...`) ditulis sebagai komentar di vhost itu sendiri — sama
seperti homepoin, tidak ada database terpisah untuk "apa isi vhost ini".
Konsekuensinya: `listDomains()` (dan seluruh menu Website) SELALU baca
LANGSUNG dari server tiap dipanggil (lewat cache pendek 5 detik di atas,
bukan tabel SQLite) — tidak ada state domain yang disimpan lokal sama
sekali, server itu sendiri satu-satunya sumber kebenaran.

### Domain lama buatan homepoin — dikenali & bisa dikelola juga, bukan cuma poinhost-native

Ditemukan lewat laporan user: server yang sebelumnya dipakai lewat
homepoin, begitu dibuka di poinhost, daftar domainnya kosong — padahal
situsnya tetap jalan normal di Nginx. Sebabnya: `listDomains()` cuma
men-scan file bernama `poinhost-<domain>.conf` (lihat `confPrefix` di
atas), sedangkan homepoin menulis vhost dengan nama
`homepoin-<domain>.conf` (`confPrefix` beda di kode homepoin sendiri) —
dua tool ini sengaja saling tidak menyentuh vhost buatan tool lain lewat
prefix nama file yang berbeda, tapi efeknya domain lama jadi tidak
terlihat sama sekali dari poinhost sampai dibuat ulang manual.

Diperbaiki dengan mengenali KEDUA prefix (`vhost.go:legacyConfPrefix =
"homepoin-"`), bukan cuma menampilkannya read-only — dicek dulu format
metadata vhost homepoin (`internal/modules/servers/hosting/domains/
parse.go` di repo homepoin) ternyata nyaris identik dengan punya
poinhost sendiri: marker `# homepoin-managed domain: <domain> php=...
proxy=... ssl=on ...` dan `# homepoin-proxy path=... target=... ws=1`,
urutan & arti field SAMA PERSIS dengan `# poinhost-managed domain: ...`
poinhost (poinhost memang meniru pola self-describing-vhost ini dari
homepoin sejak awal). Jadi `parseVhostFile` cukup MENORMALKAN baris
marker "homepoin-" ke "poinhost-" sebelum diproses — bukan menulis
parser kedua yang terpisah dan bisa drift.

Satu pengecualian: `homepoin-port-*.conf` (file reverse-proxy
port-forward SERVER-WIDE milik homepoin, modul lain yang kebetulan
berbagi awalan nama file yang sama) sengaja DIKECUALIKAN dari scan —
bukan vhost domain, dan poinhost belum punya modul setara (lihat §18
roadmap).

Operasi tulis (`Delete`/`SetEnabled`) yang tadinya menduga path SELALU
berprefix `poinhost-` (`configFilePath(domain, ...)`) diperbaiki memakai
path FILE YANG SESUNGGUHNYA ADA (`configFilePathVariants(info.ConfigPath)`)
— penting karena domain warisan homepoin path aslinya berprefix
`homepoin-`, bukan `poinhost-`. Operasi edit (`rewriteVhost`, dipakai
PHP/SSL/Proxy) sudah otomatis benar sejak awal karena selalu memakai
`info.ConfigPath` hasil parse, bukan menduga ulang dari nama domain.
Begitu satu domain warisan diedit lewat poinhost, ISI file-nya ditulis
ulang ke format marker poinhost sendiri (via `buildMetaComment`) — tapi
NAMA file-nya (prefix `homepoin-`) sengaja tidak di-rename, tidak ada
alasan selain kosmetik dan itu berarti round-trip SSH tambahan.

### PHP-FPM per domain — ubah `fastcgi_pass`, bukan pool config

Sama seperti homepoin: mengaktifkan versi PHP untuk satu domain BUKAN
membuat pool php-fpm baru — cuma menulis ulang `fastcgi_pass unix:<socket
versi X>` di blok `location ~ \.php$` vhost domain itu (`php.go:
PHPSetDomain` -> `rewriteVhost`), lalu reload Nginx. Socket path
mengikuti konvensi package manager (`phpFastCGISocket`: Debian/Ubuntu
`/run/php/php<versi>-fpm.sock`, RHEL/Remi `/var/opt/remi/php<compact>/
run/php-fpm/www.sock`).

### SSL — webroot mode, satu sertifikat mencakup seluruh keluarga

`ssl.go` mem-porting pola certbot homepoin apa adanya: `certbot certonly
--webroot` (BUKAN plugin nginx certbot, BUKAN `--standalone`) — setiap
domain parent + `www.<domain>` + semua subdomain-nya divalidasi dan
dicakup DALAM SATU permintaan sertifikat (`sanTargets`), lalu diterapkan
ke vhost parent DAN tiap vhost subdomain (`vhostFamily` + `SSLEnable`)
lewat `rewriteVhost` masing-masing. `Issue` TIDAK otomatis mengaktifkan
SSL di vhost (`Enable` terpisah) — supaya user bisa pastikan sertifikat
berhasil terbit dulu sebelum situs production ikut pindah ke HTTPS.

### SSL — auto-renew: deploy-hook reload Nginx + cron jaring pengaman

Sertifikat Let's Encrypt kedaluwarsa tiap ~90 hari. homepoin (dicek lewat
riset) sama sekali TIDAK punya mekanisme perpanjangan otomatis — cuma
tombol "Renew" manual. Root cause sebenarnya bukan soal *penjadwalan*
perpanjangan (paket OS certbot biasanya sudah membawa systemd
timer/cron sendiri untuk itu), tapi soal **Nginx tidak pernah reload**
setelah sertifikat baru ditulis ke disk — worker process Nginx terus
menyajikan sertifikat LAMA dari memori sampai ada reload eksplisit,
jadi auto-renew tanpa hook reload sebenarnya percuma. Ditambah lagi tidak
semua distro menjamin timer/cron OS-level itu ada/aktif.

`ensureAutoRenew` (`ssl.go`) memasang DUA lapis, keduanya idempotent lewat
SATU round-trip:

1. **Deploy-hook** di
   `/etc/letsencrypt/renewal-hooks/deploy/poinhost-reload-nginx.sh` —
   dipanggil OTOMATIS oleh certbot sendiri setelah PERPANJANGAN BERHASIL
   apa pun sumbernya (timer/cron bawaan OS, atau tombol "Perbarui
   sertifikat" manual di poinhost), menjalankan `nginx -t` lalu reload.
2. **Cron jaring pengaman** di `/etc/cron.d/poinhost-certbot-renew`
   (`certbot renew --quiet` tiap hari jam 03:12) — aman dijalankan harian
   karena certbot sendiri cuma benar-benar memperbarui sertifikat yang
   mendekati kedaluwarsa (<30 hari); ini jaminan untuk distro yang tidak
   punya timer/cron bawaan certbot.

`ensureAutoRenew` dipanggil best-effort (kegagalan tidak membatalkan
operasi utama) di akhir `SSLIssue` dan `SSLEnable`, jadi otomatis
terpasang begitu SSL pertama kali diterbitkan/diaktifkan lewat poinhost.
Untuk sertifikat yang sudah ada SEBELUM fitur ini ditambahkan (mis.
diterbitkan manual via SSH), ada binding eksplisit
`EnableWebsiteSSLAutoRenew` + tombol "Pasang auto-renew" di tab SSL,
muncul kalau `SSLStatus.autoRenewEnabled` terbaca `false` untuk sertifikat
yang sudah ada.

### UX: alur "Buat Website" gabungan, bukan 3 halaman terpisah

Di homepoin, membuat situs baru + PHP + SSL adalah TIGA kunjungan halaman
terpisah (Domains -> PHP -> SSL), masing-masing mulai dari nol lagi.
`CreateWebsiteModal.tsx` + `website.Service.CreateWebsite` menggabungkan
ketiganya jadi SATU submit: nama domain + versi PHP (opsional, dropdown
diisi dari versi yang sudah terpasang) + centang SSL (opsional, dengan
email). Kegagalan PHP/SSL (keduanya opsional) dilaporkan sebagai
**warning**, BUKAN membatalkan domain yang sudah berhasil dibuat — situs
statis yang sudah jadi tetap berguna walau mis. penerbitan SSL gagal
karena DNS belum diarahkan.

Instalasi komponen (Nginx/repo PHP/versi PHP/Certbot) memakai SATU
mekanisme stream generik (`Service.StreamInstall`, kind:
`"nginx"|"php-repo"|"php"|"certbot"`) lewat event Wails
`website:install:<streamId>` — pola yang sama dengan instalasi Docker
engine di §11 (stream registry `a.streams` di `app.go` dipakai ULANG,
bukan dibuat baru khusus Website), mengikuti homepoin yang juga memakai
SATU handler WS untuk semua jenis instalasi hosting (bukan endpoint
terpisah per jenis).

## 13. Website — 7 tab per-domain: Files, Logs, Proxy, DNS, SFTP, Cron, Database

Melengkapi seluruh menu Website (§12) — 7 tab yang tadinya placeholder di
`DomainDetailModal.tsx` sekarang semua fungsional. Semuanya memanfaatkan
`getDomain`/`listDomains` yang SUDAH ter-cache & O(1) round-trip (§12) —
riset homepoin menemukan tab-tab ini (khususnya Cron dan SSL/domain
lainnya) memanggil ulang resolusi vhost per operasi (`domains.ResolveRoot`)
yang di homepoin sendiri N+1; di poinhost bug itu otomatis tidak ada sejak
awal karena `getDomain` sudah diperbaiki di §12, bukan perbaikan baru
per-tab.

### DNS — generator zona BIND, TANPA exec SSH sama sekali

Temuan riset paling mengejutkan: homepoin **tidak menjalankan DNS server
apa pun** di VPS untuk fitur ini — ini murni kalkulator/generator file
zona BIND untuk di-import manual ke provider DNS (mis. Cloudflare).
`website.DNSPreview` (`dns.go`) menghitung record A/AAAA (apex + tiap
subdomain, dari `listDomains`) dan CNAME (`www`) mengarah ke `Server.Host`
— seluruhnya komputasi Go murni, nol SSH. Export memakai dialog "Simpan"
native (`ExportWebsiteDNSZone`, pola sama dengan Upload/Download di
Files), bukan trik Blob/`<a download>` ala browser.

### Logs — snapshot + stream, path ikut vhost yang sesungguhnya

`domainLogPath` (`logs.go`) memakai path yang SAMA dengan `access_log`/
`error_log` yang ditulis `buildVhostConfig` (§12) — tidak ada konvensi
path terpisah yang bisa drift dari vhost aslinya. Snapshot (`tail -n`) +
stream realtime (`tail -f` via `ExecStreamDedicated`, event
`website:logs:<streamId>`) — pola identik dengan log container Docker
(§11).

### Proxy — mengedit field vhost yang SUDAH ada, bukan mekanisme baru

Tab ini cuma UI+API untuk field `ProxyTarget`/`ProxyWebSocket`/`ProxyRules`
di `DomainInfo` yang sudah ada sejak §12 (dipakai juga oleh PHP/SSL) —
`ProxySetDomain`/`ProxySetRule`/dst (`proxy.go`) semuanya lewat
`rewriteVhost` yang sama. Proxy whole-domain SELALU menang atas PHP di
`buildLocationBlocks`; proxy per-path bisa hidup berdampingan dengan
PHP/static di path lain. Reverse-proxy port-forward SERVER-WIDE (menu
"Reverse Proxy" terpisah di homepoin, tidak terikat satu domain) SENGAJA
tidak ikut di-porting — itu bukan bagian dari menu Website.

### Files — reuse penuh modul Files yang sudah ada, bukan file manager baru

`DomainFilesTab.tsx` cuma resolve document root domain (`GetWebsiteDomainRoot`,
1x panggilan `getDomain` yang sudah cache) lalu merender ULANG
`FilesPanel` yang sama dipakai modul Files server-wide (§10), dengan prop
baru `rootPath` yang mengunci navigasi (tombol "Naik" & breadcrumb
berhenti di root domain, tidak bisa naik ke direktori lain atau file
sistem). Backend `files.Service` TIDAK disentuh sama sekali — jail-nya
murni di lapisan UI (poinhost aplikasi desktop single-user memakai
kredensial SSH miliknya sendiri, beda dari homepoin yang perlu
memisahkan akses antar pelanggan di backend multi-tenant-nya).

### SFTP — akun Linux ter-chroot sungguhan, bukan reuse user SSH

`sftp.go` mem-porting mekanisme homepoin apa adanya: `useradd -M -s
/usr/sbin/nologin` (tanpa home dir, tanpa shell login) + drop-in
`/etc/ssh/sshd_config.d/poinhost-sftp-<user>.conf` yang mengatur
`Match User` + `ChrootDirectory` + `ForceCommand internal-sftp`. Titik
kritis yang dipertahankan: **chroot default ke folder INDUK
document root** (`domainBaseFromRoot`, biasanya satu tingkat di atas
`public_html`), BUKAN document root itu sendiri — sshd mewajibkan
`ChrootDirectory` (dan semua leluhurnya sampai `/`) dimiliki `root:root`
& tidak bisa ditulis group/other, padahal `public_html` sengaja dimiliki
grup web (§12) supaya PHP-FPM & SFTP bisa sama-sama menulis. Beda dari
homepoin (~9 exec berurutan untuk satu pembuatan akun): SEMUA langkah —
dedupe/cleanup akun yatim, rantai kepemilikan chroot, `useradd`,
`chpasswd`, kepemilikan home dir, tulis config, `sshd -t` + reload — jadi
SATU script/round-trip dengan `trap ... ERR` untuk rollback.

### Cron — file tunggal `/etc/cron.d/poinhost`, format self-describing

Sama seperti homepoin: SATU file untuk semua job semua domain (bukan
`crontab -e` per user, bukan systemd timer), dibedakan lewat baris
komentar metadata. Beda desain dari homepoin: setiap field (schedule,
command/url/method/payload/headers, deskripsi) disimpan **base64 penuh di
baris metadata**, bukan di-infer balik dari baris cron mentahnya —
`buildCronLine` SELALU menulis ulang baris eksekusi dari metadata saat
serialisasi, baris itu sendiri tidak pernah dibaca balik oleh parser.
Ini menghilangkan seluruh kelas bug parsing (spasi/kutip aneh di command)
yang harus diwaspadai kalau mencoba round-trip lewat sintaks shell
mentah. Baris non-poinhost (kalau admin server menambah entri manual di
file yang sama) dipertahankan apa adanya. Perbaikan performa: homepoin
me-resolve root SETIAP domain berbeda yang muncul di file cron per
operasi tulis (N+1 di atas N+1 — lihat riset); poinhost cukup satu
`listDomains()` (sudah cache) untuk membangun peta domain→root sekali per
operasi.

### Database — TERNYATA bukan benar-benar domain-scoped (sama seperti homepoin)

Temuan riset: tab "Database" di bawah satu domain di homepoin ternyata
**tidak pernah memfilter apa pun berdasarkan domain** — field `Domain` di
request cuma dibawa sebagai breadcrumb UI. Ini murni utilitas provisioning
ringan (status/install MySQL atau PostgreSQL, buat database, buat user +
grants) yang kebetulan bisa dibuka dari tab manapun. `database.go`
mem-porting itu apa adanya (privilege 6-kategori tetap: read/write/
create/alter/drop/execute, dipetakan ke grant SQL konkret berbeda per
engine). **Perbedaan besar dari homepoin**: karena semua perintah SQL di
sini dieksekusi via SSH LANGSUNG di server target (bukan tunnel TCP dari
proses homepoin yang terpisah), poinhost SAMA SEKALI TIDAK PERLU membuka
akses remote database (bind ke `0.0.0.0`, edit `pg_hba.conf`, buka
firewall) yang dilakukan homepoin — jauh lebih sederhana dan tidak
memperluas permukaan serangan server target tanpa alasan. Database
browser server-wide ala `mysqlmanager`/`pgmanager` homepoin (tabel/baris/
query arbitrer) awalnya di luar scope bagian ini — kemudian dibangun
khusus untuk MySQL dengan arsitektur berbeda, lihat §14.

## 14. Database Manager: vault lokal + Explore koneksi driver asli (MySQL & PostgreSQL) + tautan domain

Homepoin punya fitur "MySQL Manager" (`mysqlmanager`) yang SEPENUHNYA
terpisah dari tab Database per-domain (§13) — server-wide, tidak terkait
domain/website apa pun. Riset mendalam atas fitur itu (lihat konteks di
bawah) jadi dasar desain ulang berikut, bukan port apa adanya. Fitur yang
sama kemudian diminta juga untuk PostgreSQL — lihat subbagian PostgreSQL
di bawah untuk apa yang beda dari MySQL dan kenapa.

### Temuan homepoin: koneksi asli + password terenkripsi DI SERVER TARGET

`mysqlmanager` homepoin membuka koneksi protokol MySQL ASLI
(`go-sql-driver/mysql`) yang ditunnel lewat channel SSH (bukan exec CLI)
supaya browse tabel besar tetap cepat — desain ini benar dan dipertahankan
di poinhost. Yang JADI masalah: passwordnya disimpan di file JSON
terenkripsi `/root/.homepoin_mysql_creds.json` **DI SERVER TARGET itu
sendiri**, kuncinya diturunkan (PBKDF2) dari SATU master password
level-aplikasi homepoin (bukan per-server/per-user) — artinya (1) satu
titik gagal untuk kredensial SEMUA server yang dikelola, (2) menaruh
artefak kredensial tambahan di server produksi customer tanpa perlu, dan
(3) sama sekali tidak ada mekanisme kalau password diganti manual di
server (`ALTER USER` langsung) — koneksi berikutnya gagal diam-diam,
user harus sadar sendiri lalu masuk UI untuk menyimpan ulang.

### Desain poinhost: vault LOKAL di mesin user, bukan di server target

poinhost adalah desktop app single-user (bukan web app yang mungkin
dipakai bersama banyak operator seperti homepoin) — tidak ada alasan
menaruh kredensial tambahan di server yang dikelola. `internal/core/
secrets` (`vault.go`/`keyring.go`/`file.go`/`service.go`) menyimpan
password HANYA di mesin yang menjalankan poinhost:

1. **OS keychain** (`github.com/zalando/go-keyring` — Keychain di macOS,
   Credential Manager di Windows, Secret Service/libsecret di Linux)
   sebagai pilihan utama — tidak perlu kelola kunci enkripsi sendiri,
   sudah terlindungi login OS user. `probeKeyring()` benar-benar mencoba
   set+delete sebelum dipakai (bukan cuma cek biner ada).
2. **Fallback file lokal AES-256-GCM** (`~/.poinhost/secrets.json`, kunci
   acak 32-byte di `~/.poinhost/secret.key` permission 0600) kalau OS
   keychain tidak tersedia (mis. Linux headless tanpa Secret Service) —
   beda dari homepoin, kuncinya murni acak spesifik-mesin-ini, BUKAN
   diturunkan dari master password yang diketik user (poinhost sengaja
   tidak punya konsep itu, lihat §7).

Tabel `website_db_credentials` di SQLite poinhost sendiri (migrations)
cuma menyimpan METADATA (server/engine/username/host mana yang sudah
tersimpan) supaya UI bisa menampilkan daftarnya — beberapa backend OS
keychain tidak mendukung listing, jadi metadata inilah yang dipakai UI,
isi password aslinya selalu dari vault (`dbcreds.go`).

### Explore MySQL: koneksi driver asli, TANPA registry dialer global

Per arahan eksplisit saat desain ("kalau ada potensi lambat pagination
mending pakai driver langsung saja") — `mysqlexplore.go` memakai
`go-sql-driver/mysql` sungguhan, BUKAN exec CLI per halaman seperti tab
Database (§13) — supaya paginasi tabel besar tetap cepat. Bedanya dari
pendekatan homepoin: dialer per-tunnel dipasang lewat `cfg.DialFunc`
(field di `mysql.Config`, dikonsumsi lewat `mysql.NewConnector(cfg)` +
`sql.OpenDB`, BUKAN `sql.Open` dengan DSN string yang tidak bisa membawa
closure), jadi tidak perlu registry nama-network global per server seperti
`mysqlmanager` homepoin. Tunnel-nya sendiri (`sshpool.Executor.DialTunnel`,
`internal/core/sshpool/tunnel.go`) memakai SLOT SHARED yang SAMA dipakai
exec modul lain — Dial lewat `ssh.Client.Dial` (channel `direct-tcpip`)
tidak pernah memicu handshake SSH baru. Alamat MySQL di sisi remote selalu
`127.0.0.1:3306` (localhost dari sudut pandang server itu sendiri) — grant
host user (`%`, `localhost`, dst) cuma kunci lookup kredensial, bukan
alamat jaringan, konsisten dengan `runMySQL` di §13.

Operasi yang didukung: `ListDatabases` (otomatis terbatas sesuai grants
MySQL user itu sendiri — `SHOW DATABASES` MySQL memang begitu),
`ListTables`/`ListColumns` (dari `information_schema`), `TableRows`
(paginated LIMIT/OFFSET + total count), `InsertRow`/`UpdateRow`/
`DeleteRow` (identifier divalidasi regex + di-quote backtick, WHERE WAJIB
diisi untuk update/delete — default ke kolom PRIMARY KEY, fallback ke
seluruh kolom kalau tabel tidak punya PK), dan `ExecuteQuery` (satu
statement bebas, guard sederhana menolak `;` ganda).

### Koreksi setelah audit ulang: cache koneksi, bukan dial per panggilan

Versi pertama fitur ini (sebelum ditinjau ulang atas permintaan eksplisit
"saya tidak ingin ada handshake yang tidak perlu") ternyata **membuka
koneksi MySQL baru DAN menutupnya lagi di setiap panggilan** — artinya
setiap klik ganti halaman/edit sel/pindah tabel memicu handshake MySQL
baru (bukan handshake SSH — itu tetap satu, lewat `DialTunnel` — tapi
handshake protokol MySQL-nya sendiri, yang tidak murah: negosiasi versi,
auth native password, dst). Ini persis masalah yang coba dihindari sejak
awal proyek ini (lihat §2/§3), jadi diperbaiki:

- **Cache koneksi per (server, user, host)** di `Service.mysqlConns` —
  `getOrDialMySQLExplore` mengembalikan koneksi yang sudah hidup kalau
  ada, dial baru HANYA kalau belum pernah/sudah idle lama. Semua endpoint
  Explore (List*/TableRows/Insert/Update/Delete/ExecuteQuery) lewat jalur
  ini, jadi handshake MySQL cuma terjadi SEKALI per kredensial yang aktif
  dipakai, bukan sekali per klik.
- **`SetConnMaxLifetime(0)`** (sebelumnya 2 menit) — batas umur 2 menit
  akan memaksa reconnect periodik walau koneksi sedang aktif dipakai
  terus-menerus, sama saja menambah handshake tak perlu secara berkala.
  Koneksi sekarang hidup selama benar-benar dipakai; MySQL server sendiri
  yang menutup kalau betul-betul menganggur (`wait_timeout`).
- **Sapu idle 5 menit** (`mysqlConnIdleTTL`) — supaya tidak menahan
  koneksi ke server yang sudah lama tidak di-browse selamanya.
- **Dedup dial bersamaan** (`mysqlDialing`) — kalau dua panggilan datang
  nyaris bersamaan untuk kredensial yang SAMA sebelum ada apa pun di
  cache (mis. dua tab dibuka hampir serentak), panggilan kedua menunggu
  hasil dial yang pertama alih-alih ikut dial sendiri — race sederhana
  yang kalau dibiarkan bisa membuat salah satu panggilan gagal dengan
  "database is closed" (koneksi yang "kalah" langsung ditutup lagi oleh
  yang lain).
- **Eviction saat kredensial berubah** — `SaveDBCredential` (menyimpan
  password baru) dan `ForgetDBCredential` menutup & membuang entry cache
  untuk kredensial itu, supaya operasi berikutnya tidak diam-diam
  memakai sesi lama dengan password sebelumnya. `verifyMySQLCredential`
  (opsi Verify saat simpan) langsung MENYIMPAN koneksi hasil verifikasi
  ke cache (bukan menutupnya) — Explore pertama setelah menyimpan
  kredensial jadi instan.
- `CloseAllMySQLConns` dipanggil dari `app.go` (`shutdown`), sejalan
  dengan `pool.Close()` untuk koneksi SSH.

Audit yang sama menemukan dua isu lain: (1) `openMySQLExplore` semula
memanggil `resolveAccess` yang mensyaratkan user SSH root/sudo — padahal
Explore cuma perlu koneksi SSH untuk DI-TUNNEL, sama sekali tidak exec
apa pun di server, jadi server dengan SSH user terbatas (non-root,
non-sudo — praktik yang lebih aman) salah ditolak; sekarang cuma
memvalidasi server-nya ada (`servers.Get`), tanpa syarat privilege. (2)
`ExecuteQuery` menjalankan `USE database` lalu query lewat `*sql.DB`
biasa — karena `database/sql` bebas memakai koneksi fisik BERBEDA dari
pool untuk tiap panggilan, `USE` di satu koneksi tidak dijamin berlaku
untuk query di koneksi lain begitu pool membesar (jadi lebih mungkin
gagal justru SETELAH koneksi mulai di-cache/reuse dengan >1 koneksi
fisik); diperbaiki dengan `db.Conn(ctx)` — mengambil SATU koneksi fisik
eksplisit dan memakainya untuk `USE` + query yang sama.

Kedua bug bookkeeping cache di atas (reuse, eviction, idle sweep, dedup
dial bersamaan) punya test tersendiri di
`internal/modules/website/mysqlexplore_test.go` (lulus dengan `-race`) —
satu-satunya modul di proyek ini yang punya test unit sejauh ini, sengaja
ditambahkan karena bagian ini genuinely concurrency-sensitive.

### Lanjutan koreksi: pengalaman "senyaman tools desktop MySQL sungguhan"

Setelah koneksinya sendiri tidak lagi dial ulang per klik, giliran pola
pemanggilan di frontend yang diaudit — permintaan eksplisitnya adalah
performa & efisiensi seperti Navicat, bukan cuma "tidak ada handshake
berlebih". Ditemukan frontend (`MySQLExplorerModal.tsx`) masih mengulang
kerja yang tidak perlu di setiap interaksi, walau koneksinya sendiri
sudah dipakai ulang:

- **`SELECT COUNT(*)` dihitung ulang di SETIAP pindah halaman & setiap
  edit satu sel** — padahal jumlah baris tidak berubah oleh keduanya.
  Untuk tabel InnoDB besar, `COUNT(*)` tetap harus scan index klaster
  penuh (tidak instan seperti MyISAM), jadi ini nyata-nyata lambat kalau
  diulang tiap klik. Diperbaiki: `MySQLTableRowsRequest.SkipTotal` —
  paginasi & edit-sel kirim `skipTotal: true` (pakai angka total yang
  sudah ada), hanya insert/delete (yang MEMANG mengubah jumlah baris) dan
  buka tabel pertama kali yang minta hitung ulang.
- **Struktur kolom (`information_schema.columns`) diambil ulang di
  SETIAP pindah halaman & setiap edit/insert/delete satu baris** —
  padahal struktur tabel tidak berubah oleh operasi-operasi itu.
  Diperbaiki: `openTable` (dipanggil HANYA saat benar-benar pindah ke
  tabel lain) satu-satunya titik yang mengambil kolom; `loadRows`
  (paginasi + refresh sesudah mutasi) tidak menyentuhnya sama sekali.
- **Klik database/tabel yang SUDAH terbuka tetap memicu round-trip
  baru** (daftar tabel / kolom+baris) — sekarang di-skip kalau targetnya
  sama dengan yang sedang aktif.
- **Tombol "⟳ Refresh" eksplisit** ditambahkan di toolbar tabel — karena
  total baris & isi tidak lagi otomatis dihitung ulang di setiap
  interaksi, user butuh cara sadar untuk memaksa muat ulang kalau tahu
  data berubah dari tempat lain (query manual, proses lain) — pola yang
  sama dipakai tools desktop MySQL manapun (Navicat/DBeaver/TablePlus
  semuanya punya tombol refresh eksplisit, bukan auto-poll).

Paginasi LIMIT/OFFSET sendiri (biaya O(offset) untuk halaman yang sangat
dalam) SENGAJA tidak diubah ke keyset pagination — ini batasan bawaan
pendekatan LIMIT/OFFSET yang sama dipakai semua tools desktop MySQL
populer (Navicat/DBeaver/dst), bukan sesuatu yang poinhost lakukan lebih
buruk dari mereka.

### Solusi masalah password basi: deteksi eksplisit, bukan diam

`classifyMySQLConnError` menandai kegagalan "Access denied" dengan prefix
`AUTENTIKASI_GAGAL:` yang dideteksi `MySQLExplorerModal.tsx` di frontend —
alih-alih gagal diam-diam seperti homepoin, modal langsung menampilkan
form "masukkan ulang password", yang saat disimpan otomatis diverifikasi
dulu (`SaveDBCredentialRequest.Verify`, coba konek beneran) sebelum
ditulis ke vault, supaya vault tidak pernah menyimpan kredensial salah.

### Tautan domain<->database: kurasi lokal, beda dari field dekoratif homepoin

`domaindb.go` + tabel `website_domain_databases` (migrations) menautkan
database ke domain — TAPI murni metadata kurasi di SQLite lokal poinhost,
BUKAN scoping akses sungguhan (MySQL/PostgreSQL sendiri tetap
server-wide, grants tidak berubah). Ini yang membuat tab Database poinhost benar-benar
terelasi dengan Website: user bisa menandai "database X dipakai situs
ini" dan tab Database domain tersebut menampilkannya balik — beda dari
homepoin yang field `Domain`-nya di tab Database sekadar breadcrumb UI,
tidak pernah dibaca ulang di mana pun (lihat §13).

### PostgreSQL Explore: satu koneksi per DATABASE, bukan lintas database

Fitur yang sama diminta untuk PostgreSQL (`pgexplore.go`) — sengaja BUKAN
sekadar salin-tempel `mysqlexplore.go`, karena PostgreSQL punya satu
perbedaan protokol yang mengubah desain cache-nya: **koneksi PostgreSQL
SELALU terikat ke satu database saat connect**, tidak ada padanan `USE
database` MySQL untuk pindah database di koneksi yang sama. Konsekuensinya:

- **Key cache beda bentuk.** MySQL: `(server, user, host)` — satu koneksi
  bisa melihat banyak database. PostgreSQL: `(server, role, DATABASE)` —
  pindah database berarti membuka entry cache LAIN. Satu role bisa punya
  BANYAK koneksi ter-cache sekaligus (satu per database yang pernah
  di-browse). `evictDBConnsWithPrefix` (bukan `evictDBConn` biasa) dipakai
  saat password berubah, supaya SEMUA koneksi role itu (lintas database)
  ikut ditutup, bukan cuma satu.
- **Placeholder `$1, $2, ...`**, bukan `?` — driver PostgreSQL (`pgx`,
  lewat `pgx/v5/stdlib` supaya kompatibel `database/sql`) mensyaratkan
  ini, beda dari `go-sql-driver/mysql`.
  `buildPGSetClause`/`buildPGWhereClause` membangun placeholder
  bernomor urut, bisa digabung (SET dulu, lalu WHERE lanjut nomornya)
  dalam satu statement UPDATE.
- **Tidak ada `UPDATE/DELETE ... LIMIT 1`** di PostgreSQL — `PGExploreUpdateRow`/
  `PGExploreDeleteRow` memakai pola standar `WHERE ctid = (SELECT ctid
  FROM ... WHERE <kondisi> LIMIT 1)` (`ctid` = pengenal fisik baris bawaan
  PostgreSQL) supaya tetap kena PERSIS satu baris seperti versi MySQL.
- **Schema, bukan cuma database** — PostgreSQL punya lapisan
  `database > schema > tabel` (MySQL cuma `database > tabel`). Default
  `"public"`, bisa diganti lewat input schema di sidebar
  `PGExplorerModal.tsx`. Primary key dideteksi lewat join
  `information_schema.table_constraints`+`key_column_usage` (MySQL:
  `COLUMN_KEY` sudah tersedia langsung di `information_schema.columns`).
- **`ExecuteQuery` pakai `SET search_path` (bukan `USE`) di koneksi FISIK
  yang sama** (`db.Conn(ctx)`) — persis alasan yang sama dengan bug `USE`
  MySQL yang sudah diperbaiki (lihat "Koreksi setelah audit ulang" di
  atas): dua statement terpisah lewat `db.Exec`/`db.Query` biasa bisa
  jatuh di koneksi fisik pool yang berbeda begitu `MaxOpenConns` > 1.

**Yang SAMA persis dengan MySQL** (sengaja digeneralisasi, bukan
diduplikasi): mekanisme cache-koneksi (reuse, idle sweep 5 menit, dedup
dial bersamaan) sekarang satu implementasi generik di `dbconnpool.go`
dipakai KEDUA engine — `getOrDialDBConn`/`putCachedDBConn`/`evictDBConn`/
`CloseAllDBConns`, diuji di `dbconnpool_test.go` (lulus `-race`). Begitu
juga penyimpanan kredensial (`dbcreds.go` — `SaveDBCredential`/
`ForgetDBCredential`/vault lokal) dan tautan domain<->database
(`domaindb.go`) — sudah engine-generic sejak awal, PostgreSQL tinggal
pakai jalur yang sama tanpa perubahan (`dbCredentialHost` memaksa kolom
host jadi `"-"` untuk PostgreSQL karena role tidak terikat host seperti
user MySQL — itu urusan `pg_hba.conf`, bukan identitas role).
`DBCreateUserRequest.SaveCredential` (centang "simpan untuk Explore
nanti") tadinya cuma berlaku untuk MySQL, sekarang berlaku untuk
PostgreSQL juga.

### Alur pemakaian end-to-end

1. Buat user/role database baru lewat tab Database (§13) — centang
   opsional "simpan untuk Explore nanti" (`DBCreateUserRequest.
   SaveCredential`) supaya password yang sudah diketik user langsung
   tersimpan ke vault, tanpa perlu diketik ulang. Berlaku untuk MySQL
   maupun PostgreSQL.
2. Untuk user/role yang sudah ada sebelumnya (dibuat di luar poinhost,
   atau sebelum fitur ini ada): tombol "🔗 Hubungkan kredensial" —
   masukkan password, diverifikasi dulu, baru disimpan.
3. Tombol "🔍 Explore" muncul begitu kredensial tersimpan — membuka
   `MySQLExplorerModal` atau `PGExplorerModal` (sidebar database/tabel,
   grid baris dengan edit inline dobel-klik, insert/delete baris, kotak
   query bebas, paginasi).
4. Password basi terdeteksi otomatis lewat marker `AUTENTIKASI_GAGAL:`,
   modal menawarkan form simpan-ulang alih-alih gagal diam-diam.
5. Database bisa ditautkan ke domain manapun lewat tombol "🔗 Tautkan" di
   tabel Database — murni kurasi, tidak mengubah akses.

### Titik masuk: tab per-domain DAN modul "Database" top-level per server

Awalnya satu-satunya jalan ke sini adalah tab Database di bawah SATU
domain tertentu (§13) — padahal, seperti dicatat di §13, modul ini
TERNYATA bukan benar-benar domain-scoped sama sekali. Praktiknya ini
berarti server yang belum punya domain/website apa pun (mis. server
database murni) tidak bisa memakai fitur ini sama sekali, meski
backend-nya sudah sanggup — murni keterbatasan navigasi UI, bukan
keterbatasan backend.

Diperbaiki dengan mengekstrak seluruh isi tab Database
(`DomainDatabaseTab.tsx` lama) jadi `DatabaseManagerPanel.tsx` dengan
prop `domain` OPSIONAL: diisi → tampilkan section "tautkan ke domain"
(dipakai dari `DomainDetailModal.tsx`, sama seperti sebelumnya);
dikosongkan → section itu dilewati saja. Modul baru "Database" didaftarkan
di `ServerWorkspace.tsx` sebagai peer Website/Files/Terminal/Docker —
sekarang setiap tab server punya akses langsung ke provisioning + Explore
database TANPA perlu domain/website apa pun dulu. Nol perubahan Go: kedua
titik masuk memanggil binding `website.Service` yang PERSIS SAMA.

### Instalasi: bawaan distro (default) vs versi PINNED lewat repo resmi vendor

Pertanyaan user: apakah instalasi MySQL/PostgreSQL mengambil versi paling
baru, dan bisa dipilih? Jawaban sebelum fitur ini: TIDAK ke keduanya —
`dbInstallScript` cuma menjalankan `apt-get install -y mariadb-server` /
`apt-get install -y postgresql` polos, tanpa repo pihak ketiga apa pun,
jadi versi yang terpasang murni ikut apa pun yang jadi default di repo
BAWAAN distro itu (mis. Ubuntu 22.04 memberi PostgreSQL 14, Ubuntu 24.04
memberi PostgreSQL 16) — bisa jauh dari rilis terbaru, dan tidak bisa
dipilih sama sekali.

Riset pembanding: homepoin sendiri juga TIDAK punya pemilihan versi untuk
MySQL/PostgreSQL. Tapi modul PHP poinhost (§12) sudah punya pola persis
untuk masalah yang sama (PHP-FPM juga cuma dapat satu versi bawaan distro
tanpa repo pihak ketiga): `phpRepoScript` menambah repo (PPA `ondrej/php`/
`sury.org`/Remi) yang menyediakan banyak versi sekaligus, lalu
`phpInstallVersionScript` memasang versi yang dipilih. Pola itu yang
diikuti di sini (`internal/modules/website/dbversion.go`):

- **MariaDB** (engine `"mysql"`): repo resmi MariaDB Foundation sendiri,
  dipasang lewat skrip resmi mereka (`mariadb_repo_setup
  --mariadb-server-version="mariadb-<versi>"`) — skrip ini SATU-SATUNYA
  yang perlu dijalankan, mendeteksi & menyiapkan repo apt MAUPUN dnf/yum
  otomatis. Beda dari pola PHP: repo MariaDB terikat ke SATU versi
  tertentu (bukan "satu repo, banyak versi tersedia" ala PPA ondrej/php),
  jadi repo-setup dan install versi digabung jadi SATU skrip, bukan dua
  tahap terpisah. Nama paket beda per package manager: `mariadb-server`
  (apt) vs `MariaDB-server` (dnf/yum, penamaan resmi RPM MariaDB pakai
  kapital).
- **PostgreSQL**: repo resmi PGDG. Di apt (`apt.postgresql.org`), SEMUA
  versi terpasang dipayungi SATU service generik `postgresql` (pg_wrapper)
  — mirip pola PHP (satu repo, pilih versi lewat nama paket
  `postgresql-<versi>`). Di dnf/yum (RHEL/AlmaLinux dst), PGDG memberi
  TIAP versi service TER-VERSI-nya sendiri (`postgresql-16`, dst — supaya
  beberapa versi bisa hidup berdampingan), dan module bawaan distro
  (`dnf module disable postgresql`) harus dimatikan dulu supaya tidak
  bentrok dengan paket PGDG.
- Versi PINNED yang ditawarkan: `DBSupportedVersions(engine)` — statis,
  tanpa round-trip SSH (dipanggil sekali di frontend, sama seperti
  `GetWebsiteDBPrivileges`). Di luar daftar itu, opsi "(bawaan distro)"
  (versi kosong) tetap selalu tersedia dan perilakunya TIDAK berubah sama
  sekali dari sebelum fitur ini ada.

**Konsekuensi arsitektur nama service PostgreSQL ter-versi**: karena
service PostgreSQL BISA ter-versi (jalur dnf/PGDG) atau generik (jalur
apt/default), `dbStatusScriptPostgres` dan `DBStart` tidak bisa lagi
menebak satu nama service tetap — keduanya sekarang mengekstrak versi
mayor dari `psql --version` di dalam skrip yang sama, coba nama
ter-versi (`postgresql-$VER`) DULU, baru fallback ke nama generik
(`postgresql`). Ini jalan untuk KEDUA jalur instalasi tanpa perlu tahu
lebih dulu jalur mana yang dipakai, dan tetap backward-compatible untuk
instalasi default yang sudah ada (tidak pernah punya service ter-versi,
jadi selalu jatuh ke fallback generik seperti sebelumnya).
`DBEngineStatus.RepoConfigured` (dideteksi dari keberadaan file repo
MariaDB/PGDG) melengkapi info ini di UI.

### MySQL asli (Oracle): belum diimplementasikan, tapi seam-nya sudah disiapkan

Follow-up dari fitur di atas: user sempat bertanya kenapa engine
`"mysql"` sebenarnya MariaDB, bukan MySQL asli, dan apakah bisa MySQL
saja. Setelah dicek: performanya nyaris setara untuk beban kerja tipikal
poinhost (website PHP/CRUD biasa) — bedanya lebih ke fitur spesifik
(optimizer/DDL MySQL 8.x vs storage engine tambahan MariaDB), bukan
kecepatan mentah. Mekanisme resmi Oracle untuk otomasi non-interaktif
(`mysql-apt-config_<versi>-1_all.deb` + `debconf-set-selections`, atau
RPM `mysql80-community-release-el<N>-<rev>.noarch.rpm`) memakai nama
file BER-VERSI yang berubah dari waktu ke waktu — jauh lebih rawan basi
dibanding `mariadb_repo_setup`/PGDG yang dipakai di atas (skrip resmi
vendor yang stabil, tidak perlu tahu nomor versi paket bootstrap-nya).
Keputusan: tunda MySQL asli sampai benar-benar dibutuhkan, tapi siapkan
dulu seam-nya supaya nanti tinggal diisi, bukan refactor besar:

- Value yang dikembalikan `DBSupportedVersions("mysql")` diberi PREFIX
  produk (`"mariadb-10.11"`, bukan cuma `"10.11"`) — supaya kalau
  `"mysql-8.0"` dkk ditambahkan nanti, keduanya hidup berdampingan di
  SATU dropdown yang sama tanpa field/DTO baru.
- `dbVersionedInstallScript` sudah membaca prefix itu dan mem-dispatch ke
  `mariaDBVersionedInstallScript`/`mysqlVersionedInstallScript` — yang
  kedua ITU SENDIRI sudah ada sebagai fungsi, tapi cuma `return "", false`
  (belum diisi skripnya). `DBSupportedVersions` tidak pernah menawarkan
  versi `"mysql-*"` apa pun, jadi jalur ini belum bisa dicapai dari UI
  sama sekali — murni titik perluasan yang menunggu diisi, bukan kode
  mati yang membingungkan.
- Konsekuensinya: `dbInstallScript`, `StreamInstall`, frontend
  (`DatabaseManagerPanel.tsx`'s `versionLabel` sudah menangani prefix
  `"mysql-"` juga), dan binding Wails semuanya SUDAH SIAP — menambah
  MySQL asli nanti tinggal mengisi `mysqlVersionedInstallScript` +
  menambah `supportedMySQLVersions` ke `DBSupportedVersions`, tidak ada
  yang lain perlu disentuh.

### Akses Docker→database: bind otomatis + firewall terbatas, bukan setting manual

Keluhan user: di homepoin, container Docker di host yang sama tidak bisa
connect ke MySQL/PostgreSQL yang terpasang di host itu, walau
`host.docker.internal` sudah diarahkan dengan benar di sisi Docker. Itu
bukan masalah Docker — akar masalahnya di database itu sendiri: instalasi
paket distro bawaan (MariaDB/PostgreSQL) SELALU cuma mendengarkan di
`127.0.0.1` (loopback). Koneksi yang datang dari container (lewat
interface bridge Docker, bukan loopback) ditolak di level TCP sebelum
sempat autentikasi sama sekali — grant user/role sama sekali tidak
relevan di titik ini. `host.docker.internal` menyelesaikan resolusi DNS
container→host dengan benar; yang menolaknya adalah socket listener
database, bukan DNS-nya.

Solusi (`internal/modules/website/dockeraccess.go`) dibuat SEMPIT, bukan
"buka ke internet" ala fitur remote-access `pgmanager` homepoin yang
sengaja tidak diporting (§14 di atas, alasan arsitektur tunnel):

- **Bind**: MySQL/MariaDB → `bind-address = 0.0.0.0` lewat file drop-in
  baru `99-poinhost-docker.cnf` (di `mariadb.conf.d/` untuk distro
  apt-based, `my.cnf.d/` untuk dnf/yum) — bukan mengedit file config
  vendor yang lokasinya beda-beda antar distro. PostgreSQL →
  `ALTER SYSTEM SET listen_addresses = '*';`, ditulis ke
  `postgresql.auto.conf` (prioritas tertinggi, jalur SQL yang stabil di
  semua versi, tidak perlu mencari `postgresql.conf` di path spesifik
  distro).
- **Pembatasan jangkauan**: bukan `0.0.0.0/0`, tapi `172.16.0.0/12` —
  rentang alamat bridge network Docker BAWAAN (`docker0` di
  `172.17.0.0/16`, plus jaringan bridge kustom yang secara default
  dialokasikan Docker dari `172.18.0.0/16` sampai `172.31.0.0/16`).
  Hanya mengizinkan traffic dari Docker lokal di mesin yang sama, bukan
  membuka port ke jaringan/internet luar.
- **Asimetri keamanan MySQL vs PostgreSQL**: PostgreSQL punya lapisan
  proteksi kedua yang independen dari firewall OS — `pg_hba.conf`
  menolak sumber IP yang tidak cocok di level protokolnya sendiri,
  sebelum autentikasi, jadi tetap aman walau firewall OS tidak aktif.
  Baris `host all all 172.16.0.0/12 md5` ditambahkan ke `pg_hba.conf`
  (dicari via `SHOW hba_file;`, idempotent lewat marker
  `poinhost-docker-access`). MySQL TIDAK punya mekanisme setara — sintaks
  grant `user@host` MySQL adalah pola ala `LIKE`, bukan pencocokan CIDR
  sungguhan — sehingga untuk MySQL, firewall OS yang aktif jadi lapisan
  proteksi utama satu-satunya.
- **Firewall**: fitur ini TIDAK PERNAH memasang atau mengaktifkan
  firewall baru (keputusan sebesar itu di luar cakupan fitur ini) —
  hanya menambah SATU aturan allow sempit (scope `172.16.0.0/12`, per
  port) ke `ufw`/`firewalld` KALAU salah satunya terdeteksi sedang aktif.
  Kalau tidak ada firewall aktif terdeteksi untuk MySQL, bind-address
  tetap diubah sesuai permintaan, tapi hasilnya membawa pesan peringatan
  eksplisit (bukan diam-diam meninggalkan port terbuka tanpa
  pemberitahuan).
- **Dua jalur, satu implementasi**: (1) otomatis dibakukan ke akhir
  `dbInstallScript` — instalasi BARU lewat wizard poinhost langsung siap
  diakses dari Docker tanpa langkah tambahan apa pun, sesuai permintaan
  user; (2) jalur retrofit mandiri (`EnsureDBDockerAccess` /
  `GetDBDockerAccessStatus`, tab Database menampilkan status +
  tombol "Aktifkan akses dari Docker") untuk instalasi yang sudah ada
  SEBELUM fitur ini ditambahkan. Keduanya memanggil fungsi pembangun
  skrip yang sama (`dbDockerAccessApplyScript`), jadi hanya ada satu
  implementasi untuk dijaga, bukan dua yang berpotensi berbeda.

### Bug nyata ditemukan: klik "Aktifkan akses dari Docker" untuk MySQL terkesan tidak ada efek

Dilaporkan user ("ketika saya pilih tidak ada efek apapun") — diinvestigasi
dengan cara paling meyakinkan: instal MariaDB 10.11 SUNGGUHAN (paket resmi
Ubuntu, versi sama yang dipakai poinhost sendiri) di sandbox ini dan
menjalankan skrip yang sebenarnya, bukan cuma membaca kode.

Root cause: SELURUH pemeriksaan admin MySQL di modul ini (`runMySQL` di
`database.go`, dipakai untuk List/Create Database, List/Create User, Set
Grants — DAN pengecekan `bind_address` di `dockeraccess.go`) memakai
`mysql -h 127.0.0.1 -u root` (paksa TCP) ke akun `root@127.0.0.1` yang
sengaja dibuat berpassword kosong saat instalasi. Terbukti di server
sungguhan: perintah ini GAGAL KONSISTEN dengan
`ERROR 1698: Access denied for user 'root'@'localhost'` — perhatikan
identitas yang ditolak adalah `'root'@'localhost'`, BUKAN
`'root'@'127.0.0.1'` yang sebenarnya login. MariaDB/MySQL (dengan
`skip_name_resolve` default OFF) mencocokkan koneksi TCP ke `127.0.0.1`
lewat resolusi `/etc/hosts` (yang di HAMPIR SEMUA distro Linux memetakan
`127.0.0.1` ke `localhost`) dan memilih akun `root@localhost` (auth
`unix_socket`, PASTI menolak koneksi TCP apa pun) — bukan mencoba akun
`root@127.0.0.1` sama sekali, walau password-nya sudah benar (dikonfirmasi
juga dengan password eksplisit, hasilnya identik).

Akibatnya: skrip `dbDockerAccessApplyScript` (tulis config + restart
service) SEBENARNYA BERHASIL mengubah `bind-address` jadi `0.0.0.0` — tapi
`EnsureDBDockerAccess` memanggil `GetDBDockerAccessStatus` untuk membaca
hasilnya balik, dan pembacaan ITU yang gagal diam-diam (`2>/dev/null`),
jadi UI tetap melaporkan "belum bisa connect" walau perubahan sungguhan
sudah terjadi di server — persis keluhan "tidak ada efek apapun". Bug yang
sama membuat SELURUH fitur admin MySQL (bukan cuma Docker access) rawan
gagal dengan cara yang sama, tergantung urutan/timing pembuatan akun
`root@127.0.0.1` saat instalasi.

Perbaikan: ganti SEMUA pemakaian `mysql -h 127.0.0.1 -u root` jadi
`mysql -u root` (unix socket lokal, tanpa `-h` sama sekali) — dijalankan
sebagai root lewat SSH exec, konteks trust yang sama dipakai di seluruh
modul lain, dan MariaDB/MySQL packaging Debian/Ubuntu/RHEL modern SEMUANYA
mengonfigurasi `root@localhost` dengan auth `unix_socket` otomatis saat
instalasi — tidak perlu akun tambahan apa pun. Bootstrap
`CREATE USER 'root'@'127.0.0.1' IDENTIFIED BY ''` di `dbInstallScript`
(`database.go`) dan `mariaDBVersionedInstallScript` (`dbversion.go`)
DIHAPUS sepenuhnya, bukan cuma dibiarkan tidak dipakai — akun root
ber-password kosong yang bisa dijangkau lewat TCP (walau cuma loopback)
adalah beban keamanan yang sebenarnya tidak pernah diperlukan.

Diverifikasi end-to-end di MariaDB 10.11 sungguhan (bukan cuma unit test):
`mysql -u root` bekerja dari kondisi benar-benar bersih (root@127.0.0.1
DIHAPUS dulu, mensimulasikan server yang belum pernah dibootstrap sama
sekali) untuk `SHOW DATABASES`; siklus penuh tulis-config→restart→baca-
ulang status via socket terbukti melaporkan `bind_address` berubah dari
`127.0.0.1` ke `0.0.0.0` dengan benar; dan koneksi TCP sungguhan dari
alamat NON-loopback (mensimulasikan container Docker) dengan user
aplikasi biasa (`host='%'`, bukan root) berhasil connect — membuktikan
mekanisme inti (bind-address + firewall rule) sendiri SELALU bekerja
sebagaimana mestinya; yang rusak murni pembacaan status oleh poinhost
sendiri. Ditambahkan test regresi (`database_test.go`) yang meng-assert
skrip-skrip ini tidak pernah balik memakai `-h 127.0.0.1` atau membuat
akun `root@127.0.0.1`.

## 15. Backup: export/import data lintas perangkat (tanpa akun/server)

Latar belakang: user bertanya soal sign-in Google + sync otomatis antar
perangkat. Itu diskusikan dulu (bukan langsung dibangun) karena berlawanan
dengan keputusan desain §7 (poinhost sengaja tanpa login/server) — begitu
ada akun+sync, kredensial (password SSH, vault MySQL §14) HARUS lewat
suatu tempat di luar mesin lokal, menambah permukaan risiko yang tadinya
sengaja dihindari. Tiga opsi didiskusikan (manual export/import terenkripsi
vs Google Sign-In+Drive milik user vs backend cloud sendiri) — user
memilih opsi pertama: **paling ringan, tanpa infrastruktur baru sama
sekali**, konsisten dengan sikap "tanpa server" yang sudah ada.

### Format: satu file, terenkripsi passphrase, dibawa user sendiri

`internal/core/backup` (`dto.go`/`crypto.go`/`service.go`) membangun satu
`Payload` JSON dari beberapa modul sekaligus (BUKAN dump mentah tabel
SQLite, supaya format stabil walau skema internal berubah):

- **servers** — semua field `Server` + password SSH (`servers.Service.
  SudoPassword`), plus opsional **isi file private key** (base64, HANYA
  kalau user centang eksplisit "Sertakan isi private key" — default off,
  supaya membaca & menyertakan kunci privat SSH ke arsip adalah keputusan
  sadar, bukan otomatis).
- **website_db_credentials** — password diambil dari vault lokal
  (`website.Service.ExportCredentialPassword`, wrapper baru khusus dipakai
  backup, TIDAK diekspos sebagai binding umum lain).
- **website_domain_databases** — tautan domain<->database (§14).
- SENGAJA TIDAK menyertakan `activity_logs` (audit, bisa besar/tak
  relevan dipindah) atau `ui_tabs` (state UI device-specific).

Payload itu dienkripsi jadi satu file biner: `magic || salt || nonce ||
ciphertext(AES-256-GCM)`, kuncinya PBKDF2-HMAC-SHA256 (200rb iterasi) dari
**passphrase yang diingat user** — beda dari kunci vault lokal (§14) yang
acak & spesifik satu mesin: arsip ini sengaja dibawa ke mesin LAIN, jadi
kuncinya harus bisa direproduksi dari sesuatu yang TIDAK disimpan di mesin
mana pun. Tidak ada mekanisme "lupa passphrase" — itu memang harga dari
tidak adanya akun/server yang bisa menyimpan/reset kredensial.

### Import: upsert mempertahankan ID, aman dijalankan berulang

`servers.Service.UpsertFromBackup` (beda dari `Save` biasa yang SELALU
generate ID baru atau meng-update-tanpa-insert) mempertahankan ID dari
arsip — cek `repo.Get(id)` dulu, insert kalau belum ada (kasus normal:
perangkat baru), update kalau sudah (mis. import ulang arsip yang sama,
atau restore ke mesin yang sama) — idempotent. Kredensial database &
tautan domain memakai fungsi upsert yang sudah ada (`SaveDBCredential`,
`LinkDomainDatabase`), jadi tidak ada duplikasi juga. Private key yang
dibawa arsip ditulis ke `~/.poinhost/imported_keys/<serverID>_<nama>`
(permission 0600) — folder terpisah, TIDAK menimpa file kunci apa pun yang
sudah ada di mesin tujuan.

### UI: dialog native, bukan upload/download browser

`ExportBackup`/`ImportBackup` di `app.go` memakai `runtime.SaveFileDialog`/
`OpenFileDialog` (pola sama dengan Download file di Files §10 dan Export
DNS zone §13) — dialog OS asli, file tidak pernah "singgah" di memori JS.
`BackupModal.tsx` (dipicu tombol "⇄ Backup" di header `ServersPage`) punya
2 tab: Export (passphrase + konfirmasi + checkbox sertakan key file) dan
Import (passphrase + ringkasan hasil: jumlah server/kredensial/tautan yang
berhasil masuk).

## 16. Tema: terang/gelap via token CSS, bukan palet indigo bawaan template

Permintaan user: toggle tema terang/gelap (ikon, bukan teks), dan desainnya
jangan terkesan "AI slop" — layout/tipografi/gaya generik yang langsung
kelihatan bukan pilihan sadar. Sebelum ini, seluruh app HANYA punya satu
tema (gelap), warnanya hardcoded sebagai hex literal tersebar di
`App.css` (~17 warna unik dipakai berulang), dan aksennya `#6366f1` —
indigo di atas slate/navy biru dingin, kombinasi yang jadi default hampir
semua dashboard bikinan AI karena itu warna aksen default Tailwind.

### Token CSS, bukan tambal-sulam per-komponen

`frontend/src/theme.css` (baru) mendefinisikan semua warna sebagai custom
property: `:root` = tema terang (nilai dasar), `:root[data-theme='dark']`
menimpanya untuk tema gelap. `App.css` diubah SATU KALI mengganti tiap
hex literal (`#232f45`, `#2b3a52`, dst) dengan `var(--surface-hover)`,
`var(--border)`, dst — bukan menulis ulang tiap file komponen, karena
hampir semua warna ternyata sudah terpusat di satu file itu (komponen
lain pakai className yang menunjuk ke sana, bukan inline style warna).
Teks sekunder (`opacity: 0.5/0.6/dst`, dipakai puluhan kali) SENGAJA tidak
disentuh — itu sudah otomatis theme-aware selama warna dasarnya
(`color: var(--text)`) benar, opacity tinggal mengalikan dari situ.

Satu pengecualian disengaja: `--well-bg`/`--well-fg` (panel terminal,
log stream instalasi) TETAP gelap konstan di kedua tema — konvensi umum
(terminal/log selalu gelap, lihat VS Code dkk), dan sudah cocok dengan
tema JS-level xterm.js sendiri (`TerminalPanel.tsx`/`ContainerExecModal.tsx`
hardcode `#12192a`/`#e5e7eb` di config Terminal-nya sendiri, terpisah dari
CSS — sengaja tidak diubah, xterm.js merender ke canvas, tidak baca
custom property CSS sama sekali).

### Palet: terracotta/tembaga hangat, bukan indigo-di-atas-slate

Aksen diganti dari `#6366f1` (indigo) ke `#b85c3a` (terang) /`#d97c57`
(gelap) — terracotta hangat, dipasangkan dengan netral HANGAT (`#f6f3ed`
kertas di tema terang, `#1a1712` nyaris-hitam hangat di tema gelap),
bukan abu-abu/navy dingin bawaan skeleton lama. Warna semantik (sukses/
bahaya/peringatan) tetap keluarga hijau/merah/kuning yang familiar, cuma
nada per-tema disesuaikan supaya kontras cukup (mis. hijau lebih gelap di
tema terang, lebih terang di tema gelap — bukan satu hex tetap yang
kebetulan kontras di satu tema saja). `COLOR_SWATCHES[0]` (opsi warna
pertama untuk kartu server, `ServerFormModal.tsx`) dan fallback warna tab
(`TabBar.tsx`) memakai `var(--accent)` langsung (bukan hex tetap) supaya
otomatis ikut tema aktif; default warna server baru di backend
(`servers.Service.Save`, migrasi `001_core.sql`) juga diperbarui ke
`#b85c3a` — bukan lagi sisa indigo lama.

### Toggle: Zustand + localStorage, diterapkan sebelum render pertama

`frontend/src/store/theme.ts` — state preferensi TERPISAH dari
`store/tabs.ts` (yang semuanya lewat binding Wails/SQLite): preferensi
tema murni lokal browser, tidak ada alasan disimpan di server/DB. Modul
ini menerapkan `document.documentElement.dataset.theme` SEKALI saat
modul di-import (bukan di dalam `useEffect` komponen manapun) — supaya
atribut `data-theme` sudah benar SEBELUM render pertama React, tidak ada
kedipan "salah tema" sesaat. `ThemeToggle.tsx` (ikon SVG matahari/bulan
polos, BUKAN emoji — konsisten dengan permintaan "jangan AI slop", emoji
sebagai ikon tombol adalah salah satu ciri paling gampang dikenali dari
UI bikinan AI) dipasang di header `ServersPage`, ikon yang tampil
menunjukkan tema TUJUAN kalau diklik (ikon matahari saat gelap = "klik
untuk terang"), pola umum toggle tema.

### Ikon emoji sebagai ikon tombol — sudah diberesin di §17

Waktu perubahan tema ini ditulis, ikon emoji (🐳🔍🔗✖🔑📄⚙🗑▶⏹⌨📊, dst)
masih dipakai luas sebagai ikon tombol di puluhan tempat, sengaja tidak
dibundel diam-diam ke perubahan tema ini karena jauh lebih besar
(menyentuh puluhan file). Sudah dikerjakan sebagai pekerjaan terpisah,
lihat §17.

## 17. Ikon: SVG konsisten (lucide-react), bukan emoji

Permintaan user lanjutan dari §16: ganti semua ikon emoji ke sistem SVG
yang konsisten — emoji sebagai ikon tombol adalah salah satu ciri paling
gampang dikenali dari UI bikinan AI (disebut eksplisit di §16 juga).
Sebelum ini, ~90 pemakaian emoji/simbol tersebar di ~30 file TSX
(`DockerPanel`, `FilesPanel`, `DatabaseManagerPanel`,
`RecreateContainerModal`, dst).

### Library: `lucide-react`, bukan hand-rolled SVG per-ikon

`ThemeToggle.tsx` (§16) sudah punya contoh SVG matahari/bulan tulisan
tangan, tapi menulis ulang ~40 ikon berbeda dengan tangan tidak realistis
dan hasilnya gampang tidak konsisten (stroke-width, viewBox beda-beda).
`lucide-react` ditambahkan sebagai dependency (`frontend/package.json`) —
satu set ikon outline konsisten, tree-shakeable per-komponen yang
di-import.

### Menyatukan simbol yang tadinya tidak konsisten

Proses penggantian juga jadi kesempatan membenahi inkonsistensi lama yang
tersembunyi di balik variasi glyph: tiga simbol "refresh" berbeda
(`↻`/`⟲`/`⟳`) dipakai di file berbeda untuk aksi yang SAMA — disatukan
jadi satu `RotateCw`. Tiga simbol "tutup" berbeda (`×`/`✕`/`✖`) juga
dipakai tidak konsisten antar modal — disatukan jadi satu `X`. Satu typo
ditemukan di `WebsitePanel.tsx`: tombol "Tambah subdomain" memakai
karakter fullwidth-plus (U+FF0B, `＋`) alih-alih `+` biasa yang dipakai
di tempat lain — diganti `Plus`. Glyph `⬆` dipakai untuk DUA arti berbeda
tergantung konteks (navigasi ke folder induk di `FilesPanel` vs upload
file) — dibedakan jadi `ArrowUp` dan `Upload` sesuai konteks masing-
masing, bukan disamakan.

Satu aturan layout ditambahkan sekali di `App.css` (bukan style per-
instance di tiap tombol):

```css
button svg {
  vertical-align: -2px;
  flex-shrink: 0;
}
```

### Bug yang ditemukan saat verifikasi visual: `[hidden]` kalah cascade

Verifikasi dilakukan dengan Playwright (stub `window.go.main.App` dan
`window.runtime` karena app ini Wails asli, tidak ada binding itu di
browser polos) untuk screenshot tiap panel di kedua tema. Saat berpindah
modul dalam satu tab server (mis. Files → Docker), panel yang sudah
pernah dikunjungi tetap kelihatan (numpuk di bawah panel aktif) walau
atribut HTML `hidden` sudah benar terpasang di DOM.

Penyebabnya: `.workspace__module { display: flex; ... }` adalah aturan
CSS ber-origin "author" (ditulis developer), sedangkan styling bawaan
`[hidden]` (`display: none`) ber-origin "user-agent" (bawaan browser) —
dalam cascade CSS, aturan author MENANG atas aturan user-agent pada
specificity yang setara, terlepas dari urutan penulisan. Jadi
`display: flex` di `.workspace__module` diam-diam mengalahkan `hidden`
bawaan browser. Dikonfirmasi lewat `getComputedStyle` sebelum (`display:
"flex"` walau `hidden: true`) dan sesudah (`display: "none"`) perbaikan.
Bug ini sudah ada sebelum sesi ini, tidak berkaitan dengan tema/ikon,
ditemukan murni sebagai efek samping verifikasi visual. Perbaikan:

```css
.workspace__module[hidden] {
  display: none;
}
```

## 18. Perbaikan UX round berikutnya: subdomain warisan, spinner, scroll modal, accordion domain, split tab database

Lima laporan user dalam satu sesi, dikerjakan sebagai satu batch karena
saling terkait (beberapa perubahan struktural yang sama menyentuh beberapa
laporan sekaligus).

### Subdomain warisan homepoin tampil sebagai website sendiri

Bug lanjutan dari §12: domain lama buatan homepoin sudah bisa dikenali
sejak perbaikan sebelumnya, TAPI subdomain-nya (mis. `blog.example.com`)
masih tampil sebagai baris rata di daftar, bukan terindentasi di bawah
`example.com`. Sebabnya: marker metadata subdomain homepoin
(`# homepoin-managed subdomain: ...`) TIDAK PERNAH menyertakan field
`parent=` (beda dari marker poinhost sendiri yang selalu menulisnya) —
`IsSubdomain` terbaca benar (`true`), tapi `Parent` kosong, dan
`WebsitePanel.tsx` mengelompokkan baris berdasarkan `Parent` yang cocok
dengan `domain` induknya.

Diperbaiki dengan `inferMissingParents` (`domains.go`) — pencocokan
akhiran nama domain terhadap domain lain di server yang sama, port
langsung dari algoritma `findParentDomain` milik homepoin sendiri (riset
kode homepoin lagi, sama seperti riset marker sebelumnya). HANYA berlaku
untuk entri yang `IsSubdomain` SUDAH true dari marker vhost-nya sendiri
tapi `Parent` masih kosong — domain top-level (`IsSubdomain=false`) sama
sekali tidak disentuh, supaya tidak ada tebak-tebakan nama yang salah
mengelompokkan dua domain top-level yang kebetulan namanya bersufiks sama.

### Spinner loading di tombol eksekusi

Sebelumnya banyak tombol aksi (simpan, hapus, install, dst) cuma
`disabled` + ganti teks ("Menyimpan…") tanpa indikator visual berputar —
gampang terlihat seperti tombolnya tidak merespons klik sama sekali.
Ditambahkan `.spinner` (CSS murni, border-based, `@keyframes spin`) di
`App.css`, dipasang di depan label tombol yang sedang busy:
`{busy && <span className="spinner" />} Label` — pola dipakai konsisten
di ~25 file (modal generik seperti PromptModal/CompressModal/ChmodModal,
tab-tab domain, DockerPanel per-container action, dst). Untuk tombol
ikon-only yang state busy-nya per-baris (`busyId === c.id`, bukan flag
global), ikon-nya DIGANTI jadi spinner selama busy (bukan ditambah di
depan) supaya tidak mengubah lebar tombol.

### Modal edit environment Docker tidak bisa di-scroll

`.modal-card--editor` (dipakai EditFileModal DAN RecreateContainerModal)
punya tinggi tetap (`80vh`) + `overflow: hidden` di level kartu — didesain
untuk CodeMirror (`EditFileModal`) yang mengisi `height: 100%` dan urus
scroll internal sendiri. `RecreateContainerModal` numpang class yang sama
tapi isinya form biasa (bisa berapa pun env var/port/volume, TIDAK
auto-scroll sendiri) — begitu env var lebih dari muat satu layar, sisanya
(termasuk section Port/Volume/Memory dan tombol "Terapkan") ke-clip oleh
`overflow: hidden`, tidak ada cara mencapainya. Diperbaiki dengan
`overflow-y: auto` di `.modal-card__body--editor` — tidak berdampak ke
CodeMirror (yang selalu presisi 100% tinggi wadahnya, tidak pernah
overflow wadah itu sendiri), tapi mengaktifkan scroll untuk konten yang
memang lebih tinggi dari box-nya.

### Detail domain: accordion inline, bukan modal 9-tab

Permintaan eksplisit: modal dengan 9 tab (PHP/SSL/Files/Logs/Proxy/
Database/Cron/DNS/SFTP) "kurang baik secara UI". `DomainDetailModal.tsx`
(modal + `<div className="subnav">` TANPA styling khusus sama sekali —
salah satu penyebab kesan kurang baik itu) dihapus, diganti
`DomainDetailPanel.tsx` — dirender INLINE di `WebsitePanel.tsx`
menggantikan tabel daftar domain (drill-down, bukan overlay): klik
"Kelola" pada satu domain mengganti tampilan panel jadi header (tombol
kembali + nama domain + badge status/PHP/SSL + aksi cepat
tambah-subdomain/toggle-aktif/hapus) plus `<div className="accordion">`
berisi satu `AccordionSection` collapse per fitur.

Konten tiap section BARU di-mount saat PERTAMA kali dibuka (`mounted`
state di `AccordionSection`, ditoggle visible via atribut `hidden` bukan
unmount) — kalau semua 9 section dimuat serentak begitu domain dipilih,
itu ~9 round-trip SSH sekaligus (tiap tab motret status sendiri di
`useEffect`), padahal user mungkin cuma mau lihat satu. Sekali dibuka,
tetap ter-mount supaya tutup-buka berikutnya tidak fetch ulang.

### Explorer MySQL/PostgreSQL: tab "Users & Akses" vs "Data", bukan campur + modal

`DatabaseManagerPanel.tsx` sebelumnya mencampur DUA concern berbeda di
satu scroll panjang (status/install, akses Docker, provisioning
database/user/grants, DAN kredensial-untuk-Explore) — lalu tombol
"Explore" (browse tabel/baris) membuka modal terpisah lagi
(`MySQLExplorerModal`/`PGExplorerModal`) di atas semuanya. User
eksplisit minta dipisah: satu tempat urus SIAPA yang boleh akses, satu
tempat urus BROWSE isinya.

Diselesaikan dengan segmented tab baru di dalam panel yang sama ("Users &
Akses" / "Data") — BUKAN modal lagi untuk sisi Data: `MySQLExplorerModal`/
`PGExplorerModal` dihapus, isinya dipindah ke `MySQLExplorer.tsx`/
`PGExplorer.tsx` (komponen sama persis, cuma tanpa wrapper
`modal-overlay`/`modal-card`) yang dirender langsung sebagai konten tab
"Data". Tab ini menampilkan selector "Browse sebagai: <kredensial>" (dari
`ListWebsiteDBCredentials`, bukan cuma satu kredensial tetap) — klik
"Explore" di baris user pada tab "Users & Akses" langsung lompat ke tab
"Data" dengan kredensial itu terpilih (`openExplore`), bukan lagi
membuka modal baru. `.db-explorer` yang tadinya mewarisi tinggi dari
`.modal-card--explorer` (kelas ini jadi tidak terpakai, dihapus) sekarang
punya tinggi eksplisit sendiri (`60vh`) supaya tetap punya area scroll
yang jelas walau dirender inline di dalam accordion/tab, bukan modal.

## 19. Yang BELUM di-porting di skeleton ini (roadmap)

Skeleton ini sengaja dibatasi ke fondasi (sshpool + session/tab + 1 modul
contoh) supaya bisa direview dulu sebelum porting besar-besaran. Belum ada:

- ~~Enkripsi kredensial SSH server saat disimpan~~ — **SUDAH SELESAI**:
  password SSH server sekarang dipindah ke `secrets.Vault` yang sama dipakai
  kredensial database, plus migrasi otomatis satu-kali untuk instalasi lama
  yang masih punya plaintext di kolom `password_enc` (lihat subsection baru
  di §6).
- **Jobs & event bus untuk operasi jangka panjang** (mis. instalasi paket,
  migrasi file/DB/Docker) — polanya sudah ada (`runtime.EventsEmit`/
  `EventsOn`, dipakai server status di §8 dan terminal di §9). ~~migrasi
  file/DB/Docker belum di-porting~~ — **SUDAH SELESAI**: migrasi file
  antar server (`internal/modules/filexfer`), migrasi container Docker
  antar server (`internal/modules/dockerxfer`, §21), dan migrasi database
  MySQL/PostgreSQL antar server (`internal/modules/dbxfer`, §22) semuanya
  sudah ada — ketiga sub-menu tab "Migrasi" kini terisi penuh.
- **activitylog** (audit trail tiap operasi).
- **`access`/sudo untuk MODUL LAIN** — elevasi ke user/root lewat sudo
  sekarang ada di tiga modul: `files` (§10, `asUser` penuh — List/Mkdir/
  Rename/Delete/Compress/Extract/Read/Write/Chmod/Upload/Download/Copy/
  Search), `docker` (§11, lebih sederhana — cuma naik ke root) dan
  `website` (§12, mirip docker — selalu butuh root, tidak ada "jalankan
  sebagai user lain"). Ketiganya masih implementasi sendiri-sendiri
  (`fileAccess`/`dockerAccess`/`websiteAccess`) — belum ada abstraksi
  lintas-modul untuk "user efektif", sengaja ditunda sampai polanya makin
  jelas dari modul keempat (`dbmanager`, dst) supaya tidak salah abstraksi
  lebih awal.
- Files: copy & search sudah ada (§10). Yang masih sengaja belum:
  editor gambar/preview biner, drag-drop upload dari file explorer OS.
- Docker (§11): Containers & Networks sudah penuh. Images/Volumes/Compose
  masih placeholder — **sama seperti di homepoin sendiri**, bukan utang
  porting sepihak poinhost (lihat §11).
- Website (§12-§13): domain/vhost Nginx + PHP-FPM + SSL + seluruh 7 tab
  per-domain (Files/Logs/Proxy/DNS/SFTP/Cron/Database) sudah penuh — menu
  Website homepoin sudah selesai di-porting semuanya.
- ~~dbmanager browser server-wide (mysql/pg, tabel/baris/query)~~ —
  **SUDAH SELESAI**: Explore (§14) sekarang punya modul "Database"
  top-level per server (`DatabaseManagerPanel.tsx`), tidak lagi terkunci
  di bawah tab per-domain Website. Masih pakai bind lokal `127.0.0.1:3306`/
  `5432` (instalasi standar di server yang sama) — koneksi ke host/port
  arbitrer (mis. instance di port non-default) belum didukung, tapi itu
  di luar gap yang diminta ("server-wide", bukan "host mana pun").
- Modul lain: services, migration, reverse-proxy port-forward server-wide,
  email/ftp server-wide. Semua akan mengikuti pola
  `servers/`/`terminal/`/`files/`/`docker/`/`website/` di atas satu per
  satu.
- **Split-pane multi-terminal per tab** (>1 sesi shell dalam satu tab) —
  fondasinya sudah ada di backend (`TerminalRegistry` & `terminal.Service`
  sudah mendukung N sesi per tab, lihat §9), yang belum ada cuma UI-nya
  (`ServerWorkspace`/`TerminalPanel` masih 1 sesi per tab).
- Restore tab saat startup sudah tersimpan (`ui_tabs`), tapi UI belum
  menampilkan indikator "reconnecting" per tab saat restore — perlu
  ditambah saat modul overview/monitoring di-porting.

## 20. Menjalankan (development)

Butuh dependency native Wails (Linux: `libwebkit2gtk`, `libgtk-3-dev`,
`pkg-config`, `build-essential`; lihat `wails doctor`). Sandbox CI/dev
container ini tidak punya display + `libwebkit`, jadi hanya `go build ./...`
dan `npm run build` (frontend) yang diverifikasi di sini — **belum pernah
dijalankan interaktif**. Setelah dependency native terpasang di mesin dev:

```bash
wails dev      # hot-reload, buka window otomatis
wails build    # binary rilis per-platform (Windows/macOS/Linux)
```

Setelah mengubah signature method di `App` (app.go), regenerate binding TS:

```bash
wails generate module
```

## 21. Migrasi: file antar server + container Docker antar server

Tab "Migrasi" (`MigrationWorkspace.tsx`) punya tiga sub-modul:
`migration-files` (selesai), `migration-docker` (selesai, dibahas di sini),
`migration-database` (masih placeholder). Keduanya yang selesai memakai
mekanisme dasar yang sama — **tar disalurkan lewat `io.Pipe` di memori
proses ini, dua sesi SSH dedicated (bukan slot shared), tidak pernah
menyentuh disk komputer yang menjalankan poinhost** — tapi dianalisis dan
dibangun terpisah karena unitnya beda: file adalah kumpulan path bebas,
Docker adalah satu container utuh dengan konfigurasi yang harus dibaca dan
dibuat ulang, bukan sekadar disalin.

### Migrasi file (`internal/modules/filexfer`) — fondasi yang dipakai bersama

`dto.go`/`job.go`/`engine.go`/`service.go`/`paths.go`. Pola intinya, yang
juga jadi pola untuk migrasi Docker di bawah:

- **Satu job aktif per waktu** (`Service.activeJob`): sengaja, bukan
  keterbatasan yang tidak disadari — ini aplikasi desktop satu operator, dua
  transfer besar bersamaan cuma berebut bandwidth yang sama sambil membuat
  "apa yang sedang terjadi" ambigu di UI.
- **Progress lewat satu callback emitter** (`Service.emit`, disambungkan
  `app.go` ke `runtime.EventsEmit(ctx, "xfer:progress:"+jobId, ...)`) —
  BUKAN fan-out ala WebSocket seperti homepoin, karena di aplikasi desktop
  cuma ada satu frontend yang mendengarkan, jadi fan-out tidak perlu.
- **Koneksi SSH dedicated** (`sshpool.Pool.OpenDedicated`/`CloseDedicated`),
  bukan slot shared: transfer bisa berjam-jam, memakai slot shared akan
  membuat panel lain ke server yang sama ikut tersendat.
- **`countingReader`**: progress dihitung dari byte yang BENAR-BENAR lewat
  pipe ke tujuan, bukan tebakan di sisi sumber.
- Sesi tujuan (`tar x`) selalu di-`Start()` SEBELUM sesi sumber (`tar c`) —
  urutan ini mencegah deadlock (tar sumber memblokir di write kalau belum
  ada yang membaca).

### Migrasi Docker (`internal/modules/dockerxfer` + `internal/modules/docker/migration.go`)

Diminta eksplisit: tiru mekanisme homepoin, tapi analisis dulu apa yang
kurang di sana, lalu terapkan versi yang lebih baik di poinhost — bukan
port apa adanya. Audit homepoin (`internal/modules/migration/dockerxfer`,
`dockerjob`, `service_docker.go` + `internal/modules/servers/docker/
service_migration.go`) menemukan mekanisme dasarnya solid (pola yang sama
dengan §di atas: dual SSH session + `io.Pipe`, tanpa staging ke disk) tapi
dengan gap nyata, yang masing-masing punya perbaikan eksplisit di poinhost:

| Gap di homepoin | Perbaikan di poinhost |
|---|---|
| Opsi `migrateVolumes`/`stopSource`/`startAfterCreate` di request **diabaikan total** oleh backend (hardcoded `true` semua) — UI menjanjikan kontrol yang tidak pernah benar-benar ada (juga dicatat sebagai gap diketahui di MEMORY.md homepoin sendiri). | **Tidak ada toggle sama sekali** untuk hal-hal itu — bukan toggle baru yang juga berisiko jadi dead option, tapi keputusan desain tetap yang didokumentasikan: source SELALU dihentikan dulu (konsistensi data) dan SENGAJA tidak dinyalakan lagi otomatis (dua salinan container aktif menulis ke "sumber kebenaran" yang beda akan diam-diam bercabang datanya) — lihat `service.go` `run()`. Semua opsi yang memang ditampilkan ke user (server tujuan, kompresi) benar-benar dipakai. |
| Volume/bind data SELALU ditransfer tanpa kompresi (`tar cf -`), TAPI image SELALU di-gzip (`docker save \| gzip`) — tidak konsisten dalam file yang sama, tanpa opsi menyamakannya. | Satu flag `Compress` dipakai KONSISTEN untuk mount ATAUPUN image (`runOneMount`/`runImage` di `engine.go`) — nyala berarti nyala di semuanya, mati berarti mati di semuanya. |
| Named volume difilter habis dari `ContainerInspectResponse.Volumes` di parser inspect poinhost sendiri (bug yang ditemukan saat riset ini, `!EqualFold(Type,"bind") { continue }`) — kalau tidak diperbaiki dulu, migrasi Docker apa pun di poinhost akan diam-diam menjatuhkan data di named volume, pola penyimpanan paling umum untuk data yang harus bertahan. | Diperbaiki DULU sebagai prasyarat (`docker/config.go` `parseVolumes`, lihat §11 dan `volumes_test.go`) sebelum menulis satu baris pun modul migrasi — `ContainerInspectResponse.Volumes` dan editor Recreate satu-server ikut mendapat manfaatnya juga, bukan cuma jalur migrasi. |
| Hanya network UTAMA (`NetworkMode`) yang dibawa ke container hasil migrasi — container yang tersambung ke >1 custom network kehilangan semua yang lain TANPA peringatan. | `MigrationBlueprint.ExtraNetworks` (`docker/migration.go` `attachedNetworks`) menangkap SEMUA network yang diikuti container, bukan cuma yang utama; `CreateFromBlueprint` menyambungkan tiap network tambahan lewat `docker network connect --alias` setelah `docker create`, karena `docker create` sendiri cuma bisa menyambung satu network. |
| Destination-only preflight: Docker engine SUMBER tidak pernah dicek — kalau mati, transfer baru gagal di detik pertama dengan pesan yang tidak jelas asalnya. | `Service.preflight` (`dockerxfer/service.go`) mengecek `DetectEngineStatus` di KEDUA sisi sebelum satu byte pun dipindah, plus cek nama container di tujuan (gagal cepat dengan pesan jelas, bukan gagal di tengah `docker create`). |
| `EnsureNamedVolume`/mkdir bind tujuan bersifat idempotent lalu data DIGABUNG/DITIMPA ke isi yang sudah ada TANPA pemeriksaan atau pemberitahuan apa pun — re-run migrasi atau dua sumber berbeda memakai nama volume yang sama bisa diam-diam mencampur data lama+baru. | `mountHasExistingData` (`engine.go`) mengecek isi tujuan SEBELUM transfer per mount; kalau tidak kosong, item itu dapat `Warning` yang tampil di UI ("Tujuan sudah berisi data — akan digabung, bukan ditimpa bersih") — perilaku merge-nya SAMA (tidak ada mode "wipe tujuan dulu" di versi ini), tapi sekarang TERLIHAT sebelum terjadi, bukan diam-diam. |
| **Tidak ada verifikasi pasca-migrasi sama sekali** — "sukses" murni berarti perintah SSH tidak keluar exit code error; tidak ada pengecekan container beneran menyala atau datanya utuh. | Verifikasi RINGAN (keputusan yang dikunci eksplisit bareng user, bukan checksum penuh yang mahal untuk volume besar): `ContainerIsRunning` di tujuan (`docker/migration.go`) + jumlah file & ukuran total per mount dibandingkan sumber vs tujuan (`verify.go` `verifyMount`) — cukup menangkap kasus nyata seperti "tar terputus di tengah" atau "sebagian file tidak ke-tar karena permission", ditampilkan per item (match/mismatch/tidak-diverifikasi) di `Progress.Items[].Verify`. |
| **Nol test otomatis** untuk seluruh jalur migrasi Docker (`dockerxfer/`, `dockerjob/`, `service_migration.go` di homepoin semuanya tanpa test). | `docker/migration_test.go` (konversi blueprint↔template, tangkap network tambahan) + `docker/volumes_test.go` (parsing named volume) + `dockerxfer/dockerxfer_test.go` (label item, parsing statistik verifikasi, urutan item) — mengikuti kebiasaan proyek ini menulis test untuk setiap fitur backend baru. |

**Sengaja DIPERTAHANKAN sama seperti homepoin** (bukan kelalaian — batas
cakupan yang dikunci eksplisit bareng user sebelum implementasi):
copy-only, tidak ada opsi "pindahkan lalu hapus sumber"; satu container per
job (tidak ada migrasi stack/compose sekaligus); writable layer container,
CPU/ulimit/device/cap-add/health-check/logging-driver TIDAK ikut
dimigrasikan; network custom dibuat ulang dengan driver/subnet default
Docker, bukan menyalin subnet/IPAM/IP statis sumber persis; tidak ada
resume/retry-partial kalau transfer terputus di tengah (retry mengulang
dari awal). Semua ini secara eksplisit diungkap ke user lewat kotak info di
`DockerMigrationPanel.tsx` (`migration.docker.notice`) sebelum migrasi
dimulai — bukan sesuatu yang baru ketahuan sesudahnya.

**Alur satu job** (`dockerxfer.Service.run`): inspect (`docker.Service.
InspectMigrationBlueprint`, satu-satunya titik yang boleh menyentuh
`createTemplate`/`inspectToDetail` internal paket `docker` — lihat
`docker/migration.go`) → preflight → stop container sumber → scan ukuran
tiap mount → salin tiap mount (bind: tar langsung atas path host, sama
seperti filexfer; named volume: tar lewat container bantu `alpine`
me-mount volume itu ke `/dxfer_from`/`/dxfer_to`, karena lokasi filesystem
sebenarnya sebuah named volume di host adalah detail internal Docker yang
tidak boleh diasumsikan — lihat `docker.IsNamedVolume`) → salin image
(`docker pull` langsung kalau image punya `RepoDigests`/ditarik dari
registry, fallback `docker save|gzip` → `gunzip|docker load` lewat pipe
yang sama; dilewati total kalau image sudah ada di tujuan) →
`CreateFromBlueprint` (create + connect network tambahan + start) →
verifikasi ringan. Item mount dan item image digabung dalam SATU daftar
progress (`Job.items`), bukan dua progress bar terpisah "Stage 1/Stage 2"
seperti homepoin — pola yang sama dengan daftar path di filexfer, byte
mount dan byte image digabung jadi satu persentase tapi status per-item
tetap terlihat baris demi baris.

Binding Wails: `DockerXferStart`/`DockerXferStatus`/`DockerXferCancel`
(`app.go`), event `dockerxfer:progress:<jobId>` — daftar container sumber
memakai `ListDockerContainers` yang sudah ada (§11), tidak ada binding
baru untuk itu. Frontend: `ContainerPicker.tsx` (padanan `RemoteBrowser`
tapi memilih satu container, bukan menjelajah folder) + `DockerMigration
Panel.tsx`, mengikuti kelas CSS `.mig-panel*`/`.mig-browser*` yang sama
dipakai `FileTransferPanel`.

## 22. Migrasi Database (`internal/modules/dbxfer`) — MySQL & PostgreSQL antar server

Diminta eksplisit dengan pola yang sama seperti §21: tiru mekanisme
migrasi database homepoin (`internal/modules/dbxfer` di sana — dump/
restore lewat pipe, BUKAN download ke lokal dulu), tapi analisis dulu apa
yang kurang, lalu terapkan versi yang lebih baik di poinhost.

### Analisis homepoin: solid di mekanisme intinya, dengan gap konkret

Audit homepoin (`internal/modules/dbxfer/engine.go`, `engine_pg.go`,
`service.go`, `job.go`, plus modul lama `mysqlxfer` yang TERPISAH dan
tidak disentuh) menemukan mekanisme dasarnya sudah baik — live pipe dua
sesi SSH lewat `transfer.SidePool`, `bash -lc 'set -o pipefail; ...'`
supaya `mysqldump | gzip` yang gagal di tengah tidak lolos sebagai sukses,
guard "stream kosong" (byte counter nol = gagal), deteksi dukungan
`--set-gtid-purged` sebelum dipakai (MariaDB lama tidak kenal flag ini),
dan worker pool (bukan sekadar loop) untuk migrasi banyak database
sekaligus dalam satu job. Semua ini dipertahankan di poinhost. Gap
konkret yang ditemukan, dan perbaikannya:

| Gap di homepoin | Perbaikan di poinhost |
|---|---|
| Dump/restore berjalan sebagai USER APLIKASI tertentu — perlu resolve kredensial per username+host, menulis file `.cnf`/`.pgpass` SEMENTARA di kedua server (cleanup best-effort, bisa gagal diam-diam kalau sesi mati di tengah), dan men-strip `DEFINER=user@host` dari hasil dump karena restore sebagai user biasa butuh privilege SUPER/SET_USER_ID untuk rutin/trigger/event bawaan root. | Modul Database poinhost yang sudah ada (§website `database.go`) SELALU jalan sebagai root — MySQL lewat unix socket, PostgreSQL lewat `sudo -u postgres` — TANPA password apa pun (lihat catatan `runMySQL`). Dump/restore migrasi mengikuti akses yang SAMA (`website.Service.WrapCommand`, `internal/modules/website/migration.go`): tidak ada kredensial yang ditulis ke server mana pun, dan **DEFINER stripping jadi tidak perlu sama sekali** — root/postgres sudah punya privilege yang dibutuhkan definer manapun. |
| PostgreSQL dump TIDAK memakai `--clean --if-exists` (MySQL punya `--add-drop-table`, jadi restore ulang aman menimpa tabel bernama sama) — restore ulang ke database tujuan yang sudah pernah diisi GAGAL TOTAL di objek pertama yang bentrok ("relation already exists"), karena `ON_ERROR_STOP=1` menghentikan seluruh restore di kegagalan pertama. Asimetri ini tampak tidak disengaja, tidak ada komentar/test yang mengakuinya. | `buildDumpCmd` PostgreSQL (`dbxfer/engine.go`) menambahkan `--clean --if-exists`, menyamakan perilakunya dengan MySQL: menimpa tabel bernama sama, tidak menyentuh tabel lain di tujuan. |
| Tidak ada preflight status engine di KEDUA sisi — database mati di sumber baru ketahuan saat dump sungguhan gagal di tengah jalan, dengan pesan yang tidak langsung jelas asalnya. | `Service.preflight` (`dbxfer/service.go`) mengecek `website.Service.DBStatus` di sumber DAN tujuan sebelum satu byte pun dipindah — pola yang sama dipakai migrasi Docker (§21). |
| Kolisi database tujuan tidak pernah diperiksa/diberitahu — restore langsung menimpa tabel bernama sama tanpa peringatan apa pun ke user. | `DBDatabaseExists` (`website/migration.go`) dicek sebelum restore; kalau tujuan sudah ada, item itu dapat `Warning` yang tampil di UI ("Database tujuan sudah ada — tabel bernama sama akan ditimpa") — perilaku overwrite-nya SAMA, tapi sekarang terlihat sebelum terjadi. |
| Verifikasi cuma menaksir dari katalog SISI TUJUAN saja (`information_schema.tables`/`pg_class.reltuples` — taksiran yang bisa selisih 40-50% untuk InnoDB), TIDAK PERNAH membandingkan ke sumber — jadi "verifikasi" di homepoin sebenarnya cuma "database tujuan tidak kosong", bukan "data yang sampai memang cocok". Database yang isinya cuma VIEW pula salah ditandai "0 tabel = gagal" (filter `BASE TABLE` doang), dan tabel di schema PostgreSQL custom (bukan `public`) tidak pernah terhitung sama sekali walau tetap ikut ter-dump/ter-restore. | `verifyDatabase`/`compareRowCounts` (`dbxfer/verify.go`) menghitung `SELECT COUNT(*)` NYATA (bukan taksiran) per tabel di KEDUA sisi lalu membandingkannya — keputusan yang dikunci eksplisit bareng user, sadar konsekuensinya (scan penuh tiap tabel, lebih lambat untuk tabel sangat besar, tapi hasilnya benar-benar bisa dipercaya). `DBTableNames` (`website/migration.go`) menghitung SEMUA jenis relasi (termasuk VIEW) dan SEMUA schema non-sistem PostgreSQL, bukan cuma `public`. |
| Tidak ada status "partial success" — job dengan sebagian database gagal tetap dilaporkan `status: done` di level atas; konsumen yang hanya membaca field status (bukan meng-iterasi `items[]`) salah mengira migrasi sukses total. | `StatusPartial` eksplisit (`dbxfer/dto.go`) — `done==0` → gagal, `failed==0` → selesai, selain itu → sebagian selesai. Ketiganya dibedakan, bukan digabung jadi "done". |
| Zero test untuk mekanisme intinya (`runOneItem`/`runOnePGItem`, guard stream kosong, `verifyItem`, helper quoting shell/SQL) — yang dites cuma "apakah command builder menghasilkan substring yang benar", bukan perilaku sungguhan. | `dbxfer/engine_test.go` (pipefail, gtid-purged opsional, quoting shell aman, `--clean --if-exists` PostgreSQL) + `dbxfer/dbxfer_test.go` (`compareRowCounts` match/mismatch/banyak-mismatch, status partial, urutan item) + `website/migration_test.go` — mengikuti kebiasaan proyek ini menulis test untuk setiap fitur backend baru. |

**Sengaja DIPERTAHANKAN sama seperti homepoin** (bukan kelalaian — batas
cakupan yang jelas): tidak ada migrasi lintas engine (MySQL ↔ PostgreSQL —
butuh penerjemah skema, di luar cakupan); dump/restore selalu mencakup
skema+data+rutin/trigger/event sekaligus, tidak ada mode schema-only/
data-only; tidak ada resume untuk job yang gagal/dibatalkan (retry
mengulang dari awal — untuk MySQL ini aman berkat `--add-drop-table`,
untuk PostgreSQL sekarang JUGA aman berkat `--clean --if-exists` yang
baru ditambahkan); satu job aktif per waktu KHUSUS untuk dbxfer sendiri
(tidak dibagi dengan filexfer/dockerxfer — migrasi database dan migrasi
Docker boleh berjalan bersamaan, keduanya independen, sama seperti empat
guard independen yang ditemukan di homepoin sendiri antara fitur-fitur
migrasinya). Semua ini diungkap ke user lewat kotak info di
`DBMigrationPanel.tsx` (`migration.db.notice`) sebelum migrasi dimulai.

**Beda dari migrasi Docker (§21) yang sengaja satu container per job**:
migrasi database MENDUKUNG banyak database sekaligus dalam satu job,
jalan paralel lewat worker pool (`workers = 2`, `dbxfer/service.go`) —
keputusan yang dikunci ulang secara sadar bareng user, bukan sekadar
meniru homepoin, karena migrasi beberapa database aplikasi dalam satu
kali jalan adalah kasus nyata yang berguna. Tiap database juga bisa
memilih SUBSET tabel saja (bukan wajib seluruh database) lewat
`DatabasePicker.tsx` — expand per baris database menampilkan checklist
tabelnya (dimuat lewat `DbXferListTables`, lazy saat baris itu dibuka).

**Alur satu job** (`dbxfer.Service.run`): preflight (engine aktif di
kedua sisi) → per database: selesaikan "semua tabel" jadi daftar nyata
(`DBTableNames`) + taksir ukuran untuk progress bar (`DBEstimateSize` —
taksiran katalog, HANYA untuk mengisi persentase, bukan untuk verifikasi)
+ peringatan kolisi tujuan → worker pool: pastikan database tujuan ada
(`DBCreateDatabase`, sudah dipakai fitur provisioning database biasa) →
dump/restore lewat pipe (`runOneItem`) → guard stream kosong → tahap
verifikasi terpisah (`StatusVerifying`) membandingkan COUNT(*) nyata
sumber vs tujuan per tabel.

Binding Wails: `DbXferStart`/`DbXferStatus`/`DbXferCancel`/
`DbXferListTables` (`app.go`), event `dbxfer:progress:<jobId>` — daftar
database sumber memakai `ListWebsiteDatabases` yang sudah ada (§14),
tidak ada binding baru untuk itu. Frontend: `DatabasePicker.tsx` (server +
engine + checklist database multi-select + expand tabel per baris) +
`DBMigrationPanel.tsx`, kelas CSS `.mig-panel*`/`.mig-browser*` yang sama
dipakai kedua fitur migrasi lainnya, plus beberapa kelas `.mig-db-picker__*`
kecil untuk expand tabel.
