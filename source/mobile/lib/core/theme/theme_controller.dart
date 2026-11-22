import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Pilihan tampilan per perangkat: ikut sistem (bawaan), terang, atau gelap. Disimpan di SharedPreferences.
final themeModeProvider = NotifierProvider<ThemeController, ThemeMode>(
  ThemeController.new,
);

class ThemeController extends Notifier<ThemeMode> {
  static const _key = 'arus.theme_mode';

  @override
  ThemeMode build() {
    Future.microtask(_load);
    return ThemeMode.system;
  }

  Future<void> _load() async {
    try {
      final v = (await SharedPreferences.getInstance()).getString(_key);
      state = ThemeMode.values.firstWhere(
        (m) => m.name == v,
        orElse: () => ThemeMode.system,
      );
    } catch (_) {
      // Penyimpanan tidak tersedia: tetap ikut sistem.
    }
  }

  Future<void> set(ThemeMode mode) async {
    state = mode;
    try {
      await (await SharedPreferences.getInstance()).setString(_key, mode.name);
    } catch (_) {}
  }

  /// Urutan putar tombol: sistem → terang → gelap → sistem.
  Future<void> cycle() => set(switch (state) {
    ThemeMode.system => ThemeMode.light,
    ThemeMode.light => ThemeMode.dark,
    ThemeMode.dark => ThemeMode.system,
  });
}
