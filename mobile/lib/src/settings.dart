import 'package:flutter/material.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Tempat pilihan tampilan disimpan.
///
/// Memakai ULANG flutter_secure_storage, bukan menambah shared_preferences
/// demi satu nilai. Tema bukan rahasia, jadi secara makna ini agak berlebihan
/// — tapi plugin kedua berarti satu lagi permukaan build native yang bisa
/// patah, dan di proyek ini hal itu sudah terbukti mahal: build Windows
/// gagal gara-gara satu plugin menuntut ATL. Satu kali baca saat start tidak
/// akan terasa.
abstract class SettingsStore {
  Future<String?> read(String key);
  Future<void> write(String key, String value);
}

class SecureSettingsStore implements SettingsStore {
  const SecureSettingsStore([
    this._storage = const FlutterSecureStorage(
      iOptions: IOSOptions(accessibility: KeychainAccessibility.first_unlock),
    ),
  ]);

  final FlutterSecureStorage _storage;

  @override
  Future<String?> read(String key) => _storage.read(key: key);

  @override
  Future<void> write(String key, String value) =>
      _storage.write(key: key, value: value);
}

class InMemorySettingsStore implements SettingsStore {
  final Map<String, String> _values = {};

  @override
  Future<String?> read(String key) async => _values[key];

  @override
  Future<void> write(String key, String value) async => _values[key] = value;
}

/// Pilihan tampilan.
class AppSettings extends ChangeNotifier {
  AppSettings({required SettingsStore store}) : _store = store;

  final SettingsStore _store;

  static const _keyThemeMode = 'ui.themeMode';

  /// Bawaannya IKUT SISTEM, bukan gelap.
  ///
  /// App yang memaksakan temanya sendiri adalah app yang menyala terang di
  /// tangan orang yang sudah menyetel seluruh HP-nya gelap — dan sebaliknya.
  /// Yang memang ingin memaksa punya pilihannya di sidebar.
  ThemeMode mode = ThemeMode.system;

  Future<void> load() async {
    mode = _parse(await _store.read(_keyThemeMode));
    notifyListeners();
  }

  Future<void> setMode(ThemeMode next) async {
    if (next == mode) return;
    mode = next;
    notifyListeners();
    // Disimpan SETELAH layar berubah: menulis ke keychain butuh waktu, dan
    // menunggunya lebih dulu membuat tombol terasa macet.
    await _store.write(_keyThemeMode, next.name);
  }

  static ThemeMode _parse(String? raw) {
    for (final m in ThemeMode.values) {
      if (m.name == raw) return m;
    }
    return ThemeMode.system;
  }
}

/// Label tiap pilihan, dalam bahasa yang dipakai layar.
extension ThemeModeLabel on ThemeMode {
  String get label => switch (this) {
        ThemeMode.system => 'Follow system',
        ThemeMode.light => 'Light',
        ThemeMode.dark => 'Dark',
      };

  IconData get icon => switch (this) {
        ThemeMode.system => Icons.brightness_auto_outlined,
        ThemeMode.light => Icons.light_mode_outlined,
        ThemeMode.dark => Icons.dark_mode_outlined,
      };
}
