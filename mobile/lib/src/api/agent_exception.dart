/// Kenapa panggilan ke agent gagal.
///
/// Dibedakan satu per satu, bukan dilebur jadi satu "error", karena agent
/// memang membedakannya — dan tiap sebab menuntut hal berbeda dari pemilik
/// HP. [pendingApproval] berarti tunggu dan jangan lakukan apa-apa,
/// [tokenRejected] berarti daftar ulang, [accessRevoked] berarti aksesnya
/// memang diambil. Satu layar "gagal" untuk ketiganya membuat pemiliknya
/// tidak pernah tahu yang mana.
enum AgentFailure {
  /// Token perangkat tidak dikenal agent (401). Perlu daftar ulang.
  tokenRejected,

  /// Perangkat sudah terdaftar tapi belum disetujui (403, status pending).
  pendingApproval,

  /// Perangkat dicabut dari app desktop (403, status revoked).
  accessRevoked,

  /// Pendaftaran belum dibuka, atau agent belum siap (503).
  agentNotReady,

  /// Tidak sampai ke agent sama sekali: DNS, jaringan seluler putus, TLS.
  unreachable,

  /// Agent menjawab, tapi bukan yang bisa dipahami versi app ini.
  unexpected,
}

/// Galat dari agent, beserta pesan yang sudah layak ditampilkan.
class AgentException implements Exception {
  AgentException(this.failure, this.message, {this.statusCode});

  final AgentFailure failure;

  /// Pesan dari agent kalau ada, kalau tidak pesan buatan app.
  ///
  /// Agent mengirim pesannya dalam Bahasa Indonesia, jadi bisa langsung
  /// ditampilkan — tapi JANGAN diandalkan untuk mengambil keputusan di kode;
  /// itulah gunanya [failure].
  final String message;

  final int? statusCode;

  @override
  String toString() => 'AgentException($failure, $statusCode): $message';
}
