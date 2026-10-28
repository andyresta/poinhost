import 'package:flutter/material.dart';

import '../api/agent_exception.dart';
import '../api/models.dart';
import '../session.dart';

/// Unit systemd di satu server: melihat, dan — kalau perangkat ini diberi
/// izin — start, stop, restart.
///
/// Izinnya dibaca dari jawaban agent, bukan ditebak: layar yang menampilkan
/// tombol yang pasti ditolak membuat pemiliknya menyalahkan aplikasi, bukan
/// izin yang memang belum diberikan.
class ServicesScreen extends StatefulWidget {
  const ServicesScreen({super.key, required this.connection});

  final ServerConnection connection;

  @override
  State<ServicesScreen> createState() => _ServicesScreenState();
}

class _ServicesScreenState extends State<ServicesScreen> {
  ServiceList? _list;
  String? _error;
  bool _loading = true;

  /// Nama service yang aksinya sedang berjalan.
  String? _busy;

  String _cari = '';

  @override
  void initState() {
    super.initState();
    _muat();
  }

  Future<void> _muat() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final list = await widget.connection.services();
      if (!mounted) return;
      setState(() => _list = list);
    } on AgentException catch (e) {
      if (!mounted) return;
      setState(() => _error = e.message);
    } catch (e) {
      // Jaring terakhir: layar yang diam setelah dibuka tidak memberi
      // pemiliknya satu pun petunjuk.
      if (!mounted) return;
      setState(() => _error = 'Could not read services: $e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _aksi(ServiceInfo svc, String action) async {
    if (action == 'stop' || action == 'restart') {
      final ok = await _konfirmasi(svc, action);
      if (ok != true) return;
    }
    setState(() => _busy = svc.name);
    try {
      final baru = await widget.connection.serviceAction(svc.name, action);
      if (!mounted) return;
      // Hanya baris yang berubah yang ditulis ulang. Menarik seluruh daftar
      // sesudah tiap aksi berarti satu round-trip SSH lagi di jaringan
      // seluler, demi 90 baris yang tidak berubah.
      setState(() {
        final l = _list;
        if (l == null) return;
        _list = ServiceList(
          systemdAvailable: l.systemdAvailable,
          canControl: l.canControl,
          services: [
            for (final s in l.services) if (s.name == baru.name) baru else s,
          ],
        );
      });
    } on AgentException catch (e) {
      _pesan(e.message);
    } catch (e) {
      _pesan('Failed: $e');
    } finally {
      if (mounted) setState(() => _busy = null);
    }
  }

  Future<bool?> _konfirmasi(ServiceInfo svc, String action) {
    final kata = action == 'stop' ? 'Stop' : 'Restart';
    return showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('$kata ${svc.name}?'),
        content: Text(
          action == 'stop'
              ? 'Anything depending on it stops too.'
              : 'It goes down for a moment before coming back.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: Text(kata),
          ),
        ],
      ),
    );
  }

  void _pesan(String teks) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(teks)));
  }

  /// Urutannya BUKAN abjad murni: yang gagal naik ke atas, lalu yang jalan.
  /// Daftar systemd itu panjang, dan yang dicari orang saat membukanya dari
  /// HP hampir selalu ada di dua kelompok pertama.
  List<ServiceInfo> get _terurut {
    final q = _cari.trim().toLowerCase();
    final out = [
      for (final s in _list?.services ?? const <ServiceInfo>[])
        if (q.isEmpty ||
            s.name.toLowerCase().contains(q) ||
            s.description.toLowerCase().contains(q))
          s,
    ];
    out.sort((a, b) {
      int peringkat(ServiceInfo s) => s.failed ? 0 : (s.running ? 1 : 2);
      final p = peringkat(a).compareTo(peringkat(b));
      return p != 0 ? p : a.name.compareTo(b.name);
    });
    return out;
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final list = _list;

    return Scaffold(
      appBar: AppBar(
        title: const Text('Services'),
        actions: [
          IconButton(
            onPressed: _loading ? null : _muat,
            icon: _loading
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.refresh),
            tooltip: 'Refresh',
          ),
          const SizedBox(width: 4),
        ],
      ),
      body: Column(
        children: [
          if (_error != null)
            _Banner(text: _error!, danger: true)
          else if (list != null && !list.systemdAvailable)
            const _Banner(text: 'This server does not use systemd.')
          else if (list != null && !list.canControl)
            const _Banner(
              text: 'View only. Grant the control permission from poinhost '
                  'on the desktop to start, stop and restart.',
            ),

          if (list != null && list.systemdAvailable)
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 4),
              child: TextField(
                onChanged: (v) => setState(() => _cari = v),
                decoration: const InputDecoration(
                  hintText: 'Search services',
                  prefixIcon: Icon(Icons.search, size: 18),
                ),
              ),
            ),

          Expanded(
            child: _loading && list == null
                ? const Center(child: CircularProgressIndicator())
                : RefreshIndicator(
                    onRefresh: _muat,
                    child: ListView.separated(
                      padding: const EdgeInsets.fromLTRB(16, 8, 16, 28),
                      physics: const AlwaysScrollableScrollPhysics(),
                      itemCount: _terurut.length,
                      separatorBuilder: (_, _) => const SizedBox(height: 8),
                      itemBuilder: (context, i) {
                        final svc = _terurut[i];
                        return _ServiceTile(
                          service: svc,
                          canControl: list?.canControl ?? false,
                          busy: _busy == svc.name,
                          anyBusy: _busy != null,
                          onAction: (a) => _aksi(svc, a),
                        );
                      },
                    ),
                  ),
          ),

          if (list != null && list.systemdAvailable && _terurut.isEmpty)
            Padding(
              padding: const EdgeInsets.all(24),
              child: Text(
                'Nothing matches.',
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ),
        ],
      ),
    );
  }
}

class _ServiceTile extends StatelessWidget {
  const _ServiceTile({
    required this.service,
    required this.canControl,
    required this.busy,
    required this.anyBusy,
    required this.onAction,
  });

  final ServiceInfo service;
  final bool canControl;
  final bool busy;
  final bool anyBusy;
  final void Function(String action) onAction;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final warna = service.failed
        ? theme.colorScheme.error
        : service.running
            ? const Color(0xFF2E9E5B)
            : theme.colorScheme.outline;

    return Card(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(14, 12, 10, 12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Container(
                  width: 9,
                  height: 9,
                  decoration: BoxDecoration(color: warna, shape: BoxShape.circle),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    service.name,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: theme.textTheme.titleSmall?.copyWith(
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ),
                // Titik berwarna SELALU berpasangan dengan teksnya: titik
                // sendirian tidak terbaca oleh sekitar satu dari dua belas
                // laki-laki, dan ini layar yang gunanya menyampaikan status.
                Text(
                  service.failed
                      ? 'failed'
                      : service.running
                          ? 'running'
                          : 'stopped',
                  style: theme.textTheme.labelSmall?.copyWith(color: warna),
                ),
              ],
            ),
            if (service.description.isNotEmpty) ...[
              const SizedBox(height: 3),
              Text(
                service.description,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ],
            if (canControl) ...[
              const SizedBox(height: 6),
              Row(
                mainAxisAlignment: MainAxisAlignment.end,
                children: [
                  if (busy)
                    const Padding(
                      padding: EdgeInsets.symmetric(horizontal: 12),
                      child: SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      ),
                    )
                  else if (service.running) ...[
                    TextButton(
                      onPressed: anyBusy ? null : () => onAction('restart'),
                      child: const Text('Restart'),
                    ),
                    TextButton(
                      onPressed: anyBusy ? null : () => onAction('stop'),
                      child: const Text('Stop'),
                    ),
                  ] else
                    TextButton(
                      onPressed: anyBusy ? null : () => onAction('start'),
                      child: const Text('Start'),
                    ),
                ],
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _Banner extends StatelessWidget {
  const _Banner({required this.text, this.danger = false});

  final String text;
  final bool danger;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Container(
      width: double.infinity,
      margin: const EdgeInsets.fromLTRB(16, 10, 16, 0),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: danger
            ? theme.colorScheme.errorContainer
            : theme.colorScheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(
        text,
        style: theme.textTheme.bodySmall?.copyWith(
          color: danger ? theme.colorScheme.onErrorContainer : null,
        ),
      ),
    );
  }
}
