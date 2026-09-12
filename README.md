# poinhost

Rewrite [homepoin](https://github.com/andyresta/homepoin) menjadi aplikasi
desktop native cross-platform (Windows/macOS/Linux) — Go + Wails + React,
dengan skema koneksi SSH & multi-tab yang lebih matang.

Lihat [ARCHITECTURE.md](./ARCHITECTURE.md) untuk desain lengkap: model
connection-vs-tab, kenapa slot terminal tunggal homepoin diganti, dan
roadmap modul yang belum di-porting.

## Menjalankan (development)

Butuh Wails CLI + dependency native (`wails doctor` untuk cek):

```bash
wails dev      # hot-reload
wails build    # binary rilis
```

Setelah mengubah signature method publik di `App` (`app.go`), regenerate
binding TypeScript:

```bash
wails generate module
```

## Struktur

```
internal/core/      infra lintas-modul: config, database, sshpool
internal/session/   tab manager (BARU — lihat ARCHITECTURE.md §3)
internal/modules/    vertical slice per fitur (servers sudah ada, sisanya menyusul)
frontend/            React + TypeScript + Vite + Zustand
migrations/          schema SQLite (idempotent, jalan tiap startup)
```
