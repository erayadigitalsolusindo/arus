import 'dart:async';

import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:intl/intl.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/session/session_controller.dart';
import '../../core/theme/app_theme.dart';
import '../../core/theme/theme_controller.dart';
import '../../l10n/gen/app_localizations.dart';
import '../../core/locale/locale_controller.dart';
import '../../core/outlet/outlet_switcher.dart';
import 'empty_cart.dart';
import 'costs_sheet.dart';
import 'item_thumb.dart';
import 'line_adjust_sheet.dart';
import 'member_picker.dart';
import 'pay_sheet.dart';
import 'pending_controller.dart';
import 'pending_sheet.dart';
import 'pos_controller.dart';
import 'pos_models.dart';
import 'pos_repository.dart';
import 'sales_today_page.dart';
import 'salesperson_picker.dart';
import 'shortcuts_strip.dart';
import 'scan_page.dart';
import 'shift_close_page.dart';
import 'shift_dialog.dart';

const _wideBreakpoint = 840.0;

/// Layar kasir. Meniru kasir web: pencarian + kisi barang di tengah, keranjang di kanan, panel info outlet
/// ("MAIN") di kiri yang TERTUTUP secara bawaan (laci di HP, panel lipat di layar lebar).
class PosPage extends ConsumerStatefulWidget {
  const PosPage({super.key});

  @override
  ConsumerState<PosPage> createState() => _PosPageState();
}

class _PosPageState extends ConsumerState<PosPage> {
  final _scaffold = GlobalKey<ScaffoldState>();
  bool _panelOpen =
      false; // panel kiri di layar lebar; tertutup sendiri saat halaman dibuka
  bool _shiftPrompted = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref.read(cartProvider.notifier).restore();
      ref.read(pendingProvider.notifier).load();
      _ensureShift();
    });
  }

  Future<void> _ensureShift() async {
    if (_shiftPrompted || !mounted) return;
    _shiftPrompted = true;
    try {
      final shift = await ref.read(shiftProvider.future);
      if (shift == null && mounted) await showOpenShiftDialog(context);
    } on ApiError {
      // Galat dimuat ulang lewat panel info; bayar tetap menegakkan SHIFT_REQUIRED di server.
    }
  }

  /// Tutup shift berjalan. Setelah ditutup, kasir bisa langsung membuka shift baru; keranjang dikosongkan.
  Future<void> _closeShift() async {
    final shift = ref.read(shiftProvider).asData?.value;
    if (shift == null) {
      await showOpenShiftDialog(context);
      return;
    }
    final openNew = await showCloseShiftPage(context, shift.id);
    if (openNew == null || !mounted) return;
    ref.read(cartProvider.notifier).clear();
    if (openNew) await showOpenShiftDialog(context);
  }

  // Memakai context halaman (bukan laci yang sudah ditutup).
  void _switchOutlet() => showOutletSwitcher(context, pos: true);

  void _togglePanel(bool wide) {
    if (wide) {
      setState(() => _panelOpen = !_panelOpen);
    } else {
      _scaffold.currentState?.openDrawer();
    }
  }

  Future<void> _pay() async {
    final result = await showPaySheet(context);
    if (result == null || !mounted) return;
    await _showSuccess(result);
  }

  Future<void> _showSuccess(SaleResult r) {
    final l = AppLocalizations.of(context);
    return showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (ctx) => AlertDialog(
        icon: Icon(Icons.check_circle, size: 48, color: context.pal.success),
        title: Text(l.paySuccessTitle),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(l.paySuccessDoc(r.docNo)),
            const SizedBox(height: 8),
            Text(
              formatMoney(r.total),
              style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w800),
            ),
            if (r.receivable > Decimal.zero) ...[
              const SizedBox(height: 6),
              Text(
                l.paySuccessReceivable(formatMoney(r.receivable)),
                style: TextStyle(
                  color: context.pal.danger,
                  fontWeight: FontWeight.w800,
                ),
              ),
              if (DateTime.tryParse(r.dueDate ?? '') != null)
                Text(
                  l.paySuccessDue(
                    DateFormat('d MMM yyyy').format(DateTime.parse(r.dueDate!)),
                  ),
                  style: TextStyle(color: context.pal.textTertiary),
                ),
            ],
            if (r.surcharge > Decimal.zero) ...[
              const SizedBox(height: 6),
              Text(
                l.paySurchargeDone(
                  formatMoney(r.surcharge),
                  formatMoney(r.total + r.surcharge),
                ),
                style: TextStyle(color: context.pal.warningText),
              ),
            ],
            if (r.change > Decimal.zero) ...[
              const SizedBox(height: 6),
              Text(
                l.payChangeDue(formatMoney(r.change)),
                style: TextStyle(
                  color: context.pal.success,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ],
          ],
        ),
        actions: [
          FilledButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: Text(l.payNewSale),
          ),
        ],
      ),
    );
  }

  void _openCartSheet() {
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (ctx) => DraggableScrollableSheet(
        expand: false,
        initialChildSize: 0.85,
        minChildSize: 0.4,
        maxChildSize: 0.95,
        builder: (_, scroll) => CartPane(
          scrollController: scroll,
          onPay: () {
            Navigator.of(ctx).pop();
            _pay();
          },
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final wide = MediaQuery.sizeOf(context).width >= _wideBreakpoint;
    final session = ref.watch(sessionProvider);
    final tenantName = session is SessionSignedIn
        ? session.profile.tenant.name
        : '';
    final outletId = session is SessionSignedIn
        ? session.profile.outlet.id
        : '';

    // Cabang berganti: keranjang cabang tujuan dipulihkan (yang lama tersimpan di kuncinya sendiri), shift dimuat ulang, katalog dimuat ulang (ProductPane ber-key).
    ref.listen(sessionProvider, (prev, next) {
      if (prev is SessionSignedIn &&
          next is SessionSignedIn &&
          prev.profile.outlet.id != next.profile.outlet.id) {
        ref
            .read(cartProvider.notifier)
            .restore(); // keranjang milik cabang tujuan (kosong bila tak ada)
        ref
            .read(pendingProvider.notifier)
            .load(); // nota pending milik cabang tujuan
        ref.invalidate(
          shortcutsProvider,
        ); // harga pintasan = harga efektif cabang aktif
        ref.invalidate(shiftProvider);
        _shiftPrompted = false;
        _ensureShift();
      }
    });

    return Scaffold(
      key: _scaffold,
      drawer: wide
          ? null
          : Drawer(
              width: 300,
              child: SafeArea(
                child: OutletPanel(
                  onClose: () => Navigator.of(context).pop(),
                  onSalesToday: () => showSalesTodayPage(context),
                  onCloseShift: _closeShift,
                  onSwitchOutlet: _switchOutlet,
                ),
              ),
            ),
      appBar: AppBar(
        leading: IconButton(
          tooltip: l.posPanelToggle,
          icon: Icon(wide && _panelOpen ? Icons.menu_open : Icons.menu),
          onPressed: () => _togglePanel(wide),
        ),
        titleSpacing: 0,
        // Nama usaha (PT/toko) menggantikan tulisan "Kasir" + kode cabang.
        title: Text(
          tenantName.isEmpty ? l.posTitle : tenantName,
          overflow: TextOverflow.ellipsis,
          style: const TextStyle(fontWeight: FontWeight.w700),
        ),
        actions: [const LanguageButton(), const ThemeToggleButton()],
      ),
      body: wide
          ? Row(
              children: [
                // Panel info outlet: lebar 0 saat tertutup (bawaan).
                AnimatedContainer(
                  duration: const Duration(milliseconds: 220),
                  curve: Curves.easeOutCubic,
                  width: _panelOpen ? 260 : 0,
                  decoration: BoxDecoration(
                    border: Border(
                      right: BorderSide(
                        color: _panelOpen ? pal.border : Colors.transparent,
                      ),
                    ),
                  ),
                  clipBehavior: Clip.hardEdge,
                  child: OverflowBox(
                    alignment: Alignment.centerLeft,
                    minWidth: 260,
                    maxWidth: 260,
                    child: OutletPanel(
                      onClose: () => setState(() => _panelOpen = false),
                      onSalesToday: () => showSalesTodayPage(context),
                      onCloseShift: _closeShift,
                      onSwitchOutlet: _switchOutlet,
                    ),
                  ),
                ),
                Expanded(child: ProductPane(key: ValueKey(outletId))),
                Container(
                  width: 380,
                  decoration: BoxDecoration(
                    border: Border(left: BorderSide(color: pal.border)),
                  ),
                  child: CartPane(onPay: _pay),
                ),
              ],
            )
          : Column(
              children: [
                Expanded(child: ProductPane(key: ValueKey(outletId))),
                _CartBar(onOpen: _openCartSheet, onPay: _pay),
              ],
            ),
    );
  }
}

// ------------------------------------------------------------------ panel info outlet ("MAIN")

class OutletPanel extends ConsumerWidget {
  const OutletPanel({
    super.key,
    required this.onClose,
    required this.onSalesToday,
    required this.onCloseShift,
    required this.onSwitchOutlet,
  });

  final VoidCallback onClose;
  final VoidCallback onSalesToday;
  final VoidCallback onCloseShift;
  final VoidCallback onSwitchOutlet;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final session = ref.watch(sessionProvider);
    final shift = ref.watch(shiftProvider);
    if (session is! SessionSignedIn) return const SizedBox.shrink();
    final p = session.profile;

    Widget kv(String k, String v) => Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            k.toUpperCase(),
            style: TextStyle(
              fontSize: 10.5,
              letterSpacing: 0.5,
              fontWeight: FontWeight.w600,
              color: pal.textTertiary,
            ),
          ),
          const SizedBox(height: 2),
          Text(v, style: const TextStyle(fontWeight: FontWeight.w600)),
        ],
      ),
    );

    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Container(
          padding: const EdgeInsets.all(14),
          decoration: BoxDecoration(
            color: pal.primarySoft,
            borderRadius: BorderRadius.circular(14),
          ),
          child: Row(
            children: [
              Icon(Icons.storefront, color: pal.primary),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      p.outlet.code,
                      style: TextStyle(
                        fontSize: 18,
                        fontWeight: FontWeight.w800,
                        color: pal.primary,
                      ),
                    ),
                    Text(
                      p.outlet.name,
                      style: TextStyle(color: pal.textTertiary, fontSize: 12.5),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 8),
        kv(l.posPanelBusiness, p.tenant.name),
        kv(l.posPanelCashier, p.user.name),
        shift.when(
          data: (s) => kv(
            l.posPanelShift,
            s == null
                ? l.posShiftNone
                : '${s.docNo}\n${l.posShiftOpenedAt(DateFormat('d MMM HH:mm').format(s.openedAt))}',
          ),
          loading: () => kv(l.posPanelShift, '...'),
          error: (e, _) => kv(
            l.posPanelShift,
            e is ApiError ? e.message(l) : l.errorUnknown,
          ),
        ),
        const SizedBox(height: 12),
        OutlinedButton.icon(
          onPressed: () {
            onClose();
            onSalesToday();
          },
          icon: const Icon(Icons.receipt_long, size: 18),
          label: Text(l.posSalesTodayTooltip),
        ),
        const SizedBox(height: 8),
        OutlinedButton.icon(
          onPressed: () {
            onClose();
            onCloseShift();
          },
          icon: const Icon(Icons.lock_clock, size: 18),
          label: Text(l.posShiftCloseTooltip),
        ),
        const SizedBox(height: 8),
        OutlinedButton.icon(
          onPressed: () {
            onClose();
            onSwitchOutlet();
          },
          icon: const Icon(Icons.swap_horiz, size: 18),
          label: Text(l.outletSwitch),
        ),
        const SizedBox(height: 8),
        OutlinedButton.icon(
          onPressed: () {
            onClose();
            context.go('/');
          },
          icon: const Icon(Icons.arrow_back, size: 18),
          label: Text(l.posBackHome),
        ),
      ],
    );
  }
}

// ------------------------------------------------------------------ pencarian + kisi barang

class ProductPane extends ConsumerStatefulWidget {
  const ProductPane({super.key});

  @override
  ConsumerState<ProductPane> createState() => _ProductPaneState();
}

class _ProductPaneState extends ConsumerState<ProductPane> {
  final _search = TextEditingController();
  final _focus = FocusNode();
  Timer? _debounce;
  int _seq = 0;

  List<PosItem> _items = const [];
  bool _loading = true;
  ApiError? _error;

  @override
  void initState() {
    super.initState();
    _load('');
    _loadCategories();
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _search.dispose();
    _focus.dispose();
    super.dispose();
  }

  String? _categoryId;
  List<Salesperson> _categories = const [];

  Future<void> _loadCategories() async {
    try {
      final c = await ref.read(posRepositoryProvider).categories();
      if (mounted) setState(() => _categories = c);
    } on ApiError {
      // Tanpa daftar kategori, katalog tetap bisa dicari; filter saja yang tidak tampil.
    }
  }

  Future<void> _load(String q) async {
    final my = ++_seq;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final r = await ref
          .read(posRepositoryProvider)
          .search(q, categoryId: _categoryId);
      if (my != _seq || !mounted) return;
      setState(() => _items = r);
    } on ApiError catch (e) {
      if (my == _seq && mounted) setState(() => _error = e);
    } finally {
      if (my == _seq && mounted) setState(() => _loading = false);
    }
  }

  void _onChanged(String q) {
    setState(() {}); // tombol hapus pada kotak cari
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 300), () => _load(q.trim()));
  }

  /// Enter (juga dari pemindai barcode HID): kode/barcode persis → langsung masuk keranjang.
  Future<void> _onSubmit(String raw) async {
    final q = raw.trim();
    if (q.isEmpty) return _focus.requestFocus();
    _debounce?.cancel();
    try {
      final exact = await ref.read(posRepositoryProvider).exact(q);
      if (!mounted) return;
      if (exact.length == 1) {
        ref.read(cartProvider.notifier).add(exact.first);
        _search.clear();
        _load('');
      } else {
        await _load(q);
      }
    } on ApiError catch (e) {
      if (mounted) setState(() => _error = e);
    }
    _focus.requestFocus();
  }

  /// Kamera: tiap kode yang terbaca dicari persis di server; satu barang cocok → masuk keranjang.
  Future<void> _scan() async {
    final l = AppLocalizations.of(context);
    await showScanPage(
      context,
      onCode: (code) async {
        try {
          final exact = await ref.read(posRepositoryProvider).exact(code);
          if (exact.length == 1) {
            ref.read(cartProvider.notifier).add(exact.first);
            return ScanOutcome(ok: true, text: l.scanAdded(exact.first.name));
          }
          return ScanOutcome(
            ok: false,
            text: exact.isEmpty ? l.scanNotFound(code) : l.scanAmbiguous(code),
          );
        } on ApiError catch (e) {
          return ScanOutcome(ok: false, text: e.message(l));
        }
      },
    );
    if (mounted) _focus.requestFocus();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(12, 12, 12, 8),
          child: TextField(
            controller: _search,
            focusNode: _focus,
            textInputAction: TextInputAction.search,
            onChanged: _onChanged,
            onSubmitted: _onSubmit,
            decoration: InputDecoration(
              hintText: l.posSearchHint,
              prefixIcon: Icon(Icons.search, color: pal.textTertiary),
              suffixIcon: _search.text.isEmpty
                  ? IconButton(
                      tooltip: l.posScanTooltip,
                      icon: const Icon(Icons.qr_code_scanner),
                      onPressed: _scan,
                    )
                  : IconButton(
                      icon: const Icon(Icons.close, size: 18),
                      onPressed: () {
                        _search.clear();
                        _load('');
                        setState(() {});
                      },
                    ),
            ),
          ),
        ),
        if (_categories.isNotEmpty)
          SizedBox(
            height: 40,
            child: ListView.separated(
              scrollDirection: Axis.horizontal,
              padding: const EdgeInsets.symmetric(horizontal: 12),
              itemCount: _categories.length + 1,
              separatorBuilder: (_, _) => const SizedBox(width: 8),
              itemBuilder: (_, i) {
                final c = i == 0 ? null : _categories[i - 1];
                return ChoiceChip(
                  label: Text(c?.name ?? l.categoryAll),
                  selected: _categoryId == c?.id,
                  onSelected: (_) {
                    setState(() => _categoryId = c?.id);
                    _load(_search.text.trim());
                  },
                );
              },
            ),
          ),
        if (_search.text.isEmpty && _categoryId == null) const ShortcutsStrip(),
        Expanded(
          child: _error != null
              ? Center(
                  child: Text(
                    _error!.message(l),
                    style: TextStyle(color: pal.danger),
                  ),
                )
              : _loading && _items.isEmpty
              ? const Center(child: CircularProgressIndicator())
              : _items.isEmpty
              ? Center(
                  child: Text(
                    l.posSearchEmpty,
                    style: TextStyle(color: pal.textTertiary),
                  ),
                )
              : GridView.builder(
                  padding: const EdgeInsets.fromLTRB(12, 4, 12, 12),
                  gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(
                    maxCrossAxisExtent: 190,
                    mainAxisExtent: 196,
                    crossAxisSpacing: 10,
                    mainAxisSpacing: 10,
                  ),
                  itemCount: _items.length,
                  itemBuilder: (_, i) => _ProductCard(item: _items[i]),
                ),
        ),
      ],
    );
  }
}

class _ProductCard extends ConsumerWidget {
  const _ProductCard({required this.item});

  final PosItem item;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final out = item.isGoods && item.stockDisplay <= Decimal.zero;
    return Material(
      color: pal.surface,
      borderRadius: BorderRadius.circular(14),
      child: InkWell(
        borderRadius: BorderRadius.circular(14),
        onTap: () => ref.read(cartProvider.notifier).add(item),
        child: Container(
          padding: const EdgeInsets.all(10),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(14),
            border: Border.all(color: pal.border),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              ClipRRect(
                borderRadius: BorderRadius.circular(9),
                child: SizedBox(
                  height: 84,
                  width: double.infinity,
                  child: ItemThumb(itemId: item.id, imageId: item.mainImageId),
                ),
              ),
              const SizedBox(height: 8),
              Expanded(
                child: Text(
                  item.name,
                  maxLines: 3,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontWeight: FontWeight.w600,
                    fontSize: 13,
                    height: 1.25,
                  ),
                ),
              ),
              const SizedBox(height: 4),
              Text(
                formatMoney(item.price),
                style: TextStyle(
                  fontWeight: FontWeight.w800,
                  color: pal.accent,
                ),
              ),
              const SizedBox(height: 2),
              Text(
                item.isGoods
                    ? l.posStock(formatQty(item.stockDisplay))
                    : item.unit,
                style: TextStyle(
                  fontSize: 11,
                  color: out ? pal.danger : pal.textTertiary,
                  fontWeight: out ? FontWeight.w600 : FontWeight.w400,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// ------------------------------------------------------------------ keranjang

class _CartBar extends ConsumerWidget {
  const _CartBar({required this.onOpen, required this.onPay});

  final VoidCallback onOpen;
  final VoidCallback onPay;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final cart = ref.watch(cartProvider);
    return Material(
      color: pal.surface,
      elevation: 8,
      child: SafeArea(
        top: false,
        child: Padding(
          padding: const EdgeInsets.fromLTRB(14, 10, 14, 10),
          child: Row(
            children: [
              Expanded(
                child: InkWell(
                  onTap: onOpen,
                  borderRadius: BorderRadius.circular(10),
                  child: Padding(
                    padding: const EdgeInsets.symmetric(vertical: 4),
                    child: Row(
                      children: [
                        Badge(
                          isLabelVisible: !cart.isEmpty,
                          label: Text(formatQty(cart.itemCount)),
                          child: const Icon(
                            Icons.shopping_cart_outlined,
                            size: 28,
                          ),
                        ),
                        const SizedBox(width: 12),
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                l.posTotal,
                                style: TextStyle(
                                  fontSize: 11,
                                  color: pal.textTertiary,
                                ),
                              ),
                              Text(
                                cart.isEmpty
                                    ? '—'
                                    : (cart.quote == null
                                          ? '...'
                                          : formatMoney(cart.quote!.total)),
                                style: const TextStyle(
                                  fontSize: 18,
                                  fontWeight: FontWeight.w800,
                                ),
                              ),
                            ],
                          ),
                        ),
                        const Icon(Icons.keyboard_arrow_up),
                      ],
                    ),
                  ),
                ),
              ),
              const SizedBox(width: 10),
              SizedBox(
                width: 120,
                child: FilledButton(
                  onPressed: cart.canPay ? onPay : null,
                  child: Text(l.posPay),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class CartPane extends ConsumerWidget {
  const CartPane({super.key, required this.onPay, this.scrollController});

  final VoidCallback onPay;
  final ScrollController? scrollController;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final cart = ref.watch(cartProvider);
    final ctrl = ref.read(cartProvider.notifier);
    final q = cart.quote;
    final pendingCount = ref.watch(pendingProvider.select((p) => p.length));

    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 8, 8, 4),
          child: Row(
            children: [
              Expanded(
                child: Row(
                  children: [
                    Flexible(
                      child: Text(
                        l.posCartTitle,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontSize: 16,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ),
                    const SizedBox(width: 8),
                    if (!cart.isEmpty)
                      Flexible(
                        child: Text(
                          l.posItemsCount(
                            int.tryParse(formatQty(cart.itemCount)) ??
                                cart.lines.length,
                          ),
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(
                            color: pal.textTertiary,
                            fontSize: 12.5,
                          ),
                        ),
                      ),
                  ],
                ),
              ),
              if (!cart.isEmpty)
                IconButton(
                  tooltip: l.pendingHoldTooltip,
                  icon: const Icon(Icons.pause_circle_outline),
                  onPressed: () async {
                    final held = await showHoldDialog(context, ref);
                    if (held && scrollController != null && context.mounted) {
                      Navigator.of(context)
                          .pop(); // tutup lembar keranjang di HP
                    }
                  },
                ),
              IconButton(
                tooltip: l.pendingListTooltip,
                icon: Badge(
                  isLabelVisible: pendingCount > 0,
                  label: Text('$pendingCount'),
                  child: const Icon(Icons.pending_actions_outlined),
                ),
                onPressed: () async {
                  final opened = await showPendingSheet(context);
                  if (opened && scrollController != null && context.mounted) {
                    Navigator.of(context).pop();
                  }
                },
              ),
              if (!cart.isEmpty)
                IconButton(
                  tooltip: l.posClearCart,
                  icon: const Icon(Icons.delete_sweep_outlined),
                  onPressed: ctrl.clear,
                ),
            ],
          ),
        ),
        const _MemberBar(),
        Expanded(
          child: cart.isEmpty
              ? const EmptyCart()
              : ListView.separated(
                  controller: scrollController,
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  itemCount: cart.lines.length,
                  separatorBuilder: (_, _) =>
                      Divider(height: 1, color: pal.border),
                  itemBuilder: (_, i) {
                    final line = cart.lines[i];
                    final ql = q != null && q.lines.length == cart.lines.length
                        ? q.lines[i]
                        : null;
                    return _CartRow(line: line, quote: ql);
                  },
                ),
        ),
        Material(
          color: pal.surface,
          child: Container(
            padding: const EdgeInsets.fromLTRB(16, 10, 16, 14),
            decoration: BoxDecoration(
              border: Border(top: BorderSide(color: pal.border)),
            ),
            child: SafeArea(
              top: false,
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  SwitchListTile(
                    dense: true,
                    contentPadding: EdgeInsets.zero,
                    title: Text(
                      l.posTaxToggle,
                      style: const TextStyle(fontSize: 13),
                    ),
                    value: cart.applyTax,
                    onChanged: ctrl.setTax,
                  ),
                  if (!cart.isEmpty)
                    Align(
                      alignment: Alignment.centerLeft,
                      child: TextButton.icon(
                        style: TextButton.styleFrom(
                          minimumSize: const Size(0, 36),
                          padding: EdgeInsets.zero,
                        ),
                        onPressed: () => showCostsSheet(context),
                        icon: Icon(
                          cart.costs.isNotEmpty || cart.note.isNotEmpty
                              ? Icons.edit_note
                              : Icons.add_circle_outline,
                          size: 18,
                        ),
                        label: Text(
                          cart.costs.isNotEmpty || cart.note.isNotEmpty
                              ? [
                                  if (cart.note.isNotEmpty) cart.note,
                                  if (cart.costs.isNotEmpty)
                                    '${l.posOtherCost} ${formatMoney(cart.costsTotal)}',
                                ].join(' · ')
                              : l.posCostsButton,
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                    ),
                  if (q != null) ...[
                    _sum(l.posSubtotal, formatMoney(q.subtotal), pal),
                    if (q.discount > Decimal.zero)
                      _sum(l.posDiscount, '- ${formatMoney(q.discount)}', pal),
                    if (q.redeemAmount > Decimal.zero)
                      _sum(
                        l.memberRedeemTitle,
                        '- ${formatMoney(q.redeemAmount)}',
                        pal,
                      ),
                    if (q.taxStore > Decimal.zero)
                      _sum(
                        l.posTaxStore(formatQty(q.taxStorePct)),
                        formatMoney(q.taxStore),
                        pal,
                      ),
                    if (q.taxGov > Decimal.zero)
                      _sum(
                        l.posTaxGov(formatQty(q.taxGovPct)),
                        formatMoney(q.taxGov),
                        pal,
                      ),
                    if (q.otherCost > Decimal.zero)
                      _sum(l.posOtherCost, formatMoney(q.otherCost), pal),
                  ],
                  if (cart.quoteError != null)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 4),
                      child: Text(
                        cart.quoteError!.fields.containsKey('redeem_points')
                            ? l.errorRedeemInvalid
                            : cart.quoteError!.message(l),
                        style: TextStyle(color: pal.danger, fontSize: 12.5),
                      ),
                    ),
                  Row(
                    mainAxisAlignment: MainAxisAlignment.spaceBetween,
                    children: [
                      Text(
                        l.posTotal,
                        style: const TextStyle(fontWeight: FontWeight.w700),
                      ),
                      Row(
                        children: [
                          if (cart.quoting)
                            const SizedBox(
                              width: 14,
                              height: 14,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            ),
                          const SizedBox(width: 8),
                          Text(
                            q == null ? '—' : formatMoney(q.total),
                            style: const TextStyle(
                              fontSize: 22,
                              fontWeight: FontWeight.w800,
                            ),
                          ),
                        ],
                      ),
                    ],
                  ),
                  const SizedBox(height: 10),
                  FilledButton(
                    onPressed: cart.canPay ? onPay : null,
                    child: Text(l.posPay),
                  ),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }

  Widget _sum(String k, String v, AppPalette pal) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 2),
    child: Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(k, style: TextStyle(color: pal.textTertiary, fontSize: 13)),
        Text(v, style: const TextStyle(fontSize: 13)),
      ],
    ),
  );
}

/// Baris member di atas keranjang: pelanggan umum / member terpilih (poin, deposit, poin yang akan didapat) + tombol tukar poin.
class _MemberBar extends ConsumerWidget {
  const _MemberBar();

  Widget _salesperson(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final sp = ref.watch(cartProvider.select((c) => c.salesperson));
    return InkWell(
      borderRadius: BorderRadius.circular(8),
      onTap: () => showSalespersonPicker(context),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(4, 6, 4, 0),
        child: Row(
          children: [
            Icon(Icons.badge_outlined, size: 16, color: pal.textTertiary),
            const SizedBox(width: 6),
            Text(
              '${l.salespersonLabel}: ',
              style: TextStyle(fontSize: 12, color: pal.textTertiary),
            ),
            Flexible(
              child: Text(
                sp?.name ?? l.salespersonNone,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: 12,
                  fontWeight: sp == null ? FontWeight.w400 : FontWeight.w700,
                  color: sp == null ? pal.textTertiary : pal.primary,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final cart = ref.watch(cartProvider);
    final m = cart.member;
    final qm = cart.quote?.member;
    final earn = cart.quote?.pointsEarn ?? 0;

    String detail() {
      if (m == null) return '';
      final parts = <String>[
        if (m.level.isNotEmpty) m.level,
        if (qm != null)
          l.memberPointsAndDeposit(qm.points, formatMoney(qm.deposit)),
        if (earn > 0) l.memberEarn(earn),
      ];
      return parts.join(' · ');
    }

    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 0, 12, 6),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Material(
            color: m == null ? pal.sunken : pal.primarySoft,
            borderRadius: BorderRadius.circular(12),
            child: InkWell(
              borderRadius: BorderRadius.circular(12),
              onTap: () => showMemberPicker(context),
              child: Padding(
                padding: const EdgeInsets.fromLTRB(12, 8, 4, 8),
                child: Row(
                  children: [
                    Icon(
                      m == null ? Icons.person_outline : Icons.person,
                      color: m == null ? pal.textTertiary : pal.primary,
                    ),
                    const SizedBox(width: 10),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            m == null
                                ? l.memberGeneral
                                : '${m.name} [${m.code}]',
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: TextStyle(
                              fontWeight: FontWeight.w600,
                              fontSize: 13.5,
                              color: m == null ? pal.textTertiary : null,
                            ),
                          ),
                          if (m != null)
                            Text(
                              detail(),
                              maxLines: 2,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(
                                fontSize: 11.5,
                                color: pal.textTertiary,
                              ),
                            ),
                        ],
                      ),
                    ),
                    if (m != null && !cart.isEmpty)
                      IconButton(
                        tooltip: l.memberRedeemTitle,
                        onPressed: () => showRedeemDialog(context),
                        icon: Badge(
                          isLabelVisible: cart.redeemPoints > 0,
                          label: Text('-${cart.redeemPoints}'),
                          child: const Icon(Icons.loyalty_outlined),
                        ),
                      )
                    else
                      Padding(
                        padding: const EdgeInsets.only(right: 8),
                        child: Text(
                          m == null ? l.memberChoose : l.memberChange,
                          style: TextStyle(
                            fontSize: 12,
                            fontWeight: FontWeight.w600,
                            color: pal.primary,
                          ),
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ),
          _salesperson(context, ref),
        ],
      ),
    );
  }
}

class _CartRow extends ConsumerWidget {
  const _CartRow({required this.line, required this.quote});

  final CartLine line;
  final QuoteLine? quote;

  Future<void> _editQty(BuildContext context, WidgetRef ref) async {
    final c = TextEditingController(text: formatQty(line.qty));
    final v = await showDialog<Decimal>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(
          line.item.name,
          maxLines: 2,
          overflow: TextOverflow.ellipsis,
        ),
        content: TextField(
          controller: c,
          autofocus: true,
          keyboardType: const TextInputType.numberWithOptions(decimal: true),
          onSubmitted: (_) => Navigator.of(ctx).pop(parseInput(c.text)),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: Text(MaterialLocalizations.of(ctx).cancelButtonLabel),
          ),
          FilledButton(
            style: FilledButton.styleFrom(minimumSize: const Size(0, 44)),
            onPressed: () => Navigator.of(ctx).pop(parseInput(c.text)),
            child: Text(MaterialLocalizations.of(ctx).okButtonLabel),
          ),
        ],
      ),
    );
    c.dispose();
    if (v != null) ref.read(cartProvider.notifier).setQty(line.item.id, v);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final ctrl = ref.read(cartProvider.notifier);
    final issue = quote?.issue;
    final unit = quote?.unitPrice ?? line.item.price;
    final approval = ref.watch(cartProvider.select((c) => c.approval));
    final total = quote?.lineTotal ?? (line.item.price * line.qty);

    return Dismissible(
      key: ValueKey(line.item.id),
      direction: DismissDirection.endToStart,
      background: Container(
        alignment: Alignment.centerRight,
        padding: const EdgeInsets.only(right: 20),
        color: pal.dangerSoft,
        child: Icon(Icons.delete_outline, color: pal.dangerText),
      ),
      onDismissed: (_) => ctrl.remove(line.item.id),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 10),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Expanded(
                  child: Text(
                    line.item.name,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontWeight: FontWeight.w600,
                      fontSize: 13.5,
                    ),
                  ),
                ),
                const SizedBox(width: 8),
                Text(
                  formatMoney(total),
                  style: const TextStyle(fontWeight: FontWeight.w700),
                ),
              ],
            ),
            const SizedBox(height: 6),
            Row(
              children: [
                Flexible(
                  child: Text(
                    '${formatMoney(unit)} / ${line.item.unit}',
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(fontSize: 12, color: pal.textTertiary),
                  ),
                ),
                InkWell(
                  borderRadius: BorderRadius.circular(8),
                  onTap: () => showLineAdjustSheet(
                    context,
                    line: line,
                    listPrice: quote?.listPrice ?? line.item.price,
                  ),
                  child: Padding(
                    padding: const EdgeInsets.all(6),
                    child: Icon(
                      Icons.edit_outlined,
                      size: 17,
                      semanticLabel: l.adjustEditTooltip,
                      color: line.adjusted ? pal.warningText : pal.textTertiary,
                    ),
                  ),
                ),
                const Spacer(),
                _QtyBtn(
                  icon: Icons.remove,
                  onTap: () =>
                      ctrl.setQty(line.item.id, line.qty - Decimal.one),
                ),
                InkWell(
                  onTap: () => _editQty(context, ref),
                  borderRadius: BorderRadius.circular(8),
                  child: Container(
                    constraints: const BoxConstraints(minWidth: 44),
                    padding: const EdgeInsets.symmetric(
                      horizontal: 8,
                      vertical: 6,
                    ),
                    alignment: Alignment.center,
                    child: Text(
                      formatQty(line.qty),
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 15,
                      ),
                    ),
                  ),
                ),
                _QtyBtn(
                  icon: Icons.add,
                  onTap: () =>
                      ctrl.setQty(line.item.id, line.qty + Decimal.one),
                ),
              ],
            ),
            if (line.adjusted)
              Padding(
                padding: const EdgeInsets.only(top: 6),
                child: Text(
                  [
                    if (line.overridePrice != null) l.adjustBadgePrice,
                    if (line.discountForServer != null)
                      l.adjustBadgeDiscount(
                        formatMoney(line.discountForServer!),
                      ),
                    if (approval != null) l.adjustApprovedBy(approval.name),
                  ].join(' · '),
                  style: TextStyle(
                    fontSize: 11.5,
                    fontWeight: FontWeight.w600,
                    color: pal.warningText,
                  ),
                ),
              ),
            if (issue == 'STOCK_INSUFFICIENT')
              _Issue(
                text: l.posStockShort(
                  formatQty(quote?.available ?? Decimal.zero),
                ),
                pal: pal,
              )
            else if (issue == 'BELOW_COST')
              _Issue(text: l.posBelowCost, pal: pal),
          ],
        ),
      ),
    );
  }
}

class _QtyBtn extends StatelessWidget {
  const _QtyBtn({required this.icon, required this.onTap});

  final IconData icon;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) => Material(
    color: context.pal.sunken,
    shape: CircleBorder(side: BorderSide(color: context.pal.border)),
    child: InkWell(
      customBorder: const CircleBorder(),
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.all(7),
        child: Icon(icon, size: 18),
      ),
    ),
  );
}

class _Issue extends StatelessWidget {
  const _Issue({required this.text, required this.pal});

  final String text;
  final AppPalette pal;

  @override
  Widget build(BuildContext context) => Container(
    margin: const EdgeInsets.only(top: 6),
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
    decoration: BoxDecoration(
      color: pal.dangerSoft,
      borderRadius: BorderRadius.circular(8),
    ),
    child: Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(Icons.warning_amber_rounded, size: 14, color: pal.dangerText),
        const SizedBox(width: 4),
        Flexible(
          child: Text(
            text,
            style: TextStyle(fontSize: 11.5, color: pal.dangerText),
          ),
        ),
      ],
    ),
  );
}
