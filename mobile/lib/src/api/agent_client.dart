import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:http/http.dart' as http;

import 'agent_exception.dart';
import 'models.dart';

/// Klien untuk REST agent poinhost.
///
/// Seluruh logika yang dimiliki app ini ada di berkas ini, dan sengaja tidak
/// lebih dari: kirim permintaan, terjemahkan jawabannya. Tidak ada SSH, tidak
/// ada kriptografi, tidak ada orkestrasi — semua itu tinggal di agent. Itu
/// bukan kebetulan melainkan batasan yang ditetapkan pemilik proyek, dan
/// alasannya ada di mobile-control.md.
///
/// Dua endpoint saja:
///   POST /v1/devices  — tukar token pendaftaran dengan token perangkat
///   GET  /v1/server   — identitas mesin + metriknya, satu permintaan
class AgentClient {
  AgentClient({required this.baseUrl, http.Client? httpClient})
      : _http = httpClient ?? http.Client();

  /// Akar URL agent, mis. `https://agent.example.com`.
  final Uri baseUrl;

  final http.Client _http;

  static const _timeout = Duration(seconds: 15);

  /// Menukar token pendaftaran dengan token milik perangkat ini.
  ///
  /// Hasilnya selalu berstatus menunggu — tokennya belum membuka apa pun
  /// sampai perangkatnya disetujui dari app desktop poinhost. Itu yang
  /// membuat token pendaftaran aman diketik ulang atau dikirim lewat chat.
  Future<Enrollment> register({
    required String enrollToken,
    required String deviceName,
    required String platform,
  }) async {
    final json = await _send(
      'POST',
      '/v1/devices',
      token: enrollToken,
      body: {'name': deviceName, 'platform': platform},
    );
    return Enrollment.fromJson(json);
  }

  /// Membaca keadaan server. Inilah yang dipanggil berulang oleh layar utama.
  ///
  /// Satu permintaan memuat identitas mesin DAN metriknya, karena di jaringan
  /// seluler dua permintaan berarti dua kali menunggu.
  Future<ServerSnapshot> server({required String deviceToken}) async {
    final json = await _send('GET', '/v1/server', token: deviceToken);
    return ServerSnapshot.fromJson(json);
  }

  /// Memeriksa agent hidup, tanpa token.
  ///
  /// Berguna di layar pendaftaran: membedakan "URL-nya salah" dari "token
  /// saya salah" sebelum user menyalahkan tokennya.
  Future<bool> reachable() async {
    try {
      await _send('GET', '/healthz');
      return true;
    } on AgentException {
      return false;
    }
  }

  void close() => _http.close();

  Future<Map<String, dynamic>> _send(
    String method,
    String path, {
    String? token,
    Map<String, Object?>? body,
  }) async {
    final request = http.Request(method, baseUrl.resolve(path));
    if (token != null && token.isNotEmpty) {
      request.headers['Authorization'] = 'Bearer $token';
    }
    if (body != null) {
      request.headers['Content-Type'] = 'application/json';
      request.body = jsonEncode(body);
    }

    http.Response response;
    try {
      final streamed = await _http.send(request).timeout(_timeout);
      response = await http.Response.fromStream(streamed);
    } on TimeoutException {
      throw AgentException(
        AgentFailure.unreachable,
        'Agent tidak menjawab dalam ${_timeout.inSeconds} detik.',
      );
    } on SocketException catch (e) {
      throw AgentException(
        AgentFailure.unreachable,
        'Tidak bisa menghubungi agent: ${e.message}',
      );
    } on HandshakeException {
      // Dipisah dari SocketException: sertifikat yang belum terbit adalah
      // kegagalan yang sangat mungkin terjadi tepat setelah menerbitkan
      // agent, dan menyebutnya "tidak bisa terhubung" akan mengirim
      // pemiliknya memeriksa jaringan, bukan sertifikatnya.
      throw AgentException(
        AgentFailure.unreachable,
        'Sertifikat HTTPS agent tidak bisa diverifikasi.',
      );
    }

    return _parse(response);
  }

  Map<String, dynamic> _parse(http.Response response) {
    Map<String, dynamic> json = const {};
    if (response.body.trim().isNotEmpty) {
      try {
        final decoded = jsonDecode(response.body);
        if (decoded is Map<String, dynamic>) json = decoded;
      } on FormatException {
        // Bukan JSON sama sekali biasanya berarti yang menjawab BUKAN agent:
        // halaman galat nginx, portal wifi, atau domain yang menunjuk tempat
        // lain. Disebut apa adanya supaya tidak terbaca sebagai masalah token.
        throw AgentException(
          AgentFailure.unexpected,
          'Jawaban dari ${baseUrl.host} bukan dari agent poinhost.',
          statusCode: response.statusCode,
        );
      }
    }

    if (response.statusCode >= 200 && response.statusCode < 300) {
      return json;
    }

    final message = json['error'] is String
        ? json['error'] as String
        : 'Agent menjawab ${response.statusCode}.';

    switch (response.statusCode) {
      case 401:
        throw AgentException(AgentFailure.tokenRejected, message,
            statusCode: 401);
      case 403:
        // Agent mengirim `status` justru supaya HP bisa membedakan keduanya,
        // jadi itu yang dibaca — bukan teks pesannya, yang boleh berubah.
        final revoked = json['status'] == 'revoked';
        throw AgentException(
          revoked ? AgentFailure.accessRevoked : AgentFailure.pendingApproval,
          message,
          statusCode: 403,
        );
      case 503:
        throw AgentException(AgentFailure.agentNotReady, message,
            statusCode: 503);
      default:
        throw AgentException(AgentFailure.unexpected, message,
            statusCode: response.statusCode);
    }
  }
}
