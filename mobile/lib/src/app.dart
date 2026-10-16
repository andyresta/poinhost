import 'package:flutter/material.dart';

import 'screens/enroll_screen.dart';
import 'screens/server_screen.dart';
import 'screens/waiting_screen.dart';
import 'session.dart';

/// Akar app.
///
/// Layarnya dipilih dari fase sesi, bukan dari navigator stack. Alur
/// pendaftaran tidak pernah bergerak maju-mundur atas kemauan user: yang
/// memindahkannya adalah jawaban agent, termasuk persetujuan yang datang
/// saat layar tunggu sedang terbuka.
class PoinhostApp extends StatelessWidget {
  const PoinhostApp({super.key, required this.session});

  final Session session;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'poinhost',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorSchemeSeed: const Color(0xFF2F6FED),
        useMaterial3: true,
      ),
      darkTheme: ThemeData(
        colorSchemeSeed: const Color(0xFF2F6FED),
        brightness: Brightness.dark,
        useMaterial3: true,
      ),
      home: AnimatedBuilder(
        animation: session,
        builder: (context, _) {
          switch (session.phase) {
            case SessionPhase.loading:
              return const Scaffold(
                body: Center(child: CircularProgressIndicator()),
              );
            case SessionPhase.needsEnrollment:
              return EnrollScreen(session: session);
            case SessionPhase.awaitingApproval:
              return WaitingScreen(session: session);
            case SessionPhase.revoked:
              return WaitingScreen(session: session, revoked: true);
            case SessionPhase.ready:
              return ServerScreen(session: session);
          }
        },
      ),
    );
  }
}
