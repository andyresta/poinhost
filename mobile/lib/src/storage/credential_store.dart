import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Satu server yang sudah didaftarkan dari HP ini.
class Credentials {
  const Credentials({
    required this.id,
    required this.baseUrl,
    required this.deviceToken,
    this.label = '',
  });

  /// ID lokal, dibuat HP. Bukan id server dari agent: server yang belum
  /// pernah berhasil dibaca belum punya id itu, sementara kartunya sudah
  /// harus tampil di grid dengan tombol hapusnya sendiri.
  final String id;

  final Uri baseUrl;
  final String deviceToken;

  /// Nama yang diketik user saat menambahkan. Dipakai sampai agent
  /// menyebutkan nama server sebenarnya.
  final String label;

  Map<String, Object?> toJson() => {
        'id': id,
        'baseUrl': baseUrl.toString(),
        'deviceToken': deviceToken,
        'label': label,
      };

  static Credentials? fromJson(Map<String, Object?> json) {
    final url = Uri.tryParse(json['baseUrl'] as String? ?? '');
    final token = json['deviceToken'] as String? ?? '';
    final id = json['id'] as String? ?? '';
    // Entri yang tidak lengkap DIBUANG, bukan dipakai apa adanya: entri
    // tanpa token akan gagal di setiap permintaan tanpa pernah menawarkan
    // jalan keluar selain menghapusnya.
    if (id.isEmpty || token.isEmpty || url == null || !url.hasScheme) {
      return null;
    }
    return Credentials(
      id: id,
      baseUrl: url,
      deviceToken: token,
      label: json['label'] as String? ?? '',
    );
  }
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
  Future<List<Credentials>> readAll();
  Future<void> writeAll(List<Credentials> list);
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

  /// Seluruh daftar disimpan di SATU kunci sebagai JSON.
  ///
  /// Bukan satu kunci per server: keychain tidak punya operasi "baca semua
  /// yang berawalan X" yang bisa diandalkan lintas platform, jadi daftar
  /// per-kunci menuntut kunci indeks tersendiri — yang bisa tidak sinkron
  /// dengan isinya dan meninggalkan token yatim yang tidak terlihat
  /// siapa pun.
  static const _key = 'agent.connections';

  @override
  Future<List<Credentials>> readAll() async {
    final raw = await _storage.read(key: _key);
    if (raw == null || raw.trim().isEmpty) return const [];

    try {
      final decoded = jsonDecode(raw);
      if (decoded is! List) return const [];
      return decoded
          .whereType<Map<String, Object?>>()
          .map(Credentials.fromJson)
          .whereType<Credentials>()
          .toList();
    } on FormatException {
      // Isi yang rusak diperlakukan sebagai kosong. Melempar galat di sini
      // berarti app tidak bisa dibuka sama sekali, tanpa cara memperbaiki
      // dari dalam app.
      return const [];
    }
  }

  @override
  Future<void> writeAll(List<Credentials> list) async {
    if (list.isEmpty) {
      await _storage.delete(key: _key);
      return;
    }
    await _storage.write(
      key: _key,
      value: jsonEncode(list.map((c) => c.toJson()).toList()),
    );
  }
}

/// Penyimpanan di memori, untuk test dan untuk menjalankan app di desktop
/// atau web saat mengembangkan tampilan.
class InMemoryCredentialStore implements CredentialStore {
  InMemoryCredentialStore([List<Credentials> awal = const []])
      : _list = List.of(awal);

  List<Credentials> _list;

  @override
  Future<List<Credentials>> readAll() async => List.of(_list);

  @override
  Future<void> writeAll(List<Credentials> list) async => _list = List.of(list);
}
