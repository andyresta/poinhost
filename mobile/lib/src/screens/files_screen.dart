import 'package:flutter/material.dart';

import '../api/agent_exception.dart';
import '../api/models.dart';
import '../format.dart';
import '../session.dart';

/// Menjelajah berkas di server.
///
/// Hanya baca untuk saat ini. Jangkauannya TIDAK dibatasi ke folder tertentu,
/// mengikuti modul Files di app desktop — yang membatasinya adalah izin
/// kendali di sisi agent, bukan layar ini.
class FilesScreen extends StatefulWidget {
  const FilesScreen({super.key, required this.connection, this.path = '/'});

  final ServerConnection connection;
  final String path;

  @override
  State<FilesScreen> createState() => _FilesScreenState();
}

class _FilesScreenState extends State<FilesScreen> {
  FileListing? _listing;
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
      final l = await widget.connection.files(widget.path);
      if (!mounted) return;
      setState(() => _listing = l);
    } on AgentException catch (e) {
      if (!mounted) return;
      setState(() => _error = e.message);
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = 'Gagal membaca folder: $e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _buka(FileEntry e) {
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => e.isDir
            ? FilesScreen(connection: widget.connection, path: e.path)
            : _FileViewScreen(connection: widget.connection, entry: e),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final listing = _listing;

    return Scaffold(
      appBar: AppBar(
        // Judulnya path, bukan "Files": di tumpukan layar yang dalam, yang
        // perlu dijawab adalah "saya ada di mana", dan hanya path yang
        // menjawabnya. Dipotong dari KIRI supaya nama folder terakhir —
        // bagian yang berubah — selalu terlihat.
        title: Text(
          widget.path,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          textDirection: TextDirection.rtl,
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
            tooltip: 'Segarkan',
          ),
          const SizedBox(width: 4),
        ],
      ),
      body: _error != null
          ? _Pesan(text: _error!, danger: true, onRetry: _muat)
          : listing == null
              ? const Center(child: CircularProgressIndicator())
              : listing.entries.isEmpty
                  ? const _Pesan(text: 'Folder ini kosong.')
                  : RefreshIndicator(
                      onRefresh: _muat,
                      child: ListView.separated(
                        physics: const AlwaysScrollableScrollPhysics(),
                        itemCount: listing.entries.length,
                        separatorBuilder: (_, _) => const Divider(height: 1),
                        itemBuilder: (context, i) {
                          final e = listing.entries[i];
                          return ListTile(
                            leading: Icon(
                              e.isDir
                                  ? Icons.folder_outlined
                                  : Icons.description_outlined,
                              color: e.isDir
                                  ? theme.colorScheme.primary
                                  : theme.colorScheme.onSurfaceVariant,
                            ),
                            title: Text(
                              e.name,
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                            ),
                            subtitle: Text(
                              e.isDir
                                  ? e.mode
                                  : '${formatBytes(e.size)} · ${e.mode}',
                              style: theme.textTheme.bodySmall?.copyWith(
                                color: theme.colorScheme.onSurfaceVariant,
                              ),
                            ),
                            trailing: e.isDir
                                ? const Icon(Icons.chevron_right, size: 18)
                                : null,
                            onTap: () => _buka(e),
                          );
                        },
                      ),
                    ),
    );
  }
}

/// Isi satu file teks.
class _FileViewScreen extends StatefulWidget {
  const _FileViewScreen({required this.connection, required this.entry});

  final ServerConnection connection;
  final FileEntry entry;

  @override
  State<_FileViewScreen> createState() => _FileViewScreenState();
}

class _FileViewScreenState extends State<_FileViewScreen> {
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
      final f = await widget.connection.fileContent(widget.entry.path);
      if (!mounted) return;
      setState(() => _isi = f.content);
    } on AgentException catch (e) {
      if (!mounted) return;
      setState(() => _error = e.message);
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = 'Gagal membaca berkas: $e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(
        title: Text(
          widget.entry.name,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
        ),
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : _error != null
              ? _Pesan(text: _error!, danger: true, onRetry: _muat)
              : SingleChildScrollView(
                  // Dua arah: berkas config penuh baris panjang, dan
                  // membungkusnya membuat indentasi — yang di config justru
                  // menyampaikan struktur — hilang sama sekali.
                  scrollDirection: Axis.vertical,
                  child: SingleChildScrollView(
                    scrollDirection: Axis.horizontal,
                    padding: const EdgeInsets.all(16),
                    child: SelectableText(
                      _isi ?? '',
                      style: theme.textTheme.bodySmall?.copyWith(
                        fontFamily: 'monospace',
                        height: 1.5,
                      ),
                    ),
                  ),
                ),
    );
  }
}

class _Pesan extends StatelessWidget {
  const _Pesan({required this.text, this.danger = false, this.onRetry});

  final String text;
  final bool danger;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(28),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              text,
              textAlign: TextAlign.center,
              style: theme.textTheme.bodyMedium?.copyWith(
                color: danger
                    ? theme.colorScheme.error
                    : theme.colorScheme.onSurfaceVariant,
              ),
            ),
            if (onRetry != null) ...[
              const SizedBox(height: 16),
              OutlinedButton(onPressed: onRetry, child: const Text('Coba lagi')),
            ],
          ],
        ),
      ),
    );
  }
}
