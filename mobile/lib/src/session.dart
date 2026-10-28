import 'dart:async';
import 'dart:io' show Platform;
import 'dart:math';

import 'package:flutter/foundation.dart';

import 'api/agent_client.dart';
import 'api/agent_exception.dart';
import 'api/models.dart';
import 'storage/credential_store.dart';

/// Keadaan satu server di grid.
enum ServerPhase {
  /// Belum pernah dibaca sejak app dibuka.
  loading,

  /// Terdaftar, menunggu disetujui dari app desktop poinhost.
  awaitingApproval,

  /// Perangkat ini dicabut di server itu.
  revoked,

  /// Token tidak dikenal lagi — perangkatnya dihapus dari daftar di sana.
  rejected,

  /// Tidak sampai ke agent: jaringan, DNS, TLS, atau agent mati.
  unreachable,

  /// Agent MENJAWAB, tapi belum bisa melayani — pendaftaran belum dibuka,
  /// atau ia belum mengelola mesinnya sendiri.
  ///
  /// Dibedakan dari [unreachable] karena perbaikannya ada di tempat yang
  /// sama sekali berbeda: yang ini ditekan di panel Mobile Control, bukan
  /// dicari di jaringan.
  notReady,

  /// Terbaca; [ServerConnection.snapshot] berisi keadaannya.
  ready,
}

/// Satu agent yang dipegang app ini.
///
/// Satu agent mengurus satu server — itu batasan di sisi server, dan tidak
/// berubah. Yang menampung banyak di sini adalah HP-nya: tiap server yang
/// dipasangi agent jadi satu [ServerConnection], dan satu kartu di grid.
class ServerConnection extends ChangeNotifier {
  ServerConnection({
    required this.credentials,
    required AgentClient client,
  }) : _client = client;

  final Credentials credentials;
  final AgentClient _client;

  String get id => credentials.id;

  ServerPhase phase = ServerPhase.loading;
  ServerSnapshot? snapshot;

  /// Pesan terakhir yang layak ditampilkan, kalau ada.
  String? message;

  /// Nama yang ditampilkan di kartu.
  ///
  /// Nama dari agent menang kalau sudah ada — itu nama server yang
  /// sebenarnya. Sebelum itu, label yang diketik user saat menambahkan;
  /// kartu tanpa nama sama sekali membuat grid berisi beberapa server
  /// mustahil dibaca.
  String get displayName {
    final fromAgent = snapshot?.name.trim() ?? '';
    if (fromAgent.isNotEmpty) return fromAgent;
    if (credentials.label.trim().isNotEmpty) return credentials.label.trim();
    return credentials.baseUrl.host;
  }

  String get host => snapshot?.host.trim().isNotEmpty == true
      ? snapshot!.host
      : credentials.baseUrl.host;

  bool get isBusy => _inFlight;
  bool _inFlight = false;

  Future<void> refresh() async {
    // Satu permintaan pada satu waktu per server. Tanpa ini, tarik-segarkan
    // saat polling sedang jalan membuat dua jawaban berlomba dan yang lebih
    // tua bisa menang.
    if (_inFlight) return;
    _inFlight = true;
    notifyListeners();

    try {
      snapshot = await _client.server(deviceToken: credentials.deviceToken);
      message = null;
      phase = ServerPhase.ready;
    } on AgentException catch (e) {
      message = e.message;
      phase = switch (e.failure) {
        AgentFailure.pendingApproval => ServerPhase.awaitingApproval,
        AgentFailure.accessRevoked => ServerPhase.revoked,
        AgentFailure.tokenRejected => ServerPhase.rejected,
        AgentFailure.agentNotReady => ServerPhase.notReady,
        // Agent MENJAWAB, hanya versinya tertinggal. Lebih dekat ke "belum
        // siap" daripada ke "tidak terjangkau", dan pesannya menyebut
        // perbaikannya.
        AgentFailure.moduleMissing => ServerPhase.notReady,
        // controlNotAllowed tidak pernah datang dari /v1/server — endpoint
        // itu tidak memeriksa izin kendali sama sekali. Kalau toh muncul,
        // artinya agent menjawab sesuatu yang versi app ini tidak pahami,
        // dan itu persis arti unexpected. Pesannya tetap dari agent.
        AgentFailure.controlNotAllowed ||
        AgentFailure.unreachable ||
        AgentFailure.unexpected =>
          ServerPhase.unreachable,
      };
    } catch (e) {
      // Sama alasannya dengan layar tambah server: yang lolos dari
      // AgentClient tidak boleh membuat kartunya diam. Di sini akibatnya
      // lebih halus dan lebih lama — kartu itu akan tertinggal di "Memuat…"
      // selamanya, karena polling berikutnya gagal dengan cara yang sama.
      message = 'Gagal membaca server: $e';
      phase = ServerPhase.unreachable;
    } finally {
      _inFlight = false;
      notifyListeners();
    }
  }

  /// Daftar service di server ini.
  ///
  /// Token perangkat tidak pernah keluar dari kelas ini — layar cukup
  /// memanggil, tidak perlu memegang kredensialnya.
  Future<ServiceList> services() =>
      _client.services(deviceToken: credentials.deviceToken);

  Future<ServiceInfo> serviceAction(String name, String action) =>
      _client.serviceAction(
        deviceToken: credentials.deviceToken,
        name: name,
        action: action,
      );

  void close() => _client.close();
}

/// Seluruh server yang dipegang app ini.
class Workspace extends ChangeNotifier {
  Workspace({
    required CredentialStore store,
    AgentClient Function(Uri)? clientFor,
  })  : _store = store,
        _clientFor = clientFor ?? ((url) => AgentClient(baseUrl: url));

  final CredentialStore _store;
  final AgentClient Function(Uri) _clientFor;

  /// Seberapa sering grid menyegarkan dirinya.
  ///
  /// Agent membaca metrik dari snapshot yang sudah hangat, tanpa round-trip
  /// SSH, jadi polling murah di sisi server. Yang membatasi bukan server
  /// melainkan baterai dan kuota di sisi HP.
  static const refreshInterval = Duration(seconds: 20);

  final List<ServerConnection> _connections = [];
  List<ServerConnection> get connections => List.unmodifiable(_connections);

  bool get isEmpty => _connections.isEmpty;

  /// Selesai memuat kredensial tersimpan.
  bool restored = false;

  Timer? _timer;

  Future<void> restore() async {
    for (final c in await _store.readAll()) {
      _connections.add(_build(c));
    }
    restored = true;
    notifyListeners();

    // Dijalankan serentak: menunggu satu server yang mati berarti server
    // lain yang sehat ikut kosong sampai permintaan itu timeout.
    await Future.wait(_connections.map((c) => c.refresh()));
    _startPolling();
  }

  /// Menukar token pendaftaran dengan token milik perangkat ini, lalu
  /// menambahkan servernya ke grid.
  ///
  /// Hasilnya selalu MENUNGGU — tokennya belum membuka apa pun sampai
  /// seseorang menyetujuinya dari app desktop poinhost.
  Future<ServerConnection> add({
    required Uri baseUrl,
    required String enrollToken,
    required String label,
  }) async {
    final client = _clientFor(baseUrl);
    final enrollment = await client.register(
      enrollToken: enrollToken,
      deviceName: _deviceName(label),
      platform: _platformName(),
    );

    final creds = Credentials(
      id: _newId(),
      baseUrl: baseUrl,
      deviceToken: enrollment.token,
      label: label,
    );
    // Disimpan SEBELUM permintaan berikutnya. Token ini dikembalikan agent
    // satu kali saja — agent cuma menyimpan hash-nya — jadi kehilangan ia
    // di sini berarti perangkat yang menggantung: terdaftar di server,
    // menunggu persetujuan, tapi HP-nya tidak punya tokennya lagi.
    final conn = ServerConnection(credentials: creds, client: client);
    _connections.add(conn);
    await _store.writeAll(_connections.map((c) => c.credentials).toList());
    notifyListeners();

    await conn.refresh();
    _startPolling();
    return conn;
  }

  /// Melupakan satu server di sisi HP.
  ///
  /// Tidak menyentuh daftar perangkat di agent: pencabutan adalah hak
  /// pemilik server, bukan hak HP yang dicabut. Perangkatnya tetap terlihat
  /// di panel Mobile Control sampai dihapus dari sana.
  Future<void> remove(String id) async {
    final i = _connections.indexWhere((c) => c.id == id);
    if (i < 0) return;
    _connections.removeAt(i).close();
    await _store.writeAll(_connections.map((c) => c.credentials).toList());
    if (_connections.isEmpty) _stopPolling();
    notifyListeners();
  }

  ServerConnection? byId(String id) {
    for (final c in _connections) {
      if (c.id == id) return c;
    }
    return null;
  }

  /// Apakah penyegaran yang DIMINTA user sedang berjalan.
  ///
  /// Polling 20 detik sengaja tidak menyalakannya: ikon yang berputar
  /// sendiri tiap 20 detik bukan jawaban atas apa pun, cuma kebisingan —
  /// dan justru membuat putaran yang menjawab tekanan jari tidak terbaca.
  bool get isRefreshing => _refreshing;
  bool _refreshing = false;

  Future<void> refreshAll({bool manual = false}) async {
    if (!manual) {
      await Future.wait(_connections.map((c) => c.refresh()));
      return;
    }
    if (_refreshing) return;
    _refreshing = true;
    notifyListeners();
    try {
      await Future.wait(_connections.map((c) => c.refresh()));
    } finally {
      _refreshing = false;
      notifyListeners();
    }
  }

  ServerConnection _build(Credentials c) =>
      ServerConnection(credentials: c, client: _clientFor(c.baseUrl));

  void _startPolling() {
    if (_connections.isEmpty) return;
    _timer ??= Timer.periodic(refreshInterval, (_) => refreshAll());
  }

  void _stopPolling() {
    _timer?.cancel();
    _timer = null;
  }

  @override
  void dispose() {
    _stopPolling();
    for (final c in _connections) {
      c.close();
    }
    super.dispose();
  }
}

String _newId() {
  final r = Random.secure();
  return List.generate(8, (_) => r.nextInt(256).toRadixString(16).padLeft(2, '0'))
      .join();
}

/// Nama yang dikirim ke agent, yang akan dibaca pemilik server saat
/// menyetujui. Label dari user ikut, karena "Android" saja tidak cukup
/// untuk membedakan dua HP di daftar yang sama.
String _deviceName(String label) {
  final l = label.trim();
  return l.isEmpty ? _platformName() : l;
}

String _platformName() {
  if (kIsWeb) return 'web';
  if (Platform.isAndroid) return 'android';
  if (Platform.isIOS) return 'ios';
  return Platform.operatingSystem;
}
