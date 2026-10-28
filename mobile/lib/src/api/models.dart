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

/// Satu unit systemd di server.
class ServiceInfo {
  const ServiceInfo({
    required this.name,
    required this.description,
    required this.activeState,
    required this.subState,
    required this.running,
    required this.enabled,
  });

  final String name;
  final String description;

  /// "active" | "inactive" | "failed" | "activating" | dst.
  final String activeState;
  final String subState;

  /// Sedang berjalan SEKARANG.
  final bool running;

  /// Ikut jalan saat boot. Dimensi yang BERBEDA dari [running]: service bisa
  /// aktif sekarang tapi tidak autostart, atau sebaliknya.
  final bool enabled;

  bool get failed => activeState == 'failed';

  static ServiceInfo fromJson(Map<String, dynamic> json) {
    return ServiceInfo(
      name: _str(json['name']),
      description: _str(json['description']),
      activeState: _str(json['activeState']),
      subState: _str(json['subState']),
      running: json['running'] == true,
      enabled: json['enabled'] == true,
    );
  }
}

/// Jawaban `GET /v1/services`.
class ServiceList {
  const ServiceList({
    required this.services,
    required this.systemdAvailable,
    required this.canControl,
  });

  final List<ServiceInfo> services;

  /// Server tanpa systemd bukan kegagalan — modul ini cuma tidak berlaku.
  final bool systemdAvailable;

  /// Apakah perangkat INI boleh mengubah. Dikirim agent supaya layarnya tidak
  /// menampilkan tombol yang pasti ditolak.
  final bool canControl;

  static ServiceList fromJson(Map<String, dynamic> json) {
    final raw = json['services'];
    return ServiceList(
      services: raw is List
          ? raw
              .whereType<Map<String, dynamic>>()
              .map(ServiceInfo.fromJson)
              .toList()
          : const [],
      systemdAvailable: json['systemdAvailable'] == true,
      canControl: json['canControl'] == true,
    );
  }
}

/// Satu entri direktori di server.
class FileEntry {
  const FileEntry({
    required this.name,
    required this.path,
    required this.size,
    required this.isDir,
    required this.mode,
    required this.modTime,
  });

  final String name;
  final String path;
  final int size;
  final bool isDir;
  final String mode;
  final String modTime;

  static FileEntry fromJson(Map<String, dynamic> json) => FileEntry(
        name: _str(json['name']),
        path: _str(json['path']),
        size: _int(json['size']),
        isDir: json['isDir'] == true,
        mode: _str(json['mode']),
        modTime: _str(json['modTime']),
      );
}

/// Jawaban `GET /v1/files`.
class FileListing {
  const FileListing({
    required this.path,
    required this.parent,
    required this.entries,
  });

  final String path;
  final String parent;
  final List<FileEntry> entries;

  static FileListing fromJson(Map<String, dynamic> json) {
    final raw = json['entries'];
    return FileListing(
      path: _str(json['path']),
      parent: _str(json['parent']),
      entries: raw is List
          ? raw.whereType<Map<String, dynamic>>().map(FileEntry.fromJson).toList()
          : const [],
    );
  }
}

/// Jawaban `GET /v1/files/content`.
class FileText {
  const FileText({required this.path, required this.content, required this.size});

  final String path;
  final String content;
  final int size;

  static FileText fromJson(Map<String, dynamic> json) => FileText(
        path: _str(json['path']),
        content: _str(json['content']),
        size: _int(json['size']),
      );
}

/// Satu vhost nginx di server.
class WebsiteInfo {
  const WebsiteInfo({
    required this.domain,
    required this.parent,
    required this.isSubdomain,
    required this.root,
    required this.enabled,
    required this.phpVersion,
    required this.phpEnabled,
    required this.proxyTarget,
    required this.sslEnabled,
  });

  final String domain;
  final String parent;
  final bool isSubdomain;
  final String root;
  final bool enabled;
  final String phpVersion;
  final bool phpEnabled;
  final String proxyTarget;
  final bool sslEnabled;

  static WebsiteInfo fromJson(Map<String, dynamic> json) => WebsiteInfo(
        domain: _str(json['domain']),
        parent: _str(json['parent']),
        isSubdomain: json['isSubdomain'] == true,
        root: _str(json['root']),
        enabled: json['enabled'] == true,
        phpVersion: _str(json['phpVersion']),
        phpEnabled: json['phpEnabled'] == true,
        proxyTarget: _str(json['proxyTarget']),
        sslEnabled: json['sslEnabled'] == true,
      );
}

/// Jawaban `GET /v1/websites`.
class WebsiteListing {
  const WebsiteListing({
    required this.sites,
    required this.nginxInstalled,
    required this.nginxActive,
    required this.nginxVersion,
    required this.canControl,
  });

  final List<WebsiteInfo> sites;
  final bool nginxInstalled;
  final bool nginxActive;
  final String nginxVersion;
  final bool canControl;

  static WebsiteListing fromJson(Map<String, dynamic> json) {
    final raw = json['sites'];
    return WebsiteListing(
      sites: raw is List
          ? raw.whereType<Map<String, dynamic>>().map(WebsiteInfo.fromJson).toList()
          : const [],
      nginxInstalled: json['nginxInstalled'] == true,
      nginxActive: json['nginxActive'] == true,
      nginxVersion: _str(json['nginxVersion']),
      canControl: json['canControl'] == true,
    );
  }
}

/// Jawaban `GET /v1/websites/{domain}/ssl`.
class WebsiteSSL {
  const WebsiteSSL({
    required this.domain,
    required this.sslEnabled,
    required this.autoRenew,
    required this.certExists,
    required this.notAfter,
    required this.certDomains,
    required this.missingSans,
    this.expiresInDays,
  });

  final String domain;
  final bool sslEnabled;
  final bool autoRenew;
  final bool certExists;

  /// RFC3339 kalau agent bisa menguraikannya, apa adanya kalau tidak.
  final String notAfter;

  final List<String> certDomains;

  /// Domain yang terdaftar tapi BELUM tercakup sertifikat. Selama ini tidak
  /// kosong, HTTPS untuk domain itu tidak bekerja walau statusnya aktif.
  final List<String> missingSans;

  /// null berarti tanggalnya tidak terbaca — BUKAN nol, yang akan tampil
  /// seperti "kedaluwarsa hari ini".
  final int? expiresInDays;

  static WebsiteSSL fromJson(Map<String, dynamic> json) => WebsiteSSL(
        domain: _str(json['domain']),
        sslEnabled: json['sslEnabled'] == true,
        autoRenew: json['autoRenew'] == true,
        certExists: json['certExists'] == true,
        notAfter: _str(json['notAfter']),
        certDomains: _daftarTeks(json['certDomains']),
        missingSans: _daftarTeks(json['missingSans']),
        expiresInDays: json['expiresInDays'] is num
            ? (json['expiresInDays'] as num).toInt()
            : null,
      );
}

List<String> _daftarTeks(Object? v) =>
    v is List ? v.whereType<String>().toList() : const [];

/// Satu container Docker di server.
class ContainerInfo {
  const ContainerInfo({
    required this.id,
    required this.name,
    required this.image,
    required this.status,
    required this.state,
    required this.ports,
  });

  final String id;
  final String name;
  final String image;

  /// Teks panjang dari docker, mis. "Up 3 days" atau "Exited (0) 2 hours ago".
  final String status;

  /// Satu kata: running | exited | paused | restarting | created.
  final String state;

  final String ports;

  bool get running => state == 'running';

  static ContainerInfo fromJson(Map<String, dynamic> json) => ContainerInfo(
        id: _str(json['id']),
        name: _str(json['name']),
        image: _str(json['image']),
        status: _str(json['status']),
        state: _str(json['state']),
        ports: _str(json['ports']),
      );
}

/// Jawaban `GET /v1/docker`.
class ContainerListing {
  const ContainerListing({
    required this.containers,
    required this.dockerOk,
    required this.version,
    required this.canControl,
  });

  final List<ContainerInfo> containers;

  /// false berarti Docker tidak terpasang atau enginenya mati.
  final bool dockerOk;

  final String version;
  final bool canControl;

  static ContainerListing fromJson(Map<String, dynamic> json) {
    final raw = json['containers'];
    return ContainerListing(
      containers: raw is List
          ? raw
              .whereType<Map<String, dynamic>>()
              .map(ContainerInfo.fromJson)
              .toList()
          : const [],
      dockerOk: json['dockerOk'] == true,
      version: _str(json['version']),
      canControl: json['canControl'] == true,
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
