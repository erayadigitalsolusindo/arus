import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_svg/flutter_svg.dart';
import 'package:go_router/go_router.dart';

import '../core/locale/locale_controller.dart';
import '../core/money/money.dart';
import '../core/outlet/outlet_switcher.dart';
import '../core/session/session_controller.dart';
import '../core/theme/app_theme.dart';
import '../core/theme/theme_controller.dart';
import '../core/widgets/beach_background.dart';
import '../core/widgets/ocean_background.dart';
import '../features/pos/pos_controller.dart';
import '../features/pos/pos_repository.dart';
import '../features/pos/sales_models.dart';
import '../l10n/gen/app_localizations.dart';

/// Penjualan kasir ini hari ini untuk ringkasan beranda. Dimuat ulang tiap beranda dibuka.
final homeTodayProvider = FutureProvider.autoDispose<SalesToday>(
  (ref) => ref.read(posRepositoryProvider).salesToday(),
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
          const LanguageButton(),
          const ThemeToggleButton(),
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
              ref.invalidate(homeTodayProvider);
              try {
                await ref.read(homeTodayProvider.future);
              } catch (_) {
                // Galat ditampilkan di papan ("–"); penarikan tetap selesai.
              }
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
                        place: '${p.tenant.name} · ${p.outlet.name}',
                        onSwitch: () => showOutletSwitcher(context),
                        switchLabel: l.outletSwitch,
                      ),
                      const SizedBox(height: 24),
                      Column(
                        children: [
                          const _TodayBoard(),
                          if (p.permissions.can('sales_orders', 'create')) ...[
                            const SizedBox(height: 12),
                            _Stall(
                              icon: Icons.point_of_sale,
                              title: l.posTitle,
                              hint: l.homeStallHint,
                              onTap: () => context.go('/kasir'),
                            ),
                          ],
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

/// Kartu sapaan semi-transparan di atas latar pantai.
class _Banner extends StatelessWidget {
  const _Banner({
    required this.greeting,
    required this.place,
    required this.onSwitch,
    required this.switchLabel,
  });

  final String greeting;
  final String place;
  final VoidCallback onSwitch;
  final String switchLabel;

  @override
  Widget build(BuildContext context) {
    final pal = context.pal;
    return Container(
      padding: const EdgeInsets.fromLTRB(16, 14, 8, 8),
      decoration: BoxDecoration(
        color: pal.surface.withValues(alpha: 0.88),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: pal.border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            greeting,
            style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w800),
          ),
          const SizedBox(height: 2),
          Row(
            children: [
              Expanded(
                child: Text(
                  place,
                  style: TextStyle(fontSize: 16, color: pal.textTertiary),
                ),
              ),
              TextButton.icon(
                onPressed: onSwitch,
                icon: const Icon(Icons.swap_horiz, size: 20),
                label: Text(switchLabel),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

/// Papan ringkasan hari ini, gaya informasi keranjang POS tetapi dengan huruf besar:
/// total penjualan, jumlah nota, status shift, dan total per metode bayar.
class _TodayBoard extends ConsumerStatefulWidget {
  const _TodayBoard();

  @override
  ConsumerState<_TodayBoard> createState() => _TodayBoardState();
}

class _TodayBoardState extends ConsumerState<_TodayBoard> {
  bool _open = false; // default tertutup

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final today = ref.watch(homeTodayProvider);
    final shift = ref.watch(shiftProvider);
    final data = today.asData?.value;
    final active = data?.rows.where((r) => !r.isVoid).length;

    final shiftText = shift.when(
      data: (s) => s == null
          ? l.posShiftNone
          : l.posShiftOpenedAt(
              MaterialLocalizations.of(context).formatTimeOfDay(
                TimeOfDay.fromDateTime(s.openedAt),
                alwaysUse24HourFormat: true,
              ),
            ),
      loading: () => '…',
      error: (_, _) => '–',
    );
    final shiftOpen = shift.asData?.value != null;

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: pal.surface.withValues(alpha: 0.92),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: pal.border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          InkWell(
            borderRadius: BorderRadius.circular(8),
            onTap: () => setState(() => _open = !_open),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    l.homeTodayTitle,
                    style: const TextStyle(
                      fontSize: 20,
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ),
                Icon(
                  _open ? Icons.expand_less : Icons.expand_more,
                  size: 32,
                  color: pal.textTertiary,
                ),
              ],
            ),
          ),
          if (_open) ...[
            const SizedBox(height: 4),
            FittedBox(
              fit: BoxFit.scaleDown,
              alignment: Alignment.centerLeft,
              child: Text(
                data == null
                    ? (today.hasError ? '–' : '…')
                    : formatMoney(data.total),
                style: TextStyle(
                  fontSize: 40,
                  fontWeight: FontWeight.w800,
                  color: pal.accent,
                ),
              ),
            ),
            const SizedBox(height: 10),
            Row(
              children: [
                Expanded(
                  child: _Stat(
                    label: l.homeTodayNotes,
                    value: active == null ? '–' : '$active',
                    color: pal.primary,
                  ),
                ),
                const SizedBox(width: 10),
                Expanded(
                  flex: 2,
                  child: _Stat(
                    label: l.homeShiftLabel,
                    value: shiftText,
                    color: shiftOpen ? pal.success : pal.warningText,
                    small: true,
                  ),
                ),
              ],
            ),
            if (data != null && data.byMethod.isNotEmpty) ...[
              const SizedBox(height: 12),
              Divider(height: 1, color: pal.border),
              const SizedBox(height: 8),
              for (final m in data.byMethod)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 3),
                  child: Row(
                    mainAxisAlignment: MainAxisAlignment.spaceBetween,
                    children: [
                      Flexible(
                        child: Text(
                          m.name,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(
                            fontSize: 18,
                            color: pal.textTertiary,
                          ),
                        ),
                      ),
                      const SizedBox(width: 12),
                      Text(
                        formatMoney(m.amount),
                        style: const TextStyle(
                          fontSize: 20,
                          fontWeight: FontWeight.w800,
                        ),
                      ),
                    ],
                  ),
                ),
            ],
          ],
        ],
      ),
    );
  }
}

class _Stat extends StatelessWidget {
  const _Stat({
    required this.label,
    required this.value,
    required this.color,
    this.small = false,
  });

  final String label;
  final String value;
  final Color color;
  final bool small;

  @override
  Widget build(BuildContext context) {
    final pal = context.pal;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: pal.sunken,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: TextStyle(fontSize: 14, color: pal.textTertiary)),
          const SizedBox(height: 2),
          Text(
            value,
            maxLines: small ? 2 : 1,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(
              fontSize: small ? 20 : 30,
              fontWeight: FontWeight.w800,
              color: color,
            ),
          ),
        ],
      ),
    );
  }
}

/// Kartu modul (lapak) di beranda.
class _Stall extends StatelessWidget {
  const _Stall({
    required this.icon,
    required this.title,
    required this.hint,
    required this.onTap,
  });

  final IconData icon;
  final String title;
  final String hint;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final pal = context.pal;
    return Column(
      children: [
        Material(
          color: pal.surface.withValues(alpha: 0.95),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(16),
            side: BorderSide(color: pal.border),
          ),
          child: InkWell(
            customBorder: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(16),
            ),
            onTap: onTap,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(18, 18, 14, 20),
              child: Row(
                children: [
                  Container(
                    width: 64,
                    height: 64,
                    decoration: BoxDecoration(
                      color: pal.primarySoft,
                      shape: BoxShape.circle,
                    ),
                    child: Icon(icon, size: 36, color: pal.primary),
                  ),
                  const SizedBox(width: 16),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          title,
                          style: const TextStyle(
                            fontSize: 26,
                            fontWeight: FontWeight.w800,
                          ),
                        ),
                        Text(
                          hint,
                          style: TextStyle(
                            fontSize: 16,
                            color: pal.textTertiary,
                          ),
                        ),
                      ],
                    ),
                  ),
                  const Icon(Icons.chevron_right, size: 32),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }
}
