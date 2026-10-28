import 'package:flutter/material.dart';

import '../api/agent_exception.dart';
import '../api/models.dart';
import '../session.dart';

/// Container Docker di satu server: melihat, membaca log, dan — kalau
/// perangkat ini diberi izin — start, stop, restart.
class DockerScreen extends StatefulWidget {
  const DockerScreen({super.key, required this.connection});

  final ServerConnection connection;

  @override
  State<DockerScreen> createState() => _DockerScreenState();
}

class _DockerScreenState extends State<DockerScreen> {
  ContainerListing? _listing;
  String? _error;
  bool _loading = true;

  /// Id container yang aksinya sedang berjalan.
  String? _busy;

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
      final l = await widget.connection.containers();
      if (!mounted) return;
      setState(() => _listing = l);
    } on AgentException catch (e) {
      if (!mounted) return;
      setState(() => _error = e.message);
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = 'Could not read containers: $e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _aksi(ContainerInfo c, String action) async {
    if (action != 'start') {
      final ok = await _konfirmasi(c, action);
      if (ok != true) return;
    }
    setState(() => _busy = c.id);
    try {
      await widget.connection.containerAction(c.id, action);
      // Dibaca ulang dari server, bukan ditebak: `docker stop` menunggu
      // container berhenti sendiri sampai batas waktunya, jadi keadaan
      // sesudahnya bukan sesuatu yang bisa disimpulkan dari aksinya.
      await _muat();
    } on AgentException catch (e) {
      _pesan(e.message);
    } catch (e) {
      _pesan('Failed: $e');
    } finally {
      if (mounted) setState(() => _busy = null);
    }
  }

  Future<bool?> _konfirmasi(ContainerInfo c, String action) {
    final kata = action == 'stop' ? 'Stop' : 'Restart';
    return showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('$kata ${c.name}?'),
        content: Text(
          action == 'stop'
              ? 'Anything it serves stops with it.'
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

  /// Yang berhenti naik ke atas. Container yang mati adalah satu-satunya
  /// alasan orang membuka layar ini dari HP.
  List<ContainerInfo> get _terurut {
    final out = [...?_listing?.containers];
    out.sort((a, b) {
      if (a.running != b.running) return a.running ? 1 : -1;
      return a.name.compareTo(b.name);
    });
    return out;
  }

  @override
  Widget build(BuildContext context) {
    final listing = _listing;

    return Scaffold(
      appBar: AppBar(
        title: const Text('Docker'),
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
          else if (listing != null && !listing.dockerOk)
            const _Banner(
              text: 'Docker is not installed or its engine is not running.',
              danger: true,
            )
          else if (listing != null && !listing.canControl)
            const _Banner(
              text: 'View only. Grant the control permission from poinhost '
                  'on the desktop to read logs and start, stop or restart.',
            ),

          Expanded(
            child: _loading && listing == null
                ? const Center(child: CircularProgressIndicator())
                : listing == null || _terurut.isEmpty
                    ? const Center(child: Text('No containers.'))
                    : RefreshIndicator(
                        onRefresh: _muat,
                        child: ListView.separated(
                          padding: const EdgeInsets.fromLTRB(16, 8, 16, 28),
                          physics: const AlwaysScrollableScrollPhysics(),
                          itemCount: _terurut.length,
                          separatorBuilder: (_, _) => const SizedBox(height: 8),
                          itemBuilder: (context, i) {
                            final c = _terurut[i];
                            return _ContainerTile(
                              container: c,
                              canControl: listing.canControl,
                              busy: _busy == c.id,
                              anyBusy: _busy != null,
                              onAction: (a) => _aksi(c, a),
                              onLogs: () => Navigator.of(context).push(
                                MaterialPageRoute<void>(
                                  builder: (_) => _LogsScreen(
                                    connection: widget.connection,
                                    container: c,
                                  ),
                                ),
                              ),
                            );
                          },
                        ),
                      ),
          ),
        ],
      ),
    );
  }
}

class _ContainerTile extends StatelessWidget {
  const _ContainerTile({
    required this.container,
    required this.canControl,
    required this.busy,
    required this.anyBusy,
    required this.onAction,
    required this.onLogs,
  });

  final ContainerInfo container;
  final bool canControl;
  final bool busy;
  final bool anyBusy;
  final void Function(String action) onAction;
  final VoidCallback onLogs;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final c = container;
    final warna = switch (c.state) {
      'running' => const Color(0xFF2E9E5B),
      'restarting' || 'created' => const Color(0xFFD98A00),
      _ => theme.colorScheme.error,
    };

    return Card(
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        // Seluruh kartu membuka log, bukan satu tombol kecil: membaca log
        // adalah yang paling sering dilakukan di layar ini, dan sasaran
        // sentuh sebesar kartu jauh lebih mudah dikenai daripada ikon.
        onTap: canControl && !anyBusy ? onLogs : null,
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
                    decoration:
                        BoxDecoration(color: warna, shape: BoxShape.circle),
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      c.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: theme.textTheme.titleSmall?.copyWith(
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ),
                  // Titik berwarna SELALU berpasangan dengan teksnya.
                  Text(
                    c.state,
                    style: theme.textTheme.labelSmall?.copyWith(color: warna),
                  ),
                ],
              ),
              const SizedBox(height: 3),
              Text(
                c.image,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
              if (c.ports.isNotEmpty) ...[
                const SizedBox(height: 2),
                Text(
                  c.ports,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: theme.textTheme.labelSmall?.copyWith(
                    color: theme.colorScheme.onSurfaceVariant,
                  ),
                ),
              ],
              if (canControl) ...[
                const SizedBox(height: 6),
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        c.status,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: theme.textTheme.labelSmall?.copyWith(
                          color: theme.colorScheme.onSurfaceVariant,
                        ),
                      ),
                    ),
                    if (busy)
                      const Padding(
                        padding: EdgeInsets.symmetric(horizontal: 12),
                        child: SizedBox(
                          width: 16,
                          height: 16,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        ),
                      )
                    else if (c.running) ...[
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
      ),
    );
  }
}

/// Ekor log satu container.
class _LogsScreen extends StatefulWidget {
  const _LogsScreen({required this.connection, required this.container});

  final ServerConnection connection;
  final ContainerInfo container;

  @override
  State<_LogsScreen> createState() => _LogsScreenState();
}

class _LogsScreenState extends State<_LogsScreen> {
  String? _isi;
  String? _error;
  bool _loading = true;

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
      final teks = await widget.connection.containerLogs(widget.container.id);
      if (!mounted) return;
      setState(() => _isi = teks);
    } on AgentException catch (e) {
      if (!mounted) return;
      setState(() => _error = e.message);
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = 'Could not read the log: $e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final kosong = (_isi ?? '').trim().isEmpty;

    return Scaffold(
      appBar: AppBar(
        title: Text(
          widget.container.name,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
        ),
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
      body: _loading && _isi == null
          ? const Center(child: CircularProgressIndicator())
          : _error != null
              ? _Banner(text: _error!, danger: true)
              : kosong
                  ? const Center(child: Text('This container logged nothing.'))
                  : SingleChildScrollView(
                      // Mulai dari bawah: baris terakhir adalah yang paling
                      // mungkin menjelaskan kenapa layar ini dibuka.
                      reverse: true,
                      child: SingleChildScrollView(
                        // Dua arah — baris log rutin lebih lebar dari layar,
                        // dan membungkusnya membuat stack trace tidak
                        // terbaca sama sekali.
                        scrollDirection: Axis.horizontal,
                        padding: const EdgeInsets.all(16),
                        child: SelectableText(
                          _isi ?? '',
                          style: theme.textTheme.bodySmall?.copyWith(
                            fontFamily: 'monospace',
                            height: 1.45,
                          ),
                        ),
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
