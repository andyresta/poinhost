import 'package:flutter/material.dart';

import '../api/agent_exception.dart';
import '../session.dart';

/// Langkah 2 alurnya: tempel URL agent dan token pendaftaran.
///
/// Token yang diketik di sini BUKAN kredensial akses — menukarnya hanya
/// menghasilkan perangkat yang menunggu di antrean yang terlihat di layar
/// pemilik server. Itu sebabnya layar ini boleh sesederhana ini.
class EnrollScreen extends StatefulWidget {
  const EnrollScreen({super.key, required this.session});

  final Session session;

  @override
  State<EnrollScreen> createState() => _EnrollScreenState();
}

class _EnrollScreenState extends State<EnrollScreen> {
  final _formKey = GlobalKey<FormState>();
  final _urlController = TextEditingController();
  final _tokenController = TextEditingController();
  final _nameController = TextEditingController();

  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _urlController.dispose();
    _tokenController.dispose();
    _nameController.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await widget.session.enroll(
        baseUrl: _normalizeUrl(_urlController.text),
        enrollToken: _tokenController.text.trim(),
        deviceName: _nameController.text.trim(),
      );
    } on AgentException catch (e) {
      // pendingApproval BUKAN galat di sini: itu hasil yang diharapkan dari
      // pendaftaran, dan Session sudah memindahkan layarnya ke ruang tunggu.
      if (e.failure != AgentFailure.pendingApproval) {
        setState(() => _error = e.message);
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Hubungkan server')),
      body: SafeArea(
        child: Form(
          key: _formKey,
          child: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              TextFormField(
                controller: _urlController,
                keyboardType: TextInputType.url,
                autocorrect: false,
                decoration: const InputDecoration(
                  labelText: 'Alamat agent',
                  hintText: 'agent.example.com',
                ),
                validator: (v) => (v == null || v.trim().isEmpty)
                    ? 'Isi alamat agent'
                    : null,
              ),
              const SizedBox(height: 16),
              TextFormField(
                controller: _tokenController,
                autocorrect: false,
                decoration: const InputDecoration(
                  labelText: 'Token pendaftaran',
                  helperText: 'Dari panel Mobile Control di poinhost',
                ),
                validator: (v) =>
                    (v == null || v.trim().isEmpty) ? 'Isi token' : null,
              ),
              const SizedBox(height: 16),
              TextFormField(
                controller: _nameController,
                decoration: const InputDecoration(
                  labelText: 'Nama perangkat',
                  helperText: 'Supaya kamu mengenalinya saat menyetujui',
                ),
                validator: (v) =>
                    (v == null || v.trim().isEmpty) ? 'Isi nama' : null,
              ),
              const SizedBox(height: 24),
              FilledButton(
                onPressed: _busy ? null : _submit,
                child: _busy
                    ? const SizedBox(
                        height: 18,
                        width: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Text('Daftar'),
              ),
              if (_error != null) ...[
                const SizedBox(height: 16),
                Text(
                  _error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ],
            ],
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
Uri _normalizeUrl(String raw) {
  final text = raw.trim();
  final withScheme =
      text.startsWith('http://') || text.startsWith('https://')
          ? text
          : 'https://$text';
  return Uri.parse(withScheme.endsWith('/')
      ? withScheme.substring(0, withScheme.length - 1)
      : withScheme);
}
