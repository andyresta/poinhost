import 'package:flutter/material.dart';

import 'screens/shell.dart';
import 'screens/splash_screen.dart';
import 'session.dart';
import 'theme.dart';

/// Akar app.
///
/// Splash tetap terlihat sampai kredensial selesai dibaca DAN server-server
/// dihubungi sekali. Memotongnya lebih awal berarti grid sempat tergambar
/// kosong, yang tidak bisa dibedakan dari "belum ada server sama sekali"
/// oleh orang yang sebenarnya punya lima.
class PoinhostApp extends StatelessWidget {
  const PoinhostApp({super.key, required this.workspace});

  final Workspace workspace;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'poinhost',
      debugShowCheckedModeBanner: false,
      theme: buildTheme(Brightness.light),
      darkTheme: buildTheme(Brightness.dark),
      home: AnimatedBuilder(
        animation: workspace,
        builder: (context, _) {
          return AnimatedSwitcher(
            // Pudar, bukan geser: splash dan grid adalah layar yang sama
            // yang sedang terisi, bukan dua tempat yang berbeda.
            duration: const Duration(milliseconds: 320),
            child: workspace.restored
                ? AppShell(key: const ValueKey('shell'), workspace: workspace)
                : const SplashScreen(key: ValueKey('splash')),
          );
        },
      ),
    );
  }
}
