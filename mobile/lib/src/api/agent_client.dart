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

  /// Daftar unit systemd di server.
  Future<ServiceList> services({required String deviceToken}) async {
    final json = await _send('GET', '/v1/services', token: deviceToken);
    return ServiceList.fromJson(json);
  }

  /// Menjalankan satu aksi pada satu service.
  ///
  /// Aksinya dibatasi agent, bukan di sini: daftar tertutup di sisi server
  /// adalah yang menentukan, dan menyalinnya ke app hanya menciptakan dua
  /// daftar yang bisa berbeda.
  Future<ServiceInfo> serviceAction({
    required String deviceToken,
    required String name,
    required String action,
  }) async {
    final json = await _send(
      'POST',
      '/v1/services/$name/$action',
      token: deviceToken,
    );
    return ServiceInfo.fromJson(json);
  }

  /// Isi satu direktori di server.
  Future<FileListing> files({
    required String deviceToken,
    required String path,
  }) async {
    final json = await _send(
      'GET',
      '/v1/files?path=${Uri.encodeQueryComponent(path)}',
      token: deviceToken,
    );
    return FileListing.fromJson(json);
  }

  /// Isi satu file teks.
  Future<FileText> fileContent({
    required String deviceToken,
    required String path,
  }) async {
    final json = await _send(
      'GET',
      '/v1/files/content?path=${Uri.encodeQueryComponent(path)}',
      token: deviceToken,
    );
    return FileText.fromJson(json);
  }

  /// Menimpa isi satu berkas, membuatnya kalau belum ada.
  ///
  /// Dipakai untuk menyimpan hasil edit DAN untuk mengunggah: keduanya
  /// operasi yang sama, "tulis bita ini ke path ini".
  Future<void> writeFile({
    required String deviceToken,
    required String path,
    required List<int> bytes,
  }) async {
    await _send(
      'PUT',
      '/v1/files/content?path=${Uri.encodeQueryComponent(path)}',
      token: deviceToken,
      rawBody: bytes,
    );
  }

  Future<void> renameFile({
    required String deviceToken,
    required String oldPath,
    required String newPath,
  }) async {
    await _send(
      'POST',
      '/v1/files/rename',
      token: deviceToken,
      body: {'oldPath': oldPath, 'newPath': newPath},
    );
  }

  Future<void> deleteFiles({
    required String deviceToken,
    required List<String> paths,
  }) async {
    await _send(
      'POST',
      '/v1/files/delete',
      token: deviceToken,
      body: {'paths': paths},
    );
  }

  /// Seluruh vhost nginx di server.
  Future<WebsiteListing> websites({required String deviceToken}) async {
    final json = await _send('GET', '/v1/websites', token: deviceToken);
    return WebsiteListing.fromJson(json);
  }

  /// Keadaan sertifikat satu domain.
  ///
  /// Dipisah dari daftarnya karena agent harus MEMBACA berkas sertifikat
  /// untuk menjawabnya — satu round-trip lagi per domain.
  Future<WebsiteSSL> websiteSSL({
    required String deviceToken,
    required String domain,
  }) async {
    final json = await _send(
      'GET',
      '/v1/websites/${Uri.encodeComponent(domain)}/ssl',
      token: deviceToken,
    );
    return WebsiteSSL.fromJson(json);
  }

  Future<void> setWebsiteEnabled({
    required String deviceToken,
    required String domain,
    required bool enabled,
  }) async {
    await _send(
      'POST',
      '/v1/websites/${Uri.encodeComponent(domain)}/'
          '${enabled ? 'enable' : 'disable'}',
      token: deviceToken,
    );
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
    List<int>? rawBody,
  }) async {
    final request = http.Request(method, baseUrl.resolve(path));
    if (token != null && token.isNotEmpty) {
      request.headers['Authorization'] = 'Bearer $token';
    }
    if (body != null) {
      request.headers['Content-Type'] = 'application/json';
      request.body = jsonEncode(body);
    }
    // Bita mentah, bukan JSON: berkas yang diunggah bisa biner, dan string
    // JSON harus UTF-8 yang sah — membungkusnya akan merusak gambar dan
    // arsip tanpa satu pun pesan galat.
    if (rawBody != null) {
      request.headers['Content-Type'] = 'application/octet-stream';
      request.bodyBytes = rawBody;
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
    } on http.ClientException catch (e) {
      // Ketiga catch di atas semuanya tipe dart:io, dan di web TIDAK ADA
      // satu pun yang pernah terlempar: package:http memakai BrowserClient
      // di sana, dan semua kegagalan jaringan — termasuk CORS yang ditolak —
      // keluar sebagai ClientException. Tanpa baris ini galatnya lolos dari
      // seluruh penerjemahan di berkas ini dan muncul sebagai app yang diam.
      //
      // Di HP ia tetap terpakai: sambungan yang putus di tengah jawaban
      // sampai ke sini, bukan ke SocketException.
      throw AgentException(
        AgentFailure.unreachable,
        'Tidak bisa menghubungi agent di ${baseUrl.host}: ${e.message}',
      );
    }

    return _parse(response);
  }

  Map<String, dynamic> _parse(http.Response response) {
    // 404 diperiksa SEBELUM mencoba membaca JSON. Agent yang lebih tua
    // membalas 404 teks polos dari mux-nya, yang akan jatuh ke cabang
    // "bukan JSON" di bawah dan terbaca sebagai "ini bukan agent poinhost" —
    // menuduh alamatnya salah, padahal alamatnya benar dan agentnya cuma
    // belum punya modul ini.
    if (response.statusCode == 404) {
      throw AgentException(
        AgentFailure.moduleMissing,
        'Modul ini belum ada di agent versi ini — perbarui agent dari '
        'poinhost di desktop.',
        statusCode: 404,
      );
    }

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
        // Agent mengirim `status` justru supaya HP bisa membedakan ketiganya,
        // jadi itu yang dibaca — bukan teks pesannya, yang boleh berubah.
        // Ketiganya menuntut hal berbeda: menunggu berarti diam, dicabut
        // berarti aksesnya diambil, readOnly berarti minta satu izin yang
        // memang belum pernah diberikan.
        throw AgentException(
          switch (json['status']) {
            'revoked' => AgentFailure.accessRevoked,
            'readOnly' => AgentFailure.controlNotAllowed,
            _ => AgentFailure.pendingApproval,
          },
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
