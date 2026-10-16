import 'package:flutter/material.dart';

import '../session.dart';

/// Langkah 3 alurnya, dari sisi HP: menunggu seseorang menekan Setujui.
///
/// Layar ini tidak punya tombol yang mempercepat apa pun, dan itu memang
/// intinya — yang menentukan akses adalah tindakan manusia di depan app
/// desktop, bukan sesuatu yang bisa ditekan dari sini. Session tetap
/// memanggil agent di latar, karena tidak ada yang memberi tahu HP saat
/// persetujuan itu terjadi.
class WaitingScreen extends StatelessWidget {
  const WaitingScreen({super.key, required this.session, this.revoked = false});

  final Session session;
  final bool revoked;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: Padding(
            padding: const EdgeInsets.all(32),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Icon(
                  revoked ? Icons.block : Icons.hourglass_top,
                  size: 48,
                  color: revoked
                      ? theme.colorScheme.error
                      : theme.colorScheme.primary,
                ),
                const SizedBox(height: 20),
                Text(
                  revoked ? 'Akses dicabut' : 'Menunggu persetujuan',
                  style: theme.textTheme.titleLarge,
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: 10),
                Text(
                  revoked
                      ? 'Perangkat ini dicabut dari panel Mobile Control.'
                      : 'Buka poinhost di desktop, lalu setujui perangkat ini '
                          'di panel Mobile Control.',
                  style: theme.textTheme.bodyMedium,
                  textAlign: TextAlign.center,
                ),
                if (session.baseUrl != null) ...[
                  const SizedBox(height: 10),
                  Text(
                    session.baseUrl!.host,
                    style: theme.textTheme.bodySmall,
                  ),
                ],
                const SizedBox(height: 28),
                if (!revoked)
                  OutlinedButton(
                    onPressed: () => session.refresh(),
                    child: const Text('Periksa sekarang'),
                  ),
                TextButton(
                  onPressed: () => session.forget(),
                  child: const Text('Hubungkan ke server lain'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
