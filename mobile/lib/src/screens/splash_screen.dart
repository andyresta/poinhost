import 'package:flutter/material.dart';

import '../widgets/brand.dart';

/// Warna latar splash.
///
/// Harus sama persis dengan `splash_background` di
/// `android/app/src/main/res/values/colors.xml`. Keduanya adalah layar yang
/// sama bagi user: yang satu digambar jendela native sebelum Flutter hidup,
/// yang satu lagi oleh Flutter setelahnya. Beda satu nada saja dan
/// peralihannya terlihat sebagai kedipan.
const splashBackground = Color(0xFF0D1424);

/// Layar pembuka.
///
/// Ditampilkan selama kredensial dibaca dari keychain dan server-server
/// dihubungi pertama kali. Bukan hiasan: tanpa ini layar pertama adalah
/// grid kosong, yang tidak bisa dibedakan dari "belum ada server sama
/// sekali" oleh orang yang sebenarnya punya lima.
class SplashScreen extends StatelessWidget {
  const SplashScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return const ColoredBox(
      color: splashBackground,
      child: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            BrandMark(size: 88),
            SizedBox(height: 22),
            Text(
              'poinhost',
              style: TextStyle(
                color: Colors.white,
                fontSize: 24,
                fontWeight: FontWeight.w600,
                letterSpacing: -0.4,
              ),
            ),
            SizedBox(height: 6),
            Text(
              'Mobile Control',
              style: TextStyle(color: Color(0xFF8A96AE), fontSize: 13),
            ),
            SizedBox(height: 34),
            SizedBox(
              width: 112,
              child: LinearProgressIndicator(
                minHeight: 2,
                backgroundColor: Color(0xFF1C2740),
                color: Color(0xFF2F6FED),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
