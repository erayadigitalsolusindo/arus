import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/gen/app_localizations.dart';
import 'app_update.dart';

/// Membungkus seluruh aplikasi: bila server punya APK yang lebih baru, layar pembaruan menutup aplikasi
/// (tanpa tombol tutup) sampai versi baru terpasang. Dicek saat dibuka dan setiap kembali dari latar belakang.
class UpdateGate extends ConsumerStatefulWidget {
  const UpdateGate({super.key, required this.child});

  final Widget child;

  @override
  ConsumerState<UpdateGate> createState() => _UpdateGateState();
}

class _UpdateGateState extends ConsumerState<UpdateGate>
    with WidgetsBindingObserver {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    WidgetsBinding.instance.addPostFrameCallback(
      (_) => ref.read(appUpdateProvider.notifier).check(),
    );
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState s) {
    if (s == AppLifecycleState.resumed) {
      ref.read(appUpdateProvider.notifier).check();
    }
  }

  @override
  Widget build(BuildContext context) {
    final u = ref.watch(appUpdateProvider);
    return Stack(
      children: [
        widget.child,
        if (u.required) Positioned.fill(child: _UpdateScreen(state: u)),
      ],
    );
  }
}

class _UpdateScreen extends ConsumerWidget {
  const _UpdateScreen({required this.state});

  final UpdateState state;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final t = Theme.of(context);
    final busy =
        state.phase == UpdatePhase.downloading ||
        state.phase == UpdatePhase.installing;
    final msg = switch (state.phase) {
      UpdatePhase.downloading => l.updateDownloading,
      UpdatePhase.installing => l.updateInstalling,
      UpdatePhase.needsPermission => l.updatePermission,
      UpdatePhase.failed => l.updateFailed,
      _ => l.updateBody,
    };
    return PopScope(
      canPop: false,
      child: Material(
        color: t.colorScheme.surface,
        child: SafeArea(
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Icon(
                      Icons.system_update,
                      size: 56,
                      color: t.colorScheme.primary,
                    ),
                    const SizedBox(height: 16),
                    Text(
                      l.updateTitle,
                      style: t.textTheme.headlineSmall,
                      textAlign: TextAlign.center,
                    ),
                    const SizedBox(height: 8),
                    Text(
                      l.updateVersions(
                        state.current,
                        state.info?.version ?? '',
                      ),
                      textAlign: TextAlign.center,
                    ),
                    const SizedBox(height: 16),
                    Text(msg, textAlign: TextAlign.center),
                    const SizedBox(height: 20),
                    if (state.phase == UpdatePhase.downloading) ...[
                      LinearProgressIndicator(
                        value: state.progress > 0 ? state.progress : null,
                      ),
                      const SizedBox(height: 8),
                      Text(
                        '${(state.progress * 100).round()}%',
                        textAlign: TextAlign.center,
                      ),
                    ] else if (busy)
                      const LinearProgressIndicator()
                    else
                      FilledButton.icon(
                        onPressed: () =>
                            ref.read(appUpdateProvider.notifier).start(),
                        icon: const Icon(Icons.download),
                        label: Text(
                          state.phase == UpdatePhase.failed
                              ? l.updateRetry
                              : l.updateNow,
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
