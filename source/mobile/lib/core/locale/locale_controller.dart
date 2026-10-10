import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../l10n/gen/app_localizations.dart';
import '../session/session_controller.dart';

/// Pilihan bahasa per perangkat: null = ikut HP (bawaan), atau `id` / `en`. Disimpan di SharedPreferences.
final localeProvider = NotifierProvider<LocaleController, Locale?>(
  LocaleController.new,
);

class LocaleController extends Notifier<Locale?> {
  static const _key = 'arus.locale';

  @override
  Locale? build() {
    Future.microtask(_load);
    return null;
  }

  Future<void> _load() async {
    try {
      final v = (await SharedPreferences.getInstance()).getString(_key);
      if (v == 'id' || v == 'en') _apply(Locale(v!));
    } catch (_) {
      // Penyimpanan tidak tersedia: tetap ikut HP.
    }
  }

  void _apply(Locale? l) {
    state = l;
    // Bahasa untuk pesan/email dari server.
    if (l != null) ref.read(apiClientProvider).setLanguage(l.languageCode);
  }

  Future<void> set(Locale? l) async {
    _apply(l);
    try {
      final p = await SharedPreferences.getInstance();
      if (l == null) {
        await p.remove(_key);
      } else {
        await p.setString(_key, l.languageCode);
      }
    } catch (_) {}
  }
}

/// Tombol pemilih bahasa (Ikuti HP / Indonesia / English).
class LanguageButton extends ConsumerWidget {
  const LanguageButton({super.key, this.color});

  final Color? color;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final current = ref.watch(localeProvider)?.languageCode ?? '';
    return PopupMenuButton<String>(
      tooltip: l.languageTooltip,
      icon: Icon(Icons.translate, color: color),
      initialValue: current,
      onSelected: (v) =>
          ref.read(localeProvider.notifier).set(v.isEmpty ? null : Locale(v)),
      itemBuilder: (_) => [
        PopupMenuItem(value: '', child: Text(l.languageSystem)),
        const PopupMenuItem(value: 'id', child: Text('Bahasa Indonesia')),
        const PopupMenuItem(value: 'en', child: Text('English')),
      ],
    );
  }
}
