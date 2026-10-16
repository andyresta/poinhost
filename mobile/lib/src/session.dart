import 'dart:async';
import 'dart:io' show Platform;

import 'package:flutter/foundation.dart';

import 'api/agent_client.dart';
import 'api/agent_exception.dart';
import 'api/models.dart';
import 'storage/credential_store.dart';

/// Di mana app berada dalam alur pendaftaran.
enum SessionPhase {
  /// Membaca kredensial tersimpan.
  loading,

  /// Belum pernah mendaftar, atau tokennya ditolak.
  needsEnrollment,

  /// Sudah mendaftar, menunggu disetujui dari app desktop poinhost.
  awaitingApproval,

  /// Perangkat ini dicabut.
  revoked,

  /// Disetujui; [Session.snapshot] berisi keadaan server.
  ready,
}

/// Seluruh keadaan app.
///
/// Satu ChangeNotifier, bukan state management berlapis: app ini punya tiga
/// layar dan satu sumber data. Menambahkan lapisan di atasnya hanya
/// menyembunyikan betapa sedikit yang sebenarnya terjadi di sini.
class Session extends ChangeNotifier {
  Session({required CredentialStore store, AgentClient Function(Uri)? clientFor})
      : _store = store,
        _clientFor = clientFor ?? ((url) => AgentClient(baseUrl: url));

  final CredentialStore _store;
  final AgentClient Function(Uri) _clientFor;

  /// Seberapa sering layar utama menyegarkan dirinya.
  ///
  /// Agent membaca metrik dari snapshot yang sudah hangat, tanpa round-trip
  /// SSH, jadi polling murah di sisi server. Yang membatasi bukan server
  /// melainkan baterai dan kuota di sisi HP.
  static const refreshInterval = Duration(seconds: 20);

  SessionPhase phase = SessionPhase.loading;
  ServerSnapshot? snapshot;

  /// Pesan galat terakhir yang layak ditampilkan, kalau ada.
  String? message;

  Credentials? _credentials;
  AgentClient? _client;
  Timer? _timer;

  Uri? get baseUrl => _credentials?.baseUrl;

  /// Dipanggil sekali saat app start.
  Future<void> restore() async {
    _credentials = await _store.read();
    if (_credentials == null) {
      _set(SessionPhase.needsEnrollment);
      return;
    }
    _client = _clientFor(_credentials!.baseUrl);
    await refresh();
  }

  /// Langkah 1-2 alurnya: tukar token pendaftaran dengan token perangkat.
  ///
  /// Setelah ini perangkatnya MENUNGGU — tokennya belum membuka apa pun
  /// sampai seseorang menyetujuinya dari app desktop.
  Future<void> enroll({
    required Uri baseUrl,
    required String enrollToken,
    required String deviceName,
  }) async {
    final client = _clientFor(baseUrl);
    final enrollment = await client.register(
      enrollToken: enrollToken,
      deviceName: deviceName,
      platform: _platformName(),
    );

    _credentials = Credentials(
      baseUrl: baseUrl,
      deviceToken: enrollment.token,
    );
    // Disimpan SEBELUM permintaan berikutnya. Token ini dikembalikan agent
    // satu kali saja — agent cuma menyimpan hash-nya — jadi kehilangan ia
    // di sini berarti perangkat yang menggantung: terdaftar di server,
    // menunggu persetujuan, tapi HP-nya tidak punya tokennya lagi.
    await _store.write(_credentials!);

    _client?.close();
    _client = client;
    await refresh();
  }

  /// Mengambil keadaan server sekali.
  Future<void> refresh() async {
    final client = _client;
    if (client == null) {
      _set(SessionPhase.needsEnrollment);
      return;
    }

    try {
      snapshot = await client.server(deviceToken: _credentials!.deviceToken);
      message = null;
      _set(SessionPhase.ready);
      _startPolling();
    } on AgentException catch (e) {
      message = e.message;
      switch (e.failure) {
        case AgentFailure.pendingApproval:
          _set(SessionPhase.awaitingApproval);
          // Terus memeriksa: persetujuan datang dari layar LAIN, dan tidak
          // ada yang memberi tahu HP saat itu terjadi.
          _startPolling();
        case AgentFailure.accessRevoked:
          _stopPolling();
          _set(SessionPhase.revoked);
        case AgentFailure.tokenRejected:
          // Token tidak dikenal lagi — perangkatnya dihapus dari daftar.
          // Kredensial dibuang supaya app tidak terjebak memanggil dengan
          // token yang tidak akan pernah diterima.
          await forget();
        case AgentFailure.agentNotReady:
        case AgentFailure.unreachable:
        case AgentFailure.unexpected:
          // Keadaan sementara: jangan buang kredensial, jangan pindah
          // layar. Yang sudah tergambar tetap tergambar, dengan pesannya.
          notifyListeners();
      }
    }
  }

  /// Melupakan perangkat ini di sisi HP, dan kembali ke layar pendaftaran.
  ///
  /// Tidak menyentuh daftar perangkat di agent: pencabutan adalah hak
  /// pemilik server, bukan hak HP yang dicabut.
  Future<void> forget() async {
    _stopPolling();
    await _store.clear();
    _credentials = null;
    _client?.close();
    _client = null;
    snapshot = null;
    _set(SessionPhase.needsEnrollment);
  }

  void _startPolling() {
    _timer ??= Timer.periodic(refreshInterval, (_) => refresh());
  }

  void _stopPolling() {
    _timer?.cancel();
    _timer = null;
  }

  void _set(SessionPhase next) {
    phase = next;
    notifyListeners();
  }

  @override
  void dispose() {
    _stopPolling();
    _client?.close();
    super.dispose();
  }
}

String _platformName() {
  if (kIsWeb) return 'web';
  if (Platform.isAndroid) return 'android';
  if (Platform.isIOS) return 'ios';
  return Platform.operatingSystem;
}
