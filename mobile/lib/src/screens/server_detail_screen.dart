import 'package:flutter/material.dart';

import '../api/models.dart';
import '../session.dart';
import '../theme.dart';

/// Satu server, dibuka dari panelnya di grid.
class ServerDetailScreen extends StatelessWidget {
  const ServerDetailScreen({
    super.key,
    required this.workspace,
    required this.connection,
  });

  final Workspace workspace;
  final ServerConnection connection;

  Future<void> _remove(BuildContext context) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Hapus server ini?'),
        content: const Text(
          'Hanya dihapus dari HP ini. Perangkatnya tetap terdaftar di server '
          'sampai kamu mencabutnya dari panel Mobile Control.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: const Text('Batal'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: const Text('Hapus'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    await workspace.remove(connection.id);
    if (context.mounted) Navigator.of(context).pop();
  }

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
            title: Text(connection.displayName),
            actions: [
              IconButton(
                onPressed: connection.isBusy ? null : connection.refresh,
                icon: const Icon(Icons.refresh),
                tooltip: 'Segarkan',
              ),
              IconButton(
                onPressed: () => _remove(context),
                icon: const Icon(Icons.delete_outline),
                tooltip: 'Hapus dari HP ini',
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

                if (metrics == null)
                  const _Note(text: 'Agent belum mengumpulkan metrik.')
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
                    value: _gb(metrics.memUsedMB / 1024, metrics.memTotalMB / 1024),
                    fraction: _ratio(metrics.memUsedMB, metrics.memTotalMB),
                    faded: metrics.stale,
                  ),
                  _Gauge(
                    label: 'Disk',
                    value: _gb(metrics.diskUsedGB, metrics.diskTotalGB),
                    fraction: _ratio(metrics.diskUsedGB, metrics.diskTotalGB),
                    faded: metrics.stale,
                  ),
                  const SizedBox(height: 12),
                  _Row(label: 'OS', value: metrics.osName),
                  _Row(label: 'Hostname', value: metrics.hostname),
                  _Row(label: 'Uptime', value: _uptime(metrics.uptimeSeconds)),
                ],

                const Divider(height: 36),
                _Row(label: 'Alamat', value: connection.credentials.baseUrl.host),
                _Row(label: 'Agent', value: snapshot?.agentVersion ?? '—'),
                if (snapshot != null)
                  _Row(
                    label: 'Agent uptime',
                    value: _uptime(snapshot.agentUptimeSeconds),
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
      ServerPhase.revoked || ServerPhase.rejected || ServerPhase.unreachable =>
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

String _gb(double used, double total) =>
    '${used.toStringAsFixed(1)} / ${total.toStringAsFixed(1)} GB';

/// Pembagian yang aman dari total nol — agent mengirim nol untuk field yang
/// belum terkumpul, dan tanpa penjagaan ini bar-nya menampilkan NaN.
double _ratio(num used, num total) => total <= 0 ? 0 : used / total;

String _uptime(int seconds) {
  final d = seconds ~/ 86400;
  final h = (seconds % 86400) ~/ 3600;
  final m = (seconds % 3600) ~/ 60;
  if (d > 0) return '$d hari $h jam';
  if (h > 0) return '$h jam $m menit';
  return '$m menit';
}
