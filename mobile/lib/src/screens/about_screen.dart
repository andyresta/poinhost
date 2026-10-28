import 'package:flutter/material.dart';

import '../widgets/brand.dart';

/// Menu About.
class AboutScreen extends StatelessWidget {
  const AboutScreen({super.key, this.onOpenMenu});

  final VoidCallback? onOpenMenu;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Scaffold(
      appBar: AppBar(
        leading: onOpenMenu == null
            ? null
            : IconButton(
                onPressed: onOpenMenu,
                icon: const Icon(Icons.menu),
                tooltip: 'Menu',
              ),
        title: const Text('About'),
      ),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 32),
        children: [
          Row(
            children: [
              const BrandMark(size: 52),
              const SizedBox(width: 14),
              Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'poinhost',
                    style: theme.textTheme.titleLarge?.copyWith(
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  Text(
                    'Mobile Control',
                    style: theme.textTheme.bodyMedium?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                  ),
                ],
              ),
            ],
          ),
          const SizedBox(height: 28),
          _Section(
            title: 'One agent, one server',
            body: 'Mobile Control is installed on each server you want to '
                'manage. An agent only looks after its own machine and holds '
                'no credentials for any other — this app is what holds '
                'several agents at once, one panel each.',
          ),
          _Section(
            title: 'Devices must be approved',
            body: 'The token you paste when adding a server is not an access '
                'credential. Exchanging it only puts this phone in the queue; '
                'what opens access is approval from the Mobile Control panel '
                'in poinhost on the desktop.',
          ),
          // Dulu berjudul "Hanya baca", dan itu sudah TIDAK benar sejak
          // Service, Files, dan Website bisa mengubah server. Menerjemahkan
          // kalimat yang salah hanya akan mengawetkan kesalahannya.
          _Section(
            title: 'Two separate permissions',
            body: 'Approving a device lets it watch. Changing anything — '
                'restarting a service, editing a file, taking a site offline '
                '— needs the separate control permission, granted per device '
                'from the same panel.',
          ),
          _Section(
            title: 'Revoking access',
            body: 'Removing a server here only forgets it on this phone. To '
                'revoke it for real, revoke the device from the Mobile '
                'Control panel on that server.',
          ),
        ],
      ),
    );
  }
}

class _Section extends StatelessWidget {
  const _Section({required this.title, required this.body});

  final String title;
  final String body;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 22),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            title,
            style: theme.textTheme.titleSmall?.copyWith(
              fontWeight: FontWeight.w600,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            body,
            style: theme.textTheme.bodyMedium?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
              height: 1.45,
            ),
          ),
        ],
      ),
    );
  }
}
