import 'package:flutter/material.dart';

import '../session.dart';
import '../widgets/brand.dart';
import '../widgets/server_card.dart';
import 'add_server_screen.dart';
import 'server_detail_screen.dart';

/// Menu Server: grid panel, satu panel satu server.
class ServersScreen extends StatelessWidget {
  const ServersScreen({super.key, required this.workspace, this.onOpenMenu});

  final Workspace workspace;
  final VoidCallback? onOpenMenu;

  Future<void> _add(BuildContext context) async {
    await Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => AddServerScreen(workspace: workspace),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: workspace,
      builder: (context, _) {
        return Scaffold(
          appBar: AppBar(
            leading: onOpenMenu == null
                ? null
                : IconButton(
                    onPressed: onOpenMenu,
                    icon: const Icon(Icons.menu),
                    tooltip: 'Menu',
                  ),
            title: const Text('Server'),
            actions: [
              IconButton(
                onPressed: workspace.isEmpty || workspace.isRefreshing
                    ? null
                    : () => workspace.refreshAll(manual: true),
                icon: workspace.isRefreshing
                    ? const SizedBox(
                        width: 18,
                        height: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.refresh),
                tooltip: 'Refresh',
              ),
              const SizedBox(width: 4),
            ],
          ),
          floatingActionButton: workspace.isEmpty
              ? null
              : FloatingActionButton.extended(
                  onPressed: () => _add(context),
                  icon: const Icon(Icons.add),
                  label: const Text('Server'),
                ),
          body: workspace.isEmpty
              ? _Empty(onAdd: () => _add(context))
              : _Grid(workspace: workspace),
        );
      },
    );
  }
}

class _Grid extends StatelessWidget {
  const _Grid({required this.workspace});

  final Workspace workspace;

  @override
  Widget build(BuildContext context) {
    return RefreshIndicator(
      onRefresh: workspace.refreshAll,
      child: LayoutBuilder(
        builder: (context, constraints) {
          // Kolom dihitung dari lebar yang TERSEDIA, bukan dari lebar layar:
          // sidebar yang terbuka di layar lebar memakan ruang itu, dan grid
          // yang menghitung dari layar akan menyempit sampai kartunya tidak
          // terbaca begitu sidebar dibuka.
          final columns = (constraints.maxWidth / 190).floor().clamp(1, 5);

          return GridView.builder(
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 96),
            physics: const AlwaysScrollableScrollPhysics(),
            gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
              crossAxisCount: columns,
              mainAxisSpacing: 12,
              crossAxisSpacing: 12,
              mainAxisExtent: 186,
            ),
            itemCount: workspace.connections.length,
            itemBuilder: (context, i) {
              final conn = workspace.connections[i];
              return AnimatedBuilder(
                animation: conn,
                builder: (context, _) => ServerCard(
                  connection: conn,
                  onTap: () => Navigator.of(context).push(
                    MaterialPageRoute<void>(
                      builder: (_) => ServerDetailScreen(
                        workspace: workspace,
                        connection: conn,
                      ),
                    ),
                  ),
                ),
              );
            },
          );
        },
      ),
    );
  }
}

class _Empty extends StatelessWidget {
  const _Empty({required this.onAdd});

  final VoidCallback onAdd;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 360),
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const BrandMark(size: 56),
              const SizedBox(height: 20),
              Text('No servers yet', style: theme.textTheme.titleMedium),
              const SizedBox(height: 8),
              Text(
                'Install Mobile Control on a server from poinhost on the '
                'desktop, then add it with its enrollment token.',
                textAlign: TextAlign.center,
                style: theme.textTheme.bodyMedium?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
              const SizedBox(height: 24),
              FilledButton.icon(
                onPressed: onAdd,
                icon: const Icon(Icons.add),
                label: const Text('Add server'),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
