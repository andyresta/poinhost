import 'package:flutter/material.dart';

import '../api/models.dart';
import '../format.dart';
import '../session.dart';
import '../theme.dart';

/// Satu panel di grid: satu server yang dipasangi agent.
///
/// Urutannya tetap — nama, alamat, lalu angka — supaya kartu bisa dibaca
/// dalam sekali lirik. Angkanya menempel LANGSUNG di bawah alamat, bukan
/// didorong ke dasar kartu: yang dicari mata saat menyapu grid adalah
/// kolom angka yang sejajar di semua kartu.
///
/// Server yang BELUM bisa dibaca tetap menempati kartunya sendiri dengan
/// alasannya tertulis — menyembunyikannya sampai sehat berarti server yang
/// mati menghilang dari layar, justru saat paling dibutuhkan terlihat.
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
    final terbaca = metrics != null && metrics.collected;

    return Card(
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.fromLTRB(13, 12, 13, 12),
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
              const SizedBox(height: 1),
              Text(
                connection.host,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
              const SizedBox(height: 11),

              if (!terbaca)
                Expanded(child: _StateNote(connection: connection))
              else ...[
                _Bar(
                  label: 'CPU',
                  value: '${metrics.cpuUsedPct.toStringAsFixed(0)}%',
                  fraction: metrics.cpuUsedPct / 100,
                  faded: metrics.stale,
                ),
                const SizedBox(height: 7),
                _Bar(
                  label: 'RAM',
                  value: pct(metrics.memUsedMB, metrics.memTotalMB),
                  fraction: ratio(metrics.memUsedMB, metrics.memTotalMB),
                  faded: metrics.stale,
                ),
                const SizedBox(height: 7),
                _Bar(
                  label: 'DISK',
                  value: pct(metrics.diskUsedGB, metrics.diskTotalGB),
                  fraction: ratio(metrics.diskUsedGB, metrics.diskTotalGB),
                  faded: metrics.stale,
                ),
                const SizedBox(height: 8),
                _Line(
                  label: 'UPTIME',
                  value: formatUptime(metrics.uptimeSeconds),
                  faded: metrics.stale,
                ),
              ],
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
      ServerPhase.ready => ('Metrik belum terbaca', false),
    };

    return Align(
      alignment: Alignment.topLeft,
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

/// Satu baris berlabel dengan bar di bawahnya.
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
          _Line(label: label, value: value, faded: false),
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

/// Label di kiri, nilai di kanan — tanpa bar.
class _Line extends StatelessWidget {
  const _Line({
    required this.label,
    required this.value,
    required this.faded,
  });

  final String label;
  final String value;
  final bool faded;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final style = theme.textTheme.labelSmall;
    return Opacity(
      opacity: faded ? 0.45 : 1,
      child: Row(
        children: [
          Text(
            label,
            style: style?.copyWith(color: theme.colorScheme.onSurfaceVariant),
          ),
          const Spacer(),
          Text(value, style: style),
        ],
      ),
    );
  }
}
