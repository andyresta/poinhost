import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../api/models.dart';
import '../format.dart';
import '../session.dart';
import '../theme.dart';

/// Metrik satu server: modul pertama di menu servernya.
///
/// Dipisah dari layar menu supaya tiap modul berikutnya — Website, Files,
/// Terminal — bisa datang sebagai layarnya sendiri, bukan sebagai cabang
/// baru di dalam satu layar yang terus membesar.
class ServerMetricsScreen extends StatelessWidget {
  const ServerMetricsScreen({super.key, required this.connection});

  final ServerConnection connection;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: connection,
      builder: (context, _) {
        final theme = Theme.of(context);
        final snapshot = connection.snapshot;
        final metrics = snapshot?.metrics;
        final terbaca = metrics != null && metrics.collected;

        return Scaffold(
          appBar: AppBar(
            title: const Text('Metrik'),
            actions: [
              // Ikon berganti jadi spinner selama permintaannya berjalan.
              // Tanpa itu menekan Segarkan tidak mengubah apa pun di layar
              // sampai jawabannya tiba, dan di jaringan seluler jeda itu
              // cukup panjang untuk terbaca sebagai tombol yang rusak.
              IconButton(
                onPressed: connection.isBusy ? null : connection.refresh,
                icon: connection.isBusy
                    ? const SizedBox(
                        width: 18,
                        height: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.refresh),
                tooltip: 'Segarkan',
              ),
              const SizedBox(width: 4),
            ],
          ),
          body: RefreshIndicator(
            onRefresh: connection.refresh,
            child: ListView(
              padding: const EdgeInsets.fromLTRB(16, 6, 16, 32),
              children: [
                _StatusLine(connection: connection),
                const SizedBox(height: 14),

                // Pesan TIDAK menggantikan angka yang sudah tergambar.
                // Jaringan seluler putus-sambung terus; mengosongkan layar
                // tiap kali satu permintaan gagal membuatnya berkedip.
                if (connection.message != null)
                  _Note(text: connection.message!, danger: _danger(connection)),

                if (!terbaca)
                  const _Note(text: 'Metrik belum terbaca.')
                else ...[
                  if (metrics.stale)
                    const _Note(text: 'Angka terakhir, belum diperbarui.'),
                  if (metrics.error != null)
                    _Note(text: metrics.error!, danger: true),
                  Opacity(
                    opacity: metrics.stale ? 0.5 : 1,
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Expanded(
                          child: _Ring(
                            label: 'CPU',
                            fraction: metrics.cpuUsedPct / 100,
                            center: '${metrics.cpuUsedPct.toStringAsFixed(0)}%',
                            detail: '${metrics.cpuCores} core',
                          ),
                        ),
                        Expanded(
                          child: _Ring(
                            label: 'RAM',
                            fraction:
                                ratio(metrics.memUsedMB, metrics.memTotalMB),
                            center:
                                pct(metrics.memUsedMB, metrics.memTotalMB),
                            detail: gbPair(
                              metrics.memUsedMB / 1024,
                              metrics.memTotalMB / 1024,
                            ),
                          ),
                        ),
                        Expanded(
                          child: _Ring(
                            label: 'DISK',
                            fraction:
                                ratio(metrics.diskUsedGB, metrics.diskTotalGB),
                            center:
                                pct(metrics.diskUsedGB, metrics.diskTotalGB),
                            detail: gbPair(
                              metrics.diskUsedGB,
                              metrics.diskTotalGB,
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 22),
                  _InfoCard(
                    rows: [
                      ('OS', metrics.osName),
                      ('Hostname', metrics.hostname),
                      ('Uptime', formatUptime(metrics.uptimeSeconds, long: true)),
                    ],
                  ),
                ],

                const SizedBox(height: 12),
                _InfoCard(
                  rows: [
                    ('Alamat', connection.credentials.baseUrl.host),
                    ('Agent', snapshot?.agentVersion ?? '—'),
                    if (snapshot != null)
                      (
                        'Agent uptime',
                        formatUptime(snapshot.agentUptimeSeconds, long: true),
                      ),
                  ],
                ),
                const SizedBox(height: 18),
                Text(
                  'Hanya baca. Aplikasi ini menampilkan keadaan server, tidak '
                  'mengubahnya.',
                  style: theme.textTheme.bodySmall?.copyWith(
                    color: theme.colorScheme.onSurfaceVariant,
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}

bool _danger(ServerConnection c) => switch (c.phase) {
      ServerPhase.revoked ||
      ServerPhase.rejected ||
      ServerPhase.unreachable ||
      ServerPhase.notReady =>
        true,
      _ => false,
    };

/// Satu cincin: label, busur terisi, angka di tengah, nilai asli di bawah.
///
/// Angkanya ditulis di tengah cincin, bukan hanya diwakili panjang busur —
/// busur menjawab "kira-kira seberapa penuh", angka menjawab "berapa", dan
/// layar yang seluruh gunanya memantau butuh keduanya. Itu juga yang membuat
/// warnanya aman: ia menegaskan sesuatu yang sudah terbaca, bukan
/// menggantikannya.
class _Ring extends StatelessWidget {
  const _Ring({
    required this.label,
    required this.fraction,
    required this.center,
    required this.detail,
  });

  final String label;
  final double fraction;
  final String center;
  final String detail;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final nilai = fraction.clamp(0.0, 1.0);
    final warna = GaugeColors.of(context, nilai);

    return Column(
      children: [
        Text(
          label,
          style: theme.textTheme.labelSmall?.copyWith(
            color: theme.colorScheme.onSurfaceVariant,
            letterSpacing: 0.8,
          ),
        ),
        const SizedBox(height: 8),
        // Animasi dijalankan dari nilai LAMA ke nilai baru tiap polling,
        // bukan dari nol: mulai dari nol tiap 20 detik membuat ketiga
        // cincin melompat mundur bersamaan dan terbaca seperti kedip.
        TweenAnimationBuilder<double>(
          tween: Tween(begin: nilai, end: nilai),
          duration: const Duration(milliseconds: 550),
          curve: Curves.easeOutCubic,
          builder: (context, v, _) => SizedBox(
            width: 84,
            height: 84,
            child: CustomPaint(
              painter: _RingPainter(
                fraction: v,
                color: warna,
                track: theme.colorScheme.surfaceContainerHighest,
              ),
              child: Center(
                child: Text(
                  center,
                  style: theme.textTheme.titleMedium?.copyWith(
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
            ),
          ),
        ),
        const SizedBox(height: 8),
        Text(
          detail,
          textAlign: TextAlign.center,
          maxLines: 2,
          overflow: TextOverflow.ellipsis,
          style: theme.textTheme.labelSmall?.copyWith(
            color: theme.colorScheme.onSurfaceVariant,
          ),
        ),
      ],
    );
  }
}

class _RingPainter extends CustomPainter {
  _RingPainter({
    required this.fraction,
    required this.color,
    required this.track,
  });

  final double fraction;
  final Color color;
  final Color track;

  static const _stroke = 9.0;

  @override
  void paint(Canvas canvas, Size size) {
    final rect = Offset(_stroke / 2, _stroke / 2) &
        Size(size.width - _stroke, size.height - _stroke);

    final dasar = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = _stroke
      ..color = track;
    canvas.drawArc(rect, 0, math.pi * 2, false, dasar);

    if (fraction <= 0) return;
    final isi = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = _stroke
      ..strokeCap = StrokeCap.round
      ..color = color;
    // Mulai dari atas (-90°), bukan dari kanan, karena itu titik yang dibaca
    // orang sebagai "nol" pada sesuatu yang berbentuk jam.
    canvas.drawArc(rect, -math.pi / 2, math.pi * 2 * fraction, false, isi);
  }

  @override
  bool shouldRepaint(_RingPainter old) =>
      old.fraction != fraction || old.color != color || old.track != track;
}

/// Kartu berisi pasangan label–nilai.
class _InfoCard extends StatelessWidget {
  const _InfoCard({required this.rows});

  final List<(String, String)> rows;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Card(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 4),
        child: Column(
          children: [
            for (var i = 0; i < rows.length; i++) ...[
              if (i > 0) const Divider(height: 1),
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 11),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    SizedBox(
                      width: 104,
                      child: Text(
                        rows[i].$1,
                        style: theme.textTheme.bodySmall?.copyWith(
                          color: theme.colorScheme.onSurfaceVariant,
                        ),
                      ),
                    ),
                    Expanded(
                      child: Text(
                        rows[i].$2.isEmpty ? '—' : rows[i].$2,
                        style: theme.textTheme.bodyMedium,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _StatusLine extends StatelessWidget {
  const _StatusLine({required this.connection});

  final ServerConnection connection;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final conn = connection.snapshot?.metrics?.connection;
    final label = switch (connection.phase) {
      ServerPhase.loading => 'Memuat…',
      ServerPhase.awaitingApproval => 'Menunggu persetujuan',
      ServerPhase.revoked => 'Akses dicabut',
      ServerPhase.rejected => 'Perangkat dihapus di server',
      ServerPhase.unreachable => 'Tidak bisa dihubungi',
      ServerPhase.notReady => 'Agent belum siap',
      ServerPhase.ready => switch (conn) {
          AgentConnection.online => 'Terhubung',
          AgentConnection.reconnecting => 'Menyambung ulang',
          AgentConnection.offline => 'Terputus',
          _ => 'Belum diperiksa',
        },
    };

    return Row(
      children: [
        Container(
          width: 9,
          height: 9,
          decoration: BoxDecoration(
            color: StatusColors.of(
              context,
              connection.phase == ServerPhase.ready ? conn?.name : null,
            ),
            shape: BoxShape.circle,
          ),
        ),
        const SizedBox(width: 8),
        Text(label, style: theme.textTheme.labelLarge),
        const Spacer(),
        Text(connection.host, style: theme.textTheme.bodySmall),
      ],
    );
  }
}

class _Note extends StatelessWidget {
  const _Note({required this.text, this.danger = false});

  final String text;
  final bool danger;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Container(
      width: double.infinity,
      margin: const EdgeInsets.only(bottom: 12),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: danger
            ? theme.colorScheme.errorContainer
            : theme.colorScheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(
        text,
        style: theme.textTheme.bodySmall?.copyWith(
          color: danger ? theme.colorScheme.onErrorContainer : null,
        ),
      ),
    );
  }
}
