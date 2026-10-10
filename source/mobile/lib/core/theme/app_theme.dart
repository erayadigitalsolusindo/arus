import 'package:flutter/material.dart';

/// Token warna mengikuti template web (Dreams Core): primary hijau, latar abu muda, field berlatar sunken.
class AppColors {
  const AppColors._();

  static const primary50 = Color(0xFFEFFAF6);
  static const primary100 = Color(0xFFD7F2E7);
  static const primary500 = Color(0xFF24997C);
  static const primary600 = Color(0xFF187F65);
  static const primary700 = Color(0xFF146653);
  static const danger100 = Color(0xFFF9D9D8);
  static const danger600 = Color(0xFFC0392B);
  static const dangerText = Color(0xFF8E2A20);
  static const base = Color(0xFFF7F8FA);
  static const sunken = Color(0xFFF1F3F5);
  static const border = Color(0xFFE2E6EA);
  static const text = Color(0xFF1B2430);
  static const textTertiary = Color(0xFF7A8594);
}

ThemeData buildTheme() {
  const radius = BorderRadius.all(Radius.circular(10));
  OutlineInputBorder border(Color c) => OutlineInputBorder(borderRadius: radius, borderSide: BorderSide(color: c));

  final scheme = ColorScheme.fromSeed(
    seedColor: AppColors.primary600,
    primary: AppColors.primary600,
    surface: Colors.white,
    error: AppColors.danger600,
  );
  return ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    scaffoldBackgroundColor: AppColors.base,
    textTheme: ThemeData.light().textTheme.apply(bodyColor: AppColors.text, displayColor: AppColors.text),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: AppColors.sunken,
      isDense: true,
      contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 14),
      border: border(AppColors.border),
      enabledBorder: border(AppColors.border),
      focusedBorder: border(AppColors.primary600),
      errorBorder: border(AppColors.danger600),
      focusedErrorBorder: border(AppColors.danger600),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: AppColors.primary600,
        foregroundColor: Colors.white,
        minimumSize: const Size.fromHeight(48),
        shape: const RoundedRectangleBorder(borderRadius: radius),
        textStyle: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
      ),
    ),
  );
}
