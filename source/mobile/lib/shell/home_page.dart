import '../core/settings/app_mode.dart';
import '../core/settings/settings_button.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_svg/flutter_svg.dart';
import 'package:go_router/go_router.dart';

import '../core/outlet/outlet_switcher.dart';
import '../core/session/session_controller.dart';
import '../core/theme/app_theme.dart';
import '../core/widgets/beach_background.dart';
import '../core/widgets/ocean_background.dart';
import '../features/pos/pos_controller.dart';
import '../features/pos/petty_cash_page.dart';
import '../features/pos/pos_repository.dart';
import '../features/pos/shift_dialog.dart';
import '../features/ai/halloarus_page.dart';
import '../features/manage/item_manage_page.dart';
import '../features/manage/purchase_pages.dart';
import '../features/reports/erapay_settlement_page.dart';
import '../features/reports/sales_report_page.dart';
import '../l10n/gen/app_localizations.dart';

/// EraPay terpasang di toko ini? (menu Settlement hanya tampil bila ya; galat dianggap tidak).
final eraConfiguredProvider = FutureProvider.autoDispose<bool>(
  (ref) async => (await ref.read(posRepositoryProvider).eraPayConfig()).enabled,
);

/// Beranda bertema pantai & pasar: ringkasan hari ini (huruf besar agar terbaca dari jauh) dan lapak modul
/// yang disaring dari izin pengguna. Kini hanya Kasir; modul lain menyusul sebagai lapak baru.
class HomePage extends ConsumerWidget {
  const HomePage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final session = ref.watch(sessionProvider);
    if (session is! SessionSignedIn) return const SizedBox.shrink();
    final p = session.profile;
    final name = p.user.name;
    final hour = DateTime.now().hour;
    final greeting = hour < 11
        ? l.homeGreetMorning(name)
        : hour < 15
        ? l.homeGreetNoon(name)
        : hour < 18
        ? l.homeGreetAfternoon(name)
        : l.homeGreetNight(name);

    return Scaffold(
      extendBodyBehindAppBar: true,
      appBar: AppBar(
        backgroundColor: Colors.transparent,
        centerTitle: true,
        title: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
          decoration: BoxDecoration(
            // Logo biru tua: alas terang agar tetap terbaca di langit malam.
            color: Colors.white.withValues(alpha: 0.92),
            borderRadius: BorderRadius.circular(20),
          ),
          child: SvgPicture.asset(
            'assets/images/logo_dengan_text-no-bg.svg',
            height: 30,
            semanticsLabel: l.appName,
          ),
        ),
        actions: [
          const ModeButton(),
          const SettingsButton(),
          IconButton(
            tooltip: l.logout,
            icon: const Icon(Icons.logout),
            onPressed: () => ref.read(sessionProvider.notifier).logout(),
          ),
        ],
      ),
      body: BeachBackground(
        animate: ref.watch(oceanAnimateProvider),
        child: SafeArea(
          child: RefreshIndicator(
            onRefresh: () async {
              ref.invalidate(shiftProvider);
              ref.invalidate(eraConfiguredProvider);
            },
            // Sapaan di atas; papan hari ini dan menu di dasar layar supaya langit, gunung, dan laut terlihat.
            child: LayoutBuilder(
              builder: (context, box) => SingleChildScrollView(
                physics: const AlwaysScrollableScrollPhysics(),
                padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
                child: ConstrainedBox(
                  constraints: BoxConstraints(minHeight: box.maxHeight - 32),
                  child: Column(
                    mainAxisAlignment: MainAxisAlignment.spaceBetween,
                    children: [
                      _Banner(
                        greeting: greeting,
                        store: p.tenant.name,
                        branch: p.outlet.name,
                        onSwitch: () => showOutletSwitcher(context),
                      ),
                      const SizedBox(height: 24),
                      Column(
                        children: [
                          if (p.permissions.can('ai_assistant', 'view')) ...[
                            _HalloArusButton(
                              subtitle: l.halloArusSubtitle,
                              onTap: () => showHalloArusPage(context),
                            ),
                          ],
                          const SizedBox(height: 12),
                          _MenuGrid(
                            entries: [
                              if (p.permissions.can('sales_orders', 'create'))
                                _MenuEntry(
                                  Icons.point_of_sale,
                                  l.posTitle,
                                  () => context.go('/kasir'),
                                ),
                              if (p.permissions.can('sales_orders', 'create'))
                                _MenuEntry(
                                  Icons.payments_outlined,
                                  l.pettyMenu,
                                  () async {
                                    final shift = await ref.read(
                                      shiftProvider.future,
                                    );
                                    if (!context.mounted) return;
                                    if (shift == null) {
                                      await showOpenShiftDialog(context);
                                      return;
                                    }
                                    await showPettyCashPage(context, shift.id);
                                  },
                                ),
                              if (p.permissions.can('items', 'view'))
                                _MenuEntry(
                                  Icons.inventory_2_outlined,
                                  l.menuItems,
                                  () => showItemManagePage(context),
                                ),
                              if (p.permissions.can(
                                    'purchase_invoices',
                                    'view',
                                  ) ||
                                  p.permissions.can('purchase_list', 'view'))
                                _MenuEntry(
                                  Icons.local_shipping_outlined,
                                  l.menuPurchases,
                                  () => showPurchasesPage(context),
                                ),
                              // Settlement hanya untuk admin dan hanya bila EraPay terpasang di toko ini.
                              if (p.permissions.can('sales_reports', 'view') &&
                                  (ref
                                          .watch(eraConfiguredProvider)
                                          .asData
                                          ?.value ??
                                      false))
                                _MenuEntry(
                                  Icons.account_balance_wallet_outlined,
                                  l.menuSettlement,
                                  () => showEraSettlementPage(context),
                                ),
                              // Laporan hanya untuk akun berizin laporan penjualan (admin/pemilik).
                              if (p.permissions.can('sales_reports', 'view'))
                                _MenuEntry(
                                  Icons.bar_chart_rounded,
                                  l.reportTitle,
                                  () => showSalesReportPage(context),
                                ),
                            ],
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Kartu sapaan semi-transparan di atas latar pantai: sapaan + nama, lalu baris toko dan cabang (ketuk untuk ganti cabang).
class _Banner extends StatelessWidget {
  const _Banner({
    required this.greeting,
    required this.store,
    required this.branch,
    required this.onSwitch,
  });

  final String greeting;
  final String store;
  final String branch;
  final VoidCallback onSwitch;

  @override
  Widget build(BuildContext context) {
    final pal = context.pal;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: pal.surface.withValues(alpha: 0.88),
        borderRadius: BorderRadius.circular(20),
        border: Border.all(color: pal.border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            greeting,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w800),
          ),
          const SizedBox(height: 12),
          Material(
            color: pal.primarySoft,
            borderRadius: BorderRadius.circular(14),
            child: InkWell(
              borderRadius: BorderRadius.circular(14),
              onTap: onSwitch,
              child: Padding(
                padding: const EdgeInsets.fromLTRB(12, 10, 10, 10),
                child: Row(
                  children: [
                    Icon(Icons.storefront_outlined, color: pal.primary),
                    const SizedBox(width: 10),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            store,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: TextStyle(
                              fontSize: 12,
                              color: pal.textTertiary,
                            ),
                          ),
                          Text(
                            branch,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: const TextStyle(
                              fontSize: 17,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                        ],
                      ),
                    ),
                    Icon(Icons.swap_horiz, color: pal.primary),
                  ],
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Tombol HalloArus (asisten AI): kartu gelap bergradien cyan→ungu, membuka halaman chat.
class _HalloArusButton extends StatelessWidget {
  const _HalloArusButton({required this.subtitle, required this.onTap});

  final String subtitle;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.transparent,
      child: InkWell(
        borderRadius: BorderRadius.circular(16),
        onTap: onTap,
        child: Ink(
          padding: const EdgeInsets.all(16),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(16),
            gradient: const LinearGradient(
              colors: [Color(0xFF0B1224), Color(0xFF1E1B4B)],
            ),
            border: Border.all(
              color: const Color(0xFF22D3EE).withValues(alpha: 0.6),
            ),
            boxShadow: [
              BoxShadow(
                color: const Color(0xFF22D3EE).withValues(alpha: 0.25),
                blurRadius: 16,
              ),
            ],
          ),
          child: Row(
            children: [
              Container(
                width: 44,
                height: 44,
                decoration: const BoxDecoration(
                  shape: BoxShape.circle,
                  gradient: LinearGradient(
                    colors: [Color(0xFF22D3EE), Color(0xFFA78BFA)],
                  ),
                ),
                child: const Icon(Icons.auto_awesome, color: Colors.white),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text(
                      'HalloArus',
                      style: TextStyle(
                        fontFamily: 'monospace',
                        fontSize: 22,
                        fontWeight: FontWeight.w800,
                        color: Colors.white,
                      ),
                    ),
                    Text(
                      subtitle,
                      style: const TextStyle(
                        fontSize: 13,
                        color: Color(0xFF94A3B8),
                      ),
                    ),
                  ],
                ),
              ),
              const Icon(Icons.chevron_right, color: Color(0xFF22D3EE)),
            ],
          ),
        ),
      ),
    );
  }
}

class _MenuEntry {
  const _MenuEntry(this.icon, this.label, this.onTap);

  final IconData icon;
  final String label;
  final VoidCallback onTap;
}

/// Menu modul yang ringkas: ubin kecil (ikon + label) tiga per baris, hanya modul yang diizinkan untuk akun ini.
class _MenuGrid extends StatelessWidget {
  const _MenuGrid({required this.entries});

  final List<_MenuEntry> entries;

  @override
  Widget build(BuildContext context) {
    if (entries.isEmpty) return const SizedBox.shrink();
    final pal = context.pal;
    const gap = 8.0;
    return LayoutBuilder(
      builder: (context, box) {
        final w = (box.maxWidth - gap * 2) / 3;
        return Wrap(
          spacing: gap,
          runSpacing: gap,
          children: [
            for (final e in entries)
              SizedBox(
                width: w,
                child: Material(
                  color: pal.surface.withValues(alpha: 0.95),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(12),
                    side: BorderSide(color: pal.border),
                  ),
                  child: InkWell(
                    customBorder: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(12),
                    ),
                    onTap: e.onTap,
                    child: Padding(
                      padding: const EdgeInsets.symmetric(
                        vertical: 14,
                        horizontal: 6,
                      ),
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Container(
                            width: 46,
                            height: 46,
                            decoration: BoxDecoration(
                              color: pal.primarySoft,
                              borderRadius: BorderRadius.circular(10),
                            ),
                            child: Icon(e.icon, size: 26, color: pal.primary),
                          ),
                          const SizedBox(height: 8),
                          Text(
                            e.label,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: const TextStyle(
                              fontSize: 15,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
          ],
        );
      },
    );
  }
}
