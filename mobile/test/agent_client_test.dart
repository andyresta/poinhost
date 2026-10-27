import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:poinhost_mobile/src/api/agent_client.dart';
import 'package:poinhost_mobile/src/api/agent_exception.dart';

/// Tes ini menempuh rantai yang SEBENARNYA: respons HTTP mentah masuk ke
/// AgentClient, dan yang diperiksa adalah AgentFailure yang keluar.
///
/// Tes layar sebelumnya melempar AgentException langsung dari agent palsu,
/// sehingga ia menguji pemetaan di Session tapi melewati penerjemahan kode
/// status di sini — persis tempat perbedaan antara "tidak bisa dihubungi"
/// dan "belum siap" ditentukan.
AgentClient _clientYangMembalas(int status, String body) {
  return AgentClient(
    baseUrl: Uri.parse('https://agent.contoh.id'),
    httpClient: MockClient((_) async => http.Response(
          body,
          status,
          headers: {'content-type': 'application/json'},
        )),
  );
}

Future<AgentFailure> _gagalnya(AgentClient c) async {
  try {
    await c.server(deviceToken: 'token');
    fail('tidak melempar apa pun');
  } on AgentException catch (e) {
    return e.failure;
  }
}

void main() {
  test('503 berarti agent belum siap, bukan tidak terjangkau', () async {
    final f = await _gagalnya(_clientYangMembalas(
      503,
      '{"error":"agent belum mengelola mesinnya sendiri"}',
    ));
    expect(f, AgentFailure.agentNotReady);
  });

  test('401 berarti token perangkat tidak dikenal', () async {
    final f = await _gagalnya(
      _clientYangMembalas(401, '{"error":"token perangkat tidak dikenal"}'),
    );
    expect(f, AgentFailure.tokenRejected);
  });

  // 403 dibedakan lewat field `status`, bukan teks pesannya — pesan boleh
  // berubah, field itu kontrak.
  test('403 menunggu dan 403 dicabut dibedakan', () async {
    expect(
      await _gagalnya(_clientYangMembalas(
        403,
        '{"error":"menunggu","status":"pending"}',
      )),
      AgentFailure.pendingApproval,
    );
    expect(
      await _gagalnya(_clientYangMembalas(
        403,
        '{"error":"dicabut","status":"revoked"}',
      )),
      AgentFailure.accessRevoked,
    );
  });

  // Yang menjawab bukan agent: halaman galat nginx, portal wifi, atau domain
  // yang menunjuk tempat lain. Itu bukan masalah token, dan tidak boleh
  // terbaca sebagai masalah token.
  test('jawaban yang bukan JSON disebut bukan dari agent', () async {
    final f = await _gagalnya(_clientYangMembalas(502, '<html>502</html>'));
    expect(f, AgentFailure.unexpected);
  });

  // Di web SETIAP kegagalan jaringan berbentuk ini — termasuk CORS yang
  // ditolak. Sebelum ada penanganannya, ia lolos dari seluruh penerjemahan
  // di AgentClient dan sampai ke layar sebagai galat yang tidak tertangkap:
  // tombol Daftar ditekan, dan tidak terjadi apa-apa sama sekali.
  test('ClientException jadi unreachable, bukan galat yang lolos', () async {
    final c = AgentClient(
      baseUrl: Uri.parse('https://agent.contoh.id'),
      httpClient: MockClient(
        (_) async => throw http.ClientException('Failed to fetch'),
      ),
    );
    expect(await _gagalnya(c), AgentFailure.unreachable);
  });

  test('200 terbaca sebagai snapshot', () async {
    final c = _clientYangMembalas(
      200,
      '{"version":"0.3.0","uptimeSeconds":10,"id":"s1","name":"prod",'
          '"host":"10.0.0.1"}',
    );
    final snap = await c.server(deviceToken: 'token');
    expect(snap.name, 'prod');
    // Metrik DIHILANGKAN agent kalau belum terkumpul, dan harus tetap null —
    // bukan jadi objek berisi nol yang terlihat seperti server menganggur.
    expect(snap.metrics, isNull);
  });
}
