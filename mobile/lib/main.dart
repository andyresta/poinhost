import 'package:flutter/material.dart';

import 'src/app.dart';
import 'src/session.dart';
import 'src/storage/credential_store.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();

  final workspace = Workspace(store: const SecureCredentialStore());
  // Dijalankan TANPA await: kredensial dibaca dari keychain lalu tiap server
  // dihubungi, dan menunggunya sebelum runApp berarti jendela native yang
  // kosong selama itu. Workspace mulai dengan restored=false, jadi yang
  // tergambar lebih dulu adalah SplashScreen — warnanya sama persis dengan
  // jendela native, sehingga peralihannya tidak terlihat.
  workspace.restore();

  runApp(PoinhostApp(workspace: workspace));
}
