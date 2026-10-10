import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../l10n/gen/app_localizations.dart';

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

/// Tombol ikon pengganti tema; ikon menunjukkan pilihan sekarang.
class ThemeToggleButton extends ConsumerWidget {
  const ThemeToggleButton({super.key, this.color});

  final Color? color;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final mode = ref.watch(themeModeProvider);
    final (icon, label) = switch (mode) {
      ThemeMode.system => (Icons.brightness_auto, l.themeSystem),
      ThemeMode.light => (Icons.light_mode_outlined, l.themeLight),
      ThemeMode.dark => (Icons.dark_mode_outlined, l.themeDark),
    };
    return IconButton(
      tooltip: l.themeTooltip(label),
      icon: Icon(icon),
      color: color,
      onPressed: () => ref.read(themeModeProvider.notifier).cycle(),
    );
  }
}
