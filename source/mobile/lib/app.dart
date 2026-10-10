import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import 'core/locale/locale_controller.dart';
import 'core/session/session_controller.dart';
import 'core/theme/app_theme.dart';
import 'core/theme/theme_controller.dart';
import 'features/auth/login_page.dart';
import 'features/pos/pos_page.dart';
import 'l10n/gen/app_localizations.dart';
import 'shell/home_page.dart';

/// Router mengikuti status sesi: booting → splash, keluar → /login, masuk → / (akun "hanya kasir" langsung ke /kasir).
final routerProvider = Provider<GoRouter>((ref) {
  final refresh = ValueNotifier<int>(0);
  ref.listen(sessionProvider, (_, _) => refresh.value++);
  ref.onDispose(refresh.dispose);

  return GoRouter(
    refreshListenable: refresh,
    initialLocation: '/',
    redirect: (context, state) {
      final s = ref.read(sessionProvider);
      final at = state.matchedLocation;
      if (s is SessionBooting) return at == '/splash' ? null : '/splash';
      if (s is SessionSignedOut) return at == '/login' ? null : '/login';
      final posOnly = (s as SessionSignedIn).profile.permissions.posOnly;
      if (at == '/login' || at == '/splash') return posOnly ? '/kasir' : '/';
      if (posOnly && at != '/kasir') return '/kasir';
      return null;
    },
    routes: [
      GoRoute(
        path: '/splash',
        builder: (_, _) =>
            const Scaffold(body: Center(child: CircularProgressIndicator())),
      ),
      GoRoute(path: '/login', builder: (_, _) => const LoginPage()),
      GoRoute(path: '/', builder: (_, _) => const HomePage()),
      GoRoute(path: '/kasir', builder: (_, _) => const PosPage()),
    ],
  );
});

class ArusApp extends ConsumerWidget {
  const ArusApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return MaterialApp.router(
      onGenerateTitle: (c) => AppLocalizations.of(c).appName,
      theme: buildTheme(Brightness.light),
      darkTheme: buildTheme(Brightness.dark),
      themeMode: ref.watch(themeModeProvider),
      locale: ref.watch(localeProvider),
      routerConfig: ref.watch(routerProvider),
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      // Bahasa HP; selain id/en jatuh ke Indonesia (bahasa sumber, sama dengan web).
      localeResolutionCallback: (locale, supported) => supported.firstWhere(
        (s) => s.languageCode == locale?.languageCode,
        orElse: () => const Locale('id'),
      ),
      debugShowCheckedModeBanner: false,
    );
  }
}
