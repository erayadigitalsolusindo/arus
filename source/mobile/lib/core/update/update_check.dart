import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/gen/app_localizations.dart';
import 'app_update.dart';
import 'play_update.dart';

/// Ada pembaruan yang belum dipasang (untuk lencana tombol Cek pembaruan); flavor sideload sudah memaksa layarnya sendiri.
final updateAvailableProvider = Provider<bool>(
  (ref) => isPlayFlavor
      ? ref.watch(playUpdateProvider.select((s) => s.hasUpdate))
      : ref.watch(appUpdateProvider.select((s) => s.required)),
);

/// Cek pembaruan manual. Play: kartu pembaruan menampilkan hasilnya (tersedia/terbaru/gagal).
/// Sideload: layar pembaruan wajib muncul sendiri bila ada; bila tidak, kabari lewat snackbar.
Future<void> checkForUpdates(BuildContext context, WidgetRef ref) async {
  if (isPlayFlavor) {
    await ref.read(playUpdateProvider.notifier).check(manual: true);
    return;
  }
  final l = AppLocalizations.of(context);
  final messenger = ScaffoldMessenger.of(context);
  messenger
    ..clearSnackBars()
    ..showSnackBar(
      SnackBar(
        content: Text(l.updChecking),
        duration: const Duration(seconds: 2),
      ),
    );
  await ref.read(appUpdateProvider.notifier).check();
  if (ref.read(appUpdateProvider).required) {
    messenger.clearSnackBars();
    return;
  }
  messenger
    ..clearSnackBars()
    ..showSnackBar(SnackBar(content: Text(l.updUpToDate)));
}

