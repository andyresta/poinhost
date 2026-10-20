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
            title: 'Satu agent, satu server',
            body: 'Mobile Control dipasang di tiap server yang ingin dikelola. '
                'Agent itu hanya mengurus mesinnya sendiri dan tidak menyimpan '
                'kredensial server lain — aplikasi ini yang menampung '
                'beberapa agent sekaligus, satu panel untuk masing-masing.',
          ),
          _Section(
            title: 'Perangkat harus disetujui',
            body: 'Token yang kamu tempel saat menambahkan server bukan '
                'kredensial akses. Menukarnya hanya mendaftarkan HP ini ke '
                'antrean; yang membuka akses adalah persetujuan dari panel '
                'Mobile Control di poinhost desktop.',
          ),
          _Section(
            title: 'Hanya baca',
            body: 'Aplikasi ini menampilkan keadaan server, tidak mengubahnya. '
                'Tidak ada SSH, kriptografi, atau orkestrasi di HP — semuanya '
                'tinggal di agent.',
          ),
          _Section(
            title: 'Mencabut akses',
            body: 'Menghapus server dari sini hanya melupakannya di HP ini. '
                'Untuk benar-benar mencabut, cabut perangkatnya dari panel '
                'Mobile Control di server yang bersangkutan.',
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
