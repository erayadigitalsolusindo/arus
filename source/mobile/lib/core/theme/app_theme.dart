import 'package:flutter/material.dart';

/// Warna merek (sama di terang dan gelap): biru logo ARUS (sama dengan web) + oranye sebagai aksen.
class AppColors {
  const AppColors._();

  static const primary500 = Color(0xFF1A63C9);
  static const primary600 = Color(0xFF0445AB);
  static const primary700 = Color(0xFF094996);
  static const primary400 = Color(0xFF4A8BE0);
  static const accent = Color(0xFFE8730C);
  static const accentDark = Color(0xFFFFA24D);
}

/// Palet yang mengikuti mode terang/gelap. Akses lewat `context.pal`.
/// Token meniru template web (Dreams Core): latar abu muda, field "sunken", garis tipis.
@immutable
class AppPalette extends ThemeExtension<AppPalette> {
  const AppPalette({
    required this.base,
    required this.surface,
    required this.sunken,
    required this.border,
    required this.text,
    required this.textTertiary,
    required this.primary,
    required this.primarySoft,
    required this.accent,
    required this.accentSoft,
    required this.dangerSoft,
    required this.dangerText,
    required this.danger,
    required this.warningSoft,
    required this.warningText,
    required this.success,
  });

  final Color base;
  final Color surface;
  final Color sunken;
  final Color border;
  final Color text;
  final Color textTertiary;
  final Color primary;
  final Color primarySoft;

  /// Aksen oranye: harga, lencana, sorotan. Biru tetap warna aksi utama.
  final Color accent;
  final Color accentSoft;
  final Color dangerSoft;
  final Color dangerText;
  final Color danger;
  final Color warningSoft;
  final Color warningText;
  final Color success;

  static const light = AppPalette(
    base: Color(0xFFF4F7F9),
    surface: Colors.white,
    sunken: Color(0xFFF1F3F5),
    border: Color(0xFFE2E6EA),
    text: Color(0xFF1B2430),
    textTertiary: Color(0xFF6B7686),
    primary: AppColors.primary600,
    primarySoft: Color(0xFFDBE8FB),
    accent: AppColors.accent,
    accentSoft: Color(0xFFFFE8D1),
    dangerSoft: Color(0xFFF9D9D8),
    dangerText: Color(0xFF8E2A20),
    danger: Color(0xFFC0392B),
    warningSoft: Color(0xFFFFE9C7),
    warningText: Color(0xFF7A4B00),
    success: Color(0xFF1E8E5A),
  );

  static const dark = AppPalette(
    base: Color(0xFF0B1620),
    surface: Color(0xFF12222E),
    sunken: Color(0xFF0E1B25),
    border: Color(0xFF223747),
    text: Color(0xFFE6EEF3),
    textTertiary: Color(0xFF8CA0AF),
    primary: AppColors.primary400,
    primarySoft: Color(0xFF15233A),
    accent: AppColors.accentDark,
    accentSoft: Color(0xFF3F2A12),
    dangerSoft: Color(0xFF47201C),
    dangerText: Color(0xFFFFB4AB),
    danger: Color(0xFFE5645A),
    warningSoft: Color(0xFF4A3510),
    warningText: Color(0xFFFFD28A),
    success: Color(0xFF4CC38A),
  );

  @override
  AppPalette copyWith() => this;

  @override
  AppPalette lerp(ThemeExtension<AppPalette>? other, double t) {
    if (other is! AppPalette) return this;
    Color l(Color a, Color b) => Color.lerp(a, b, t)!;
    return AppPalette(
      base: l(base, other.base),
      surface: l(surface, other.surface),
      sunken: l(sunken, other.sunken),
      border: l(border, other.border),
      text: l(text, other.text),
      textTertiary: l(textTertiary, other.textTertiary),
      primary: l(primary, other.primary),
      primarySoft: l(primarySoft, other.primarySoft),
      accent: l(accent, other.accent),
      accentSoft: l(accentSoft, other.accentSoft),
      dangerSoft: l(dangerSoft, other.dangerSoft),
      dangerText: l(dangerText, other.dangerText),
      danger: l(danger, other.danger),
      warningSoft: l(warningSoft, other.warningSoft),
      warningText: l(warningText, other.warningText),
      success: l(success, other.success),
    );
  }
}

extension PaletteContext on BuildContext {
  AppPalette get pal => Theme.of(this).extension<AppPalette>()!;
}

ThemeData buildTheme(Brightness brightness) {
  final p = brightness == Brightness.dark ? AppPalette.dark : AppPalette.light;
  const radius = BorderRadius.all(Radius.circular(10));
  OutlineInputBorder border(Color c) => OutlineInputBorder(
    borderRadius: radius,
    borderSide: BorderSide(color: c),
  );

  final scheme = ColorScheme.fromSeed(
    seedColor: AppColors.primary600,
    brightness: brightness,
    primary: p.primary,
    surface: p.surface,
    error: p.danger,
  );
  final base = brightness == Brightness.dark
      ? ThemeData.dark()
      : ThemeData.light();
  return ThemeData(
    useMaterial3: true,
    brightness: brightness,
    colorScheme: scheme,
    scaffoldBackgroundColor: p.base,
    extensions: [p],
    textTheme: base.textTheme.apply(bodyColor: p.text, displayColor: p.text),
    dividerColor: p.border,
    appBarTheme: AppBarTheme(
      backgroundColor: p.surface,
      foregroundColor: p.text,
      elevation: 0,
      scrolledUnderElevation: 0,
    ),
    cardTheme: CardThemeData(
      color: p.surface,
      elevation: 0,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(14),
        side: BorderSide(color: p.border),
      ),
    ),
    dialogTheme: DialogThemeData(backgroundColor: p.surface),
    badgeTheme: BadgeThemeData(
      backgroundColor: p.accent,
      textColor: brightness == Brightness.dark ? Colors.black : Colors.white,
    ),
    bottomSheetTheme: BottomSheetThemeData(
      backgroundColor: p.surface,
      surfaceTintColor: Colors.transparent,
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: p.sunken,
      isDense: true,
      contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 14),
      hintStyle: TextStyle(color: p.textTertiary),
      border: border(p.border),
      enabledBorder: border(p.border),
      focusedBorder: border(p.primary),
      errorBorder: border(p.danger),
      focusedErrorBorder: border(p.danger),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: AppColors.primary600,
        foregroundColor: Colors.white,
        disabledBackgroundColor: p.border,
        minimumSize: const Size.fromHeight(48),
        shape: const RoundedRectangleBorder(borderRadius: radius),
        textStyle: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: p.text,
        side: BorderSide(color: p.border),
        minimumSize: const Size.fromHeight(44),
        shape: const RoundedRectangleBorder(borderRadius: radius),
      ),
    ),
  );
}
