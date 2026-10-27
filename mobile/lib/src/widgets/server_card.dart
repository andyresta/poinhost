import 'package:flutter/material.dart';

import '../api/models.dart';
import '../session.dart';
import '../theme.dart';

/// Satu panel di grid: satu server yang dipasangi agent.
///
/// Kartu ini harus terbaca dalam sekali lirik, jadi urutannya tetap: nama,
/// keadaan, lalu angka. Server yang BELUM bisa dibaca tetap menempati
/// kartunya sendiri dengan alasannya tertulis — menyembunyikannya sampai
/// sehat berarti server yang mati menghilang dari layar, yang justru saat
/// paling dibutuhkan terlihat.
class ServerCard extends StatelessWidget {
  const ServerCard({
    super.key,
    required this.connection,
    required this.onTap,
  });

  final ServerConnection connection;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final metrics = connection.snapshot?.metrics;

    return Card(
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.all(14),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  _StatusDot(connection: connection),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      connection.displayName,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: theme.textTheme.titleSmall?.copyWith(
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 2),
              Text(
                connection.host,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
              const SizedBox(height: 12),

              if (metrics == null)
                Expanded(child: _StateNote(connection: connection))
              else
                Expanded(
                  child: Column(
                    mainAxisAlignment: MainAxisAlignment.end,
                    children: [
                      _Bar(
                        label: 'CPU',
                        value: '${metrics.cpuUsedPct.toStringAsFixed(0)}%',
                        fraction: metrics.cpuUsedPct / 100,
                        faded: metrics.stale,
                      ),
                      const SizedBox(height: 8),
                      _Bar(
                        label: 'RAM',
                        value: _pct(metrics.memUsedMB, metrics.memTotalMB),
                        fraction: _ratio(metrics.memUsedMB, metrics.memTotalMB),
                        faded: metrics.stale,
                      ),
                      const SizedBox(height: 8),
                      _Bar(
                        label: 'Disk',
                        value: _pct(metrics.diskUsedGB, metrics.diskTotalGB),
                        fraction:
                            _ratio(metrics.diskUsedGB, metrics.diskTotalGB),
                        faded: metrics.stale,
                      ),
                    ],
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Titik status, SELALU berpasangan dengan teks keadaannya.
///
/// Titik berwarna sendirian tidak terbaca oleh sekitar satu dari dua belas
/// laki-laki, dan ini layar yang seluruh gunanya adalah menyampaikan status.
class _StatusDot extends StatelessWidget {
  const _StatusDot({required this.connection});

  final ServerConnection connection;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 9,
      height: 9,
      decoration: BoxDecoration(
        color: StatusColors.of(context, _conn(connection)),
        shape: BoxShape.circle,
      ),
    );
  }
}

String? _conn(ServerConnection c) {
  if (c.phase != ServerPhase.ready) return null;
  return switch (c.snapshot?.metrics?.connection) {
    AgentConnection.online => 'online',
    AgentConnection.reconnecting => 'reconnecting',
    AgentConnection.offline => 'offline',
    _ => null,
  };
}

/// Alasan kartu ini belum menampilkan angka, dalam satu kalimat.
class _StateNote extends StatelessWidget {
  const _StateNote({required this.connection});

  final ServerConnection connection;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final (text, danger) = switch (connection.phase) {
      ServerPhase.loading => ('Memuat…', false),
      ServerPhase.awaitingApproval => ('Menunggu persetujuan', false),
      ServerPhase.revoked => ('Akses dicabut', true),
      ServerPhase.rejected => ('Perangkat dihapus', true),
      ServerPhase.unreachable => ('Tidak bisa dihubungi', true),
      ServerPhase.notReady => ('Agent belum siap', true),
      ServerPhase.ready => ('Metrik belum terkumpul', false),
    };

    return Align(
      alignment: Alignment.bottomLeft,
      child: Text(
        text,
        style: theme.textTheme.bodySmall?.copyWith(
          color: danger
              ? theme.colorScheme.error
              : theme.colorScheme.onSurfaceVariant,
        ),
      ),
    );
  }
}

class _Bar extends StatelessWidget {
  const _Bar({
    required this.label,
    required this.value,
    required this.fraction,
    required this.faded,
  });

  final String label;
  final String value;
  final double fraction;
  final bool faded;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Opacity(
      opacity: faded ? 0.45 : 1,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Text(label, style: theme.textTheme.labelSmall),
              const Spacer(),
              Text(value, style: theme.textTheme.labelSmall),
            ],
          ),
          const SizedBox(height: 4),
          ClipRRect(
            borderRadius: BorderRadius.circular(3),
            child: LinearProgressIndicator(
              // Dijepit, bukan dipercaya: angka dari server yang sesaat
              // tidak masuk akal tidak boleh melukis bar di luar kotaknya.
              value: fraction.clamp(0.0, 1.0),
              minHeight: 5,
              backgroundColor: theme.colorScheme.surfaceContainerHighest,
            ),
          ),
        ],
      ),
    );
  }
}

/// Pembagian yang aman dari total nol — agent mengirim nol untuk field yang
/// belum terkumpul, dan tanpa penjagaan ini bar-nya menampilkan NaN.
double _ratio(num used, num total) => total <= 0 ? 0 : used / total;

String _pct(num used, num total) =>
    total <= 0 ? '—' : '${(used / total * 100).toStringAsFixed(0)}%';
