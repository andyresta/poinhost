import 'dart:convert';

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
  _FakeAgent(Uri baseUrl, this._reply, {this.serviceList})
      : super(baseUrl: baseUrl);

  final Future<ServerSnapshot> Function() _reply;
  final ServiceList? serviceList;

  /// Aksi yang SAMPAI ke agent. Yang diuji di layar Service bukan
  /// systemctl-nya, melainkan aksi mana yang bisa dikirim sama sekali.
  final List<String> aksi = [];

  @override
  Future<ServerSnapshot> server({required String deviceToken}) => _reply();

  @override
  Future<ServiceList> services({required String deviceToken}) async =>
      serviceList ??
      const ServiceList(services: [], systemdAvailable: true, canControl: false);

  @override
  Future<ServiceInfo> serviceAction({
    required String deviceToken,
    required String name,
    required String action,
  }) async {
    aksi.add('$action $name');
    return ServiceInfo(
      name: name,
      description: '',
      activeState: action == 'stop' ? 'inactive' : 'active',
      subState: '',
      running: action != 'stop',
      enabled: true,
    );
  }

  /// Path yang diminta, berurutan.
  final List<String> dibuka = [];

  @override
  Future<FileListing> files({
    required String deviceToken,
    required String path,
  }) async {
    dibuka.add(path);
    if (path == '/') {
      return const FileListing(
        path: '/',
        parent: '/',
        entries: [
          FileEntry(
            name: 'etc',
            path: '/etc',
            size: 0,
            isDir: true,
            mode: 'drwxr-xr-x',
            modTime: '',
          ),
        ],
      );
    }
    return const FileListing(
      path: '/etc',
      parent: '/',
      entries: [
        FileEntry(
          name: 'hosts',
          path: '/etc/hosts',
          size: 221,
          isDir: false,
          mode: '-rw-r--r--',
          modTime: '',
        ),
      ],
    );
  }

  @override
  Future<FileText> fileContent({
    required String deviceToken,
    required String path,
  }) async =>
      FileText(path: path, content: '127.0.0.1 localhost', size: 19);

  /// Perubahan berkas yang SAMPAI ke agent.
  final List<String> tulisan = [];

  @override
  Future<void> writeFile({
    required String deviceToken,
    required String path,
    required List<int> bytes,
  }) async {
    tulisan.add('write $path = ${utf8.decode(bytes)}');
  }

  @override
  Future<void> renameFile({
    required String deviceToken,
    required String oldPath,
    required String newPath,
  }) async {
    tulisan.add('rename $oldPath -> $newPath');
  }

  @override
  Future<void> deleteFiles({
    required String deviceToken,
    required List<String> paths,
  }) async {
    tulisan.add('delete ${paths.join(',')}');
  }

  /// Izin kendali yang dilaporkan agent palsu ini untuk daftar situs.
  bool bolehUbah = false;

  @override
  Future<WebsiteListing> websites({required String deviceToken}) async =>
      WebsiteListing(
        nginxInstalled: true,
        nginxActive: true,
        nginxVersion: '1.22',
        canControl: bolehUbah,
        sites: const [
          WebsiteInfo(
            domain: 'contoh.id',
            parent: '',
            isSubdomain: false,
            root: '/var/www/contoh.id',
            enabled: true,
            phpVersion: '8.2',
            phpEnabled: true,
            proxyTarget: '',
            sslEnabled: true,
          ),
        ],
      );

  @override
  Future<WebsiteSSL> websiteSSL({
    required String deviceToken,
    required String domain,
  }) async =>
      WebsiteSSL(
        domain: domain,
        sslEnabled: true,
        autoRenew: true,
        certExists: true,
        notAfter: DateTime.now().add(const Duration(days: 9)).toIso8601String(),
        certDomains: const ['contoh.id', 'www.contoh.id'],
        missingSans: const ['toko.contoh.id'],
        expiresInDays: 9,
      );

  @override
  Future<void> setWebsiteEnabled({
    required String deviceToken,
    required String domain,
    required bool enabled,
  }) async {
    tulisan.add('site $domain enabled=$enabled');
  }

  @override
  Future<ContainerListing> containers({required String deviceToken}) async =>
      ContainerListing(
        dockerOk: true,
        version: '27.0',
        canControl: bolehUbah,
        containers: const [
          ContainerInfo(
            id: 'aaa111',
            name: 'web',
            image: 'nginx:alpine',
            status: 'Up 3 days',
            state: 'running',
            ports: '0.0.0.0:80->80/tcp',
          ),
          ContainerInfo(
            id: 'bbb222',
            name: 'worker',
            image: 'app:latest',
            status: 'Exited (1) 2 hours ago',
            state: 'exited',
            ports: '',
          ),
        ],
      );

  @override
  Future<String> containerLogs({
    required String deviceToken,
    required String id,
    int lines = 500,
  }) async =>
      'panic: something broke';

  @override
  Future<void> containerAction({
    required String deviceToken,
    required String id,
    required String action,
  }) async {
    tulisan.add('container $action $id');
  }

  @override
  void close() {}
}

ServiceList _services({required bool canControl}) => ServiceList(
      systemdAvailable: true,
      canControl: canControl,
      services: const [
        ServiceInfo(
          name: 'nginx',
          description: 'A high performance web server',
          activeState: 'active',
          subState: 'running',
          running: true,
          enabled: true,
        ),
        ServiceInfo(
          name: 'mysql',
          description: 'MySQL Community Server',
          activeState: 'failed',
          subState: 'dead',
          running: false,
          enabled: true,
        ),
      ],
    );

ServerSnapshot _snapshot(String name) => ServerSnapshot(
      id: 's1',
      name: name,
      host: '10.0.0.1',
      agentVersion: '0.3.0',
      agentUptimeSeconds: 3600,
      metrics: ServerMetrics(
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
        // Wajib ada: checkedAt-lah yang menandai angka ini PERNAH diukur.
        // Tanpa itu kartunya benar kalau menolak menggambarnya.
        checkedAt: DateTime(2026, 10, 27, 12),
      ),
    );

/// Snapshot yang agent-nya menjawab tapi belum pernah memprobe mesinnya:
/// status koneksi ada, angkanya tidak.
ServerSnapshot _snapshotBelumTerbaca() => const ServerSnapshot(
      id: 's1',
      name: 'baru',
      host: '10.0.0.1',
      agentVersion: '0.3.0',
      agentUptimeSeconds: 30,
      metrics: ServerMetrics(
        connection: AgentConnection.offline,
        osName: '',
        hostname: '',
        uptimeSeconds: 0,
        cpuCores: 0,
        cpuUsedPct: 0,
        memUsedMB: 0,
        memTotalMB: 0,
        diskUsedGB: 0,
        diskTotalGB: 0,
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
  Future<ServerSnapshot> Function(Uri) reply, {
  ServiceList? services,
  List<_FakeAgent>? agents,
  bool bolehUbah = false,
}) async {
  final w = Workspace(
    store: InMemoryCredentialStore(creds),
    clientFor: (url) {
      final a = _FakeAgent(url, () => reply(url), serviceList: services)
        ..bolehUbah = bolehUbah;
      agents?.add(a);
      return a;
    },
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

    expect(find.text('One agent, one server'), findsOneWidget);
    expect(find.text('Devices must be approved'), findsOneWidget);
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

    await tester.tap(find.byTooltip('Close menu'));
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

    await tester.tap(find.byTooltip('Hide menu'));
    await tester.pumpAndSettle();

    // Disempitkan: labelnya hilang, tapi menunya masih ada lewat ikon.
    expect(find.text('About'), findsNothing);
    expect(find.byTooltip('Show menu'), findsOneWidget);
    expect(find.byTooltip('Close menu'), findsNothing);
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

    await tester.tap(find.byTooltip('Theme: Follow system'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Dark').last);
    await tester.pumpAndSettle();
    expect(settings.mode, ThemeMode.dark);

    await tester.tap(find.byTooltip('Theme: Dark'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Follow system').last);
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

    await tester.tap(find.text('Add server'));
    await tester.pumpAndSettle();

    expect(find.text('Agent address'), findsOneWidget);
    // Sengaja BUKAN teks petunjuknya: petunjuk tetap ada di pohon widget,
    // jadi mengetik ulang kalimat yang sama membuat test cocok dua kali dan
    // tidak membuktikan apa pun.
    await tester.enterText(find.byType(TextFormField).first, 'prod.contoh.id');
    await tester.pumpAndSettle();

    // Label masih ada DI SAMPING isinya, bukan tergantikan olehnya.
    expect(find.text('Agent address'), findsOneWidget);
    expect(find.text('prod.contoh.id'), findsOneWidget);

    // Keterangan panjang tidak dipotong satu baris: `helperText` bawaan
    // memotongnya dengan elipsis.
    expect(
      find.text('So you recognise it when approving'),
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
    expect(find.text('No servers yet'), findsNothing);
    w.dispose();
  });

  testWidgets('kartu memuat CPU, RAM, DISK, dan UPTIME', (tester) async {
    final w = await _workspace(
      [_creds('a', 'Produksi')],
      (_) async => _snapshot('produksi'),
    );

    await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
    await tester.pumpAndSettle();

    for (final label in ['CPU', 'RAM', 'DISK', 'UPTIME']) {
      expect(find.text(label), findsOneWidget, reason: 'label $label');
    }
    expect(find.text('25%'), findsOneWidget); // RAM 2048/8192
    expect(find.text('2h 0m'), findsOneWidget); // uptime 7200 detik
    w.dispose();
  });

  // Agent yang belum pernah memprobe mesinnya mengirim status koneksi saja.
  // Menggambar nol untuk angka yang tidak pernah diukur terbaca persis
  // seperti server sehat yang menganggur — dan bentuknya data, jadi tidak
  // ada yang membantahnya.
  testWidgets('angka yang belum pernah diukur tidak digambar sebagai nol',
      (tester) async {
    final w = await _workspace(
      [_creds('a', 'Baru')],
      (_) async => _snapshotBelumTerbaca(),
    );

    await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
    await tester.pumpAndSettle();

    expect(find.text('No metrics read yet'), findsOneWidget);
    expect(find.text('0%'), findsNothing);
    expect(find.text('CPU'), findsNothing);
    w.dispose();
  });

  // Agent ikut mengirim kenapa pembacaan terakhir gagal. Menggantinya dengan
  // "belum terbaca" yang netral membuang satu-satunya petunjuk yang ada.
  testWidgets('kartu menampilkan alasan dari agent, bukan kalimat netral',
      (tester) async {
    final w = await _workspace(
      [_creds('a', 'Prod')],
      (_) async => const ServerSnapshot(
        id: 's1',
        name: 'prod',
        host: '127.0.0.1',
        agentVersion: '0.3.2',
        agentUptimeSeconds: 60,
        metrics: ServerMetrics(
          connection: AgentConnection.offline,
          osName: '',
          hostname: '',
          uptimeSeconds: 0,
          cpuCores: 0,
          cpuUsedPct: 0,
          memUsedMB: 0,
          memTotalMB: 0,
          diskUsedGB: 0,
          diskTotalGB: 0,
          stale: true,
          error: 'SSH_HOST_KEY_MISMATCH',
        ),
      ),
    );

    await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
    await tester.pumpAndSettle();

    expect(find.text('SSH_HOST_KEY_MISMATCH'), findsOneWidget);
    expect(find.text('No metrics read yet'), findsNothing);
    w.dispose();
  });

  // Menekan panel server membuka MENU modulnya, bukan langsung metrik.
  // Modul yang belum ada tetap tampil — menyembunyikannya membuat menunya
  // berubah bentuk tiap rilis — dan tetap menjawab saat ditekan.
  testWidgets('panel server membuka menu modul', (tester) async {
    final w = await _workspace(
      [_creds('a', 'Prod')],
      (_) async => _snapshot('produksi'),
    );

    await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
    await tester.pumpAndSettle();

    await tester.tap(find.text('produksi'));
    await tester.pumpAndSettle();

    for (final label in [
      'Metrics',
      'Website',
      'Files',
      'Terminal',
      'Services',
      'Firewall',
      'Docker',
    ]) {
      expect(find.text(label), findsOneWidget, reason: 'modul $label');
    }

    // Yang belum ada menjawab, bukan diam. Firewall dipilih karena ia
    // memang masih redup — Docker dulu dipakai di sini sampai modulnya jadi.
    await tester.ensureVisible(find.text('Firewall'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Firewall'));
    await tester.pumpAndSettle();
    expect(find.text('Not available in the mobile app yet.'), findsOneWidget);

    await tester.pageBack();
    await tester.pumpAndSettle();

    // Dan Metrik membuka angkanya.
    await tester.ensureVisible(find.text('Metrics'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Metrics'));
    await tester.pumpAndSettle();
    expect(find.text('Hostname'), findsOneWidget);
    expect(find.text('prod'), findsOneWidget);
    w.dispose();
  });

  // Perangkat yang disetujui TAPI belum diberi izin kendali tidak boleh
  // melihat satu pun tombol aksi. Tombol yang pasti ditolak membuat
  // pemiliknya menyalahkan aplikasi, bukan izin yang memang belum diberikan.
  testWidgets('tanpa izin kendali, layar Service tidak punya tombol aksi',
      (tester) async {
    final agents = <_FakeAgent>[];
    final w = await _workspace(
      [_creds('a', 'Prod')],
      (_) async => _snapshot('produksi'),
      services: _services(canControl: false),
      agents: agents,
    );

    await _bukaService(tester, w);

    expect(find.text('nginx'), findsOneWidget);
    expect(find.text('mysql'), findsOneWidget);
    expect(find.textContaining('Grant the control permission'), findsOneWidget);
    for (final label in ['Start', 'Stop', 'Restart']) {
      expect(find.text(label), findsNothing, reason: 'tombol $label');
    }
    expect(agents.single.aksi, isEmpty);
    w.dispose();
  });

  testWidgets('dengan izin kendali, Start terkirim ke agent', (tester) async {
    final agents = <_FakeAgent>[];
    final w = await _workspace(
      [_creds('a', 'Prod')],
      (_) async => _snapshot('produksi'),
      services: _services(canControl: true),
      agents: agents,
    );

    await _bukaService(tester, w);

    // Yang gagal naik ke atas: daftar systemd itu panjang, dan yang dicari
    // orang saat membukanya dari HP hampir selalu ada di kelompok itu.
    final posisiMysql = tester.getTopLeft(find.text('mysql')).dy;
    final posisiNginx = tester.getTopLeft(find.text('nginx')).dy;
    expect(posisiMysql, lessThan(posisiNginx));

    // mysql mati, jadi tombolnya Start — dan Start tidak perlu konfirmasi.
    await tester.tap(find.text('Start'));
    await tester.pumpAndSettle();
    expect(agents.single.aksi, ['start mysql']);
    w.dispose();
  });

  // Server yang BELUM bisa dibaca tetap menempati kartunya dengan alasannya
  // Server yang BELUM bisa dibaca tetap menempati kartunya dengan alasannya
  // tertulis. Menyembunyikannya sampai sehat berarti server yang mati
  // menghilang dari layar, justru saat paling perlu terlihat.
  testWidgets('Files menelusuri folder lalu membuka isi berkas',
      (tester) async {
    final agents = <_FakeAgent>[];
    final w = await _workspace(
      [_creds('a', 'Prod')],
      (_) async => _snapshot('produksi'),
      agents: agents,
    );

    await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
    await tester.pumpAndSettle();
    await tester.tap(find.text('produksi'));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.text('Files'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Files'));
    await tester.pumpAndSettle();

    expect(agents.single.dibuka, ['/']);
    expect(find.text('etc'), findsOneWidget);

    // Folder membuka LAYAR BARU, bukan mengganti isi layar yang sama: itu
    // yang membuat tombol kembali membawa ke folder induk, bukan keluar
    // dari Files sama sekali.
    await tester.tap(find.text('etc'));
    await tester.pumpAndSettle();
    expect(agents.single.dibuka, ['/', '/etc']);
    expect(find.text('hosts'), findsOneWidget);

    await tester.tap(find.text('hosts'));
    await tester.pumpAndSettle();
    expect(find.text('127.0.0.1 localhost'), findsOneWidget);

    await tester.pageBack();
    await tester.pumpAndSettle();
    expect(find.text('hosts'), findsOneWidget);
    w.dispose();
  });

  // Menghapus HARUS lewat konfirmasi. Direktori dihapus rekursif, jadi satu
  // salah tekan di layar sempit bisa menghapus banyak sekaligus.
  testWidgets('hapus berkas minta konfirmasi dulu', (tester) async {
    final agents = <_FakeAgent>[];
    final w = await _workspace(
      [_creds('a', 'Prod')],
      (_) async => _snapshot('produksi'),
      agents: agents,
    );
    await _bukaFiles(tester, w);

    await tester.tap(find.byIcon(Icons.more_vert).first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Delete'));
    await tester.pumpAndSettle();

    // Dialognya muncul, dan belum ada apa pun yang dikirim.
    expect(find.textContaining('Everything inside goes with it'), findsOneWidget);
    expect(agents.single.tulisan, isEmpty);

    await tester.tap(find.widgetWithText(TextButton, 'Cancel'));
    await tester.pumpAndSettle();
    expect(agents.single.tulisan, isEmpty);

    // Kali kedua, disetujui.
    await tester.tap(find.byIcon(Icons.more_vert).first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Delete'));
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(FilledButton, 'Delete'));
    await tester.pumpAndSettle();
    expect(agents.single.tulisan, ['delete /etc']);
    w.dispose();
  });

  testWidgets('suntingan tersimpan sebagai isi baru berkas', (tester) async {
    final agents = <_FakeAgent>[];
    final w = await _workspace(
      [_creds('a', 'Prod')],
      (_) async => _snapshot('produksi'),
      agents: agents,
    );
    await _bukaFiles(tester, w);

    await tester.tap(find.text('etc'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('hosts'));
    await tester.pumpAndSettle();
    expect(find.text('127.0.0.1 localhost'), findsOneWidget);

    await tester.tap(find.byIcon(Icons.edit_outlined));
    await tester.pumpAndSettle();
    await tester.enterText(find.byType(TextField), '10.0.0.1 baru');
    await tester.tap(find.byIcon(Icons.check));
    await tester.pumpAndSettle();

    expect(agents.single.tulisan, ['write /etc/hosts = 10.0.0.1 baru']);
    w.dispose();
  });

  // Sisa hari ditulis di depan tanggalnya, dan SAN yang belum tercakup
  // diperingatkan — itu jebakan yang paling mudah terlewat: statusnya "SSL
  // aktif", tapi HTTPS untuk domain itu tidak bekerja, dan renew tidak
  // memperbaikinya.
  testWidgets('detail domain menyebut sisa hari dan SAN yang kurang',
      (tester) async {
    final agents = <_FakeAgent>[];
    final w = await _workspace(
      [_creds('a', 'Prod')],
      (_) async => _snapshot('produksi'),
      agents: agents,
    );
    await _bukaModul(tester, w, 'Website');

    expect(find.text('contoh.id'), findsOneWidget);
    expect(find.textContaining('HTTPS'), findsOneWidget);

    await tester.tap(find.text('contoh.id'));
    await tester.pumpAndSettle();

    expect(find.text('9 days left'), findsOneWidget);
    expect(find.textContaining('toko.contoh.id'), findsOneWidget);
    expect(find.textContaining('reissued'), findsOneWidget);

    // Tanpa izin kendali tidak ada sakelar untuk mematikan situs.
    expect(find.byType(SwitchListTile), findsNothing);
    w.dispose();
  });

  testWidgets('mematikan situs minta konfirmasi dulu', (tester) async {
    final agents = <_FakeAgent>[];
    final w = await _workspace(
      [_creds('a', 'Prod')],
      (_) async => _snapshot('produksi'),
      agents: agents,
      bolehUbah: true,
    );
    await _bukaModul(tester, w, 'Website');
    await tester.tap(find.text('contoh.id'));
    await tester.pumpAndSettle();

    await tester.tap(find.byType(SwitchListTile));
    await tester.pumpAndSettle();
    expect(find.textContaining('stops being served'), findsOneWidget);
    expect(agents.single.tulisan, isEmpty);

    await tester.tap(find.widgetWithText(FilledButton, 'Take offline'));
    await tester.pumpAndSettle();
    expect(agents.single.tulisan, ['site contoh.id enabled=false']);
    w.dispose();
  });

  // Yang BERHENTI naik ke atas: container mati adalah satu-satunya alasan
  // orang membuka layar ini dari HP.
  testWidgets('Docker menaikkan container yang berhenti, lalu buka lognya',
      (tester) async {
    final agents = <_FakeAgent>[];
    final w = await _workspace(
      [_creds('a', 'Prod')],
      (_) async => _snapshot('produksi'),
      agents: agents,
      bolehUbah: true,
    );
    await _bukaModul(tester, w, 'Docker');

    final posisiWorker = tester.getTopLeft(find.text('worker')).dy;
    final posisiWeb = tester.getTopLeft(find.text('web')).dy;
    expect(posisiWorker, lessThan(posisiWeb));

    // Seluruh kartu membuka log, bukan satu tombol kecil.
    await tester.tap(find.text('worker'));
    await tester.pumpAndSettle();
    expect(find.text('panic: something broke'), findsOneWidget);

    await tester.pageBack();
    await tester.pumpAndSettle();

    // worker mati, jadi tombolnya Start — dan Start tidak perlu konfirmasi.
    await tester.tap(find.text('Start'));
    await tester.pumpAndSettle();
    expect(agents.single.tulisan, ['container start bbb222']);
    w.dispose();
  });

  testWidgets('tanpa izin kendali, Docker tidak membuka log atau aksi',
      (tester) async {
    final agents = <_FakeAgent>[];
    final w = await _workspace(
      [_creds('a', 'Prod')],
      (_) async => _snapshot('produksi'),
      agents: agents,
    );
    await _bukaModul(tester, w, 'Docker');

    expect(find.text('web'), findsOneWidget);
    expect(find.textContaining('Grant the control permission'), findsOneWidget);
    for (final label in ['Start', 'Stop', 'Restart']) {
      expect(find.text(label), findsNothing, reason: 'tombol $label');
    }

    // Menekan kartunya pun tidak membuka log.
    await tester.tap(find.text('worker'));
    await tester.pumpAndSettle();
    expect(find.text('panic: something broke'), findsNothing);
    expect(agents.single.tulisan, isEmpty);
    w.dispose();
  });

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

    expect(find.text('Waiting for approval'), findsOneWidget);
    expect(find.text('Cannot be reached'), findsOneWidget);
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
    expect(find.text('Agent not ready'), findsOneWidget);
    expect(find.text('Cannot be reached'), findsNothing);
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

/// Membuka Server -> menu -> Service.
Future<void> _bukaService(WidgetTester tester, Workspace w) async {
  await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
  await tester.pumpAndSettle();
  await tester.tap(find.text('produksi'));
  await tester.pumpAndSettle();
  await tester.ensureVisible(find.text('Services'));
  await tester.pumpAndSettle();
  await tester.tap(find.text('Services'));
  await tester.pumpAndSettle();
}

/// Membuka Server -> menu -> Files.
Future<void> _bukaFiles(WidgetTester tester, Workspace w) async {
  await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
  await tester.pumpAndSettle();
  await tester.tap(find.text('produksi'));
  await tester.pumpAndSettle();
  await tester.ensureVisible(find.text('Files'));
  await tester.pumpAndSettle();
  await tester.tap(find.text('Files'));
  await tester.pumpAndSettle();
}

/// Membuka Server -> menu -> satu modul.
Future<void> _bukaModul(WidgetTester tester, Workspace w, String modul) async {
  await tester.pumpWidget(PoinhostApp(workspace: w, settings: _settings()));
  await tester.pumpAndSettle();
  await tester.tap(find.text('produksi'));
  await tester.pumpAndSettle();
  await tester.ensureVisible(find.text(modul));
  await tester.pumpAndSettle();
  await tester.tap(find.text(modul));
  await tester.pumpAndSettle();
}
