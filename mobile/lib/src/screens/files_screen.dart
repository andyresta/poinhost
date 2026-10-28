import 'dart:convert';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import '../api/agent_exception.dart';
import '../api/models.dart';
import '../format.dart';
import '../session.dart';

/// Menjelajah berkas di server, dan mengubahnya.
///
/// Jangkauannya TIDAK dibatasi ke folder tertentu, mengikuti modul Files di
/// app desktop — yang membatasinya adalah izin kendali di sisi agent, bukan
/// layar ini.
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

  /// Path yang operasinya sedang berjalan, kalau ada.
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
      final l = await widget.connection.files(widget.path);
      if (!mounted) return;
      setState(() => _listing = l);
    } on AgentException catch (e) {
      if (!mounted) return;
      setState(() => _error = e.message);
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = 'Could not read this folder: $e');
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

  /// Menjalankan satu operasi, lalu memuat ulang folder.
  ///
  /// Dimuat ulang dari server, bukan ditebak di sisi HP: nama yang bentrok,
  /// izin yang menolak, dan berkas yang ternyata sudah berubah hanya
  /// terlihat dari jawaban server.
  Future<void> _jalankan(String path, Future<void> Function() op) async {
    setState(() => _busy = path);
    try {
      await op();
      await _muat();
    } on AgentException catch (e) {
      _pesan(e.message);
    } catch (e) {
      _pesan('Failed: $e');
    } finally {
      if (mounted) setState(() => _busy = null);
    }
  }

  void _pesan(String teks) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(teks)));
  }

  Future<void> _rename(FileEntry e) async {
    final c = TextEditingController(text: e.name);
    final nama = await showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Rename'),
        content: TextField(
          controller: c,
          autofocus: true,
          decoration: const InputDecoration(hintText: 'New name'),
          onSubmitted: (v) => Navigator.of(ctx).pop(v),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(ctx).pop(c.text),
            child: const Text('Rename'),
          ),
        ],
      ),
    );
    final baru = nama?.trim();
    if (baru == null || baru.isEmpty || baru == e.name) return;
    if (baru.contains('/')) {
      _pesan('A name cannot contain a slash.');
      return;
    }
    await _jalankan(
      e.path,
      () => widget.connection.renameFile(e.path, _gabung(widget.path, baru)),
    );
  }

  Future<void> _hapus(FileEntry e) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('Delete ${e.name}?'),
        // Konsekuensinya ditulis di dialog, bukan sebagai peringatan tetap
        // di layar: direktori dihapus REKURSIF, dan itu satu-satunya hal
        // yang perlu diketahui tepat sebelum menekan Hapus.
        content: Text(
          e.isDir
              ? 'Everything inside goes with it, and this cannot be undone.'
              : 'This cannot be undone.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            style: FilledButton.styleFrom(
              backgroundColor: Theme.of(ctx).colorScheme.error,
              foregroundColor: Theme.of(ctx).colorScheme.onError,
            ),
            onPressed: () => Navigator.of(ctx).pop(true),
            child: const Text('Delete'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    await _jalankan(e.path, () => widget.connection.deleteFile(e.path));
  }

  Future<void> _unggah() async {
    final dipilih = await FilePicker.pickFiles();
    final berkas = dipilih.firstOrNull;
    if (berkas == null) return;

    // Isinya dibaca ke memori karena itulah yang dikirim ke agent. Batas 16
    // MB di sisi agent yang menjaganya tetap wajar; di sini tidak ada
    // salinan kedua.
    final bytes = await berkas.readAsBytes();
    if (!mounted) return;

    final tujuan = _gabung(widget.path, berkas.name);
    final bentrok =
        _listing?.entries.any((e) => e.name == berkas.name) ?? false;
    if (bentrok) {
      // Menimpa diam-diam adalah cara orang kehilangan berkas tanpa pernah
      // tahu kapan.
      final ok = await showDialog<bool>(
        context: context,
        builder: (ctx) => AlertDialog(
          title: Text('Overwrite ${berkas.name}?'),
          content: const Text('A file with that name is already here.'),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(ctx).pop(false),
              child: const Text('Cancel'),
            ),
            FilledButton(
              onPressed: () => Navigator.of(ctx).pop(true),
              child: const Text('Overwrite'),
            ),
          ],
        ),
      );
      if (ok != true) return;
    }

    await _jalankan(tujuan, () => widget.connection.writeFile(tujuan, bytes));
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final listing = _listing;
    final bisaUbah = listing != null && _error == null;

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
          if (bisaUbah)
            IconButton(
              onPressed: _busy != null ? null : _unggah,
              icon: const Icon(Icons.upload_file_outlined),
              tooltip: 'Upload to this folder',
            ),
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
      body: _error != null
          ? _Pesan(text: _error!, danger: true, onRetry: _muat)
          : listing == null
              ? const Center(child: CircularProgressIndicator())
              : listing.entries.isEmpty
                  ? const _Pesan(text: 'This folder is empty.')
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
                            trailing: _busy == e.path
                                ? const SizedBox(
                                    width: 16,
                                    height: 16,
                                    child: CircularProgressIndicator(
                                      strokeWidth: 2,
                                    ),
                                  )
                                : _MenuEntri(
                                    enabled: _busy == null,
                                    onRename: () => _rename(e),
                                    onDelete: () => _hapus(e),
                                  ),
                            onTap: _busy == null ? () => _buka(e) : null,
                          );
                        },
                      ),
                    ),
    );
  }
}

/// Menu per baris.
///
/// Selalu terlihat, bukan muncul saat ditahan lama: aksi yang hanya ada
/// setelah gestur tersembunyi sama saja dengan tidak ada bagi orang yang
/// tidak tahu gesturnya.
class _MenuEntri extends StatelessWidget {
  const _MenuEntri({
    required this.enabled,
    required this.onRename,
    required this.onDelete,
  });

  final bool enabled;
  final VoidCallback onRename;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return PopupMenuButton<String>(
      enabled: enabled,
      icon: const Icon(Icons.more_vert, size: 20),
      onSelected: (v) => v == 'rename' ? onRename() : onDelete(),
      itemBuilder: (_) => [
        const PopupMenuItem(value: 'rename', child: Text('Rename')),
        PopupMenuItem(
          value: 'delete',
          child: Text(
            'Delete',
            style: TextStyle(color: theme.colorScheme.error),
          ),
        ),
      ],
    );
  }
}

String _gabung(String dir, String nama) =>
    dir == '/' ? '/$nama' : '$dir/$nama';

/// Isi satu file teks, dan penyuntingnya.
class _FileViewScreen extends StatefulWidget {
  const _FileViewScreen({required this.connection, required this.entry});

  final ServerConnection connection;
  final FileEntry entry;

  @override
  State<_FileViewScreen> createState() => _FileViewScreenState();
}

class _FileViewScreenState extends State<_FileViewScreen> {
  final _teks = TextEditingController();
  String? _asli;
  String? _error;
  bool _loading = true;
  bool _menyunting = false;
  bool _menyimpan = false;

  @override
  void dispose() {
    _teks.dispose();
    super.dispose();
  }

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
      setState(() {
        _asli = f.content;
        _teks.text = f.content;
      });
    } on AgentException catch (e) {
      if (!mounted) return;
      setState(() => _error = e.message);
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = 'Could not read this file: $e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _simpan() async {
    setState(() => _menyimpan = true);
    try {
      await widget.connection.writeFile(
        widget.entry.path,
        utf8.encode(_teks.text),
      );
      if (!mounted) return;
      setState(() {
        _asli = _teks.text;
        _menyunting = false;
      });
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Saved.')),
      );
    } on AgentException catch (e) {
      _pesan(e.message);
    } catch (e) {
      _pesan('Could not save: $e');
    } finally {
      if (mounted) setState(() => _menyimpan = false);
    }
  }

  void _pesan(String teks) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(teks)));
  }

  Future<void> _batal() async {
    if (_teks.text == _asli) {
      setState(() => _menyunting = false);
      return;
    }
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Discard changes?'),
        content: const Text('Unsaved edits will be lost.'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: const Text('Keep editing'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: const Text('Discard'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    setState(() {
      _teks.text = _asli ?? '';
      _menyunting = false;
    });
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final gaya = theme.textTheme.bodySmall?.copyWith(
      fontFamily: 'monospace',
      height: 1.5,
    );

    return Scaffold(
      appBar: AppBar(
        leading: _menyunting
            ? IconButton(
                onPressed: _menyimpan ? null : _batal,
                icon: const Icon(Icons.close),
                tooltip: 'Cancel',
              )
            : null,
        title: Text(
          widget.entry.name,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
        ),
        actions: [
          if (_error == null && !_loading)
            _menyunting
                ? IconButton(
                    onPressed: _menyimpan ? null : _simpan,
                    icon: _menyimpan
                        ? const SizedBox(
                            width: 18,
                            height: 18,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : const Icon(Icons.check),
                    tooltip: 'Save',
                  )
                : IconButton(
                    onPressed: () => setState(() => _menyunting = true),
                    icon: const Icon(Icons.edit_outlined),
                    tooltip: 'Edit',
                  ),
          const SizedBox(width: 4),
        ],
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : _error != null
              ? _Pesan(text: _error!, danger: true, onRetry: _muat)
              : _menyunting
                  // Saat menyunting teksnya MEMBUNGKUS, walau saat dibaca
                  // tidak: kursor yang hanyut ke kanan di luar layar membuat
                  // menyunting di HP mustahil, dan itu kerugian yang lebih
                  // besar daripada indentasi yang bergeser sementara.
                  ? Padding(
                      padding: const EdgeInsets.all(16),
                      child: TextField(
                        controller: _teks,
                        maxLines: null,
                        expands: true,
                        textAlignVertical: TextAlignVertical.top,
                        keyboardType: TextInputType.multiline,
                        style: gaya,
                        decoration: const InputDecoration(
                          border: InputBorder.none,
                          filled: false,
                          contentPadding: EdgeInsets.zero,
                        ),
                      ),
                    )
                  : SingleChildScrollView(
                      // Dua arah: berkas config penuh baris panjang, dan
                      // membungkusnya membuat indentasi — yang di config
                      // justru menyampaikan struktur — hilang sama sekali.
                      scrollDirection: Axis.vertical,
                      child: SingleChildScrollView(
                        scrollDirection: Axis.horizontal,
                        padding: const EdgeInsets.all(16),
                        child: SelectableText(_asli ?? '', style: gaya),
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
              OutlinedButton(onPressed: onRetry, child: const Text('Try again')),
            ],
          ],
        ),
      ),
    );
  }
}
