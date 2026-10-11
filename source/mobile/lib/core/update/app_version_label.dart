import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/gen/app_localizations.dart';
import 'app_update.dart';

/// Teks kecil "Versi 1.0.0+1"; kosong bila versi tak diketahui.
class AppVersionLabel extends ConsumerWidget {
  const AppVersionLabel({super.key, this.style});

  final TextStyle? style;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final v = ref.watch(appVersionProvider).value;
    if (v == null) return const SizedBox.shrink();
    return Text(
      AppLocalizations.of(context).appVersion(v),
      style: style,
      textAlign: TextAlign.center,
    );
  }
}
