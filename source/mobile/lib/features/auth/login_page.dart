import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_svg/flutter_svg.dart';

import '../../core/api/api_error.dart';
import '../../core/config/app_config.dart';
import '../../core/session/session_controller.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';

/// Layar masuk. Disamakan dengan halaman login web: judul + logo, email, kata sandi (tampil/sembunyi),
/// "Tetap masuk", tombol Masuk, kotak galat merah, dan hitung mundur saat akun dikunci sementara.
class LoginPage extends ConsumerStatefulWidget {
  const LoginPage({super.key});

  @override
  ConsumerState<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends ConsumerState<LoginPage> {
  final _form = GlobalKey<FormState>();
  final _email = TextEditingController();
  final _password = TextEditingController();
  final _passwordFocus = FocusNode();

  bool _remember = true;
  bool _showPassword = false;
  bool _loading = false;
  ApiError? _error;
  DateTime? _lockedUntil;
  Timer? _tick;

  static final _emailRe = RegExp(r'^[^\s@]+@[^\s@]+\.[^\s@]+$');

  @override
  void dispose() {
    _tick?.cancel();
    _email.dispose();
    _password.dispose();
    _passwordFocus.dispose();
    super.dispose();
  }

  int get _lockedSeconds {
    final u = _lockedUntil;
    if (u == null) return 0;
    final s = u.difference(DateTime.now()).inSeconds + 1;
    return s > 0 ? s : 0;
  }

  void _startLockTimer() {
    _tick?.cancel();
    _tick = Timer.periodic(const Duration(seconds: 1), (t) {
      if (!mounted) return t.cancel();
      if (_lockedSeconds == 0) {
        t.cancel();
        _lockedUntil = null;
      }
      setState(() {});
    });
  }

  Future<void> _submit() async {
    if (_loading || _lockedSeconds > 0) return;
    setState(() => _error = null);
    if (!_form.currentState!.validate()) return;

    setState(() => _loading = true);
    try {
      await ref.read(sessionProvider.notifier).login(
            email: _email.text.trim(),
            password: _password.text,
            remember: _remember,
          );
      // Router mengalihkan ke beranda begitu sesi terbentuk.
    } on ApiError catch (e) {
      if (!mounted) return;
      if (e.code == 'ACCOUNT_LOCKED' && e.retryAfter > 0) {
        _lockedUntil = DateTime.now().add(Duration(seconds: e.retryAfter));
        _startLockTimer();
      }
      setState(() => _error = e);
      if (e.code == 'INVALID_CREDENTIALS') _password.clear();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final session = ref.watch(sessionProvider);
    final expired = session is SessionSignedOut && session.expired;
    final locked = _lockedSeconds > 0;
    final errorText = locked
        ? ApiError(code: 'ACCOUNT_LOCKED', retryAfter: _lockedSeconds).message(l)
        : expired && _error == null
            ? ApiError(code: 'SESSION_INVALID').message(l)
            : _error?.message(l);

    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 32),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 400),
              child: Form(
                key: _form,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Text(
                      l.loginTitleLine1,
                      textAlign: TextAlign.center,
                      style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w700),
                    ),
                    const SizedBox(height: 4),
                    SvgPicture.asset('assets/images/logo_dengan_text-no-bg.svg', height: 80, semanticsLabel: l.loginTitleLine2),
                    const SizedBox(height: 6),
                    Text(
                      l.loginSubtitle,
                      textAlign: TextAlign.center,
                      style: const TextStyle(fontSize: 12.5, color: AppColors.textTertiary),
                    ),
                    const SizedBox(height: 28),
                    if (errorText != null) ...[
                      _AlertBox(text: errorText, bold: _error?.attemptsLeft == 1),
                      const SizedBox(height: 16),
                    ],
                    _Label(l.loginEmail),
                    const SizedBox(height: 6),
                    TextFormField(
                      controller: _email,
                      keyboardType: TextInputType.emailAddress,
                      textInputAction: TextInputAction.next,
                      autofillHints: const [AutofillHints.username, AutofillHints.email],
                      autocorrect: false,
                      enableSuggestions: false,
                      onFieldSubmitted: (_) => _passwordFocus.requestFocus(),
                      decoration: InputDecoration(
                        hintText: l.loginEmailHint,
                        prefixIcon: const Icon(Icons.mail_outline, size: 18, color: AppColors.textTertiary),
                      ),
                      validator: (v) {
                        final s = v?.trim() ?? '';
                        if (s.isEmpty) return l.loginEmailRequired;
                        return _emailRe.hasMatch(s) ? null : l.loginEmailInvalid;
                      },
                    ),
                    const SizedBox(height: 16),
                    _Label(l.loginPassword),
                    const SizedBox(height: 6),
                    TextFormField(
                      controller: _password,
                      focusNode: _passwordFocus,
                      obscureText: !_showPassword,
                      textInputAction: TextInputAction.done,
                      autofillHints: const [AutofillHints.password],
                      autocorrect: false,
                      enableSuggestions: false,
                      onFieldSubmitted: (_) => _submit(),
                      decoration: InputDecoration(
                        hintText: '••••••••',
                        prefixIcon: const Icon(Icons.lock_outline, size: 18, color: AppColors.textTertiary),
                        suffixIcon: IconButton(
                          tooltip: _showPassword ? l.loginHidePassword : l.loginShowPassword,
                          icon: Icon(_showPassword ? Icons.visibility_off_outlined : Icons.visibility_outlined, size: 18),
                          color: AppColors.textTertiary,
                          onPressed: () => setState(() => _showPassword = !_showPassword),
                        ),
                      ),
                      validator: (v) => (v ?? '').isEmpty ? l.loginPasswordRequired : null,
                    ),
                    const SizedBox(height: 8),
                    InkWell(
                      onTap: () => setState(() => _remember = !_remember),
                      borderRadius: BorderRadius.circular(6),
                      child: Padding(
                        padding: const EdgeInsets.symmetric(vertical: 6),
                        child: Row(
                          children: [
                            SizedBox(
                              width: 22,
                              height: 22,
                              child: Checkbox(
                                value: _remember,
                                onChanged: (v) => setState(() => _remember = v ?? false),
                                activeColor: AppColors.primary600,
                                visualDensity: VisualDensity.compact,
                              ),
                            ),
                            const SizedBox(width: 8),
                            Expanded(child: Text(l.loginRemember, style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w500))),
                          ],
                        ),
                      ),
                    ),
                    const SizedBox(height: 16),
                    FilledButton(
                      onPressed: _loading || locked ? null : _submit,
                      child: Row(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: [
                          Text(l.loginSubmit),
                          const SizedBox(width: 8),
                          _loading
                              ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white))
                              : const Icon(Icons.arrow_forward, size: 16),
                        ],
                      ),
                    ),
                    const SizedBox(height: 24),
                    Text(
                      '${l.loginServer}: ${AppConfig.apiUrl}',
                      textAlign: TextAlign.center,
                      style: const TextStyle(fontSize: 11, color: AppColors.textTertiary),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _Label extends StatelessWidget {
  const _Label(this.text);

  final String text;

  @override
  Widget build(BuildContext context) => Text(
        text.toUpperCase(),
        style: const TextStyle(fontSize: 11.5, fontWeight: FontWeight.w600, letterSpacing: 0.5, color: AppColors.textTertiary),
      );
}

class _AlertBox extends StatelessWidget {
  const _AlertBox({required this.text, this.bold = false});

  final String text;
  final bool bold;

  @override
  Widget build(BuildContext context) => Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        decoration: BoxDecoration(color: AppColors.danger100, borderRadius: BorderRadius.circular(10)),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Padding(
              padding: EdgeInsets.only(top: 1),
              child: Icon(Icons.error_outline, size: 16, color: AppColors.dangerText),
            ),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                text,
                style: TextStyle(fontSize: 12.5, color: AppColors.dangerText, fontWeight: bold ? FontWeight.w600 : FontWeight.w400),
              ),
            ),
          ],
        ),
      );
}
