import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/gen/app_localizations.dart';
import '../locale/locale_controller.dart';
import '../theme/app_theme.dart';
import '../theme/text_scale_controller.dart';
import '../theme/theme_controller.dart';
import '../update/update_check.dart';

/// Satu tombol pengaturan perangkat: bahasa, ukuran teks, tampilan (terang/gelap), dan cek pembaruan.
class SettingsButton extends ConsumerWidget {
  const SettingsButton({super.key, this.color});

  final Color? color;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    return IconButton(
      tooltip: l.settingsTooltip,
      icon: Badge(
        isLabelVisible: ref.watch(updateAvailableProvider),
        smallSize: 9,
        child: const Icon(Icons.settings_outlined),
      ),
      color: color,
      onPressed: () => showDialog<void>(
        context: context,
        builder: (_) => const _SettingsDialog(),
      ),
    );
  }
}

class _Section extends StatelessWidget {
  const _Section({required this.icon, required this.title, required this.child});

  final IconData icon;
  final String title;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final pal = context.pal;
    return Padding(
      padding: const EdgeInsets.only(bottom: 18),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(icon, size: 18, color: pal.primary),
              const SizedBox(width: 8),
              Text(
                title,
                style: const TextStyle(
                  fontWeight: FontWeight.w800,
                  fontSize: 14,
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),
          child,
        ],
      ),
    );
  }
}

class _SettingsDialog extends ConsumerWidget {
  const _SettingsDialog();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final lang = ref.watch(localeProvider)?.languageCode ?? '';
    final theme = ref.watch(themeModeProvider);
    final scale = ref.watch(textScaleProvider);
    final scaleCtrl = ref.read(textScaleProvider.notifier);
    final has = ref.watch(updateAvailableProvider);

    return AlertDialog(
      scrollable: true,
      title: Row(
        children: [
          const Icon(Icons.settings_outlined),
          const SizedBox(width: 10),
          Text(l.settingsTitle),
        ],
      ),
      content: SizedBox(
        width: 360,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _Section(
              icon: Icons.translate,
              title: l.settingsLanguage,
              child: Wrap(
                spacing: 8,
                runSpacing: 4,
                children: [
                  for (final (code, label) in [
                    ('', l.languageSystem),
                    ('id', 'Indonesia'),
                    ('en', 'English'),
                    ('ja', '日本語'),
                  ])
                    ChoiceChip(
                      label: Text(label),
                      selected: lang == code,
                      onSelected: (_) => ref
                          .read(localeProvider.notifier)
                          .set(code.isEmpty ? null : Locale(code)),
                    ),
                ],
              ),
            ),
            _Section(
              icon: Icons.format_size,
              title: '${l.settingsTextSize} · ${(scale * 100).round()}%',
              child: Row(
                children: [
                  const Icon(Icons.text_decrease, size: 18),
                  Expanded(
                    child: Slider(
                      value: scale,
                      min: kTextScaleMin,
                      max: kTextScaleMax,
                      divisions:
                          ((kTextScaleMax - kTextScaleMin) / kTextScaleStep)
                              .round(),
                      label: '${(scale * 100).round()}%',
                      onChanged: scaleCtrl.preview,
                      onChangeEnd: scaleCtrl.set,
                    ),
                  ),
                  const Icon(Icons.text_increase, size: 18),
                ],
              ),
            ),
            _Section(
              icon: Icons.contrast,
              title: l.settingsTheme,
              child: SizedBox(
                width: double.infinity,
                child: SegmentedButton<ThemeMode>(
                  showSelectedIcon: false,
                  segments: [
                    ButtonSegment(
                      value: ThemeMode.system,
                      icon: const Icon(Icons.brightness_auto, size: 18),
                      label: Text(l.themeSystem),
                    ),
                    ButtonSegment(
                      value: ThemeMode.light,
                      icon: const Icon(Icons.light_mode_outlined, size: 18),
                      label: Text(l.themeLight),
                    ),
                    ButtonSegment(
                      value: ThemeMode.dark,
                      icon: const Icon(Icons.dark_mode_outlined, size: 18),
                      label: Text(l.themeDark),
                    ),
                  ],
                  selected: {theme},
                  onSelectionChanged: (s) =>
                      ref.read(themeModeProvider.notifier).set(s.first),
                ),
              ),
            ),
            Material(
              color: pal.primarySoft,
              borderRadius: BorderRadius.circular(12),
              child: ListTile(
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(12),
                ),
                leading: Badge(
                  isLabelVisible: has,
                  smallSize: 9,
                  child: Icon(Icons.system_update_alt, color: pal.primary),
                ),
                title: Text(
                  l.updCheckTitle,
                  style: const TextStyle(fontWeight: FontWeight.w700),
                ),
                subtitle: Text(
                  has ? l.updAvailableBadge : l.settingsUpdateHint,
                  style: TextStyle(fontSize: 12, color: pal.textTertiary),
                ),
                onTap: () {
                  Navigator.pop(context);
                  checkForUpdates(context, ref);
                },
              ),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => scaleCtrl.set(1.0),
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
