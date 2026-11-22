import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

const double kTextScaleMin = 0.8;
const double kTextScaleMax = 1.3;
const double kTextScaleStep = 0.05;

/// Ukuran teks per perangkat (1.0 = normal), dikalikan dengan pengaturan font sistem HP.
/// Disimpan di SharedPreferences.
final textScaleProvider = NotifierProvider<TextScaleController, double>(
  TextScaleController.new,
);

class TextScaleController extends Notifier<double> {
  static const _key = 'arus.text_scale';

  @override
  double build() {
    Future.microtask(_load);
    return 1.0;
  }

  static double _clamp(double v) =>
      ((v.clamp(kTextScaleMin, kTextScaleMax) / kTextScaleStep).round() *
              kTextScaleStep)
          .clamp(kTextScaleMin, kTextScaleMax);

  Future<void> _load() async {
    try {
      final v = (await SharedPreferences.getInstance()).getDouble(_key);
      if (v != null) state = _clamp(v);
    } catch (_) {
      // Penyimpanan tidak tersedia: tetap ukuran normal.
    }
  }

  /// Dipakai saat slider digeser (tanpa menulis ke disk).
  void preview(double v) => state = _clamp(v);

  Future<void> set(double v) async {
    state = _clamp(v);
    try {
      await (await SharedPreferences.getInstance()).setDouble(_key, state);
    } catch (_) {}
  }
}
