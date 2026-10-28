import 'package:flutter_test/flutter_test.dart';
import 'package:poinhost_mobile/src/app.dart';
import 'package:poinhost_mobile/src/screens/add_server_screen.dart';
import 'package:poinhost_mobile/src/session.dart';
import 'package:poinhost_mobile/src/settings.dart';
import 'package:poinhost_mobile/src/storage/credential_store.dart';

void main() {
  // Splash harus bertahan sampai kredensial selesai dibaca. Memotongnya
  // lebih awal membuat grid sempat tergambar kosong, yang tidak bisa
  // dibedakan dari "belum ada server" oleh orang yang sebenarnya punya
  // beberapa.
  testWidgets('splash tampil dulu, lalu grid', (tester) async {
    final workspace = Workspace(store: InMemoryCredentialStore());
    addTearDown(workspace.dispose);

    await tester.pumpWidget(PoinhostApp(workspace: workspace, settings: _settings()));
    await tester.pump();
    expect(find.text('Mobile Control'), findsOneWidget);
    expect(workspace.restored, isFalse);

    await workspace.restore();
    await tester.pumpAndSettle();

    expect(workspace.restored, isTrue);
    expect(find.text('No servers yet'), findsOneWidget);
    expect(find.text('Add server'), findsOneWidget);
  });

  // Alamat tanpa skema harus jadi https, bukan http: agent diterbitkan
  // dengan certbot, dan menebak http akan mengirim token pendaftaran lewat
  // jaringan seluler tanpa enkripsi.
  test('alamat tanpa skema dilengkapi https', () {
    expect(
      normalizeAgentUrl('agent.example.com').toString(),
      'https://agent.example.com',
    );
    expect(
      normalizeAgentUrl('  agent.example.com/  ').toString(),
      'https://agent.example.com',
    );
    // Yang sengaja mengetik http dibiarkan — itu pilihan sadar, mis. saat
    // mencoba agent di jaringan lokal yang belum punya sertifikat.
    expect(
      normalizeAgentUrl('http://127.0.0.1:7898').toString(),
      'http://127.0.0.1:7898',
    );
  });

  // Entri tersimpan yang tidak lengkap harus dibuang, bukan dipakai apa
  // adanya: entri tanpa token gagal di setiap permintaan tanpa pernah
  // menawarkan jalan keluar selain menghapusnya.
  test('kredensial rusak tidak ikut dimuat', () {
    expect(
      Credentials.fromJson({'id': 'a', 'baseUrl': 'https://x.test'}),
      isNull,
    );
    expect(
      Credentials.fromJson({'id': '', 'baseUrl': 'https://x.test', 'deviceToken': 't'}),
      isNull,
    );
    expect(
      Credentials.fromJson({'id': 'a', 'baseUrl': 'bukan-url', 'deviceToken': 't'}),
      isNull,
    );
    expect(
      Credentials.fromJson({
        'id': 'a',
        'baseUrl': 'https://x.test',
        'deviceToken': 't',
        'label': 'HP',
      }),
      isNotNull,
    );
  });
}

AppSettings _settings() => AppSettings(store: InMemorySettingsStore());
