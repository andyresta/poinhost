import 'package:flutter/material.dart';

import 'src/app.dart';
import 'src/session.dart';
import 'src/storage/credential_store.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();

  final session = Session(store: const SecureCredentialStore());
  // Dijalankan TANPA await: kredensial dibaca dari keychain, dan menunggunya
  // sebelum runApp berarti layar putih selama pembacaan itu. Session mulai
  // di fase loading, lalu memberi tahu dirinya sendiri setelah selesai.
  session.restore();

  runApp(PoinhostApp(session: session));
}
