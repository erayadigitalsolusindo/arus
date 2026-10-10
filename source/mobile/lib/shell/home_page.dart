import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/session/session_controller.dart';
import '../core/theme/app_theme.dart';
import '../l10n/gen/app_localizations.dart';

/// Beranda sementara: membuktikan sesi login berjalan. Nanti menjadi menu modul (kasir, opname, ...)
/// yang disaring dari izin pengguna (`Profile.permissions`).
class HomePage extends ConsumerWidget {
  const HomePage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final session = ref.watch(sessionProvider);
    if (session is! SessionSignedIn) return const SizedBox.shrink();
    final p = session.profile;

    return Scaffold(
      appBar: AppBar(
        title: Text(l.appName),
        backgroundColor: Colors.white,
        actions: [
          IconButton(
            tooltip: l.logout,
            icon: const Icon(Icons.logout),
            onPressed: () => ref.read(sessionProvider.notifier).logout(),
          ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          Text(l.homeWelcome(p.user.name), style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w700)),
          const SizedBox(height: 16),
          _InfoRow(label: l.homeTenant, value: p.tenant.name),
          _InfoRow(label: l.homeOutlet, value: p.outlet.name),
          const SizedBox(height: 24),
          Text(l.homeComingSoon, style: const TextStyle(color: AppColors.textTertiary)),
        ],
      ),
    );
  }
}

class _InfoRow extends StatelessWidget {
  const _InfoRow({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.symmetric(vertical: 6),
        child: Row(
          children: [
            SizedBox(width: 110, child: Text(label, style: const TextStyle(color: AppColors.textTertiary))),
            Expanded(child: Text(value, style: const TextStyle(fontWeight: FontWeight.w600))),
          ],
        ),
      );
}
