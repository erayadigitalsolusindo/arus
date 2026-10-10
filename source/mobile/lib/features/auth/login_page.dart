import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_svg/flutter_svg.dart';

import '../../core/api/api_error.dart';
import '../../core/config/server_url.dart';
import '../../core/session/session_controller.dart';
import '../../core/theme/app_theme.dart';
import '../../core/locale/locale_controller.dart';
import '../../core/theme/theme_controller.dart';
import '../../core/widgets/ocean_background.dart';
import '../../l10n/gen/app_localizations.dart';

/// Layar masuk. Isi form disamakan dengan halaman login web (judul + logo, email, kata sandi tampil/sembunyi,
/// "Tetap masuk", tombol Masuk, kotak galat merah, sisa percobaan, hitung mundur kunci akun), di atas latar lautan.
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
      await ref
          .read(sessionProvider.notifier)
          .login(
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

  Future<void> _editServer() async {
    final l = AppLocalizations.of(context);
    final c = TextEditingController(text: ref.read(serverUrlProvider));
    String? err;
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, setD) => AlertDialog(
          title: Text(l.serverTitle),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(l.serverHelp),
              const SizedBox(height: 12),
              TextField(
                controller: c,
                autofocus: true,
                keyboardType: TextInputType.url,
                autocorrect: false,
                decoration: InputDecoration(
                  labelText: l.serverLabel,
                  hintText: 'http://192.168.1.10:8080',
                  errorText: err,
                ),
              ),
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(ctx).pop(false),
              child: Text(MaterialLocalizations.of(ctx).cancelButtonLabel),
            ),
            FilledButton(
              style: FilledButton.styleFrom(minimumSize: const Size(0, 44)),
              onPressed: () async {
                final saved = await ref
                    .read(serverUrlProvider.notifier)
                    .set(c.text);
                if (saved) {
                  if (ctx.mounted) Navigator.of(ctx).pop(true);
                } else {
                  setD(() => err = l.serverInvalid);
                }
              },
              child: Text(l.save),
            ),
          ],
        ),
      ),
    );
    c.dispose();
    if (ok == true && mounted) setState(() => _error = null);
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final dark = Theme.of(context).brightness == Brightness.dark;
    final session = ref.watch(sessionProvider);
    final expired = session is SessionSignedOut && session.expired;
    final locked = _lockedSeconds > 0;
    final errorText = locked
        ? ApiError(
            code: 'ACCOUNT_LOCKED',
            retryAfter: _lockedSeconds,
          ).message(l)
        : expired && _error == null
        ? ApiError(code: 'SESSION_INVALID').message(l)
        : _error?.message(l);

    return Scaffold(
      body: OceanBackground(
        animate: ref.watch(oceanAnimateProvider),
        child: SafeArea(
          child: Stack(
            children: [
              Center(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 20,
                    vertical: 24,
                  ),
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 420),
                    child: Container(
                      padding: const EdgeInsets.fromLTRB(24, 24, 24, 20),
                      decoration: BoxDecoration(
                        color: pal.surface.withValues(alpha: dark ? 0.82 : 0.9),
                        borderRadius: BorderRadius.circular(22),
                        border: Border.all(
                          color: Colors.white.withValues(
                            alpha: dark ? 0.08 : 0.6,
                          ),
                        ),
                        boxShadow: [
                          BoxShadow(
                            color: Colors.black.withValues(
                              alpha: dark ? 0.35 : 0.12,
                            ),
                            blurRadius: 30,
                            offset: const Offset(0, 12),
                          ),
                        ],
                      ),
                      child: Form(
                        key: _form,
                        child: Column(
                          mainAxisSize: MainAxisSize.min,
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: [
                            Text(
                              l.loginTitleLine1,
                              textAlign: TextAlign.center,
                              style: const TextStyle(
                                fontSize: 22,
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                            const SizedBox(height: 4),
                            SvgPicture.asset(
                              'assets/images/logo_dengan_text-no-bg.svg',
                              height: 80,
                              semanticsLabel: l.loginTitleLine2,
                            ),
                            const SizedBox(height: 6),
                            Text(
                              l.loginSubtitle,
                              textAlign: TextAlign.center,
                              style: TextStyle(
                                fontSize: 12.5,
                                color: pal.textTertiary,
                              ),
                            ),
                            const SizedBox(height: 24),
                            if (errorText != null) ...[
                              _AlertBox(
                                text: errorText,
                                bold: _error?.attemptsLeft == 1,
                              ),
                              const SizedBox(height: 16),
                            ],
                            _Label(l.loginEmail),
                            const SizedBox(height: 6),
                            TextFormField(
                              controller: _email,
                              keyboardType: TextInputType.emailAddress,
                              textInputAction: TextInputAction.next,
                              autofillHints: const [
                                AutofillHints.username,
                                AutofillHints.email,
                              ],
                              autocorrect: false,
                              enableSuggestions: false,
                              onFieldSubmitted: (_) =>
                                  _passwordFocus.requestFocus(),
                              decoration: InputDecoration(
                                hintText: l.loginEmailHint,
                                prefixIcon: Icon(
                                  Icons.mail_outline,
                                  size: 18,
                                  color: pal.textTertiary,
                                ),
                              ),
                              validator: (v) {
                                final s = v?.trim() ?? '';
                                if (s.isEmpty) return l.loginEmailRequired;
                                return _emailRe.hasMatch(s)
                                    ? null
                                    : l.loginEmailInvalid;
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
                                prefixIcon: Icon(
                                  Icons.lock_outline,
                                  size: 18,
                                  color: pal.textTertiary,
                                ),
                                suffixIcon: IconButton(
                                  tooltip: _showPassword
                                      ? l.loginHidePassword
                                      : l.loginShowPassword,
                                  icon: Icon(
                                    _showPassword
                                        ? Icons.visibility_off_outlined
                                        : Icons.visibility_outlined,
                                    size: 18,
                                  ),
                                  color: pal.textTertiary,
                                  onPressed: () => setState(
                                    () => _showPassword = !_showPassword,
                                  ),
                                ),
                              ),
                              validator: (v) => (v ?? '').isEmpty
                                  ? l.loginPasswordRequired
                                  : null,
                            ),
                            const SizedBox(height: 8),
                            InkWell(
                              onTap: () =>
                                  setState(() => _remember = !_remember),
                              borderRadius: BorderRadius.circular(6),
                              child: Padding(
                                padding: const EdgeInsets.symmetric(
                                  vertical: 6,
                                ),
                                child: Row(
                                  children: [
                                    SizedBox(
                                      width: 22,
                                      height: 22,
                                      child: Checkbox(
                                        value: _remember,
                                        onChanged: (v) => setState(
                                          () => _remember = v ?? false,
                                        ),
                                        activeColor: AppColors.primary600,
                                        visualDensity: VisualDensity.compact,
                                      ),
                                    ),
                                    const SizedBox(width: 8),
                                    Expanded(
                                      child: Text(
                                        l.loginRemember,
                                        style: const TextStyle(
                                          fontSize: 12,
                                          fontWeight: FontWeight.w500,
                                        ),
                                      ),
                                    ),
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
                                      ? const SizedBox(
                                          width: 16,
                                          height: 16,
                                          child: CircularProgressIndicator(
                                            strokeWidth: 2,
                                            color: Colors.white,
                                          ),
                                        )
                                      : const Icon(
                                          Icons.arrow_forward,
                                          size: 16,
                                        ),
                                ],
                              ),
                            ),
                            const SizedBox(height: 18),
                            InkWell(
                              onTap: _loading ? null : _editServer,
                              borderRadius: BorderRadius.circular(8),
                              child: Padding(
                                padding: const EdgeInsets.symmetric(
                                  vertical: 6,
                                ),
                                child: Row(
                                  mainAxisAlignment: MainAxisAlignment.center,
                                  children: [
                                    Flexible(
                                      child: Text(
                                        '${l.loginServer}: ${ref.watch(serverUrlProvider)}',
                                        overflow: TextOverflow.ellipsis,
                                        style: TextStyle(
                                          fontSize: 11,
                                          color: pal.textTertiary,
                                        ),
                                      ),
                                    ),
                                    const SizedBox(width: 6),
                                    Icon(
                                      Icons.edit_outlined,
                                      size: 13,
                                      color: pal.textTertiary,
                                    ),
                                  ],
                                ),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                  ),
                ),
              ),
              const Positioned(
                top: 4,
                right: 4,
                child: Row(children: [LanguageButton(), ThemeToggleButton()]),
              ),
            ],
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
    style: TextStyle(
      fontSize: 11.5,
      fontWeight: FontWeight.w600,
      letterSpacing: 0.5,
      color: context.pal.textTertiary,
    ),
  );
}

class _AlertBox extends StatelessWidget {
  const _AlertBox({required this.text, this.bold = false});

  final String text;
  final bool bold;

  @override
  Widget build(BuildContext context) {
    final pal = context.pal;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: pal.dangerSoft,
        borderRadius: BorderRadius.circular(10),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.only(top: 1),
            child: Icon(Icons.error_outline, size: 16, color: pal.dangerText),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              text,
              style: TextStyle(
                fontSize: 12.5,
                color: pal.dangerText,
                fontWeight: bold ? FontWeight.w600 : FontWeight.w400,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
