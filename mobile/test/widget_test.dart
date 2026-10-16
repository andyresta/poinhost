import 'package:flutter_test/flutter_test.dart';
import 'package:poinhost_mobile/src/app.dart';
import 'package:poinhost_mobile/src/session.dart';
import 'package:poinhost_mobile/src/storage/credential_store.dart';

void main() {
  // Tanpa kredensial tersimpan, app harus berhenti di layar pendaftaran —
  // bukan mencoba memanggil agent dengan token kosong.
  testWidgets('tanpa kredensial, app membuka layar pendaftaran',
      (tester) async {
    final session = Session(store: InMemoryCredentialStore());
    await session.restore();

    await tester.pumpWidget(PoinhostApp(session: session));
    await tester.pump();

    expect(session.phase, SessionPhase.needsEnrollment);
    expect(find.text('Hubungkan server'), findsOneWidget);
    expect(find.text('Token pendaftaran'), findsOneWidget);
  });
}
