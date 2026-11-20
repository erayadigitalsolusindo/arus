import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/config/app_config.dart';
import '../../core/config/server_url.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';

/// Pilih alamat server: kartu Produksi / Emulator / Alamat lain, dengan status enkripsi dan tes koneksi (`/healthz`).
/// Controller dimiliki dialog sendiri agar dibuang setelah animasi tutup selesai.
class ServerDialog extends ConsumerStatefulWidget {
  const ServerDialog({super.key});

  @override
  ConsumerState<ServerDialog> createState() => _ServerDialogState();
}

enum _Result { idle, testing, ok, fail }

class _ServerDialogState extends ConsumerState<ServerDialog> {
  late final TextEditingController _c = TextEditingController(
    text: ref.read(serverUrlProvider),
  );
  String? _err;
  _Result _result = _Result.idle;
  int _ms = 0;

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  bool get _isProd => _c.text.trim() == AppConfig.productionUrl;
  bool get _isEmu => _c.text.trim() == AppConfig.emulatorUrl;

  void _pick(String url) => setState(() {
    _c.text = url;
    _err = null;
    _result = _Result.idle;
  });

  Future<void> _test() async {
    final url = normalizeServerUrl(_c.text);
    if (url == null) {
      setState(() => _err = AppLocalizations.of(context).serverInvalid);
      return;
    }
    setState(() => _result = _Result.testing);
    final sw = Stopwatch()..start();
    try {
      await Dio(
        BaseOptions(
          connectTimeout: const Duration(seconds: 5),
          receiveTimeout: const Duration(seconds: 5),
        ),
      ).get<void>('$url/healthz');
      if (!mounted) return;
      setState(() {
        _result = _Result.ok;
        _ms = sw.elapsedMilliseconds;
      });
    } catch (_) {
      if (mounted) setState(() => _result = _Result.fail);
    }
  }

  Future<void> _save() async {
    final l = AppLocalizations.of(context);
    final saved = await ref.read(serverUrlProvider.notifier).set(_c.text);
    if (!mounted) return;
    if (saved) {
      Navigator.of(context).pop(true);
    } else {
      setState(() => _err = l.serverInvalid);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final custom = !_isProd && !_isEmu;
    final url = _c.text.trim();
    final secure = url.startsWith('https://');
    final (statusText, statusColor) = switch (_result) {
      _Result.ok => (l.serverOk(_ms), pal.success),
      _Result.fail => (l.serverFail, pal.danger),
      _Result.testing => (l.serverTesting, pal.textTertiary),
      _Result.idle => ('', pal.textTertiary),
    };
    return AlertDialog(
      icon: Icon(Icons.dns_rounded, color: pal.primary, size: 32),
      title: Text(l.serverTitle, textAlign: TextAlign.center),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _OptionCard(
              icon: Icons.cloud_done_rounded,
              title: l.serverPresetProd,
              desc: l.serverProdDesc,
              host: Uri.parse(AppConfig.productionUrl).host,
              selected: _isProd,
              onTap: () => _pick(AppConfig.productionUrl),
            ),
            _OptionCard(
              icon: Icons.phone_android_rounded,
              title: l.serverPresetEmulator,
              desc: l.serverEmuDesc,
              host: Uri.parse(AppConfig.emulatorUrl).authority,
              selected: _isEmu,
              onTap: () => _pick(AppConfig.emulatorUrl),
            ),
            _OptionCard(
              icon: Icons.edit_rounded,
              title: l.serverCustomTitle,
              desc: l.serverCustomDesc,
              selected: custom,
              onTap: () => setState(() {
                if (!custom) _c.clear();
                _result = _Result.idle;
              }),
            ),
            if (custom || _c.text.isEmpty) ...[
              const SizedBox(height: 4),
              TextField(
                controller: _c,
                autofocus: true,
                onChanged: (_) => setState(() {
                  _err = null;
                  _result = _Result.idle;
                }),
                keyboardType: TextInputType.url,
                autocorrect: false,
                decoration: InputDecoration(
                  labelText: l.serverLabel,
                  hintText: 'http://192.168.1.10:8080',
                  errorText: _err,
                ),
              ),
            ],
            const SizedBox(height: 12),
            if (url.isNotEmpty)
              Row(
                children: [
                  Icon(
                    secure ? Icons.lock_rounded : Icons.lock_open_rounded,
                    size: 15,
                    color: secure ? pal.success : pal.warningText,
                  ),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Text(
                      secure ? l.serverSecure : l.serverInsecure,
                      style: TextStyle(
                        fontSize: 12,
                        color: secure ? pal.success : pal.warningText,
                      ),
                    ),
                  ),
                ],
              ),
            const SizedBox(height: 8),
            Row(
              children: [
                OutlinedButton.icon(
                  onPressed: _result == _Result.testing ? null : _test,
                  icon: _result == _Result.testing
                      ? const SizedBox(
                          width: 14,
                          height: 14,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Icon(Icons.network_check_rounded, size: 18),
                  label: Text(l.serverTest),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    statusText,
                    style: TextStyle(fontSize: 12.5, color: statusColor),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(false),
          child: Text(MaterialLocalizations.of(context).cancelButtonLabel),
        ),
        FilledButton(
          style: FilledButton.styleFrom(minimumSize: const Size(0, 44)),
          onPressed: _save,
          child: Text(l.save),
        ),
      ],
    );
  }
}

class _OptionCard extends StatelessWidget {
  const _OptionCard({
    required this.icon,
    required this.title,
    required this.desc,
    required this.selected,
    required this.onTap,
    this.host,
  });

  final IconData icon;
  final String title;
  final String desc;
  final String? host;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final pal = context.pal;
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Material(
        color: selected ? pal.primarySoft : pal.surface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(14),
          side: BorderSide(
            color: selected ? pal.primary : pal.border,
            width: selected ? 1.5 : 1,
          ),
        ),
        child: InkWell(
          borderRadius: BorderRadius.circular(14),
          onTap: onTap,
          child: Padding(
            padding: const EdgeInsets.all(12),
            child: Row(
              children: [
                Icon(icon, color: selected ? pal.primary : pal.textTertiary),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        title,
                        style: TextStyle(
                          fontWeight: FontWeight.w700,
                          color: pal.text,
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        desc,
                        style: TextStyle(fontSize: 12, color: pal.textTertiary),
                      ),
                      if (host != null) ...[
                        const SizedBox(height: 4),
                        Text(
                          host!,
                          style: TextStyle(
                            fontSize: 12,
                            fontFamily: 'monospace',
                            color: pal.text,
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
                Icon(
                  selected
                      ? Icons.radio_button_checked
                      : Icons.radio_button_off,
                  color: selected ? pal.primary : pal.border,
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
