import 'package:flutter/material.dart';

import '../api/models.dart';
import '../session.dart';
import '../theme.dart';
import 'files_screen.dart';
import 'server_metrics_screen.dart';
import 'services_screen.dart';

/// Modul yang bisa dibuka untuk satu server.
///
/// Daftarnya mengikuti poinhost di desktop, termasuk yang belum ada di sini.
/// Menyembunyikan yang belum jadi akan membuat menunya berubah bentuk tiap
/// rilis — dan tidak memberi tahu apa pun tentang apa yang sedang dibangun.
enum ServerModule {
  metrik('Metrik', Icons.speed_outlined, siap: true),
  website('Website', Icons.language_outlined),
  files('Files', Icons.folder_outlined, siap: true),
  terminal('Terminal', Icons.terminal_outlined),
  service('Service', Icons.manage_accounts_outlined, siap: true),
  firewall('Firewall', Icons.shield_outlined),
  docker('Docker', Icons.inventory_2_outlined);

  const ServerModule(this.label, this.icon, {this.siap = false});

  final String label;
  final IconData icon;

  /// Sudah bisa dibuka. Yang belum tetap tampil, dalam keadaan redup.
  final bool siap;
}

/// Menu satu server, dibuka dari panelnya di grid.
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

  void _open(BuildContext context, ServerModule module) {
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => switch (module) {
          ServerModule.metrik => ServerMetricsScreen(connection: connection),
          ServerModule.service => ServicesScreen(connection: connection),
          ServerModule.files => FilesScreen(connection: connection),
          _ => _BelumAdaScreen(module: module),
        },
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: connection,
      builder: (context, _) {
        return Scaffold(
          appBar: AppBar(
            title: Text(connection.displayName),
            actions: [
              IconButton(
                onPressed: () => _remove(context),
                icon: const Icon(Icons.delete_outline),
                tooltip: 'Hapus dari HP ini',
              ),
              const SizedBox(width: 4),
            ],
          ),
          body: ListView(
            padding: const EdgeInsets.fromLTRB(16, 10, 16, 28),
            children: [
              _StatusLine(connection: connection),
              const SizedBox(height: 18),
              GridView.count(
                crossAxisCount: 3,
                shrinkWrap: true,
                physics: const NeverScrollableScrollPhysics(),
                mainAxisSpacing: 10,
                crossAxisSpacing: 10,
                childAspectRatio: 1,
                children: [
                  for (final m in ServerModule.values)
                    _ModuleTile(module: m, onTap: () => _open(context, m)),
                ],
              ),
            ],
          ),
        );
      },
    );
  }
}

/// Satu kotak modul.
///
/// Yang belum siap tetap bisa ditekan, dan menjawab. Kotak redup yang tidak
/// melakukan apa pun saat ditekan terbaca sebagai aplikasi yang rusak, bukan
/// sebagai modul yang belum dibuat.
class _ModuleTile extends StatelessWidget {
  const _ModuleTile({required this.module, required this.onTap});

  final ServerModule module;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final warna = module.siap
        ? theme.colorScheme.onSurface
        : theme.colorScheme.onSurfaceVariant;

    return Card(
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: Opacity(
          opacity: module.siap ? 1 : 0.55,
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(module.icon, size: 26, color: warna),
              const SizedBox(height: 9),
              Text(
                module.label,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: theme.textTheme.labelMedium?.copyWith(color: warna),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _BelumAdaScreen extends StatelessWidget {
  const _BelumAdaScreen({required this.module});

  final ServerModule module;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(module.label)),
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                module.icon,
                size: 44,
                color: theme.colorScheme.onSurfaceVariant,
              ),
              const SizedBox(height: 16),
              Text(
                'Belum tersedia di aplikasi mobile.',
                textAlign: TextAlign.center,
                style: theme.textTheme.bodyMedium?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Keadaan server, satu baris, di atas menunya.
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
