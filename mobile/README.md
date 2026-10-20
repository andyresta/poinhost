# poinhost mobile

Aplikasi Flutter untuk Mobile Control. Satu app, satu server: ia bicara ke
**satu** poinhost-agent, yang mengurus mesinnya sendiri.

> Status: **kerangka**. Alur pendaftaran, penyimpanan token, klien API, dan
> tiga layarnya sudah ada dan `flutter analyze` bersih — tapi belum pernah
> dijalankan terhadap agent sungguhan.

Kontrak API-nya ada di [`../mobile-control.md`](../mobile-control.md).

## Batasan yang membentuk app ini

**HP tetap bodoh.** Tidak ada SSH, kriptografi, atau orkestrasi di sini.
Seluruh "logika" app adalah [`lib/src/api/agent_client.dart`](lib/src/api/agent_client.dart):
kirim permintaan, terjemahkan jawabannya. Alasannya ada di `../mobile-control.md`.

**Token yang diketik user bukan kredensial akses.** Ia ditukar di
`POST /v1/devices` dengan token milik perangkat ini, yang belum membuka apa
pun sampai seseorang menyetujuinya dari app desktop poinhost.

## Alur

```
EnrollScreen ──POST /v1/devices──► WaitingScreen ──disetujui──► ServerScreen
  URL + token                       poll /v1/server             poll /v1/server
  pendaftaran                       403 pending                 200
```

Layarnya dipilih dari `SessionPhase`, bukan dari navigator stack: yang
memindahkannya adalah jawaban agent — termasuk persetujuan yang datang saat
layar tunggu sedang terbuka, yang tidak diberitahukan ke HP oleh siapa pun.

## Struktur

```
lib/
  main.dart                     # merakit Session lalu runApp
  src/
    app.dart                    # memilih layar dari SessionPhase
    session.dart                # satu-satunya state: fase + snapshot + polling
    api/
      agent_client.dart         # dua endpoint, dan tidak lebih
      agent_exception.dart      # AgentFailure: kenapa gagal, bukan sekadar gagal
      models.dart               # JSON -> objek, parser tulis tangan
    storage/
      credential_store.dart     # keychain / EncryptedSharedPreferences
    screens/                    # enroll, waiting, server
    widgets/
```

## Menjalankan

Target produknya **Android dan iOS**. Di mesin tanpa HP tersambung, ada satu
target lagi untuk melihat tampilannya — dengan satu batasan yang perlu
diketahui lebih dulu.

### Di HP Android (yang sebenarnya)

```bash
flutter run
```

Butuh HP tersambung USB dengan USB debugging menyala. Kalau Gradle mengeluh
soal lisensi SDK, pasang `cmdline-tools` lewat Android Studio lalu jalankan
`flutter doctor --android-licenses` — persetujuan lisensi itu harus dilakukan
pemilik mesinnya sendiri.

### Pratinjau tampilan di browser

```bash
flutter build web
```

Lalu sajikan `build/web` (di repo ini ada entri `mobile-web` di
`.claude/launch.json`).

**Hanya untuk melihat tampilan.** Agent tidak mengirim header CORS — ia memang
dibuat untuk dipanggil aplikasi di HP, bukan dari halaman web — jadi setiap
panggilan API dari pratinjau ini diblokir browser dan app akan berhenti di
"tidak bisa menghubungi agent". Menambahkan CORS di agent demi pratinjau
berarti melebarkan permukaan yang dipublikasikan ke internet untuk kenyamanan
saat mengembangkan, dan itu pertukaran yang tidak sepadan.

### Windows desktop

Belum bisa tanpa satu komponen tambahan: `flutter_secure_storage_windows`
butuh ATL (`atlstr.h`), yang tidak termasuk di Visual Studio Build Tools
standar.

Untuk benar-benar memakai app ini, butuh satu agent yang sudah dijangkau dari
luar lewat vhost nginx (dibuat dari modul Website), beserta token
pendaftarannya.

## Yang belum ada

- Belum pernah diuji terhadap agent sungguhan dari HP.
- Layar server (metrik) belum pernah tergambar dengan data asli — pendaftaran
  dan layar tunggu sudah, lewat agent yang dijalankan lokal.
- Ikon, nama aplikasi, dan splash masih bawaan `flutter create`.
- Belum ada penanganan untuk beberapa server di satu app — dan itu memang
  bukan arah fiturnya: tiap server punya agent-nya sendiri.
- Tes baru satu: app berhenti di layar pendaftaran saat belum ada kredensial.
