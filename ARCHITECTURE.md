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
│   │   ├── config/             # parameter runtime (trimmed — lihat §6 roadmap)
│   │   ├── database/           # buka SQLite WAL, migration runner
│   │   └── sshpool/            # pool koneksi (lihat §3), executor, sftp, known_hosts
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
│       ├── terminal/             # PTY interaktif (lihat §9)
│       │   └── service.go        # bungkus koneksi dedicated jadi shell PTY
│       └── files/                 # file manager SFTP (lihat §10)
│           ├── dto.go
│           ├── pathutil.go        # NormalizePath/JoinPath/dst (traversal-safe)
│           ├── archive_cmd.go     # command zip/tar.gz + fallback python3
│           └── service.go         # list/mkdir/upload/download/delete/compress
│
├── migrations/
│   └── 001_core.sql             # servers, app_settings, activity_logs, ui_tabs
│
└── frontend/
    ├── src/
    │   ├── store/tabs.ts         # Zustand: servers + tabs + statuses + activeTabId
    │   ├── features/
    │   │   ├── servers/          # ServersPage, ServerFormModal, ServerWorkspace,
    │   │   │                     # OverviewPanel, TerminalPanel (xterm.js),
    │   │   │                     # FilesPanel, StatusDot, PromptModal, CompressModal
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
arsip, tujuan default = direktori yang sedang dibuka).

Konsisten dengan §3: operasi file lewat `sshpool.SFTPClient`/`Executor.Exec`
yang keduanya jalan di slot `SlotShared` — pindah dari tab Files ke tab
Terminal (atau ke server lain) TIDAK memicu dial SSH baru, koneksi shared
yang sama (sudah dipanaskan sejak startup) dipakai bersama.

**Sengaja BELUM ada** (dipersempit dari homepoin untuk porting awal ini):
`chmod`, `copy`, `search` file, dan elevasi `asUser`/sudo (homepoin punya
modul `access` terpisah untuk switch user efektif — belum di-porting ke
poinhost sama sekali, lihat §11). Semua operasi file saat ini jalan sebagai
user SSH yang login, apa adanya.

Frontend (`FilesPanel.tsx`) di-keep-alive per tab sama seperti Overview &
Terminal (§9) — direktori yang sedang dibuka & seleksi file tidak hilang
saat pindah ke modul lain lalu balik lagi. Modal kecil (`PromptModal`,
`CompressModal`) dipakai untuk nama folder/file/rename/kompres, bukan
`window.prompt()` (dukungannya tidak konsisten lintas WebView platform).
Interaksi per-baris pakai tombol yang muncul saat hover (pola yang sama
dengan `ServersPage`), bukan context-menu klik-kanan kustom — pilihan
sadar untuk mengurangi kompleksitas UI di porting awal ini.

## 11. Yang BELUM di-porting di skeleton ini (roadmap)

Skeleton ini sengaja dibatasi ke fondasi (sshpool + session/tab + 1 modul
contoh) supaya bisa direview dulu sebelum porting besar-besaran. Belum ada:

- **Enkripsi kredensial saat disimpan** (AES-256-GCM untuk password SSH/DB/
  token DNS di SQLite — bagian `crypto.go` homepoin, TERPISAH dari
  login/TOTP yang di atas sudah diputuskan tidak ikut). **Password server
  saat ini disimpan APA ADANYA** di kolom `servers.password_enc` (lihat
  komentar TODO di `repository.go`) — pakai auth key-based untuk sekarang,
  jangan simpan password produksi sampai ini di-porting. Ini tetap relevan
  walau tanpa login, karena melindungi isi file `poinhost.db` kalau
  di-copy/dicuri, bukan melindungi akses ke aplikasi.
- **Jobs & event bus untuk operasi jangka panjang** (mis. instalasi paket,
  migrasi file/DB/Docker) — polanya sudah ada (`runtime.EventsEmit`/
  `EventsOn`, dipakai server status di §8 dan terminal di §9), tinggal
  modul migrasi/instalasinya sendiri yang belum di-porting.
- **activitylog** (audit trail tiap operasi).
- **`access`/sudo** (elevasi ke user lain di server target) — homepoin
  punya modul terpisah untuk ini yang dipakai `files`/`dbmanager`/dll;
  belum di-porting, jadi operasi file (§10) saat ini selalu jalan sebagai
  user SSH yang login.
- **Files lanjutan**: chmod, copy, search, editor teks in-app (baca/tulis
  file kecil langsung di browser) — sengaja dipersempit dari homepoin di
  porting pertama ini (lihat §10).
- Modul lain: services, cron, webserver/php/ssl/dns/email/ftp, dbmanager
  (mysql/pg), docker, migration. Semua akan mengikuti pola
  `servers/`/`terminal/`/`files/` di atas satu per satu.
- **Split-pane multi-terminal per tab** (>1 sesi shell dalam satu tab) —
  fondasinya sudah ada di backend (`TerminalRegistry` & `terminal.Service`
  sudah mendukung N sesi per tab, lihat §9), yang belum ada cuma UI-nya
  (`ServerWorkspace`/`TerminalPanel` masih 1 sesi per tab).
- Restore tab saat startup sudah tersimpan (`ui_tabs`), tapi UI belum
  menampilkan indikator "reconnecting" per tab saat restore — perlu
  ditambah saat modul overview/monitoring di-porting.

## 12. Menjalankan (development)

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
