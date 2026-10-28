/// Pemformatan angka yang dipakai kartu grid DAN halaman detail.
///
/// Dikumpulkan di satu berkas karena sebelumnya keduanya punya salinan
/// sendiri: dua rumus yang sama persis, yang berarti satu perbaikan hanya
/// mengenai setengah layar.
library;

/// Pembagian yang aman dari total nol.
///
/// Agent mengirim nol untuk field yang belum terkumpul, dan tanpa penjagaan
/// ini bar-nya menampilkan NaN — yang di Flutter bukan bar kosong melainkan
/// pengecualian saat melukis.
double ratio(num used, num total) => total <= 0 ? 0 : used / total;

/// Persentase, atau tanda hubung kalau totalnya belum diketahui.
String pct(num used, num total) =>
    total <= 0 ? '—' : '${(used / total * 100).toStringAsFixed(0)}%';

/// "16,0 / 503,0 GB", atau tanda hubung kalau belum terbaca.
String gbPair(double used, double total) => total <= 0
    ? '—'
    : '${used.toStringAsFixed(1)} / ${total.toStringAsFixed(1)} GB';

/// Uptime dalam satuan terbesar yang masih berarti.
///
/// Dua satuan, tidak lebih: "8j 22m" cukup untuk menjawab "sejak kapan",
/// sementara "8 jam 22 menit 7 detik" hanya menambah panjang di kartu yang
/// lebarnya satu kolom grid.
String formatUptime(int seconds, {bool long = false}) {
  if (seconds <= 0) return '—';
  final d = seconds ~/ 86400;
  final h = (seconds % 86400) ~/ 3600;
  final m = (seconds % 3600) ~/ 60;
  if (long) {
    if (d > 0) return '$d hari $h jam';
    if (h > 0) return '$h jam $m menit';
    return '$m menit';
  }
  if (d > 0) return '${d}h ${h}j';
  if (h > 0) return '${h}j ${m}m';
  return '${m}m';
}

/// Ukuran berkas dalam satuan terbesar yang masih di bawah empat digit.
String formatBytes(int bytes) {
  if (bytes < 1024) return '$bytes B';
  const satuan = ['KB', 'MB', 'GB', 'TB'];
  var nilai = bytes / 1024;
  var i = 0;
  while (nilai >= 1024 && i < satuan.length - 1) {
    nilai /= 1024;
    i++;
  }
  // Satu desimal di bawah 10, tidak sama sekali di atasnya: "1,4 MB" berguna,
  // "847,3 MB" cuma lebih panjang.
  return nilai < 10
      ? '${nilai.toStringAsFixed(1)} ${satuan[i]}'
      : '${nilai.toStringAsFixed(0)} ${satuan[i]}';
}
