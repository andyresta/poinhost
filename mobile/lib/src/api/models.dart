/// Bentuk respons agent, dipetakan satu arah: JSON -> objek Dart.
///
/// Tidak ada kode generator di sini. Kontraknya kecil dan tetap — dua
/// endpoint — dan parser tulis tangan membuat setiap keputusan soal field
/// yang hilang terlihat jelas, yang justru bagian paling penting: agent
/// MENGHILANGKAN field yang belum punya nilai, bukan mengirimnya sebagai nol.
library;

/// Keadaan koneksi agent ke mesinnya.
///
/// Bukan `ConnectionState`: Flutter sudah memakai nama itu untuk status
/// Stream-nya, dan dua nama sama yang ikut terbawa lewat
/// `package:flutter/material.dart` membuat setiap pemakaiannya ambigu.
enum AgentConnection {
  online,
  offline,
  reconnecting,

  /// Agent belum pernah menyentuh mesinnya sejak start.
  unknown;

  static AgentConnection parse(String? raw) {
    switch (raw) {
      case 'online':
        return AgentConnection.online;
      case 'offline':
        return AgentConnection.offline;
      case 'reconnecting':
        return AgentConnection.reconnecting;
      default:
        return AgentConnection.unknown;
    }
  }
}

/// Metrik satu server.
class ServerMetrics {
  const ServerMetrics({
    required this.connection,
    required this.osName,
    required this.hostname,
    required this.uptimeSeconds,
    required this.cpuCores,
    required this.cpuUsedPct,
    required this.memUsedMB,
    required this.memTotalMB,
    required this.diskUsedGB,
    required this.diskTotalGB,
    required this.stale,
    this.checkedAt,
    this.error,
  });

  final AgentConnection connection;
  final String osName;
  final String hostname;
  final int uptimeSeconds;
  final int cpuCores;
  final double cpuUsedPct;
  final int memUsedMB;
  final int memTotalMB;
  final double diskUsedGB;
  final double diskTotalGB;

  /// Angkanya ada, tapi sudah kedaluwarsa — agent belum berhasil
  /// memperbaruinya. Ditampilkan sebagai angka pudar, bukan disembunyikan:
  /// angka lama yang ditandai lebih berguna daripada layar kosong.
  final bool stale;

  final DateTime? checkedAt;

  /// Apakah angkanya pernah benar-benar diukur.
  ///
  /// Agent menghilangkan seluruh field numerik selama belum pernah memprobe
  /// mesinnya, dan hanya mengirim status koneksinya. Tanpa pembedaan ini
  /// layar menggambar CPU 0%, RAM 0,0/0,0 GB, uptime 0 menit — yang terbaca
  /// persis seperti server sehat yang sedang menganggur.
  bool get collected => checkedAt != null;

  /// Kenapa pengambilan metrik terakhir gagal, kalau gagal.
  final String? error;

  static ServerMetrics fromJson(Map<String, dynamic> json) {
    return ServerMetrics(
      connection: AgentConnection.parse(json['connection'] as String?),
      osName: _str(json['osName']),
      hostname: _str(json['hostname']),
      uptimeSeconds: _int(json['uptimeSeconds']),
      cpuCores: _int(json['cpuCores']),
      cpuUsedPct: _double(json['cpuUsedPct']),
      memUsedMB: _int(json['memUsedMB']),
      memTotalMB: _int(json['memTotalMB']),
      diskUsedGB: _double(json['diskUsedGB']),
      diskTotalGB: _double(json['diskTotalGB']),
      stale: json['stale'] == true,
      checkedAt: _time(json['checkedAt']),
      error: _strOrNull(json['error']),
    );
  }
}

/// Jawaban `GET /v1/server`: identitas mesin, metriknya, dan versi agent.
class ServerSnapshot {
  const ServerSnapshot({
    required this.id,
    required this.name,
    required this.host,
    required this.agentVersion,
    required this.agentUptimeSeconds,
    this.metrics,
  });

  final String id;
  final String name;
  final String host;
  final String agentVersion;
  final int agentUptimeSeconds;

  /// null selama agent belum pernah berhasil mengumpulkan metrik.
  ///
  /// Agent sengaja MENGHILANGKAN blok ini alih-alih mengirim nol, supaya
  /// layar tidak menampilkan CPU 0% dan disk 0 GB — yang terlihat persis
  /// seperti server sehat yang menganggur.
  final ServerMetrics? metrics;

  static ServerSnapshot fromJson(Map<String, dynamic> json) {
    final status = json['status'];
    return ServerSnapshot(
      id: _str(json['id']),
      name: _str(json['name']),
      host: _str(json['host']),
      agentVersion: _str(json['version']),
      agentUptimeSeconds: _int(json['uptimeSeconds']),
      metrics: status is Map<String, dynamic>
          ? ServerMetrics.fromJson(status)
          : null,
    );
  }
}

/// Jawaban `POST /v1/devices`: token milik perangkat ini.
class Enrollment {
  const Enrollment({
    required this.deviceId,
    required this.name,
    required this.token,
    required this.status,
  });

  final String deviceId;
  final String name;

  /// Dikembalikan SATU KALI. Agent hanya menyimpan hash-nya, jadi token ini
  /// tidak bisa diminta lagi — hilang berarti daftar ulang.
  final String token;

  /// Selalu `pending` saat pendaftaran: token di atas belum membuka apa pun
  /// sampai perangkatnya disetujui dari app desktop.
  final String status;

  static Enrollment fromJson(Map<String, dynamic> json) {
    return Enrollment(
      deviceId: _str(json['deviceId']),
      name: _str(json['name']),
      token: _str(json['token']),
      status: _str(json['status']),
    );
  }
}

String _str(Object? v) => v is String ? v : '';

String? _strOrNull(Object? v) =>
    v is String && v.trim().isNotEmpty ? v : null;

int _int(Object? v) => v is num ? v.toInt() : 0;

double _double(Object? v) => v is num ? v.toDouble() : 0;

DateTime? _time(Object? v) =>
    v is String ? DateTime.tryParse(v)?.toLocal() : null;
