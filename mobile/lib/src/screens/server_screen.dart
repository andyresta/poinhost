import 'package:flutter/material.dart';

import '../api/models.dart';
import '../session.dart';
import '../widgets/metric_tile.dart';

/// Layar utama: satu server, karena satu agent memang hanya mengurus
/// mesinnya sendiri.
class ServerScreen extends StatelessWidget {
  const ServerScreen({super.key, required this.session});

  final Session session;

  @override
  Widget build(BuildContext context) {
    final snapshot = session.snapshot;
    if (snapshot == null) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    final metrics = snapshot.metrics;

    return Scaffold(
      appBar: AppBar(
        title: Text(snapshot.name),
        actions: [
          IconButton(
            onPressed: () => session.refresh(),
            icon: const Icon(Icons.refresh),
            tooltip: 'Segarkan',
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: session.refresh,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            _Header(snapshot: snapshot),
            const SizedBox(height: 16),

            // Pesan galat TIDAK menggantikan angka yang sudah tergambar.
            // Jaringan seluler putus-sambung terus; mengosongkan layar tiap
            // kali satu permintaan gagal membuatnya berkedip tanpa guna.
            if (session.message != null)
              Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: _Banner(text: session.message!),
              ),

            if (metrics == null)
              const _Banner(text: 'Agent belum mengumpulkan metrik.')
            else ...[
              if (metrics.stale)
                const Padding(
                  padding: EdgeInsets.only(bottom: 12),
                  child: _Banner(text: 'Angka terakhir, belum diperbarui.'),
                ),
              if (metrics.error != null)
                Padding(
                  padding: const EdgeInsets.only(bottom: 12),
                  child: _Banner(text: metrics.error!),
                ),
              MetricTile(
                label: 'CPU',
                value: '${metrics.cpuUsedPct.toStringAsFixed(1)}%',
                detail: '${metrics.cpuCores} core',
                fraction: metrics.cpuUsedPct / 100,
                faded: metrics.stale,
              ),
              MetricTile(
                label: 'Memori',
                value: _gb(metrics.memUsedMB / 1024, metrics.memTotalMB / 1024),
                fraction: _ratio(metrics.memUsedMB, metrics.memTotalMB),
                faded: metrics.stale,
              ),
              MetricTile(
                label: 'Disk',
                value: _gb(metrics.diskUsedGB, metrics.diskTotalGB),
                fraction: _ratio(metrics.diskUsedGB, metrics.diskTotalGB),
                faded: metrics.stale,
              ),
              const SizedBox(height: 8),
              _Row(label: 'OS', value: metrics.osName),
              _Row(label: 'Hostname', value: metrics.hostname),
              _Row(label: 'Uptime', value: _uptime(metrics.uptimeSeconds)),
            ],

            const SizedBox(height: 24),
            _Row(label: 'Agent', value: snapshot.agentVersion),
            _Row(label: 'Alamat', value: session.baseUrl?.host ?? '—'),
            const SizedBox(height: 16),
            TextButton(
              onPressed: () => session.forget(),
              child: const Text('Putuskan perangkat ini'),
            ),
          ],
        ),
      ),
    );
  }
}

class _Header extends StatelessWidget {
  const _Header({required this.snapshot});

  final ServerSnapshot snapshot;

  @override
  Widget build(BuildContext context) {
    final conn = snapshot.metrics?.connection ?? AgentConnection.unknown;
    final theme = Theme.of(context);
    return Row(
      children: [
        Icon(Icons.circle, size: 10, color: _color(theme, conn)),
        const SizedBox(width: 8),
        Text(_label(conn), style: theme.textTheme.labelLarge),
        const Spacer(),
        Text(snapshot.host, style: theme.textTheme.bodySmall),
      ],
    );
  }

  static Color _color(ThemeData theme, AgentConnection c) => switch (c) {
        AgentConnection.online => Colors.green,
        AgentConnection.reconnecting => Colors.orange,
        AgentConnection.offline => theme.colorScheme.error,
        AgentConnection.unknown => theme.disabledColor,
      };

  static String _label(AgentConnection c) => switch (c) {
        AgentConnection.online => 'Terhubung',
        AgentConnection.reconnecting => 'Menyambung ulang',
        AgentConnection.offline => 'Terputus',
        AgentConnection.unknown => 'Belum diperiksa',
      };
}

class _Banner extends StatelessWidget {
  const _Banner({required this.text});

  final String text;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: theme.colorScheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(8),
      ),
      child: Text(text, style: theme.textTheme.bodySmall),
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
            width: 96,
            child: Text(label, style: theme.textTheme.bodySmall),
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

/// Pembagian yang aman dari total nol.
///
/// Agent mengirim nol saat metriknya belum terkumpul untuk satu field, dan
/// tanpa penjagaan ini bar-nya menampilkan NaN.
double _ratio(num used, num total) => total <= 0 ? 0 : used / total;

String _uptime(int seconds) {
  final d = seconds ~/ 86400;
  final h = (seconds % 86400) ~/ 3600;
  final m = (seconds % 3600) ~/ 60;
  if (d > 0) return '$d hari $h jam';
  if (h > 0) return '$h jam $m menit';
  return '$m menit';
}
