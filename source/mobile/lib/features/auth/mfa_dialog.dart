import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_error.dart';
import '../../core/session/session_controller.dart';
import '../../l10n/gen/app_localizations.dart';

/// Langkah kedua login untuk akun ber-2FA (TOTP). Berhasil → sesi terbentuk (router mengalihkan ke beranda) dan dialog
/// menutup. Tantangan kedaluwarsa menutup dialog dengan pesan; salah kode menampilkan sisa percobaan.
Future<void> showMfaDialog(BuildContext context, String token) {
  return showDialog<void>(
    context: context,
    barrierDismissible: false,
    builder: (_) => MfaDialog(token: token),
  );
}

class MfaDialog extends ConsumerStatefulWidget {
  const MfaDialog({super.key, required this.token});

  final String token;

  @override
  ConsumerState<MfaDialog> createState() => _MfaDialogState();
}

class _MfaDialogState extends ConsumerState<MfaDialog> {
  final _code = TextEditingController();
  bool _busy = false;
  ApiError? _error;

  @override
  void dispose() {
    _code.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final code = _code.text.trim();
    if (_busy || code.isEmpty) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref
          .read(sessionProvider.notifier)
          .loginMfa(token: widget.token, code: code);
      if (mounted) Navigator.of(context).pop();
    } on ApiError catch (e) {
      if (!mounted) return;
      if (e.code == 'MFA_TOKEN_INVALID' || e.code == 'ACCOUNT_LOCKED') {
        // Tantangan hangus atau akun terkunci sementara: kembali ke login dengan alasannya.
        final l = AppLocalizations.of(context);
        final msg = e.message(l);
        Navigator.of(context).pop();
        ScaffoldMessenger.maybeOf(context)
            ?.showSnackBar(SnackBar(content: Text(msg)));
        return;
      }
      setState(() => _error = e);
      _code.clear();
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return AlertDialog(
      title: Text(l.mfaTitle),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(l.mfaIntro),
          const SizedBox(height: 16),
          TextField(
            controller: _code,
            autofocus: true,
            enabled: !_busy,
            maxLength: 14,
            keyboardType: TextInputType.visiblePassword,
            autofillHints: const [AutofillHints.oneTimeCode],
            textInputAction: TextInputAction.done,
            onSubmitted: (_) => _submit(),
            style: const TextStyle(fontSize: 20, letterSpacing: 4),
            decoration: InputDecoration(
              labelText: l.mfaCode,
              counterText: '',
              errorText: _error?.message(l),
            ),
          ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: _busy ? null : () => Navigator.of(context).pop(),
          child: Text(l.mfaCancel),
        ),
        FilledButton(
          onPressed: _busy ? null : _submit,
          child: _busy
              ? const SizedBox(
                  width: 18,
                  height: 18,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : Text(l.mfaSubmit),
        ),
      ],
    );
  }
}
