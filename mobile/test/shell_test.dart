import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:poinhost_mobile/src/api/agent_client.dart';
import 'package:poinhost_mobile/src/api/agent_exception.dart';
import 'package:poinhost_mobile/src/api/models.dart';
import 'package:poinhost_mobile/src/app.dart';
import 'package:poinhost_mobile/src/session.dart';
import 'package:poinhost_mobile/src/settings.dart';
import 'package:poinhost_mobile/src/storage/credential_store.dart';

/// Agent palsu, supaya layar bisa diuji tanpa jaringan.
class _FakeAgent extends AgentClient {
  _FakeAgent(Uri baseUrl, this._reply) : super(baseUrl: baseUrl);

  final Future<ServerSnapshot> Function() _reply;

  @override
  Future<ServerSnapshot> server({required String deviceToken}) => _reply();

  @override
  void close() {}
}

ServerSnapshot _snapshot(String name) => ServerSnapshot(
      id: 's1',
      name: name,
      host: '10.0.0.1',
      agentVersion: '0.3.0',
      agentUptimeSeconds: 3600,
      metrics: const ServerMetrics(
        connection: AgentConnection.online,
        osName: 'Ubuntu 24.04',
        hostname: 'prod',
        uptimeSeconds: 7200,
        cpuCores: 4,
        cpuUsedPct: 12.5,
        memUsedMB: 2048,
        memTotalMB: 8192,
        diskUsedGB: 31.4,
        diskTotalGB: 160,
        stale: false,
      ),
    );

Credentials _creds(String id, String label) => Credentials(
      id: id,
      baseUrl: Uri.parse('https://$id.example.com'),
      deviceToken: 'token-$id',
      label: label,
    );

Future<Workspace> _workspace(
  List<Credentials> creds,
  Future<ServerSnapshot> Function(Uri) reply,
) async {
  final w = Workspace(
    store: InMemoryCredentialStore(creds),
    clientFor: (url) => _FakeAgent(url, () => reply(url)),
  );
  await w.restore();
  return w;
}

void main() {
  // Sidebar berperilaku BERBEDA menurut lebar layar, dan dua-duanya diuji.
  // Satu perilaku untuk kedua ukuran selalu salah di salah satunya: rail
  // yang menetap memakan ruang yang dibutuhkan grid di HP tegak, sementara
  // hamburger di layar lebar menyembunyikan menu yang muat dengan nyaman.
  testWidgets('di layar sempit menu lewat hamburger', (tester) async {
    tester.view.physicalSize = const Size(400, 800);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);

    final w = await _workspace(const [], (_) async => _snapshot('x'));
    await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('Menu'));
    await tester.pumpAndSettle();

    expect(find.text('About'), findsOneWidget);
    await tester.tap(find.text('About'));
    await tester.pumpAndSettle();

    expect(find.text('Satu agent, satu server'), findsOneWidget);
    expect(find.text('Perangkat harus disetujui'), findsOneWidget);
    w.dispose();
  });

  // Menutup lapisan menu sebenarnya bisa dengan menyentuh bagian gelap di
  // sebelahnya atau menggeser, tapi dua-duanya tidak terlihat sebagai apa
  // pun — yang membuka menu dengan tombol mencari tombol untuk menutupnya.
  testWidgets('menu di layar sempit punya tombol tutup', (tester) async {
    tester.view.physicalSize = const Size(400, 800);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);

    final w = await _workspace(const [], (_) async => _snapshot('x'));
    await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('Menu'));
    await tester.pumpAndSettle();
    expect(find.text('About'), findsOneWidget);

    await tester.tap(find.byTooltip('Tutup menu'));
    await tester.pumpAndSettle();

    // Menunya tertutup, dan layarnya tidak berpindah ke mana-mana.
    expect(find.text('About'), findsNothing);
    expect(find.byTooltip('Menu'), findsOneWidget);
    w.dispose();
  });

  testWidgets('di layar lebar sidebar menetap dan bisa disempitkan',
      (tester) async {
    tester.view.physicalSize = const Size(1000, 800);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);

    final w = await _workspace(const [], (_) async => _snapshot('x'));
    await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
    await tester.pumpAndSettle();

    // Menu langsung terlihat, tanpa hamburger.
    expect(find.byTooltip('Menu'), findsNothing);
    expect(find.text('About'), findsOneWidget);

    await tester.tap(find.byTooltip('Sembunyikan menu'));
    await tester.pumpAndSettle();

    // Disempitkan: labelnya hilang, tapi menunya masih ada lewat ikon.
    expect(find.text('About'), findsNothing);
    expect(find.byTooltip('Tampilkan menu'), findsOneWidget);
    expect(find.byTooltip('Tutup menu'), findsNothing);
    w.dispose();
  });

  // Pilihan tampilan harus punya TIGA keadaan. Sakelar dua arah mengubah
  // "ikut sistem" jadi pilihan sekali jalan: begitu ditekan, app berhenti
  // mengikuti setelan HP selamanya dan tidak ada jalan kembali.
  testWidgets('tampilan bisa diubah dan bisa kembali ikut sistem',
      (tester) async {
    final w = await _workspace(const [], (_) async => _snapshot('x'));
    final settings = _settings();
    await settings.load();
    expect(settings.mode, ThemeMode.system);

    await tester.pumpWidget(PoinhostApp(workspace: w, settings: settings));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('Tampilan: Ikut sistem'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Gelap').last);
    await tester.pumpAndSettle();
    expect(settings.mode, ThemeMode.dark);

    await tester.tap(find.byTooltip('Tampilan: Gelap'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Ikut sistem').last);
    await tester.pumpAndSettle();
    expect(settings.mode, ThemeMode.system);

    w.dispose();
  });

  // Pilihan harus bertahan: tema yang kembali ke bawaan tiap kali app
  // dibuka sama saja dengan tidak bisa dipilih.
  test('pilihan tampilan tersimpan', () async {
    final store = InMemorySettingsStore();
    await AppSettings(store: store).setMode(ThemeMode.light);

    final lagi = AppSettings(store: store);
    await lagi.load();
    expect(lagi.mode, ThemeMode.light);
  });

  // Label isian harus TETAP terbaca saat sedang diketik. Label mengambang
  // bawaan Material naik ke tempat garis tepi — yang di kotak terisi tanpa
  // garis berarti menabrak tepi atasnya — lalu menghilang begitu ada isinya,
  // sehingga form yang sudah diisi tidak bisa dibaca lagi maksud tiap
  // kotaknya.
  testWidgets('label form tetap ada saat diketik', (tester) async {
    tester.view.physicalSize = const Size(420, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);

    final w = await _workspace(const [], (_) async => _snapshot('x'));
    await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Tambah server'));
    await tester.pumpAndSettle();

    expect(find.text('Alamat agent'), findsOneWidget);
    // Sengaja BUKAN teks petunjuknya: petunjuk tetap ada di pohon widget,
    // jadi mengetik ulang kalimat yang sama membuat test cocok dua kali dan
    // tidak membuktikan apa pun.
    await tester.enterText(find.byType(TextFormField).first, 'prod.contoh.id');
    await tester.pumpAndSettle();

    // Label masih ada DI SAMPING isinya, bukan tergantikan olehnya.
    expect(find.text('Alamat agent'), findsOneWidget);
    expect(find.text('prod.contoh.id'), findsOneWidget);

    // Keterangan panjang tidak dipotong satu baris: `helperText` bawaan
    // memotongnya dengan elipsis.
    expect(
      find.text('Supaya kamu mengenalinya saat menyetujui'),
      findsOneWidget,
    );
    w.dispose();
  });

  // Tiap server yang terhubung harus jadi SATU panel. Inilah bentuk layar
  // utamanya; kalau dua server menghasilkan satu kartu, gridnya tidak ada
  // gunanya.
  testWidgets('tiap server jadi satu panel di grid', (tester) async {
    final w = await _workspace(
      [_creds('a', 'Produksi'), _creds('b', 'Staging')],
      (url) async => _snapshot(url.host.startsWith('a') ? 'produksi' : 'staging'),
    );

    await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
    await tester.pumpAndSettle();

    expect(find.text('produksi'), findsOneWidget);
    expect(find.text('staging'), findsOneWidget);
    expect(find.text('Belum ada server'), findsNothing);
    w.dispose();
  });

  // Server yang BELUM bisa dibaca tetap menempati kartunya dengan alasannya
  // tertulis. Menyembunyikannya sampai sehat berarti server yang mati
  // menghilang dari layar, justru saat paling perlu terlihat.
  testWidgets('server bermasalah tetap punya panel beserta alasannya',
      (tester) async {
    final w = await _workspace(
      [_creds('a', 'Menunggu'), _creds('b', 'Mati')],
      (url) async {
        if (url.host.startsWith('a')) {
          throw AgentException(
            AgentFailure.pendingApproval,
            'menunggu persetujuan di app poinhost',
          );
        }
        throw AgentException(AgentFailure.unreachable, 'tidak tersambung');
      },
    );

    await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
    await tester.pumpAndSettle();

    expect(find.text('Menunggu persetujuan'), findsOneWidget);
    expect(find.text('Tidak bisa dihubungi'), findsOneWidget);
    // Labelnya tetap dipakai selama agent belum menyebut nama servernya.
    expect(find.text('Menunggu'), findsOneWidget);
    expect(find.text('Mati'), findsOneWidget);
    w.dispose();
  });

  // 503 berarti agent MENJAWAB — ia terjangkau, hanya belum siap melayani.
  // Menyebutnya "tidak bisa dihubungi" mengirim pemiliknya memeriksa
  // jaringan, padahal perbaikannya ada di panel Mobile Control.
  testWidgets('agent yang menjawab 503 bukan "tidak bisa dihubungi"',
      (tester) async {
    final w = await _workspace(
      [_creds('a', 'Belum siap')],
      (_) async => throw AgentException(
        AgentFailure.agentNotReady,
        'agent belum mengelola mesinnya sendiri',
      ),
    );

    await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
    await tester.pumpAndSettle();

    expect(w.connections.single.phase, ServerPhase.notReady);
    expect(find.text('Agent belum siap'), findsOneWidget);
    expect(find.text('Tidak bisa dihubungi'), findsNothing);
    w.dispose();
  });

  // Menghapus di HP tidak boleh menyentuh daftar perangkat di agent — itu
  // hak pemilik server. Yang diuji di sini: entri benar-benar hilang dari
  // penyimpanan, bukan hanya dari layar.
  test('hapus server melupakannya di penyimpanan', () async {
    final store = InMemoryCredentialStore([_creds('a', 'A'), _creds('b', 'B')]);
    final w = Workspace(
      store: store,
      clientFor: (url) => _FakeAgent(url, () async => _snapshot('x')),
    );
    addTearDown(w.dispose);
    await w.restore();

    await w.remove('a');

    expect(w.connections.map((c) => c.id), ['b']);
    expect((await store.readAll()).map((c) => c.id), ['b']);
  });
}

AppSettings _settings() => AppSettings(store: InMemorySettingsStore());
