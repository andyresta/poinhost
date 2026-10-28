import 'package:flutter/material.dart';

/// Tema app.
///
/// Satu seed color, sisanya diturunkan Material 3. Alasannya bukan kemalasan:
/// palet yang diturunkan menjamin kontras teks terhadap setiap permukaan di
/// mode terang MAUPUN gelap, dan daftar warna yang dipilih tangan satu per
/// satu adalah cara paling umum sebuah app jadi tidak terbaca di salah satu
/// mode tanpa pembuatnya sadar.
const seedColor = Color(0xFF2F6FED);

ThemeData buildTheme(Brightness brightness) {
  final scheme = ColorScheme.fromSeed(
    seedColor: seedColor,
    brightness: brightness,
  );

  return ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    scaffoldBackgroundColor: scheme.surface,
    appBarTheme: AppBarTheme(
      backgroundColor: scheme.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      centerTitle: false,
      titleTextStyle: TextStyle(
        color: scheme.onSurface,
        fontSize: 20,
        fontWeight: FontWeight.w600,
      ),
    ),
    cardTheme: CardThemeData(
      elevation: 0,
      color: scheme.surfaceContainerLow,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: scheme.outlineVariant.withValues(alpha: 0.5)),
      ),
      margin: EdgeInsets.zero,
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: scheme.surfaceContainerHighest.withValues(alpha: 0.4),
      isDense: true,
      // Label mengambang DIMATIKAN, dan labelnya ditulis sebagai teks
      // tersendiri di atas kotaknya (lihat LabeledField).
      //
      // Dengan OutlineInputBorder tanpa garis, label yang mengambang naik ke
      // tempat garis itu seharusnya ada — dan karena garisnya tidak digambar,
      // yang terlihat adalah label menabrak tepi atas kotak. Mengatur
      // posisinya tidak menyelesaikan apa pun: masalahnya label itu memang
      // milik rancangan yang punya garis.
      floatingLabelBehavior: FloatingLabelBehavior.never,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(10),
        borderSide: BorderSide.none,
      ),
      contentPadding: const EdgeInsets.symmetric(horizontal: 13, vertical: 12),
      hintStyle: TextStyle(color: scheme.onSurfaceVariant),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        minimumSize: const Size.fromHeight(44),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(10)),
        textStyle: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
      ),
    ),
    dividerTheme: DividerThemeData(
      color: scheme.outlineVariant.withValues(alpha: 0.5),
      space: 1,
      thickness: 1,
    ),
  );
}

/// Warna untuk tiap keadaan koneksi.
///
/// Hijau/kuning/merah TIDAK dipakai sendirian di mana pun — selalu
/// berpasangan dengan teks atau ikon. Sekitar satu dari dua belas laki-laki
/// tidak bisa membedakan merah dan hijau, dan "titik berwarna" tanpa label
/// adalah cara tercepat membuat layar status tidak berguna bagi mereka.
class GaugeColors {
  const GaugeColors._();

  /// Warna cincin menurut seberapa penuh.
  ///
  /// Ambangnya sengaja tinggi: disk 60% terpakai itu normal, dan layar yang
  /// menguning sejak setengah penuh mengajari pemiliknya mengabaikan warna —
  /// sehingga 95% pun tidak lagi menarik perhatian.
  ///
  /// Sama seperti [StatusColors], warna ini tidak pernah berdiri sendiri:
  /// angkanya selalu tertulis di tengah cincin.
  static Color of(BuildContext context, double fraction) {
    final scheme = Theme.of(context).colorScheme;
    if (fraction >= 0.9) return scheme.error;
    if (fraction >= 0.75) return const Color(0xFFD98A00);
    return scheme.primary;
  }
}

class StatusColors {
  const StatusColors._();

  static Color of(BuildContext context, String? connection) {
    final scheme = Theme.of(context).colorScheme;
    switch (connection) {
      case 'online':
        return const Color(0xFF2E9E5B);
      case 'reconnecting':
        return const Color(0xFFD98A00);
      case 'offline':
        return scheme.error;
      default:
        return scheme.outline;
    }
  }
}
