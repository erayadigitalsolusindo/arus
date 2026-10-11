import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../l10n/gen/app_localizations.dart';

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

/// Tombol ikon yang membuka dialog slider ukuran teks.
class TextScaleButton extends ConsumerWidget {
  const TextScaleButton({super.key, this.color});

  final Color? color;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    return IconButton(
      tooltip: l.textSizeTooltip,
      icon: const Icon(Icons.format_size),
      color: color,
      onPressed: () => showDialog<void>(
        context: context,
        builder: (_) => const _TextScaleDialog(),
      ),
    );
  }
}

class _TextScaleDialog extends ConsumerWidget {
  const _TextScaleDialog();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final scale = ref.watch(textScaleProvider);
    final ctrl = ref.read(textScaleProvider.notifier);
    final divisions = ((kTextScaleMax - kTextScaleMin) / kTextScaleStep)
        .round();
    return AlertDialog(
      title: Text(l.textSizeTitle),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(l.textSizeSample),
          const SizedBox(height: 8),
          Row(
            children: [
              IconButton(
                tooltip: l.textSizeDecrease,
                icon: const Icon(Icons.text_decrease),
                onPressed: scale <= kTextScaleMin
                    ? null
                    : () => ctrl.set(scale - kTextScaleStep),
              ),
              Expanded(
                child: Slider(
                  value: scale,
                  min: kTextScaleMin,
                  max: kTextScaleMax,
                  divisions: divisions,
                  label: '${(scale * 100).round()}%',
                  onChanged: ctrl.preview,
                  onChangeEnd: ctrl.set,
                ),
              ),
              IconButton(
                tooltip: l.textSizeIncrease,
                icon: const Icon(Icons.text_increase),
                onPressed: scale >= kTextScaleMax
                    ? null
                    : () => ctrl.set(scale + kTextScaleStep),
              ),
            ],
          ),
          Center(child: Text('${(scale * 100).round()}%')),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () => ctrl.set(1.0),
          child: Text(l.textSizeReset),
        ),
        FilledButton(
          onPressed: () => Navigator.pop(context),
          child: Text(l.textSizeDone),
        ),
      ],
    );
  }
}
