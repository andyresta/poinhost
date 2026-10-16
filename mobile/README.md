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

```bash
flutter pub get
flutter run
```

Butuh satu agent yang sudah diterbitkan lewat nginx + certbot dari panel
Mobile Control, beserta token pendaftarannya.

## Yang belum ada

- Belum pernah diuji terhadap agent sungguhan.
- Ikon, nama aplikasi, dan splash masih bawaan `flutter create`.
- Belum ada penanganan untuk beberapa server di satu app — dan itu memang
  bukan arah fiturnya: tiap server punya agent-nya sendiri.
- Tes baru satu: app berhenti di layar pendaftaran saat belum ada kredensial.
