import 'package:flutter/material.dart';

import '../api/agent_exception.dart';
import '../api/models.dart';
import '../session.dart';

/// Daftar situs di satu server.
class WebsitesScreen extends StatefulWidget {
  const WebsitesScreen({super.key, required this.connection});

  final ServerConnection connection;

  @override
  State<WebsitesScreen> createState() => _WebsitesScreenState();
}

class _WebsitesScreenState extends State<WebsitesScreen> {
  WebsiteListing? _listing;
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
      final l = await widget.connection.websites();
      if (!mounted) return;
      setState(() => _listing = l);
    } on AgentException catch (e) {
      if (!mounted) return;
      setState(() => _error = e.message);
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = 'Could not read the site list: $e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final listing = _listing;

    return Scaffold(
      appBar: AppBar(
        title: const Text('Website'),
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
          else if (listing != null && !listing.nginxInstalled)
            const _Banner(text: 'Nginx is not installed on this server.')
          else if (listing != null && !listing.nginxActive)
            const _Banner(text: 'Nginx is installed but not running.', danger: true),

          Expanded(
            child: _loading && listing == null
                ? const Center(child: CircularProgressIndicator())
                : listing == null || listing.sites.isEmpty
                    ? const Center(child: Text('No sites yet.'))
                    : RefreshIndicator(
                        onRefresh: _muat,
                        child: ListView.separated(
                          physics: const AlwaysScrollableScrollPhysics(),
                          itemCount: listing.sites.length,
                          separatorBuilder: (_, _) => const Divider(height: 1),
                          itemBuilder: (context, i) {
                            final s = listing.sites[i];
                            return ListTile(
                              leading: Container(
                                width: 9,
                                height: 9,
                                margin: const EdgeInsets.only(top: 16),
                                decoration: BoxDecoration(
                                  color: s.enabled
                                      ? const Color(0xFF2E9E5B)
                                      : theme.colorScheme.outline,
                                  shape: BoxShape.circle,
                                ),
                              ),
                              title: Text(
                                s.domain,
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                              ),
                              subtitle: Text(
                                _ringkasan(s),
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: theme.textTheme.bodySmall?.copyWith(
                                  color: theme.colorScheme.onSurfaceVariant,
                                ),
                              ),
                              trailing: const Icon(Icons.chevron_right, size: 18),
                              onTap: () => Navigator.of(context).push(
                                MaterialPageRoute<void>(
                                  builder: (_) => _DomainScreen(
                                    connection: widget.connection,
                                    site: s,
                                    canControl: listing.canControl,
                                    onChanged: _muat,
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

/// Satu baris keterangan: apa yang melayani domain ini, dan apakah HTTPS.
String _ringkasan(WebsiteInfo s) {
  final bagian = <String>[
    if (!s.enabled) 'off',
    s.sslEnabled ? 'HTTPS' : 'HTTP only',
    if (s.proxyTarget.isNotEmpty)
      'proxy ${s.proxyTarget}'
    else if (s.phpEnabled && s.phpVersion.isNotEmpty)
      'PHP ${s.phpVersion}'
    else
      'static',
  ];
  return bagian.join(' · ');
}

class _DomainScreen extends StatefulWidget {
  const _DomainScreen({
    required this.connection,
    required this.site,
    required this.canControl,
    required this.onChanged,
  });

  final ServerConnection connection;
  final WebsiteInfo site;
  final bool canControl;
  final VoidCallback onChanged;

  @override
  State<_DomainScreen> createState() => _DomainScreenState();
}

class _DomainScreenState extends State<_DomainScreen> {
  WebsiteSSL? _ssl;
  String? _sslError;
  bool _memuatSSL = true;

  late bool _enabled = widget.site.enabled;
  bool _mengubah = false;

  @override
  void initState() {
    super.initState();
    _muatSSL();
  }

  Future<void> _muatSSL() async {
    setState(() {
      _memuatSSL = true;
      _sslError = null;
    });
    try {
      final st = await widget.connection.websiteSSL(widget.site.domain);
      if (!mounted) return;
      setState(() => _ssl = st);
    } on AgentException catch (e) {
      if (!mounted) return;
      setState(() => _sslError = e.message);
    } catch (e) {
      if (!mounted) return;
      setState(() => _sslError = 'Could not read the certificate: $e');
    } finally {
      if (mounted) setState(() => _memuatSSL = false);
    }
  }

  Future<void> _ubahAktif(bool nilai) async {
    if (!nilai) {
      final ok = await showDialog<bool>(
        context: context,
        builder: (ctx) => AlertDialog(
          title: Text('Take ${widget.site.domain} offline?'),
          content: const Text(
            'It stops being served until you turn it back on.',
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
              child: const Text('Take offline'),
            ),
          ],
        ),
      );
      if (ok != true) return;
    }

    setState(() => _mengubah = true);
    try {
      await widget.connection.setWebsiteEnabled(widget.site.domain, nilai);
      if (!mounted) return;
      setState(() => _enabled = nilai);
      widget.onChanged();
    } on AgentException catch (e) {
      _pesan(e.message);
    } catch (e) {
      _pesan('Failed: $e');
    } finally {
      if (mounted) setState(() => _mengubah = false);
    }
  }

  void _pesan(String teks) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(teks)));
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final s = widget.site;
    final ssl = _ssl;

    return Scaffold(
      appBar: AppBar(
        title: Text(s.domain, maxLines: 1, overflow: TextOverflow.ellipsis),
      ),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(16, 12, 16, 32),
        children: [
          if (widget.canControl)
            Card(
              child: SwitchListTile(
                title: const Text('Served'),
                subtitle: Text(
                  _enabled ? 'This site is live' : 'This site is offline',
                  style: theme.textTheme.bodySmall?.copyWith(
                    color: theme.colorScheme.onSurfaceVariant,
                  ),
                ),
                value: _enabled,
                onChanged: _mengubah ? null : _ubahAktif,
              ),
            )
          else
            _Baris(
              label: 'Served',
              value: _enabled ? 'Yes' : 'No',
            ),

          const SizedBox(height: 12),
          _Kartu(
            rows: [
              ('Document root', s.root),
              if (s.proxyTarget.isNotEmpty) ('Proxies to', s.proxyTarget),
              (
                'PHP',
                s.phpEnabled && s.phpVersion.isNotEmpty ? s.phpVersion : 'No',
              ),
              if (s.isSubdomain) ('Subdomain of', s.parent),
            ],
          ),

          const SizedBox(height: 18),
          Text('Certificate', style: theme.textTheme.labelLarge),
          const SizedBox(height: 8),

          if (_memuatSSL)
            const Padding(
              padding: EdgeInsets.all(20),
              child: Center(child: CircularProgressIndicator()),
            )
          else if (_sslError != null)
            _Banner(text: _sslError!, danger: true)
          else if (ssl == null || !ssl.certExists)
            const _Banner(text: 'No certificate for this domain yet.')
          else ...[
            // Sisa hari ditulis di depan tanggalnya: yang ingin dijawab
            // layar ini adalah "masih aman atau tidak", bukan "tanggal
            // berapa" — dan tanggal mentah menuntut pembacanya menghitung
            // sendiri.
            _Kadaluarsa(ssl: ssl),
            if (ssl.missingSans.isNotEmpty) ...[
              const SizedBox(height: 10),
              // Ini yang paling mudah terlewat: statusnya "SSL aktif",
              // tapi HTTPS untuk domain-domain ini tidak bekerja — dan
              // `certbot renew` TIDAK memperbaikinya, karena renew tidak
              // pernah mengubah daftar domain di sertifikat.
              _Banner(
                danger: true,
                text: 'Not covered by the certificate: '
                    '${ssl.missingSans.join(", ")}. It has to be reissued, '
                    'not renewed.',
              ),
            ],
            const SizedBox(height: 10),
            _Kartu(
              rows: [
                ('Auto-renew', ssl.autoRenew ? 'Yes' : 'No'),
                if (ssl.certDomains.isNotEmpty)
                  ('Covers', ssl.certDomains.join(', ')),
              ],
            ),
          ],
        ],
      ),
    );
  }
}

class _Kadaluarsa extends StatelessWidget {
  const _Kadaluarsa({required this.ssl});

  final WebsiteSSL ssl;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final sisa = ssl.expiresInDays;

    final (teks, warna) = switch (sisa) {
      null => (ssl.notAfter, theme.colorScheme.onSurfaceVariant),
      < 0 => ('Expired', theme.colorScheme.error),
      < 14 => (_sisaHari(sisa), theme.colorScheme.error),
      < 30 => (_sisaHari(sisa), const Color(0xFFD98A00)),
      _ => (_sisaHari(sisa), const Color(0xFF2E9E5B)),
    };

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Row(
          children: [
            Icon(Icons.lock_outline, size: 18, color: warna),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    teks,
                    style: theme.textTheme.titleSmall?.copyWith(
                      color: warna,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  if (sisa != null && ssl.notAfter.isNotEmpty) ...[
                    const SizedBox(height: 2),
                    Text(
                      'Until ${_tanggal(ssl.notAfter)}',
                      style: theme.textTheme.bodySmall?.copyWith(
                        color: theme.colorScheme.onSurfaceVariant,
                      ),
                    ),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

String _sisaHari(int hari) => hari == 1 ? '1 day left' : '$hari days left';

String _tanggal(String rfc3339) {
  final t = DateTime.tryParse(rfc3339)?.toLocal();
  if (t == null) return rfc3339;
  const bulan = [
    'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
    'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
  ];
  return '${t.day} ${bulan[t.month - 1]} ${t.year}';
}

class _Kartu extends StatelessWidget {
  const _Kartu({required this.rows});

  final List<(String, String)> rows;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 4),
        child: Column(
          children: [
            for (var i = 0; i < rows.length; i++) ...[
              if (i > 0) const Divider(height: 1),
              _Baris(label: rows[i].$1, value: rows[i].$2, rapat: true),
            ],
          ],
        ),
      ),
    );
  }
}

class _Baris extends StatelessWidget {
  const _Baris({required this.label, required this.value, this.rapat = false});

  final String label;
  final String value;
  final bool rapat;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Padding(
      padding: EdgeInsets.symmetric(vertical: rapat ? 11 : 14),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 128,
            child: Text(
              label,
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
          ),
          Expanded(
            child: Text(
              value.isEmpty ? '—' : value,
              style: theme.textTheme.bodyMedium,
            ),
          ),
        ],
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
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
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
