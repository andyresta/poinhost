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
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 32),
              children: [
                _StatusLine(connection: connection),
                const SizedBox(height: 16),

                // Pesan TIDAK menggantikan angka yang sudah tergambar.
                // Jaringan seluler putus-sambung terus; mengosongkan layar
                // tiap kali satu permintaan gagal membuatnya berkedip.
                if (connection.message != null)
                  _Note(text: connection.message!, danger: _danger(connection)),

                if (metrics == null || !metrics.collected)
                  const _Note(text: 'Metrik belum terbaca.')
                else ...[
                  if (metrics.stale)
                    const _Note(text: 'Angka terakhir, belum diperbarui.'),
                  if (metrics.error != null) _Note(text: metrics.error!, danger: true),
                  _Gauge(
                    label: 'CPU',
                    value: '${metrics.cpuUsedPct.toStringAsFixed(1)}%',
                    detail: '${metrics.cpuCores} core',
                    fraction: metrics.cpuUsedPct / 100,
                    faded: metrics.stale,
                  ),
                  _Gauge(
                    label: 'Memori',
                    value: gbPair(
                      metrics.memUsedMB / 1024,
                      metrics.memTotalMB / 1024,
                    ),
                    fraction: ratio(metrics.memUsedMB, metrics.memTotalMB),
                    faded: metrics.stale,
                  ),
                  _Gauge(
                    label: 'Disk',
                    value: gbPair(metrics.diskUsedGB, metrics.diskTotalGB),
                    fraction: ratio(metrics.diskUsedGB, metrics.diskTotalGB),
                    faded: metrics.stale,
                  ),
                  const SizedBox(height: 12),
                  _Row(label: 'OS', value: metrics.osName),
                  _Row(label: 'Hostname', value: metrics.hostname),
                  _Row(
                    label: 'Uptime',
                    value: formatUptime(metrics.uptimeSeconds, long: true),
                  ),
                ],

                const Divider(height: 36),
                _Row(label: 'Alamat', value: connection.credentials.baseUrl.host),
                _Row(label: 'Agent', value: snapshot?.agentVersion ?? '—'),
                if (snapshot != null)
                  _Row(
                    label: 'Agent uptime',
                    value: formatUptime(snapshot.agentUptimeSeconds, long: true),
                  ),
                const SizedBox(height: 20),
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

class _Gauge extends StatelessWidget {
  const _Gauge({
    required this.label,
    required this.value,
    required this.fraction,
    required this.faded,
    this.detail,
  });

  final String label;
  final String value;
  final String? detail;
  final double fraction;
  final bool faded;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Opacity(
      opacity: faded ? 0.5 : 1,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 10),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Text(label, style: theme.textTheme.labelLarge),
                const Spacer(),
                Text(value, style: theme.textTheme.bodyMedium),
                if (detail != null) ...[
                  const SizedBox(width: 8),
                  Text(detail!, style: theme.textTheme.bodySmall),
                ],
              ],
            ),
            const SizedBox(height: 7),
            ClipRRect(
              borderRadius: BorderRadius.circular(4),
              child: LinearProgressIndicator(
                value: fraction.clamp(0.0, 1.0),
                minHeight: 7,
                backgroundColor: theme.colorScheme.surfaceContainerHighest,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Row extends StatelessWidget {
  const _Row({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 110,
            child: Text(
              label,
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
          ),
          Expanded(
            child: Text(
              value.isEmpty ? '—' : value,
              style: theme.textTheme.bodyMedium,
            ),
          ),
        ],
      ),
    );
  }
}
