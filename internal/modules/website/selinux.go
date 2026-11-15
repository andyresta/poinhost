package website

import "strings"

// SELinux aktif sebagai enforcing secara bawaan di Rocky, AlmaLinux, CentOS,
// RHEL, dan Fedora — dan ia mengekang nginx jauh lebih ketat daripada
// AppArmor mengekangnya di Ubuntu, yang paket nginx-nya tidak membawa profil
// sama sekali. Akibatnya dua hal yang di Debian/Ubuntu berjalan sendiri harus
// diizinkan dulu di sana:
//
//   - nginx dilarang membuka koneksi keluar, sehingga SETIAP domain
//     reverse-proxy menjawab 502 dengan `connect() to 127.0.0.1:8080 failed
//     (13: Permission denied)`;
//   - nginx hanya boleh membaca berkas berlabel tertentu, sehingga berkas
//     yang DIPINDAH ke webroot tidak terbaca — `mv` membawa label lamanya
//     ikut serta, beda dari `cp`. Termasuk berkas verifikasi certbot, yang
//     membuat penerbitan sertifikat gagal dengan 403 dari server sendiri.
//
// Keduanya terjadi berurutan di satu server Rocky 8.10, dan tidak satu pun
// pesannya menyebut SELinux: yang terlihat cuma 502 dan "unauthorized" dari
// Let's Encrypt, sementara `ls -l` memperlihatkan izin yang normal.
//
// Tiga prinsip yang menentukan bentuk file ini:
//
// SEDIKIT MUNGKIN. restorecon hanya mengembalikan label ke bawaan kebijakan
// sistem itu sendiri — ia tidak mengubah isi, izin, atau kepemilikan berkas,
// dan tidak menambah keleluasaan apa pun. Boolean jaringan hanya disentuh
// untuk domain proxy, dan hanya kalau memang sedang mati.
//
// TIDAK PERNAH DIAM-DIAM. Menyalakan boolean adalah perubahan kebijakan
// keamanan yang permanen di mesin orang lain, jadi ia dicatat ke syslog
// server lewat `logger` — permanen, di mesin yang diubahnya, terbaca oleh
// siapa pun yang mengauditnya nanti — selain dicetak ke keluaran perintah.
//
// TIDAK PERNAH MENGGAGALKAN PEKERJAAN UTAMA. Kalau SELinux mati, perintahnya
// tidak ada, atau langkahnya gagal, pembuatan domain tetap diteruskan. Paling
// buruk user berada di keadaan yang sama persis seperti sebelum file ini ada.
//
// Diawali baris baru dengan SENGAJA, sama seperti SvcHelper: konstanta ini
// disambung ke skrip lain yang tidak selalu berakhir dengan baris baru, dan
// penyambungan seperti itu pernah melahirkan fungsi bernama lain tanpa satu
// pun galat sintaks.
const SELinuxHelper = `
poinhost_selinux_enforcing() {
  # Jalurnya bisa ditimpa HANYA supaya keputusan ini bisa diuji dengan
  # berkas sungguhan. Tanpa seam ini, satu-satunya cara mengujinya adalah
  # menimpa fungsinya sendiri di tes — yang berarti logikanya tidak pernah
  # benar-benar diuji, dan "permissive" bisa diam-diam berubah jadi
  # "enforcing" tanpa satu pun tes merah.
  _sel="${POINHOST_SELINUX_ENFORCE:-/sys/fs/selinux/enforce}"
  [ -r "$_sel" ] || return 1
  [ "$(cat "$_sel" 2>/dev/null)" = "1" ] || return 1
  return 0
}

poinhost_selinux_catat() {
  if command -v logger >/dev/null 2>&1; then
    logger -t poinhost "$1" 2>/dev/null || true
  fi
  echo "` + selinuxTanda + ` $1"
}

poinhost_selinux_label() {
  if ! poinhost_selinux_enforcing; then return 0; fi
  if ! command -v restorecon >/dev/null 2>&1; then return 0; fi
  restorecon -R "$1" >/dev/null 2>&1 || true
  return 0
}

poinhost_selinux_proxy() {
  if ! poinhost_selinux_enforcing; then return 0; fi
  if ! command -v getsebool >/dev/null 2>&1; then return 0; fi
  if getsebool httpd_can_network_connect 2>/dev/null | grep -q 'on$'; then return 0; fi
  if ! command -v setsebool >/dev/null 2>&1; then return 0; fi
  if setsebool -P httpd_can_network_connect 1 >/dev/null 2>&1; then
    poinhost_selinux_catat "SELinux aktif: httpd_can_network_connect dinyalakan supaya nginx boleh menyambung ke backend domain proxy."
  else
    poinhost_selinux_catat "SELinux aktif tapi httpd_can_network_connect gagal dinyalakan — domain proxy kemungkinan menjawab 502. Jalankan di server: setsebool -P httpd_can_network_connect 1"
  fi
  return 0
}
`

// selinuxTanda menandai baris yang perlu sampai ke mata user.
//
// Dipakai sebagai penanda, bukan sebagai kalimat, supaya keluaran skrip yang
// kebetulan menyebut "SELinux" tidak ikut terbawa.
const selinuxTanda = "__poinhost_SELINUX__"

// selinuxLabel menyusun pemanggilan restorecon untuk satu direktori.
func selinuxLabel(path string) string {
	return "poinhost_selinux_label " + shellQuote(path) + "\n"
}

// selinuxProxy menyusun pemanggilan boolean jaringan, hanya kalau vhost yang
// ditulis memang reverse-proxy.
//
// Dibedakan di sini, bukan di dalam skrip: domain static tidak pernah butuh
// nginx menyambung ke mana pun, dan menyalakan boolean itu untuknya berarti
// melonggarkan kebijakan server tanpa ada yang memerlukannya.
func selinuxProxy(opts *vhostOptions) string {
	if opts == nil || strings.TrimSpace(opts.ProxyTarget) == "" {
		return ""
	}
	return "poinhost_selinux_proxy\n"
}
