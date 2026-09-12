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
│       └── servers/              # SATU-SATUNYA modul yang sudah di-porting di skeleton ini
│           ├── dto.go
│           ├── repository.go     # CRUD tabel `servers`
│           └── service.go        # CRUD + register/warm ke sshpool.Pool
│
├── migrations/
│   └── 001_core.sql             # servers, app_settings, activity_logs, ui_tabs
│
└── frontend/
    ├── src/
    │   ├── store/tabs.ts         # Zustand: servers + tabs + activeTabId
    │   ├── features/
    │   │   ├── servers/          # ServersPage (list+form) & ServerWorkspace (isi 1 tab)
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

## 6. Yang BELUM di-porting di skeleton ini (roadmap)

Skeleton ini sengaja dibatasi ke fondasi (sshpool + session/tab + 1 modul
contoh) supaya bisa direview dulu sebelum porting besar-besaran. Belum ada:

- **Auth & enkripsi kredensial** (`internal/core/auth` — master password,
  TOTP, AES-256-GCM). **Password server saat ini disimpan APA ADANYA** di
  kolom `servers.password_enc` (lihat komentar TODO di
  `repository.go`) — pakai auth key-based untuk sekarang, jangan simpan
  password produksi sampai modul auth di-porting.
- **Jobs & event bus** — homepoin pakai WebSocket broadcaster custom;
  poinhost akan pakai `runtime.EventsEmit`/`EventsOn` bawaan Wails (lebih
  simpel, tidak perlu reconnect logic sendiri).
- **activitylog** (audit trail tiap operasi).
- Modul lain: files, terminal (PTY xterm.js di frontend), services, cron,
  webserver/php/ssl/dns/email/ftp, dbmanager (mysql/pg), docker, migration.
  Semua akan mengikuti pola `servers/` di atas satu per satu.
- **Split-pane multi-terminal per tab** — fondasinya sudah ada
  (`TerminalRegistry` sudah mendukung N sesi per tab), tinggal UI-nya.
- Restore tab saat startup sudah tersimpan (`ui_tabs`), tapi UI belum
  menampilkan indikator "reconnecting" per tab saat restore — perlu
  ditambah saat modul overview/monitoring di-porting.

## 7. Menjalankan (development)

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
