// Command mobile-devproxy menyajikan build web aplikasi mobile dan API agent
// dari SATU origin, supaya app itu bisa dijalankan di browser tanpa HP.
//
// Ini alat pengembangan, bukan bagian produk. Ia tidak pernah ikut terpasang
// di server dan tidak dibangun oleh `wails build` maupun `make agent`.
//
// Kenapa ada: aplikasi mobile memanggil agent lewat HTTP, dan browser
// menolak panggilan lintas origin kecuali agent mengirim header CORS. Agent
// sengaja TIDAK mengirimnya — ia dibuat untuk dipanggil aplikasi di HP, dan
// menambah CORS berarti melebarkan permukaan yang dipublikasikan ke internet
// demi kenyamanan saat mengembangkan.
//
// Jalan keluarnya bukan melonggarkan agent, melainkan menghapus "lintas
// origin"-nya: browser hanya melihat satu alamat, dan yang menyeberang ke
// agent adalah proses ini — di sisi server, tempat CORS tidak berlaku.
//
//	Browser → :8080 ─┬─ /              → build web Flutter
//	                 └─ /healthz, /v1  → agent
//
// Agent-nya boleh yang mana saja: yang berjalan lokal, atau agent Linux
// sungguhan lewat terowongan SSH —
//
//	ssh -N -L 7898:127.0.0.1:7898 root@server
//
// yang membuat port loopback agent terjangkau dari sini tanpa perlu vhost
// nginx, sertifikat, atau mempublikasikan apa pun.
package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "alamat yang didengarkan proxy")
	agent := flag.String("agent", "http://127.0.0.1:7898", "alamat agent yang dituju")
	webDir := flag.String("web", filepath.Join("mobile", "build", "web"), "direktori hasil `flutter build web`")
	flag.Parse()

	target, err := url.Parse(*agent)
	if err != nil {
		log.Fatalf("alamat agent tidak valid: %v", err)
	}
	if _, err := os.Stat(filepath.Join(*webDir, "index.html")); err != nil {
		log.Fatalf("build web tidak ditemukan di %s — jalankan `flutter build web` dulu di mobile/", *webDir)
	}

	rp := httputil.NewSingleHostReverseProxy(target)
	// Host diganti ke host tujuan. Tanpa ini agent menerima "localhost:8080",
	// dan ketika suatu saat ia ada di balik nginx sungguhan, host yang salah
	// adalah cara paling umum vhost memilih server block yang keliru.
	asli := rp.Director
	rp.Director = func(r *http.Request) {
		asli(r)
		r.Host = target.Host
	}

	mux := http.NewServeMux()
	mux.Handle("/healthz", rp)
	mux.Handle("/v1/", rp)
	mux.HandleFunc("/flutter_service_worker.js", serviceWorkerPembersih)
	mux.Handle("/", noStore(http.FileServer(http.Dir(*webDir))))

	log.Printf("mobile-devproxy: %s → web %s, agent %s", *addr, *webDir, target)
	log.Printf("buka http://%s dan isi alamat agent dengan http://%s", *addr, *addr)

	srv := &http.Server{Addr: *addr, Handler: logging(mux)}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// noStore melarang browser menyimpan apa pun dari alat ini.
//
// Tanpa ini, build web yang baru TIDAK terpakai: http.FileServer hanya
// mengirim Last-Modified, browser memakai aturan kesegaran heuristiknya
// sendiri, dan main.dart.js lama tetap dijalankan dari cache memori tanpa
// satu pun permintaan ke jaringan. Yang terlihat adalah perubahan kode yang
// "tidak berpengaruh" — dan itu menghabiskan waktu jauh lebih banyak
// daripada harga menyajikan ulang berkasnya setiap kali.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
		next.ServeHTTP(w, r)
	})
}

// swPembersih menggantikan service worker bawaan Flutter dengan satu yang
// MEMBUANG dirinya sendiri.
//
// `flutter build web` menghasilkan service worker yang menyajikan app dari
// cache lebih dulu. Untuk app yang sudah terbit itu memang gunanya; untuk
// alat ini ia berarti setiap build baru tidak terpakai, dan yang terlihat
// adalah perubahan kode yang "tidak berpengaruh" — persis jebakan yang sudah
// beberapa kali menghabiskan waktu di proyek ini, dan yang selama ini
// dihindari diam-diam dengan menambahkan ?v=... ke URL.
//
// Membangun tanpa service worker saja TIDAK cukup: yang sudah terlanjur
// terdaftar di browser tetap melayani dari cache-nya, dan tidak ada build
// baru yang bisa menyentuhnya. Maka penggantinya dibuat aktif membersihkan —
// menghapus seluruh cache, mencabut pendaftarannya, lalu memuat ulang tab
// yang sedang terbuka.
const swPembersih = `self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (e) => e.waitUntil((async () => {
  for (const k of await caches.keys()) await caches.delete(k);
  await self.registration.unregister();
  for (const c of await self.clients.matchAll({type: 'window'})) c.navigate(c.url);
})()));
`

func serviceWorkerPembersih(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, must-revalidate")
	_, _ = io.WriteString(w, swPembersih)
}

// logging mencatat tiap permintaan yang diteruskan ke agent.
//
// Hanya yang menuju agent, bukan setiap berkas statis: yang ingin dilihat
// saat menguji adalah percakapan app dengan agent, dan ratusan baris untuk
// aset Flutter akan mengubur persis bagian itu.
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/healthz" {
			log.Printf("→ %s %s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}
