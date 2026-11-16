import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../l10n/gen/app_localizations.dart';
import '../theme/app_theme.dart';

/// Mode kerja perangkat: penjualan (bawaan; stok boleh minus & sisa stok disembunyikan di kasir) atau persediaan.
enum AppMode { sales, inventory }

final appModeProvider = NotifierProvider<AppModeController, AppMode>(
  AppModeController.new,
);

class AppModeController extends Notifier<AppMode> {
  static const _key = 'arus.app_mode';

  @override
  AppMode build() {
    Future.microtask(_load);
    return AppMode.sales;
  }

  Future<void> _load() async {
    try {
      final v = (await SharedPreferences.getInstance()).getString(_key);
      state = AppMode.values.firstWhere(
        (m) => m.name == v,
        orElse: () => AppMode.sales,
      );
    } catch (_) {
      // Penyimpanan tidak tersedia: tetap mode penjualan.
    }
  }

  Future<void> set(AppMode m) async {
    state = m;
    try {
      await (await SharedPreferences.getInstance()).setString(_key, m.name);
    } catch (_) {}
  }
}

/// Tombol ikon yang membuka pilihan mode; ikon menunjukkan mode sekarang.
class ModeButton extends ConsumerWidget {
  const ModeButton({super.key, this.color});

  final Color? color;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final mode = ref.watch(appModeProvider);
    return IconButton(
      tooltip: l.modeTooltip,
      icon: Icon(
        mode == AppMode.sales
            ? Icons.point_of_sale
            : Icons.inventory_2_outlined,
      ),
      color: color,
      onPressed: () => showDialog<void>(
        context: context,
        builder: (_) => const _ModeDialog(),
      ),
    );
  }
}

class _ModeDialog extends ConsumerWidget {
  const _ModeDialog();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final mode = ref.watch(appModeProvider);
    Widget card(AppMode m, IconData icon, String name, String desc) {
      final pal = context.pal;
      final on = mode == m;
      return Padding(
        padding: const EdgeInsets.only(top: 10),
        child: Material(
          color: on ? pal.primarySoft : Colors.transparent,
          borderRadius: BorderRadius.circular(14),
          child: InkWell(
            borderRadius: BorderRadius.circular(14),
            onTap: () {
              ref.read(appModeProvider.notifier).set(m);
              Navigator.pop(context);
            },
            child: Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(14),
                border: Border.all(
                  color: on ? pal.primary : pal.border,
                  width: on ? 1.6 : 1,
                ),
              ),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Icon(icon, color: on ? pal.primary : pal.textTertiary),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          name,
                          style: const TextStyle(
                            fontWeight: FontWeight.w800,
                            fontSize: 15,
                          ),
                        ),
                        const SizedBox(height: 2),
                        Text(
                          desc,
                          style: TextStyle(
                            fontSize: 12.5,
                            color: pal.textTertiary,
                          ),
                        ),
                      ],
                    ),
                  ),
                  if (on) Icon(Icons.check_circle, color: pal.primary),
                ],
              ),
            ),
          ),
        ),
      );
    }

    return AlertDialog(
      title: Row(
        children: [
          const Icon(Icons.swap_horiz),
          const SizedBox(width: 10),
          Text(l.modeTitle),
        ],
      ),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          card(
            AppMode.sales,
            Icons.point_of_sale,
            l.modeSalesName,
            l.modeSalesDesc,
          ),
          card(
            AppMode.inventory,
            Icons.inventory_2_outlined,
            l.modeStockName,
            l.modeStockDesc,
          ),
        ],
      ),
    );
  }
}
