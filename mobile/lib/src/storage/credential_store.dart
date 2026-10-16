import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Apa yang disimpan HP setelah mendaftar.
class Credentials {
  const Credentials({required this.baseUrl, required this.deviceToken});

  final Uri baseUrl;
  final String deviceToken;
}

/// Tempat token perangkat disimpan.
///
/// Keychain di iOS, EncryptedSharedPreferences di Android — bukan
/// SharedPreferences biasa. Token ini satu-satunya yang berdiri antara
/// siapa pun yang memegang HP ini dan data server, dan di Android
/// SharedPreferences biasa terbaca apa adanya di perangkat yang di-root
/// maupun di sebagian backup.
///
/// Dibungkus interface, bukan dipakai langsung, supaya test bisa berjalan
/// tanpa plugin platform — flutter_secure_storage butuh perangkat sungguhan.
abstract class CredentialStore {
  Future<Credentials?> read();
  Future<void> write(Credentials credentials);
  Future<void> clear();
}

class SecureCredentialStore implements CredentialStore {
  const SecureCredentialStore([
    this._storage = const FlutterSecureStorage(
      // first_unlock, bukan default yang menuntut perangkat tidak terkunci
      // saat dibaca: app ini menyegarkan dirinya di latar, dan nilai yang
      // tidak terbaca saat layar mati akan terlihat seperti token hilang.
      iOptions: IOSOptions(accessibility: KeychainAccessibility.first_unlock),
    ),
  ]);

  final FlutterSecureStorage _storage;

  static const _keyBaseUrl = 'agent.baseUrl';
  static const _keyToken = 'agent.deviceToken';

  @override
  Future<Credentials?> read() async {
    final url = await _storage.read(key: _keyBaseUrl);
    final token = await _storage.read(key: _keyToken);
    if (url == null || token == null || token.isEmpty) return null;

    final parsed = Uri.tryParse(url);
    if (parsed == null || !parsed.hasScheme) {
      // Tersimpan tapi tidak terbaca berarti app akan gagal di setiap
      // permintaan tanpa pernah menawarkan jalan keluar. Diperlakukan
      // seperti belum mendaftar, sehingga layarnya kembali ke pendaftaran.
      return null;
    }
    return Credentials(baseUrl: parsed, deviceToken: token);
  }

  @override
  Future<void> write(Credentials credentials) async {
    await _storage.write(
      key: _keyBaseUrl,
      value: credentials.baseUrl.toString(),
    );
    await _storage.write(key: _keyToken, value: credentials.deviceToken);
  }

  @override
  Future<void> clear() async {
    await _storage.delete(key: _keyBaseUrl);
    await _storage.delete(key: _keyToken);
  }
}

/// Penyimpanan di memori, untuk test dan untuk menjalankan app di desktop
/// saat mengembangkan tampilan.
class InMemoryCredentialStore implements CredentialStore {
  Credentials? _value;

  @override
  Future<Credentials?> read() async => _value;

  @override
  Future<void> write(Credentials credentials) async => _value = credentials;

  @override
  Future<void> clear() async => _value = null;
}
