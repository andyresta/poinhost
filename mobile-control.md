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

## Endpoint

Dua, dan keduanya **baca**.

| Endpoint | Auth | Isi |
|----------|------|-----|
| `GET /healthz` | tidak | `ok`, `version`, `uptimeSeconds` |
| `GET /v1/server` | ya | identitas mesin + metriknya + versi agent |

```json
{
  "version": "1.0.0",
  "uptimeSeconds": 86400,
  "id": "6f1c…", "name": "production", "host": "10.0.0.1",
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

Galat selalu `{"error": "..."}` — `401` token salah atau tidak dikirim, `503`
`apiToken` belum diset atau agent belum mengelola mesinnya sendiri.

Semuanya digabung dalam satu respons supaya HP cukup satu permintaan untuk
menggambar layarnya; di jaringan seluler, dua permintaan berarti dua kali
menunggu. Angkanya dibaca dari snapshot Collector, tanpa round-trip SSH, jadi
endpoint ini murah untuk di-poll.

Nama field dipetakan eksplisit di `statusJSON`, bukan lewat tag JSON di tipe
internal: mengganti nama field internal tidak boleh diam-diam mengubah kontrak
aplikasi yang sudah terpasang di HP orang.

**Tidak ada endpoint daftar server**, dan `Reader` di sisi Go pun hanya bisa
menyebut `Self()`. Satu agent hanya mengurus mesinnya sendiri, jadi endpoint
jamak hanya akan mengembalikan satu baris yang sama selamanya — bentuk yang
menjanjikan sesuatu yang tidak pernah ada, dan membuat HP menulis loop untuk
koleksi beranggota satu. Agent yang di-update dari versi lama bisa MASIH
menyimpan server yang dulu ditautkan; interface yang tidak punya ListServers
membuat server-server itu mustahil ikut terlapor.

## Autentikasi

Bearer token, dibandingkan dengan `subtle.ConstantTimeCompare`. Token dibuat
saat pemasangan dan bisa diganti dari panel Mobile Control di app desktop
(**Ganti token**) — yang menulis config lalu me-restart agent, karena agent
membaca config saat start.

Config tanpa `apiToken` menutup seluruh endpoint selain `/healthz` dengan 503,
**bukan** membukanya: kegagalan konfigurasi tidak boleh berubah jadi akses tanpa
autentikasi.

`/healthz` sengaja terbuka — app desktop memakainya untuk membedakan "terpasang
tapi mati" dari "jalan", termasuk sebelum token ada, dan isinya tidak memuat
apa pun yang rahasia.

## Jalur jaringan

Agent **selalu** bind di `127.0.0.1` saja (dipaksa `agentcfg.ValidateListen`).
Yang menjangkaunya dari luar adalah reverse proxy nginx + certbot di server yang
sama, dan panel Mobile Control menerbitkannya sendiri: pilih domain atau
subdomain, lalu **Terbitkan**.

Langkahnya merangkai yang sudah ada di modul Website, bukan menulis config
nginx atau perintah certbot baru:

1. vhost dibuat kalau belum ada — sebagai **subdomain** kalau induknya memang
   dikelola di server ini, karena vhost domain biasa ikut memasang
   `www.<nama>` di server_name dan `www.agent.example.com` adalah nama yang
   tidak pernah ada;
2. seluruh domain di-proxy ke `http://127.0.0.1:<port agent>`, dengan port
   dibaca dari config agent, bukan diasumsikan 7898;
3. sertifikat diterbitkan lalu **langsung dipasang**. Tab SSL di modul Website
   sengaja memisahkan keduanya supaya situs production tidak berubah ke HTTPS
   sebelum diverifikasi; di sini tidak ada yang bisa rusak, dan yang dicari
   user adalah satu URL https yang tinggal ditempel ke HP.

Domain yang sudah melayani website lain **ditolak** — mengarahkannya ke agent
akan mematikan situs itu. Berhenti menerbitkan hanya melepas proxy-nya; vhost
dan sertifikatnya ditinggalkan, karena menghapus keduanya diam-diam adalah
kejutan yang tidak bisa dibatalkan.

Validasi certbot tetap jalan walau seluruh domain di-proxy: blok
`location ^~ /.well-known/acme-challenge/` ada di kedua varian vhost, jadi
permintaan validasi tidak ikut diteruskan ke agent (yang akan menjawab 404).

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
