import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_error.dart';
import '../../core/session/session_controller.dart';
import '../../l10n/gen/app_localizations.dart';

/// Lembar pendaftaran untuk akun Google yang belum terdaftar. Nama pemilik dan email diambil server dari ID token;
/// di sini hanya data usaha. Berhasil → sesi terbentuk (router mengalihkan ke beranda) dan lembar menutup.
Future<void> showGoogleRegisterSheet(BuildContext context, String idToken) {
  return showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    useSafeArea: true,
    builder: (_) => GoogleRegisterSheet(idToken: idToken),
  );
}

class GoogleRegisterSheet extends ConsumerStatefulWidget {
  const GoogleRegisterSheet({super.key, required this.idToken});

  final String idToken;

  @override
  ConsumerState<GoogleRegisterSheet> createState() =>
      _GoogleRegisterSheetState();
}

class _GoogleRegisterSheetState extends ConsumerState<GoogleRegisterSheet> {
  final _form = GlobalKey<FormState>();
  final _business = TextEditingController();
  final _outlet = TextEditingController();
  final _phone = TextEditingController();
  bool _terms = false;
  bool _termsError = false;
  bool _loading = false;
  ApiError? _error;

  @override
  void dispose() {
    _business.dispose();
    _outlet.dispose();
    _phone.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (_loading) return;
    setState(() {
      _error = null;
      _termsError = !_terms;
    });
    if (!_form.currentState!.validate() || !_terms) return;
    setState(() => _loading = true);
    try {
      await ref
          .read(sessionProvider.notifier)
          .googleRegister(
            idToken: widget.idToken,
            businessName: _business.text.trim(),
            outletName: _outlet.text.trim(),
            phone: _phone.text.trim(),
          );
      if (mounted) Navigator.of(context).pop();
    } on ApiError catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    String? required(String? v) =>
        (v ?? '').trim().isEmpty ? l.googleRegRequired : null;
    return Padding(
      padding: EdgeInsets.fromLTRB(
        20,
        20,
        20,
        20 + MediaQuery.viewInsetsOf(context).bottom,
      ),
      child: SingleChildScrollView(
        child: Form(
          key: _form,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                l.googleRegTitle,
                style: const TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w700,
                ),
              ),
              const SizedBox(height: 4),
              Text(l.googleRegIntro, style: const TextStyle(fontSize: 12.5)),
              const SizedBox(height: 16),
              if (_error != null) ...[
                Text(
                  _error!.message(l),
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
                const SizedBox(height: 12),
              ],
              TextFormField(
                controller: _business,
                textInputAction: TextInputAction.next,
                maxLength: 100,
                decoration: InputDecoration(
                  labelText: l.googleRegBusiness,
                  counterText: '',
                ),
                validator: required,
              ),
              const SizedBox(height: 12),
              TextFormField(
                controller: _outlet,
                textInputAction: TextInputAction.next,
                maxLength: 100,
                decoration: InputDecoration(
                  labelText: l.googleRegOutlet,
                  counterText: '',
                ),
                validator: required,
              ),
              const SizedBox(height: 12),
              TextFormField(
                controller: _phone,
                keyboardType: TextInputType.phone,
                textInputAction: TextInputAction.done,
                maxLength: 24,
                decoration: InputDecoration(
                  labelText: l.googleRegPhone,
                  counterText: '',
                ),
                validator: required,
                onFieldSubmitted: (_) => _submit(),
              ),
              const SizedBox(height: 8),
              CheckboxListTile(
                value: _terms,
                onChanged: (v) => setState(() => _terms = v ?? false),
                controlAffinity: ListTileControlAffinity.leading,
                contentPadding: EdgeInsets.zero,
                dense: true,
                title: Text(
                  l.googleRegTerms,
                  style: const TextStyle(fontSize: 12),
                ),
                subtitle: _termsError
                    ? Text(
                        l.googleRegTermsRequired,
                        style: TextStyle(
                          fontSize: 11.5,
                          color: Theme.of(context).colorScheme.error,
                        ),
                      )
                    : null,
              ),
              const SizedBox(height: 8),
              FilledButton(
                onPressed: _loading ? null : _submit,
                child: _loading
                    ? const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: Colors.white,
                        ),
                      )
                    : Text(l.googleRegSubmit),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
