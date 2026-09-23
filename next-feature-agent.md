# Rencana Fitur: poinhost Agent

> Status: **rencana**, belum diimplementasikan. Dokumen ini adalah hasil diskusi
> desain dan acuan implementasi. Keputusan yang masih terbuka ada di bagian
> [Keputusan terbuka](#10-keputusan-terbuka).

## 1. Ringkasan

poinhost Agent adalah **fitur tambahan opsional**: sebuah binary Linux yang
berjalan sebagai service systemd di **satu** server pilihan pengguna. Dari sana
agent:

- mengelola **server tempat ia terpasang** (eksekusi lokal), dan
- mengelola **server lain yang didaftarkan** lewat SSH, sama seperti app desktop
  sekarang.

Agent membuka akses ke fungsi-fungsi poinhost lewat beberapa **konektor**:
bot Telegram (tahap pertama), lalu REST API (untuk aplikasi mobile) dan MCP
(untuk LLM). **Semua konektor memanggil fungsi yang sama.**

Konsepnya mirip Portainer Server: satu titik kontrol yang mengelola banyak
host. Bedanya, server yang dikelola **tidak dipasangi agent apa pun**, cukup
SSH.

## 2. Prinsip yang tidak boleh dilanggar

1. **Agentless tetap default.** App desktop tetap berjalan penuh tanpa agent.
   Server yang dikelola agent dari jauh tetap hanya butuh SSH, tanpa port atau
   daemon tambahan.
2. **Satu sumber kebenaran untuk logika.** Agent memakai ulang
   `internal/modules/*`. Tidak ada logika yang ditulis ulang khusus untuk agent.
3. **Satu kumpulan fungsi, banyak konektor.** Izin, konfirmasi, dan audit
   diterapkan di satu tempat (dispatcher), bukan per konektor.
4. **Tidak membuka port baru secara default.** Bot Telegram memakai
   long-polling. Port HTTP agent hanya mendengarkan `127.0.0.1`.
5. **Tidak ada "jalankan shell bebas"** sebagai fungsi, terutama untuk LLM.
6. **Copot bersih.** Menghapus agent tidak meninggalkan sisa: binary, service,
   config, dan data ikut terhapus.

## 3. Arsitektur

```
Telegram bot ─┐
REST API     ─┼─► Dispatcher ──► Function Registry ──► internal/modules/* ──┬─► Executor lokal (server ini)
MCP (AI)     ─┘   │                                                         └─► Executor SSH (server lain)
                  ├─ autentikasi & otorisasi (chat ID / token + scope)
                  ├─ konfirmasi sesuai tingkat fungsi
                  └─ audit log
```

### 3.1 Function Registry

Setiap fungsi didefinisikan **sekali**:

| Atribut      | Keterangan | Contoh |
|--------------|-----------|--------|
| `name`       | ID stabil, bertitik | `docker.container.restart` |
| `description`| Dipakai untuk `/help` Telegram dan deskripsi tool MCP | "Restart satu container" |
| `params`     | JSON Schema | `{ server: string, container: string }` |
| `tier`       | `read` / `write` / `destructive` | `write` |
| `handler`    | `func(ctx, params) (result, error)`, memanggil modul yang ada | `docker.Service.RestartContainer` |

Konektor hanya menerjemahkan format:

| Konektor | Daftar fungsi | Memanggil fungsi |
|----------|---------------|------------------|
| Telegram | `/help` (dibangkitkan dari registry) | `/restart production nginx` → `docker.container.restart` |
| REST     | `GET /v1/functions` | `POST /v1/call/{name}` |
| MCP      | `tools/list` | `tools/call` |

### 3.2 Dispatcher: aturan per tingkat

| Tingkat | Perilaku |
|---------|----------|
| `read` | Langsung dijalankan. |
| `write` | Butuh konfirmasi eksplisit (Telegram: tombol inline ✅/❌, dengan batas waktu). |
| `destructive` | **Mati secara default.** Kalau diaktifkan per fungsi, butuh konfirmasi ulang. **Tidak boleh dipicu LLM tanpa persetujuan manusia.** |

Setiap panggilan dicatat di **audit log**: waktu, konektor, identitas pemanggil,
fungsi, parameter (tanpa rahasia), hasil, dan durasi.

### 3.3 Executor

Modul memanggil perintah lewat satu lapisan executor (`sshpool.Executor`).
Agent menambahkan **executor lokal** untuk server tempat ia terpasang, sehingga
tidak butuh kredensial SSH ke dirinya sendiri. Server lain tetap memakai
executor SSH yang sudah ada.

### 3.4 Pengaman "kelola diri sendiri"

Di server tempat agent berjalan, beberapa aksi berisiko memutus agent itu
sendiri. Aksi berikut diberi pengaman atau peringatan khusus:

- menghentikan atau mencopot agent,
- menyalakan firewall yang memblokir port agent atau koneksi keluar ke Telegram,
- restart jaringan atau sshd.

Agent dipasang sebagai **service systemd biasa, bukan container**, supaya
restart Docker tidak ikut mematikan agent.

## 4. Menu "Agent Control" di app desktop

Untuk setiap server:

1. **Deteksi dulu** apakah agent sudah terpasang. Yang diperiksa:
   - binary `/usr/local/bin/poinhost-agent` (plus `--version`),
   - unit systemd `poinhost-agent.service` (aktif? enabled?),
   - health check `http://127.0.0.1:<port>/healthz` lewat SSH.
2. **Belum terpasang**: tombol **Pasang**.
   - Deteksi arsitektur (`uname -m` → `amd64` / `arm64`).
   - Unggah binary lewat SFTP. Binary dibundel di app desktop, jadi tidak
     mengunduh apa pun dari internet.
   - Tulis config ke `/etc/poinhost-agent/config.yaml` (0600).
   - Tulis unit systemd, lalu `systemctl enable --now poinhost-agent`.
   - Verifikasi lewat health check.
3. **Sudah terpasang**: tampilkan versi, status running/enabled, port, dan
   uptime. Aksinya: **Start / Stop / Restart / Update / Konfigurasi / Lihat log
   / Copot**.
4. **Konfigurasi**:
   - port (default usulan: `7878`, bind `127.0.0.1`),
   - token bot Telegram,
   - daftar chat ID Telegram yang diizinkan,
   - daftar server lain yang boleh dikelola agent,
   - fungsi `write` / `destructive` mana yang diaktifkan.

### 4.1 Tata letak di server

| Path | Isi |
|------|-----|
| `/usr/local/bin/poinhost-agent` | binary |
| `/etc/poinhost-agent/config.yaml` | konfigurasi (0600, root) |
| `/var/lib/poinhost-agent/` | SQLite (server terdaftar, audit log), vault terenkripsi |
| `/etc/systemd/system/poinhost-agent.service` | unit systemd |

### 4.2 Pendaftaran server lain

App desktop mengirim server terpilih beserta kredensialnya ke agent **dalam
bentuk terenkripsi**, memakai ulang format backup terenkripsi yang sudah ada
(`internal/core/backup`). Agent menyimpannya di vault miliknya.

## 5. Konektor tahap pertama: bot Telegram

- **Long-polling** (`getUpdates`), jadi tidak butuh webhook atau port masuk.
- **Allowlist chat ID**: pesan dari chat lain diabaikan dan dicatat di audit.
- `/help` dibangkitkan otomatis dari registry, dan hanya menampilkan fungsi yang
  diizinkan.
- Fungsi `write` memunculkan pesan konfirmasi dengan tombol inline. Konfirmasi
  kedaluwarsa (misalnya 60 detik) dan hanya bisa dijawab oleh chat yang meminta.
- Output panjang (log) dipotong atau dikirim sebagai file.

### 5.1 Fungsi awal

| Tingkat | Fungsi |
|---------|--------|
| read | `servers.list`, `server.status` (CPU/RAM/disk/uptime), `docker.containers.list`, `website.domains.list` + status SSL, `firewall.status`, `services.list`, `docker.container.logs` (ekor pendek), `services.logs` (ekor pendek) |
| write | `docker.container.restart` / `start` / `stop`, `services.restart`, `nginx.reload` |

## 6. Tahap berikutnya

1. **REST API** untuk aplikasi mobile.
   - Token per perangkat dengan scope.
   - Akses lewat Tailscale/WireGuard, atau lewat subdomain HTTPS yang
     dipublikasikan dengan modul Website + certbot yang sudah ada.
2. **MCP server**: tool diambil dari registry yang sama.
   - Transport HTTP (streamable) di `127.0.0.1`, atau stdio lewat SSH.
   - Bot Telegram bisa diberi mode "tanya AI" yang memakai tool MCP ini.
3. **Alert terjadwal**: situs down, container berhenti, SSL hampir habis, disk
   penuh. Ini baru mungkin karena agent selalu menyala.
4. **Backup terjadwal**: memakai ulang mesin tar/dump dari migrasi website.

## 7. Keamanan

### 7.1 Ancaman utama

**Server kontrol menyimpan akses ke semua server terdaftar.** Kalau server
kontrol diretas, server lain ikut terancam. Mitigasi:

- SSH key khusus untuk agent, bukan password root.
- Opsional: user khusus di server tujuan dengan sudo terbatas.
- Vault agent terenkripsi.
- Tombol **"cabut akses agent"** di app desktop, yang menghapus key agent dari
  server tujuan.

### 7.2 LLM dan prompt injection

Log, isi file, dan output perintah yang dibaca LLM bisa berisi instruksi palsu.
Karena itu:

- Tingkat `write` / `destructive` **selalu** butuh konfirmasi manusia, dan
  isi yang dibaca LLM tidak bisa melewati aturan ini.
- Tidak ada tool shell bebas.
- Parameter divalidasi ketat per fungsi (JSON Schema + validasi modul).

### 7.3 Prasyarat: perbaikan dari analisis awal

Harus selesai **sebelum** agent dirilis, karena dampaknya lebih besar pada
service yang selalu menyala:

| Temuan | Lokasi | Kenapa kritis untuk agent |
|--------|--------|---------------------------|
| `secret.key` ditimpa diam-diam kalau gagal dibaca | `internal/core/secrets/file.go` (`loadOrCreateKey`) | Server tanpa OS keychain sepenuhnya bergantung pada vault file ini. |
| Password sudo terlihat di command line remote (`echo pw \| sudo -S`) | `wrap()` di website/docker/services/firewall/files | Agent menjalankan perintah terus-menerus. |
| Trust host key menerima key dari koneksi kedua tanpa membandingkan fingerprint | `internal/core/sshpool/probe.go` | Agent mendaftarkan server secara otomatis. |
| Input nginx/cron/SFTP bisa menyisipkan perintah yang jalan sebagai root | `website/proxy.go`, `website/cron.go`, `website/sftp.go` | Parameter bisa datang dari LLM. |

## 8. Tahapan implementasi

### Tahap A: Fondasi

1. Pisahkan lapisan inti dari `app.go` supaya bisa dipakai Wails **dan** agent
   (misalnya `internal/core/runtime` berisi konstruksi service).
2. Buat `internal/agent/registry`: tipe `Function`, tingkat, validasi JSON
   Schema.
3. Buat `internal/agent/dispatch`: auth, konfirmasi, audit log (SQLite).
4. Buat executor lokal.
5. Buat `cmd/poinhost-agent`: config, vault file, HTTP `/healthz`, graceful
   shutdown.
6. Perbaiki prasyarat keamanan (§7.3).
7. Build silang binary `linux/amd64` + `linux/arm64`, dibundel ke app desktop
   (embed) dan ke CI release.

### Tahap B: Agent Control + Telegram

1. Menu **Agent Control** di app desktop (deteksi, pasang, status, konfigurasi,
   update, copot).
2. Pendaftaran server lain ke agent.
3. Konektor Telegram + fungsi awal (§5.1).
4. Test: unit test registry/dispatcher, test konektor dengan Telegram API
   tiruan, dan uji manual di satu server.

### Tahap C dan seterusnya

REST API, MCP, alert, backup (§6).

## 9. Kriteria selesai (Tahap A + B)

- [ ] Agent Control mendeteksi dengan benar: belum terpasang / terpasang-mati / terpasang-jalan.
- [ ] Pasang → service berjalan, `/healthz` OK, port hanya di `127.0.0.1`.
- [ ] Copot → tidak ada sisa file maupun service.
- [ ] Bot hanya merespons chat ID yang diizinkan.
- [ ] Fungsi `read` jalan untuk server lokal **dan** server terdaftar lewat SSH.
- [ ] Fungsi `write` hanya jalan setelah konfirmasi, dan konfirmasinya kedaluwarsa.
- [ ] Semua panggilan tercatat di audit log.
- [ ] Prasyarat keamanan §7.3 selesai dan punya test.

## 10. Keputusan terbuka

1. **Port default**: usulan `7878`.
2. **Daftar fungsi awal** (§5.1): cukup, atau perlu ditambah (misalnya cek SSL
   hampir habis, disk penuh)?
3. **Alert otomatis** masuk Tahap B, atau menyusul di Tahap C?
4. **Akun di server tujuan**: tetap memakai kredensial yang sudah ada di
   poinhost, atau wajib SSH key + user khusus agent?
