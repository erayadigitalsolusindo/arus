import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../core/session/session_controller.dart';
import '../core/theme/app_theme.dart';
import '../core/theme/theme_controller.dart';
import '../l10n/gen/app_localizations.dart';

/// Beranda: menu modul yang disaring dari izin pengguna. Kini hanya Kasir; opname/mutasi menyusul sebagai modul baru.
class HomePage extends ConsumerWidget {
  const HomePage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final session = ref.watch(sessionProvider);
    if (session is! SessionSignedIn) return const SizedBox.shrink();
    final p = session.profile;

    return Scaffold(
      appBar: AppBar(
        title: Text(l.appName),
        actions: [
          const ThemeToggleButton(),
          IconButton(tooltip: l.logout, icon: const Icon(Icons.logout), onPressed: () => ref.read(sessionProvider.notifier).logout()),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          Text(l.homeWelcome(p.user.name), style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w700)),
          const SizedBox(height: 4),
          Text('${p.tenant.name} · ${p.outlet.name}', style: TextStyle(color: pal.textTertiary)),
          const SizedBox(height: 20),
          if (p.permissions.can('sales_orders', 'create'))
            _ModuleTile(icon: Icons.point_of_sale, title: l.posTitle, color: pal.primary, onTap: () => context.go('/kasir')),
        ],
      ),
    );
  }
}

class _ModuleTile extends StatelessWidget {
  const _ModuleTile({required this.icon, required this.title, required this.color, required this.onTap});

  final IconData icon;
  final String title;
  final Color color;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) => Card(
        child: InkWell(
          borderRadius: BorderRadius.circular(14),
          onTap: onTap,
          child: Padding(
            padding: const EdgeInsets.all(18),
            child: Row(
              children: [
                Icon(icon, size: 32, color: color),
                const SizedBox(width: 16),
                Expanded(child: Text(title, style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w700))),
                const Icon(Icons.chevron_right),
              ],
            ),
          ),
        ),
      );
}
