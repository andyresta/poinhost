import 'package:flutter/material.dart';

import '../api/agent_exception.dart';
import '../session.dart';
import '../widgets/labeled_field.dart';

/// Menambahkan satu server: tempel alamat agent dan token pendaftarannya.
///
/// Token yang diketik di sini BUKAN kredensial akses — menukarnya hanya
/// menghasilkan perangkat yang menunggu di antrean yang terlihat di layar
/// pemilik server. Itu sebabnya layar ini boleh sesederhana ini.
class AddServerScreen extends StatefulWidget {
  const AddServerScreen({super.key, required this.workspace});

  final Workspace workspace;

  @override
  State<AddServerScreen> createState() => _AddServerScreenState();
}

class _AddServerScreenState extends State<AddServerScreen> {
  final _formKey = GlobalKey<FormState>();
  final _urlController = TextEditingController();
  final _tokenController = TextEditingController();
  final _labelController = TextEditingController();

  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _urlController.dispose();
    _tokenController.dispose();
    _labelController.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await widget.workspace.add(
        baseUrl: normalizeAgentUrl(_urlController.text),
        enrollToken: _tokenController.text.trim(),
        label: _labelController.text.trim(),
      );
      if (mounted) Navigator.of(context).pop();
    } on AgentException catch (e) {
      // pendingApproval BUKAN galat di sini: itu hasil yang diharapkan dari
      // pendaftaran. Tapi ia hanya bisa muncul dari refresh SETELAH
      // pendaftaran berhasil, dan saat itu servernya sudah masuk grid.
      if (e.failure == AgentFailure.pendingApproval) {
        if (mounted) Navigator.of(context).pop();
        return;
      }
      setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Scaffold(
      appBar: AppBar(title: const Text('Tambah server')),
      body: SafeArea(
        child: Form(
          key: _formKey,
          // Lebarnya dibatasi, bukan dibiarkan mengikuti layar: isian yang
          // membentang penuh di tablet atau layar lebar jadi jauh lebih
          // panjang daripada isinya, dan matanya harus melompat dari label
          // di kiri ke kursor di kanan.
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: ListView(
                padding: const EdgeInsets.fromLTRB(18, 10, 18, 24),
                children: [
                  Text(
                    'Token pendaftaran ada di panel Mobile Control, di '
                    'poinhost desktop. Server baru muncul di grid setelah '
                    'kamu menyetujui perangkat ini dari sana.',
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                      height: 1.45,
                    ),
                  ),
                  const SizedBox(height: 20),
                  LabeledField(
                    label: 'Alamat agent',
                    controller: _urlController,
                    hint: 'agent.example.com',
                    keyboardType: TextInputType.url,
                    textInputAction: TextInputAction.next,
                    validator: (v) => (v == null || v.trim().isEmpty)
                        ? 'Isi alamat agent'
                        : null,
                  ),
                  LabeledField(
                    label: 'Token pendaftaran',
                    controller: _tokenController,
                    textInputAction: TextInputAction.next,
                    validator: (v) =>
                        (v == null || v.trim().isEmpty) ? 'Isi token' : null,
                  ),
                  LabeledField(
                    label: 'Nama perangkat ini',
                    controller: _labelController,
                    helper: 'Supaya kamu mengenalinya saat menyetujui',
                    textInputAction: TextInputAction.done,
                    onSubmitted: (_) => _busy ? null : _submit(),
                    validator: (v) =>
                        (v == null || v.trim().isEmpty) ? 'Isi nama' : null,
                  ),
                  const SizedBox(height: 10),
                  FilledButton(
                    onPressed: _busy ? null : _submit,
                    child: _busy
                        ? const SizedBox(
                            height: 16,
                            width: 16,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : const Text('Daftar'),
                  ),
                  if (_error != null) ...[
                    const SizedBox(height: 16),
                    Container(
                      padding: const EdgeInsets.all(11),
                      decoration: BoxDecoration(
                        color: theme.colorScheme.errorContainer,
                        borderRadius: BorderRadius.circular(10),
                      ),
                      child: Text(
                        _error!,
                        style: theme.textTheme.bodySmall?.copyWith(
                          color: theme.colorScheme.onErrorContainer,
                        ),
                      ),
                    ),
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Melengkapi skema yang tidak diketik user.
///
/// https, bukan http: agent diterbitkan dengan certbot, dan menebak http
/// akan mengirim token pendaftaran melewati jaringan seluler tanpa enkripsi.
/// Yang mengetik `agent.example.com` hampir pasti memaksudkan yang aman.
Uri normalizeAgentUrl(String raw) {
  final text = raw.trim();
  final withScheme = text.startsWith('http://') || text.startsWith('https://')
      ? text
      : 'https://$text';
  return Uri.parse(
    withScheme.endsWith('/')
        ? withScheme.substring(0, withScheme.length - 1)
        : withScheme,
  );
}
