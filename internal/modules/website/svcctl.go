package website

// SvcHelper adalah fungsi shell yang mengendalikan service tanpa
// mengasumsikan init system-nya.
//
// Alasannya: modul ini memanggil `systemctl` di puluhan tempat, dan Alpine
// memakai OpenRC. Akibatnya nginx BISA dipasang di Alpine — cabang apk di
// installer memang ada — lalu membuat satu domain gagal, karena yang dipakai
// untuk reload adalah systemctl. Instalasi yang berhasil lalu diikuti
// kegagalan di langkah berikutnya jauh lebih merugikan daripada penolakan di
// awal: server sudah terlanjur ditata.
//
// Keberadaan systemctl SAJA tidak cukup untuk menyimpulkan systemd berjalan —
// binarinya ada di banyak image container yang PID 1-nya bukan systemd. Yang
// menentukan adalah /run/systemd/system, direktori yang hanya ada saat
// systemd benar-benar menjadi init.
//
// Untuk server yang memang memakai systemd, perintah yang dijalankan sama
// persis dengan sebelumnya — termasuk jatuh ke restart saat reload gagal.
// Diawali baris baru dengan SENGAJA. Konstanta ini disambung ke skrip lain,
// dan yang di depannya tidak selalu berakhir dengan baris baru —
// distroDetectScript berakhir dengan `true`, yang pernah menyambung jadi
// satu kata dengan nama fungsi di bawah ini. Bash menerima itu sebagai
// definisi fungsi dengan NAMA LAIN, jadi tidak ada galat sintaks sama
// sekali: fungsinya cuma tidak pernah ada, dan setiap pemanggilnya gagal
// sebagai "command not found" yang langsung ditelan `|| echo unknown`.
const SvcHelper = `
poinhost_svc() {
  _aksi="$1"; _svc="$2"
  if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    case "$_aksi" in
      reload) systemctl reload "$_svc" 2>&1 || systemctl restart "$_svc" 2>&1 ;;
      *) systemctl "$_aksi" "$_svc" 2>&1 ;;
    esac
    return $?
  fi
  if command -v rc-service >/dev/null 2>&1; then
    case "$_aksi" in
      enable)  rc-update add "$_svc" default >/dev/null 2>&1 ;;
      disable) rc-update del "$_svc" default >/dev/null 2>&1 ;;
      reload)  rc-service "$_svc" reload 2>&1 || rc-service "$_svc" restart 2>&1 ;;
      is-active)
        rc-service "$_svc" status 2>/dev/null | grep -qi started && echo active || echo inactive ;;
      is-enabled)
        rc-update show default 2>/dev/null | grep -q "^[[:space:]]*$_svc[[:space:]]" && echo enabled || echo disabled ;;
      *) rc-service "$_svc" "$_aksi" 2>&1 ;;
    esac
    return $?
  fi
  # SysV sebagai cadangan terakhir. enable/disable sengaja tidak dipetakan ke
  # chkconfig/update-rc.d: keduanya berperilaku cukup berbeda sehingga
  # menebaknya lebih berisiko daripada melaporkan apa adanya.
  case "$_aksi" in
    enable|disable) return 0 ;;
    is-active) service "$_svc" status >/dev/null 2>&1 && echo active || echo inactive ;;
    is-enabled) echo unknown ;;
    *) service "$_svc" "$_aksi" 2>&1 ;;
  esac
}
`
