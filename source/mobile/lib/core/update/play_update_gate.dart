import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/gen/app_localizations.dart';
import 'play_update.dart';

/// Flavor Play: cek pembaruan ke Google Play saat dibuka/dilanjutkan, lalu tampilkan kartu pembaruan di atas
/// aplikasi. Kartu bisa ditunda ("Nanti") sampai [maxPlayUpdateSkips] kali; sesudah itu atau pada rilis berprioritas
/// tinggi, kartu tak bisa ditutup. Tombol Cek pembaruan memicu cek manual dan menampilkan hasilnya di kartu yang sama.
/// Memakai Stack (bukan dialog) karena gate ada di atas Navigator.
class PlayUpdateGate extends ConsumerStatefulWidget {
  const PlayUpdateGate({super.key, required this.child});

  final Widget child;

  @override
  ConsumerState<PlayUpdateGate> createState() => _PlayUpdateGateState();
}

class _PlayUpdateGateState extends ConsumerState<PlayUpdateGate>
    with WidgetsBindingObserver {
  Timer? _autoHide;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    WidgetsBinding.instance.addPostFrameCallback(
      (_) => ref.read(playUpdateProvider.notifier).check(),
    );
  }

  @override
  void dispose() {
    _autoHide?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState s) {
    if (s == AppLifecycleState.resumed) {
      ref.read(playUpdateProvider.notifier).check();
    }
  }

  @override
  Widget build(BuildContext context) {
    final u = ref.watch(playUpdateProvider);
    // Hasil "sudah terbaru"/gagal hilang sendiri; pembaruan wajib tetap kartu penuh yang tak bisa ditutup.
    ref.listen(playUpdateProvider.select((s) => s.phase), (_, p) {
      _autoHide?.cancel();
      if (p == PlayUpdatePhase.upToDate || p == PlayUpdatePhase.failed) {
        _autoHide = Timer(
          const Duration(seconds: 4),
          () => ref.read(playUpdateProvider.notifier).dismissCard(),
        );
      }
    });
    final modal = u.showCard && u.forced;
    return Stack(
      children: [
        widget.child,
        if (modal) Positioned.fill(child: _UpdateCard(state: u)),
        Positioned(
          top: 0,
          left: 0,
          right: 0,
          child: SafeArea(
            bottom: false,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 0),
              child: AnimatedSwitcher(
                duration: const Duration(milliseconds: 350),
                switchInCurve: Curves.easeOutBack,
                switchOutCurve: Curves.easeIn,
                transitionBuilder: (c, a) => SlideTransition(
                  position: Tween(
                    begin: const Offset(0, -1.4),
                    end: Offset.zero,
                  ).animate(a),
                  child: FadeTransition(opacity: a, child: c),
                ),
                child: u.showCard && !modal
                    ? _UpdateToast(key: ValueKey(u.phase), state: u)
                    : const SizedBox.shrink(key: ValueKey('none')),
              ),
            ),
          ),
        ),
      ],
    );
  }
}

class _UpdateCard extends ConsumerWidget {
  const _UpdateCard({required this.state});

  final PlayUpdateState state;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final t = Theme.of(context);
    final ctrl = ref.read(playUpdateProvider.notifier);
    final forced = state.forced;
    final phase = state.phase;

    final (IconData icon, Color color) = switch (phase) {
      PlayUpdatePhase.upToDate => (Icons.check_circle, Colors.green),
      PlayUpdatePhase.failed => (Icons.cloud_off, t.colorScheme.error),
      PlayUpdatePhase.downloaded => (Icons.restart_alt, t.colorScheme.primary),
      _ => (Icons.system_update, t.colorScheme.primary),
    };
    final title = switch (phase) {
      PlayUpdatePhase.checking => l.updChecking,
      PlayUpdatePhase.upToDate => l.updUpToDate,
      PlayUpdatePhase.failed => l.updFailedTitle,
      PlayUpdatePhase.downloading => l.updateDownloading,
      PlayUpdatePhase.downloaded => l.updReadyTitle,
      _ => l.updateTitle,
    };
    final body = switch (phase) {
      PlayUpdatePhase.upToDate => l.updUpToDateBody,
      PlayUpdatePhase.failed => l.updPlayUnavailable,
      PlayUpdatePhase.downloaded => l.updReadyBody,
      PlayUpdatePhase.available => forced ? l.updForcedBody : l.updPlayBody,
      _ => null,
    };
    final busy =
        phase == PlayUpdatePhase.checking ||
        phase == PlayUpdatePhase.downloading;

    final List<Widget> actions = switch (phase) {
      PlayUpdatePhase.available => [
        if (!forced)
          Expanded(
            child: OutlinedButton(
              onPressed: ctrl.later,
              style: OutlinedButton.styleFrom(
                minimumSize: const Size.fromHeight(46),
              ),
              child: Text(l.updLater),
            ),
          ),
        if (!forced) const SizedBox(width: 10),
        Expanded(
          child: FilledButton.icon(
            onPressed: ctrl.start,
            style: FilledButton.styleFrom(
              minimumSize: const Size.fromHeight(46),
            ),
            icon: const Icon(Icons.download),
            label: Text(l.updateNow),
          ),
        ),
      ],
      PlayUpdatePhase.downloaded => [
        Expanded(
          child: FilledButton.icon(
            onPressed: ctrl.install,
            style: FilledButton.styleFrom(
              minimumSize: const Size.fromHeight(46),
            ),
            icon: const Icon(Icons.restart_alt),
            label: Text(l.updRestart),
          ),
        ),
      ],
      PlayUpdatePhase.upToDate || PlayUpdatePhase.failed => [
        Expanded(
          child: FilledButton(
            onPressed: ctrl.dismissCard,
            style: FilledButton.styleFrom(
              minimumSize: const Size.fromHeight(46),
            ),
            child: Text(MaterialLocalizations.of(context).okButtonLabel),
          ),
        ),
      ],
      _ => const [],
    };

    return PopScope(
      canPop: !forced,
      child: Material(
        color: Colors.black54,
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 400),
            child: Card(
              margin: const EdgeInsets.all(24),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(24),
              ),
              child: Padding(
                padding: const EdgeInsets.fromLTRB(20, 24, 20, 20),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Center(
                      child: Container(
                        width: 72,
                        height: 72,
                        decoration: BoxDecoration(
                          color: color.withValues(alpha: 0.12),
                          shape: BoxShape.circle,
                        ),
                        child: Icon(icon, size: 38, color: color),
                      ),
                    ),
                    const SizedBox(height: 16),
                    Text(
                      title,
                      textAlign: TextAlign.center,
                      style: t.textTheme.titleLarge?.copyWith(
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    if (body != null) ...[
                      const SizedBox(height: 8),
                      Text(
                        body,
                        textAlign: TextAlign.center,
                        style: t.textTheme.bodyMedium,
                      ),
                    ],
                    if (busy) ...[
                      const SizedBox(height: 18),
                      const LinearProgressIndicator(),
                    ],
                    if (actions.isNotEmpty) ...[
                      const SizedBox(height: 20),
                      Row(children: actions),
                    ],
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

/// Notifikasi pembaruan yang turun dari atas layar (tidak menutup kasir): memeriksa, tersedia, siap pasang, terbaru, gagal.
class _UpdateToast extends ConsumerWidget {
  const _UpdateToast({super.key, required this.state});

  final PlayUpdateState state;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final t = Theme.of(context);
    final ctrl = ref.read(playUpdateProvider.notifier);
    final phase = state.phase;

    final (IconData icon, Color color) = switch (phase) {
      PlayUpdatePhase.upToDate => (Icons.check_circle, Colors.green),
      PlayUpdatePhase.failed => (Icons.cloud_off, t.colorScheme.error),
      PlayUpdatePhase.downloaded => (Icons.restart_alt, t.colorScheme.primary),
      _ => (Icons.system_update, t.colorScheme.primary),
    };
    final title = switch (phase) {
      PlayUpdatePhase.checking => l.updChecking,
      PlayUpdatePhase.upToDate => l.updUpToDate,
      PlayUpdatePhase.failed => l.updFailedTitle,
      PlayUpdatePhase.downloaded => l.updReadyTitle,
      _ => l.updateTitle,
    };
    final body = switch (phase) {
      PlayUpdatePhase.upToDate => l.updUpToDateBody,
      PlayUpdatePhase.failed => l.updPlayUnavailable,
      PlayUpdatePhase.downloaded => l.updReadyBody,
      PlayUpdatePhase.available => l.updPlayBody,
      _ => null,
    };
    final close =
        phase == PlayUpdatePhase.upToDate || phase == PlayUpdatePhase.failed;

    Widget? action;
    if (phase == PlayUpdatePhase.available) {
      action = Row(
        mainAxisAlignment: MainAxisAlignment.end,
        children: [
          TextButton(onPressed: ctrl.later, child: Text(l.updLater)),
          const SizedBox(width: 6),
          FilledButton.icon(
            onPressed: ctrl.start,
            icon: const Icon(Icons.download, size: 18),
            label: Text(l.updateNow),
          ),
        ],
      );
    } else if (phase == PlayUpdatePhase.downloaded) {
      action = Align(
        alignment: Alignment.centerRight,
        child: FilledButton.icon(
          onPressed: ctrl.install,
          icon: const Icon(Icons.restart_alt, size: 18),
          label: Text(l.updRestart),
        ),
      );
    }

    return Material(
      color: t.colorScheme.surface,
      elevation: 8,
      shadowColor: Colors.black45,
      borderRadius: BorderRadius.circular(18),
      clipBehavior: Clip.antiAlias,
      child: Container(
        decoration: BoxDecoration(
          border: Border(left: BorderSide(color: color, width: 5)),
        ),
        padding: const EdgeInsets.fromLTRB(12, 12, 8, 10),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Container(
                  width: 40,
                  height: 40,
                  decoration: BoxDecoration(
                    color: color.withValues(alpha: 0.12),
                    shape: BoxShape.circle,
                  ),
                  child: phase == PlayUpdatePhase.checking
                      ? const Padding(
                          padding: EdgeInsets.all(10),
                          child: CircularProgressIndicator(strokeWidth: 2.5),
                        )
                      : Icon(icon, size: 22, color: color),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        title,
                        style: t.textTheme.titleSmall?.copyWith(
                          fontWeight: FontWeight.w800,
                        ),
                      ),
                      if (body != null)
                        Padding(
                          padding: const EdgeInsets.only(top: 2),
                          child: Text(body, style: t.textTheme.bodySmall),
                        ),
                    ],
                  ),
                ),
                if (close)
                  IconButton(
                    visualDensity: VisualDensity.compact,
                    icon: const Icon(Icons.close, size: 18),
                    onPressed: ctrl.dismissCard,
                  ),
              ],
            ),
            if (action != null) ...[const SizedBox(height: 8), action],
          ],
        ),
      ),
    );
  }
}
