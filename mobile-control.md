# Mobile Control

> Status: **tahap 1 terpasang** — agent + REST baca-saja. Dokumen ini
> menggantikan rencana lama `next-feature-agent.md`, yang menggambarkan agent
> multi-server dengan konektor Telegram. Keduanya dibatalkan; alasannya ada di
> [Yang dibuang](#yang-dibuang).

## Ringkasan

Mobile Control adalah **fitur tambahan opsional**: satu binary Linux yang
berjalan sebagai service systemd di server, menyediakan REST API **baca-saja**
untuk aplikasi poinhost di HP.

App desktop tetap berjalan penuh tanpa agent. Server yang dikelola dari desktop
tetap hanya butuh SSH — agentless tetap default.

## Dua batasan yang membentuk seluruh fitur

**1. Satu agent hanya mengurus mesinnya sendiri.**

Tiap server yang ingin dilihat dari HP memasang agent-nya sendiri. Tidak ada
penautan server lain, jadi agent tidak pernah menyimpan kredensial SSH server
lain — yang memperkecil taruhan saat API-nya dibuka ke jaringan. Pemasangannya
selalu menyertakan kunci pengelolaan-diri; tanpa itu agent tidak punya apa pun
untuk dilaporkan.

**2. Aplikasi di HP tetap bodoh.**

HP hanya menampilkan data dan memanggil API. Tidak ada handshake SSH,
kriptografi, atau orkestrasi di sisi HP. Inilah yang membuat REST/JSON dipilih
di atas WebSocket: WebSocket menuntut klien mengurus reconnect, heartbeat, dan
putus-sambung saat jaringan seluler berpindah — persis logika yang tidak
diinginkan ada di HP. Kalau nanti butuh dorongan dari server, SSE lebih cocok
daripada WebSocket karena arahnya memang satu.

## Alur mendaftarkan HP

1. **Buat token pendaftaran** di panel Mobile Control.
2. **Tempel di HP.** HP memanggil `POST /v1/devices` dan menerima token
   miliknya sendiri. Perangkatnya langsung berstatus **menunggu**.
3. **Setujui di poinhost.** Perangkat yang menunggu muncul di panel dengan nama
   dan waktu pendaftarannya.
4. **Setelah disetujui**, token perangkat itu baru membuka `/v1/server`.

Yang membuat alur ini bernilai: **token pendaftaran bukan kredensial akses.**
Satu-satunya yang bisa dilakukan dengannya adalah mengantre — dan antrean itu
terlihat jelas di layar pemiliknya. Jadi ia boleh dikirim lewat chat atau
diketik ulang tanpa taruhan besar, dan yang menentukan akses tetap satu
tindakan manusia di depan app desktop.

Token perangkat **satu per HP**, disimpan sebagai hash SHA-256 — berkasnya ikut
tersalin ke backup mana pun yang menyentuh direktori data agent, jadi token apa
adanya di sana berarti setiap salinan adalah kunci yang masih hidup.
Konsekuensi yang dicari: mencabut satu HP yang hilang tidak memutus HP lain.

Memutar token pendaftaran **tidak** memutus perangkat yang sudah disetujui.
Yang hilang hanya kemampuan mendaftarkan HP baru dengan token lama.

## Endpoint

Permukaannya ada **dua, dan sengaja terpisah**.

### Publik — yang diterbitkan nginx ke internet

| Endpoint | Auth | Isi |
|----------|------|-----|
| `GET /healthz` | tidak | `ok`, `version`, `uptimeSeconds` |
| `POST /v1/devices` | token pendaftaran | menukar token pendaftaran dengan token perangkat; status `pending` |
| `GET /v1/server` | token perangkat (disetujui) | identitas mesin + metriknya + versi agent |

```json
{
  "version": "1.0.0",
  "uptimeSeconds": 86400,
  "id": "6f1c...", "name": "production", "host": "10.0.0.1",
  "status": {
    "connection": "online", "osName": "Ubuntu 24.04", "hostname": "prod",
    "uptimeSeconds": 1209600, "cpuCores": 4, "cpuUsedPct": 12.5,
    "memUsedMB": 2048, "memTotalMB": 8192,
    "diskUsedGB": 31.4, "diskTotalGB": 160,
    "stale": false, "checkedAt": "2026-10-05T04:00:00Z"
  }
}
```

`connection` bernilai `online`, `offline`, atau `reconnecting` — kosong sebelum
polling pertama. `checkedAt` dan `error` hanya muncul kalau terisi. `status`
DIHILANGKAN selama metriknya belum pernah terkumpul, bukan dikirim sebagai
nol: layar dengan CPU 0% dan disk 0 GB terlihat seperti server sehat yang
menganggur.

Galat selalu `{"error": "..."}`, dan `/v1/server` menambahkan `status` saat
perangkatnya dikenal:

| Kode | Arti | Yang harus dilakukan HP |
|------|------|--------------------------|
| `401` | token tidak dikenal | daftar ulang |
| `403` + `"status":"pending"` | belum disetujui | tampilkan layar menunggu, terus poll |
| `403` + `"status":"revoked"` | akses dicabut | beri tahu pemilik |
| `503` | pendaftaran belum dibuka, atau agent belum mengelola mesinnya sendiri | - |

Tiga jawaban itu dibedakan justru supaya layar HP tidak cuma bisa bilang
"gagal": masing-masing menuntut tindakan yang berbeda dari pemiliknya.

Tokennya dikembalikan di langkah 2, bukan nanti setelah disetujui, supaya HP
tidak perlu menyimpan token pendaftaran dan menukarnya lagi belakangan. HP
cukup menyimpan satu token lalu memanggil `/v1/server` seperti biasa - selama
masih 403, tampilkan layar menunggu. Tidak ada handshake yang perlu ditulis di
sisi HP.

### Admin — unix socket, tidak pernah di port TCP

`/var/lib/poinhost-agent/admin.sock`, mode 0600 di dalam direktori bermode
0700 milik root. App desktop memanggilnya lewat `curl --unix-socket` di dalam
sesi SSH-nya.

| Endpoint | Isi |
|----------|-----|
| `GET /admin/devices` | daftar perangkat, yang menunggu di atas |
| `POST /admin/devices/{id}/approve` | setujui |
| `POST /admin/devices/{id}/revoke` | cabut, token tetap dikenali |
| `DELETE /admin/devices/{id}` | hapus, token jadi asing |

Tidak ada token di permukaan ini, dan itu bukan kelalaian: siapa pun yang bisa
membuka socket-nya sudah root, dan root bisa membaca seluruh config agent
beserta kuncinya. Token tambahan hanya akan jadi rahasia lain yang harus
disimpan dan diputar, tanpa menghalangi siapa pun yang belum terhalang izin
berkas.

Yang dihindari dengan memisahkannya dari port publik: vhost nginx yang
menerbitkan agent meneruskan **seluruh path** ke port agent. Kalau persetujuan
hidup di port yang sama, ia ikut terbit ke internet dan hanya dijaga satu
string rahasia lagi. Di socket, tidak ada permintaan dari internet yang bisa
sampai ke sana sama sekali. Interface `DeviceStore` yang dipakai permukaan
publik juga tidak punya metode menyetujui, jadi handler publik yang menyetujui
tidak bisa ditulis tanpa lebih dulu melebarkan kontraknya.

Nama field dipetakan eksplisit di `statusJSON`, bukan lewat tag JSON di tipe
internal: mengganti nama field internal tidak boleh diam-diam mengubah kontrak
aplikasi yang sudah terpasang di HP orang.

**Tidak ada endpoint daftar server**, dan `Reader` di sisi Go pun hanya bisa
menyebut `Self()`. Satu agent hanya mengurus mesinnya sendiri. Agent yang
di-update dari versi lama bisa MASIH menyimpan server yang dulu ditautkan;
interface yang tidak punya ListServers membuat server-server itu mustahil ikut
terlapor.

## Jalur jaringan

Agent **selalu** bind di `127.0.0.1` saja (dipaksa `agentcfg.ValidateListen`).
Yang menjangkaunya dari HP adalah vhost nginx + certbot di server yang sama.

**Vhost itu dibuat dari modul Website, bukan dari panel Mobile Control.**
Buat domain atau subdomain di sana, arahkan reverse proxy-nya ke
`http://127.0.0.1:<port agent>`, lalu terbitkan sertifikatnya dari tab SSL —
semuanya pekerjaan yang memang sudah hidup di modul itu. Menduplikasinya di
Mobile Control berarti ada dua tempat yang membuat vhost, dengan dua
perangkat aturan yang lama-lama berbeda.

Panel Mobile Control **hanya mengenali hasilnya**: ia mencari domain yang
target proxy-nya persis sama dengan port agent, lalu menampilkan URL itu
sebagai endpoint yang dipakai HP. Kalau belum ada, ia menyebutkan alamat yang
harus dituju proxy-nya dan tidak lebih.

Validasi certbot tetap jalan walau seluruh domain di-proxy: blok
`location ^~ /.well-known/acme-challenge/` ada di kedua varian vhost, jadi
permintaan validasi tidak ikut diteruskan ke agent (yang akan menjawab 404).

App desktop sendiri tidak lewat jalur itu: ia memanggil agent dengan `curl` di
dalam sesi SSH-nya — port loopback untuk `/healthz`, unix socket untuk admin.

## Yang dibuang

Rencana lama membangun **dispatcher + function registry** dengan tingkatan
`read`/`write`/`destructive`, konektor **Telegram** (long-polling, kode
pairing, allowlist user), **MCP** untuk LLM, dan kemampuan satu agent
**menautkan server lain**.

Semuanya dibatalkan. Telegram menambah satu permukaan serangan, satu kredensial,
dan satu mekanisme pairing untuk sesuatu yang app mobile lakukan lebih baik;
penautan server lain memindahkan kredensial SSH ke mesin yang API-nya terbuka.
Yang tersisa adalah bagian yang benar-benar dipakai.

Menulis dari HP belum ada dan baru dipertimbangkan setelah model
autentikasinya terbukti di pemakaian nyata.
