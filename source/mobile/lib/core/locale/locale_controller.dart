import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../money/money.dart';
import '../session/session_controller.dart';

/// Pilihan bahasa per perangkat: null = ikut HP (bawaan), atau `id` / `en` / `ja`. Disimpan di SharedPreferences.
final localeProvider = NotifierProvider<LocaleController, Locale?>(
  LocaleController.new,
);

class LocaleController extends Notifier<Locale?> {
  static const _key = 'arus.locale';
  static const _candidates = {'id', 'en', 'ja'};

  @override
  Locale? build() {
    Future.microtask(_load);
    return null;
  }

  Future<void> _load() async {
    try {
      final v = (await SharedPreferences.getInstance()).getString(_key);
      if (v != null && _candidates.contains(v)) _apply(Locale(v));
    } catch (_) {
      // Penyimpanan tidak tersedia: tetap ikut HP.
    }
  }

  void _apply(Locale? l) {
    state = l;
    // Bahasa untuk pesan/email dari server dan simbol mata uang (tampilan).
    setMoneyLocale(l?.languageCode);
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
