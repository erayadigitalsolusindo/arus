import 'dart:async';
import 'dart:convert';

import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:uuid/uuid.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/session/session_controller.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import '../pos/pos_controller.dart';
import '../pos/pos_models.dart';
import 'manage_repository.dart';

/// Pembelian tunai sederhana dari HP (menambah stok). Tombol mengikuti izin: lihat = `purchase_list`/`purchase_invoices`
/// view, baru = `purchase_invoices` create, ubah = update, batal = delete. Diskon bertingkat, PPN, biaya lain, kredit,
/// dan retur tetap di web; revisi dari HP membawa nilai-nilai itu apa adanya.
Future<void> showPurchasesPage(BuildContext context) =>
    Navigator.of(context)
        .push<void>(MaterialPageRoute(builder: (_) => const PurchasesPage()));

final _apiDate = DateFormat('yyyy-MM-dd');

enum _Range { week, month30, thisMonth, custom }

class PurchasesPage extends ConsumerStatefulWidget {
  const PurchasesPage({super.key});

  @override
  ConsumerState<PurchasesPage> createState() => _PurchasesPageState();
}

class _PurchasesPageState extends ConsumerState<PurchasesPage> {
  final _search = TextEditingController();
  final _scroll = ScrollController();
  Timer? _debounce;
  int _seq = 0;
  _Range _range = _Range.month30;
  DateTimeRange? _custom;
  bool _loading = true;
  bool _loadingMore = false;
  String _cursor = '';
  List<PurchaseRow> _rows = const [];
  int _count = 0;
  Decimal _total = Decimal.zero;
  ApiError? _error;

  @override
  void initState() {
    super.initState();
    _scroll.addListener(() {
      if (_scroll.position.pixels > _scroll.position.maxScrollExtent - 300) {
        _more();
      }
    });
    _load();
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _search.dispose();
    _scroll.dispose();
    super.dispose();
  }

  DateTimeRange get _dates {
    final n = DateTime.now();
    final today = DateTime(n.year, n.month, n.day);
    switch (_range) {
      case _Range.week:
        return DateTimeRange(
          start: today.subtract(const Duration(days: 6)),
          end: today,
        );
      case _Range.thisMonth:
        return DateTimeRange(start: DateTime(n.year, n.month), end: today);
      case _Range.custom:
        return _custom ?? DateTimeRange(start: today, end: today);
      case _Range.month30:
        return DateTimeRange(
          start: today.subtract(const Duration(days: 29)),
          end: today,
        );
    }
  }

  Future<void> _load() async {
    final my = ++_seq;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final r = await ref
          .read(manageRepositoryProvider)
          .purchases(
            from: _apiDate.format(_dates.start),
            to: _apiDate.format(_dates.end),
            q: _search.text.trim(),
          );
      if (my == _seq && mounted) {
        setState(() {
          _rows = r.rows;
          _cursor = r.nextCursor;
          _count = r.count;
          _total = r.total;
        });
      }
    } on ApiError catch (e) {
      if (my == _seq && mounted) setState(() => _error = e);
    } finally {
      if (my == _seq && mounted) setState(() => _loading = false);
    }
  }

  Future<void> _more() async {
    if (_cursor.isEmpty || _loadingMore || _loading) return;
    final my = _seq;
    setState(() => _loadingMore = true);
    try {
      final r = await ref
          .read(manageRepositoryProvider)
          .purchases(
            from: _apiDate.format(_dates.start),
            to: _apiDate.format(_dates.end),
            q: _search.text.trim(),
            cursor: _cursor,
          );
      if (my != _seq || !mounted) return;
      setState(() {
        _rows = [..._rows, ...r.rows];
        _cursor = r.nextCursor;
      });
    } on ApiError {
      // dicoba lagi saat digulir
    } finally {
      if (mounted) setState(() => _loadingMore = false);
    }
  }

  void _onChanged(String _) {
    setState(() {});
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 300), _load);
  }

  Future<void> _pickRange() async {
    final now = DateTime.now();
    final r = await showDateRangePicker(
      context: context,
      initialDateRange: _dates,
      firstDate: DateTime(now.year - 3),
      lastDate: now,
    );
    if (r == null || !mounted) return;
    setState(() {
      _range = _Range.custom;
      _custom = r;
    });
    _load();
  }

  Future<void> _create() async {
    final doc = await Navigator.of(
      context,
    ).push<String>(MaterialPageRoute(builder: (_) => const PurchaseFormPage()));
    if (doc == null || !mounted) return;
    final l = AppLocalizations.of(context);
    ScaffoldMessenger.of(context)
      ..clearSnackBars()
      ..showSnackBar(SnackBar(content: Text(l.purSaved(doc))));
    _load();
  }

  Future<void> _open(PurchaseRow r) async {
    final changed = await Navigator.of(context).push<bool>(
      MaterialPageRoute(builder: (_) => PurchaseDetailPage(id: r.id)),
    );
    if (changed == true && mounted) _load();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final session = ref.watch(sessionProvider);
    final perms = session is SessionSignedIn
        ? session.profile.permissions
        : null;
    final canCreate = perms?.can('purchase_invoices', 'create') ?? false;
    final fmt = DateFormat('dd MMM', l.localeName);
    return Scaffold(
      appBar: AppBar(title: Text(l.purTitle)),
      floatingActionButton: canCreate
          ? FloatingActionButton.extended(
              onPressed: _create,
              icon: const Icon(Icons.add),
              label: Text(l.purAdd),
            )
          : null,
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 4),
              child: TextField(
                controller: _search,
                onChanged: _onChanged,
                decoration: InputDecoration(
                  hintText: l.salesTodaySearch,
                  prefixIcon: const Icon(Icons.search),
                  isDense: true,
                ),
              ),
            ),
            SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              padding: const EdgeInsets.symmetric(horizontal: 16),
              child: Row(
                children: [
                  for (final e in {
                    _Range.week: l.report7Days,
                    _Range.month30: l.purRange30,
                    _Range.thisMonth: l.reportThisMonth,
                  }.entries)
                    Padding(
                      padding: const EdgeInsets.only(right: 8),
                      child: ChoiceChip(
                        label: Text(e.value),
                        selected: _range == e.key,
                        onSelected: (_) {
                          setState(() => _range = e.key);
                          _load();
                        },
                      ),
                    ),
                  ChoiceChip(
                    avatar: const Icon(Icons.date_range, size: 18),
                    label: Text(
                      _range == _Range.custom
                          ? '${fmt.format(_dates.start)} - ${fmt.format(_dates.end)}'
                          : l.reportCustom,
                    ),
                    selected: _range == _Range.custom,
                    onSelected: (_) => _pickRange(),
                  ),
                ],
              ),
            ),
            if (!_loading && _error == null)
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 8, 16, 4),
                child: Align(
                  alignment: Alignment.centerLeft,
                  child: Text(
                    l.purSummary('$_count', formatMoney(_total)),
                    style: TextStyle(
                      fontWeight: FontWeight.w700,
                      color: pal.primary,
                    ),
                  ),
                ),
              ),
            if (_loading && _rows.isNotEmpty)
              const LinearProgressIndicator(minHeight: 2),
            Expanded(
              child: RefreshIndicator(
                onRefresh: _load,
                child: _error != null
                    ? _Msg(text: _error!.message(l), color: pal.danger)
                    : _rows.isEmpty
                    ? (_loading
                          ? const Center(child: CircularProgressIndicator())
                          : _Msg(text: l.purEmpty, color: pal.textTertiary))
                    : ListView.separated(
                        controller: _scroll,
                        physics: const AlwaysScrollableScrollPhysics(),
                        padding: const EdgeInsets.only(bottom: 88),
                        itemCount: _rows.length + (_cursor.isNotEmpty ? 1 : 0),
                        separatorBuilder: (_, _) =>
                            Divider(height: 1, color: pal.border),
                        itemBuilder: (_, i) {
                          if (i == _rows.length) {
                            return const Padding(
                              padding: EdgeInsets.all(16),
                              child: Center(
                                child: SizedBox(
                                  width: 20,
                                  height: 20,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                  ),
                                ),
                              ),
                            );
                          }
                          final r = _rows[i];
                          return ListTile(
                            dense: true,
                            onTap: () => _open(r),
                            title: Row(
                              children: [
                                Flexible(
                                  child: Text(
                                    r.docNo,
                                    overflow: TextOverflow.ellipsis,
                                    style: TextStyle(
                                      fontWeight: FontWeight.w700,
                                      decoration: r.isActive
                                          ? null
                                          : TextDecoration.lineThrough,
                                    ),
                                  ),
                                ),
                                if (!r.isActive) ...[
                                  const SizedBox(width: 8),
                                  _Badge(
                                    r.status == 'void'
                                        ? l.purStatusVoid
                                        : l.purStatusRevised,
                                    r.status == 'void'
                                        ? pal.danger
                                        : pal.warningText,
                                  ),
                                ],
                              ],
                            ),
                            subtitle: Text(
                              '${r.supplier} · ${r.date}',
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(
                                fontSize: 12,
                                color: pal.textTertiary,
                              ),
                            ),
                            trailing: Text(
                              formatMoney(r.total),
                              style: const TextStyle(
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                          );
                        },
                      ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Badge extends StatelessWidget {
  const _Badge(this.text, this.color);

  final String text;
  final Color color;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
    decoration: BoxDecoration(
      color: color.withValues(alpha: 0.12),
      borderRadius: BorderRadius.circular(4),
    ),
    child: Text(
      text,
      style: TextStyle(
        fontSize: 10.5,
        fontWeight: FontWeight.w800,
        color: color,
      ),
    ),
  );
}

class _Msg extends StatelessWidget {
  const _Msg({required this.text, required this.color});

  final String text;
  final Color color;

  @override
  Widget build(BuildContext context) => ListView(
    physics: const AlwaysScrollableScrollPhysics(),
    children: [
      Padding(
        padding: const EdgeInsets.all(48),
        child: Text(
          text,
          textAlign: TextAlign.center,
          style: TextStyle(color: color),
        ),
      ),
    ],
  );
}

// ---------------------------------------------------------------- detail

class PurchaseDetailPage extends ConsumerStatefulWidget {
  const PurchaseDetailPage({super.key, required this.id});

  final String id;

  @override
  ConsumerState<PurchaseDetailPage> createState() => _PurchaseDetailPageState();
}

class _PurchaseDetailPageState extends ConsumerState<PurchaseDetailPage> {
  PurchaseDetail? _d;
  ApiError? _error;
  bool _busy = false;
  bool _changed = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _error = null);
    try {
      final d = await ref.read(manageRepositoryProvider).purchase(widget.id);
      if (mounted) setState(() => _d = d);
    } on ApiError catch (e) {
      if (mounted) setState(() => _error = e);
    }
  }

  Future<String?> _askReason(String title, String body) {
    final l = AppLocalizations.of(context);
    final c = TextEditingController();
    return showDialog<String>(
      context: context,
      builder: (_) => AlertDialog(
        title: Text(title),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(body),
            const SizedBox(height: 12),
            TextField(
              controller: c,
              autofocus: true,
              maxLength: 200,
              decoration: InputDecoration(labelText: l.purReason),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(l.commonCancel),
          ),
          FilledButton(
            onPressed: () {
              final t = c.text.trim();
              if (t.length >= 3) Navigator.of(context).pop(t);
            },
            child: Text(title),
          ),
        ],
      ),
    );
  }

  Future<void> _void() async {
    final l = AppLocalizations.of(context);
    final d = _d!;
    final reason = await _askReason(l.purVoid, l.purVoidAsk(d.docNo));
    if (reason == null || !mounted) return;
    setState(() => _busy = true);
    try {
      await ref.read(manageRepositoryProvider).voidPurchase(d.id, reason);
      if (!mounted) return;
      ref.read(catalogVersionProvider.notifier).bump(); // stok katalog berubah
      _changed = true;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(l.purVoided)));
      Navigator.of(context).pop(true);
    } on ApiError catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(e.message(l))));
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _edit() async {
    final d = _d!;
    final ok = await Navigator.of(context).push<String>(
      MaterialPageRoute(builder: (_) => PurchaseFormPage(editing: d)),
    );
    if (ok == null || !mounted) return;
    _changed = true;
    final l = AppLocalizations.of(context);
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(l.purEdited)));
    Navigator.of(context).pop(true);
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final session = ref.watch(sessionProvider);
    final perms = session is SessionSignedIn
        ? session.profile.permissions
        : null;
    final d = _d;
    final canEdit =
        d != null &&
        d.isActive &&
        !d.hasReturns &&
        (perms?.can('purchase_invoices', 'update') ?? false);
    final canVoid =
        d != null &&
        d.isActive &&
        !d.hasReturns &&
        (perms?.can('purchase_invoices', 'delete') ?? false);
    Widget kv(String k, String v, {bool bold = false}) => Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Row(
        children: [
          Expanded(
            child: Text(
              k,
              style: TextStyle(fontWeight: bold ? FontWeight.w800 : null),
            ),
          ),
          Text(
            v,
            style: TextStyle(
              fontWeight: bold ? FontWeight.w800 : FontWeight.w600,
            ),
          ),
        ],
      ),
    );
    return PopScope(
      canPop: false,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop) Navigator.of(context).pop(_changed);
      },
      child: Scaffold(
        appBar: AppBar(title: Text(l.purDetailTitle)),
        body: SafeArea(
          child: _error != null
              ? Center(
                  child: Text(
                    _error!.message(l),
                    style: TextStyle(color: pal.danger),
                  ),
                )
              : d == null
              ? const Center(child: CircularProgressIndicator())
              : ListView(
                  padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            d.docNo,
                            style: const TextStyle(
                              fontSize: 18,
                              fontWeight: FontWeight.w800,
                            ),
                          ),
                        ),
                        if (!d.isActive)
                          _Badge(
                            d.status == 'void'
                                ? l.purStatusVoid
                                : l.purStatusRevised,
                            d.status == 'void' ? pal.danger : pal.warningText,
                          ),
                      ],
                    ),
                    const SizedBox(height: 4),
                    Text(
                      [
                        d.supplier,
                        d.date,
                        if (d.invoiceNo.isNotEmpty) d.invoiceNo,
                      ].join(' · '),
                      style: TextStyle(color: pal.textTertiary),
                    ),
                    if (d.createdBy.isNotEmpty)
                      Text(
                        l.purBy(d.createdBy),
                        style: TextStyle(fontSize: 12, color: pal.textTertiary),
                      ),
                    if (d.voidReason.isNotEmpty)
                      Padding(
                        padding: const EdgeInsets.only(top: 6),
                        child: Text(
                          l.purVoidReason(d.voidReason),
                          style: TextStyle(color: pal.danger),
                        ),
                      ),
                    const Divider(height: 24),
                    for (final ln in d.lines)
                      Padding(
                        padding: const EdgeInsets.symmetric(vertical: 4),
                        child: Row(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    ln.name,
                                    style: const TextStyle(
                                      fontWeight: FontWeight.w600,
                                    ),
                                  ),
                                  Text(
                                    '${formatQty(ln.qty)} ${ln.unit} × ${formatMoney(ln.price)}',
                                    style: TextStyle(
                                      fontSize: 12,
                                      color: pal.textTertiary,
                                    ),
                                  ),
                                ],
                              ),
                            ),
                            Text(
                              formatMoney(ln.total),
                              style: const TextStyle(
                                fontWeight: FontWeight.w600,
                              ),
                            ),
                          ],
                        ),
                      ),
                    const Divider(height: 24),
                    kv(l.purSubtotal, formatMoney(d.subtotal)),
                    if (d.taxAmount > Decimal.zero)
                      kv(l.purTax, formatMoney(d.taxAmount)),
                    if (d.otherCost > Decimal.zero)
                      kv(l.purOtherCost, formatMoney(d.otherCost)),
                    kv(l.purTotal, formatMoney(d.total), bold: true),
                    if (d.note.isNotEmpty) ...[
                      const SizedBox(height: 10),
                      Text(d.note, style: TextStyle(color: pal.textTertiary)),
                    ],
                    if (canEdit || canVoid) ...[
                      const SizedBox(height: 20),
                      Row(
                        children: [
                          if (canVoid)
                            Expanded(
                              child: OutlinedButton.icon(
                                onPressed: _busy ? null : _void,
                                icon: const Icon(Icons.block, size: 18),
                                label: Text(l.purVoid),
                                style: OutlinedButton.styleFrom(
                                  foregroundColor: pal.danger,
                                ),
                              ),
                            ),
                          if (canVoid && canEdit) const SizedBox(width: 12),
                          if (canEdit)
                            Expanded(
                              child: FilledButton.icon(
                                onPressed: _busy ? null : _edit,
                                icon: const Icon(Icons.edit_outlined, size: 18),
                                label: Text(l.purEdit),
                              ),
                            ),
                        ],
                      ),
                    ],
                  ],
                ),
        ),
      ),
    );
  }
}

// ------------------------------------------------------------------ form

/// Form pembelian tunai (baru atau revisi [editing]). Pop dengan nomor nota saat tersimpan.
class PurchaseFormPage extends ConsumerStatefulWidget {
  const PurchaseFormPage({super.key, this.editing});

  final PurchaseDetail? editing;

  @override
  ConsumerState<PurchaseFormPage> createState() => _PurchaseFormPageState();
}

class _PurchaseFormPageState extends ConsumerState<PurchaseFormPage> {
  final _invoice = TextEditingController();
  final _note = TextEditingController();
  String? _supplierId;
  String _supplierName = '';
  DateTime _date = DateTime.now();
  final List<PurchaseLineDraft> _lines = [];
  bool _busy = false;
  bool _submitted = false;
  ApiError? _error;
  String? _key;
  String? _keyHash;

  bool get _edit => widget.editing != null;

  @override
  void initState() {
    super.initState();
    final e = widget.editing;
    if (e != null) {
      _supplierId = e.supplierId;
      _supplierName = e.supplier;
      _invoice.text = e.invoiceNo;
      _note.text = e.note;
      _date = DateTime.tryParse(e.date) ?? DateTime.now();
      _lines.addAll([
        for (final l in e.lines)
          PurchaseLineDraft(
            itemId: l.itemId,
            name: l.name,
            unit: l.unit,
            qty: l.qty,
            price: l.price,
            discounts: l.discounts,
            qtyWarehouse: l.qtyWarehouse,
          ),
      ]);
    }
  }

  @override
  void dispose() {
    _invoice.dispose();
    _note.dispose();
    super.dispose();
  }

  Decimal get _subtotal => _lines.fold(Decimal.zero, (a, l) => a + l.total);

  Future<void> _pickSupplier() async {
    final s = await showModalBottomSheet<({String id, String name})>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => const _SupplierPicker(),
    );
    if (s != null && mounted) {
      setState(() {
        _supplierId = s.id;
        _supplierName = s.name;
      });
    }
  }

  Future<void> _pickDate() async {
    final now = DateTime.now();
    final d = await showDatePicker(
      context: context,
      initialDate: _date,
      firstDate: DateTime(now.year - 3),
      lastDate: now,
    );
    if (d != null && mounted) setState(() => _date = d);
  }

  Future<void> _addItem() async {
    final c = await showModalBottomSheet<PurchaseItemChoice>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => const _PurchaseItemPicker(),
    );
    if (c == null || !mounted) return;
    final i = _lines.indexWhere((l) => l.itemId == c.id);
    setState(() {
      if (i >= 0) {
        _lines[i].qty += Decimal.one;
      } else {
        _lines.add(
          PurchaseLineDraft(
            itemId: c.id,
            name: c.name,
            unit: c.unit,
            qty: Decimal.one,
            price: c.lastCost,
          ),
        );
      }
    });
  }

  bool get _linesOk =>
      _lines.isNotEmpty &&
      _lines.every((l) => l.qty > Decimal.zero && l.price >= Decimal.zero);

  Future<void> _save({String reason = ''}) async {
    setState(() => _submitted = true);
    if (_supplierId == null || !_linesOk) return;
    String? why;
    if (_edit) {
      why = reason.isNotEmpty ? reason : await _askReason();
      if (why == null || !mounted) return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    // Kunci idempotensi dipakai ulang selama isi sama (ulang tekan setelah jaringan putus tidak menggandakan nota).
    final hash = jsonEncode([
      _supplierId,
      _apiDate.format(_date),
      _invoice.text.trim(),
      _note.text.trim(),
      [for (final ln in _lines) ln.toJson()],
      why,
    ]);
    if (_keyHash != hash) {
      _key = const Uuid().v4();
      _keyHash = hash;
    }
    final repo = ref.read(manageRepositoryProvider);
    try {
      final saved = _edit
          ? await repo.editPurchase(
              widget.editing!,
              supplierId: _supplierId!,
              date: _apiDate.format(_date),
              invoiceNo: _invoice.text.trim(),
              note: _note.text.trim(),
              lines: _lines,
              reason: why!,
              idempotencyKey: _key!,
            )
          : await repo.createPurchase(
              supplierId: _supplierId!,
              date: _apiDate.format(_date),
              invoiceNo: _invoice.text.trim(),
              note: _note.text.trim(),
              lines: _lines,
              idempotencyKey: _key!,
            );
      if (!mounted) return;
      ref
          .read(catalogVersionProvider.notifier)
          .bump(); // stok di katalog kasir berubah
      Navigator.of(context).pop(saved.docNo);
    } on ApiError catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<String?> _askReason() {
    final l = AppLocalizations.of(context);
    final c = TextEditingController();
    return showDialog<String>(
      context: context,
      builder: (_) => AlertDialog(
        title: Text(l.purEditTitle),
        content: TextField(
          controller: c,
          autofocus: true,
          maxLength: 200,
          decoration: InputDecoration(labelText: l.purReason),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(l.commonCancel),
          ),
          FilledButton(
            onPressed: () {
              final t = c.text.trim();
              if (t.length >= 3) Navigator.of(context).pop(t);
            },
            child: Text(l.commonSave),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final dateFmt = DateFormat('dd MMM yyyy', l.localeName);
    return Scaffold(
      appBar: AppBar(title: Text(_edit ? l.purEditTitle : l.purAdd)),
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
                children: [
                  if (!_edit)
                    Container(
                      padding: const EdgeInsets.all(10),
                      decoration: BoxDecoration(
                        color: pal.primarySoft,
                        borderRadius: BorderRadius.circular(10),
                      ),
                      child: Row(
                        children: [
                          Icon(
                            Icons.payments_outlined,
                            size: 18,
                            color: pal.primary,
                          ),
                          const SizedBox(width: 8),
                          Expanded(
                            child: Text(
                              l.purCashNote,
                              style: TextStyle(
                                fontSize: 12.5,
                                color: pal.primary,
                              ),
                            ),
                          ),
                        ],
                      ),
                    ),
                  const SizedBox(height: 12),
                  InkWell(
                    onTap: _pickSupplier,
                    borderRadius: BorderRadius.circular(8),
                    child: InputDecorator(
                      decoration: InputDecoration(
                        labelText: l.purSupplier,
                        isDense: true,
                        suffixIcon: const Icon(Icons.arrow_drop_down),
                        errorText: _submitted && _supplierId == null
                            ? l.purNeedSupplier
                            : null,
                      ),
                      child: Text(
                        _supplierId == null ? l.purPickSupplier : _supplierName,
                        style: TextStyle(
                          color: _supplierId == null ? pal.textTertiary : null,
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 10),
                  Row(
                    children: [
                      Expanded(
                        child: InkWell(
                          onTap: _pickDate,
                          borderRadius: BorderRadius.circular(8),
                          child: InputDecorator(
                            decoration: InputDecoration(
                              labelText: l.purDate,
                              isDense: true,
                              suffixIcon: const Icon(
                                Icons.calendar_today,
                                size: 18,
                              ),
                            ),
                            child: Text(dateFmt.format(_date)),
                          ),
                        ),
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: TextField(
                          controller: _invoice,
                          maxLength: 60,
                          decoration: InputDecoration(
                            labelText: l.purInvoiceNo,
                            isDense: true,
                            counterText: '',
                          ),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 16),
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          l.purLines,
                          style: const TextStyle(fontWeight: FontWeight.w800),
                        ),
                      ),
                      TextButton.icon(
                        onPressed: _addItem,
                        icon: const Icon(Icons.add, size: 18),
                        label: Text(l.purAddItem),
                      ),
                    ],
                  ),
                  if (_submitted && _lines.isEmpty)
                    Text(l.purNoLines, style: TextStyle(color: pal.danger)),
                  if (_submitted && _lines.isNotEmpty && !_linesOk)
                    Text(l.purBadLine, style: TextStyle(color: pal.danger)),
                  for (var i = 0; i < _lines.length; i++)
                    _LineEditor(
                      key: ObjectKey(_lines[i]),
                      line: _lines[i],
                      onChanged: () => setState(() {}),
                      onRemove: () => setState(() => _lines.removeAt(i)),
                    ),
                  const SizedBox(height: 10),
                  TextField(
                    controller: _note,
                    maxLength: 200,
                    decoration: InputDecoration(
                      labelText: l.purNote,
                      isDense: true,
                      counterText: '',
                    ),
                  ),
                  if (_error != null)
                    Padding(
                      padding: const EdgeInsets.only(top: 8),
                      child: Text(
                        _error!.message(l),
                        style: TextStyle(color: pal.danger),
                      ),
                    ),
                ],
              ),
            ),
            Container(
              padding: const EdgeInsets.fromLTRB(16, 10, 16, 12),
              decoration: BoxDecoration(
                color: pal.surface,
                border: Border(top: BorderSide(color: pal.border)),
              ),
              child: Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Text(
                          l.purSubtotal,
                          style: TextStyle(
                            fontSize: 12,
                            color: pal.textTertiary,
                          ),
                        ),
                        FittedBox(
                          fit: BoxFit.scaleDown,
                          alignment: Alignment.centerLeft,
                          child: Text(
                            formatMoney(_subtotal),
                            style: const TextStyle(
                              fontSize: 20,
                              fontWeight: FontWeight.w800,
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(width: 12),
                  FilledButton(
                    style: FilledButton.styleFrom(
                      minimumSize: const Size(150, 48),
                    ),
                    onPressed: _busy ? null : () => _save(),
                    child: _busy
                        ? const SizedBox(
                            width: 18,
                            height: 18,
                            child: CircularProgressIndicator(
                              strokeWidth: 2,
                              color: Colors.white,
                            ),
                          )
                        : Text(_edit ? l.commonSave : l.purSave),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Satu baris barang di form: qty dan harga beli, hapus.
class _LineEditor extends StatefulWidget {
  const _LineEditor({
    super.key,
    required this.line,
    required this.onChanged,
    required this.onRemove,
  });

  final PurchaseLineDraft line;
  final VoidCallback onChanged;
  final VoidCallback onRemove;

  @override
  State<_LineEditor> createState() => _LineEditorState();
}

class _LineEditorState extends State<_LineEditor> {
  late final _qty = TextEditingController(text: decToApi(widget.line.qty));
  late final _price = TextEditingController(
    text: decToApi(widget.line.price, scale: 2),
  );

  @override
  void dispose() {
    _qty.dispose();
    _price.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final numPad = [FilteringTextInputFormatter.allow(RegExp(r'[0-9.,]'))];
    const kb = TextInputType.numberWithOptions(decimal: true);
    return Container(
      margin: const EdgeInsets.only(top: 8),
      padding: const EdgeInsets.fromLTRB(12, 8, 4, 10),
      decoration: BoxDecoration(
        border: Border.all(color: pal.border),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  widget.line.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(fontWeight: FontWeight.w700),
                ),
              ),
              IconButton(
                visualDensity: VisualDensity.compact,
                icon: const Icon(Icons.close, size: 18),
                onPressed: widget.onRemove,
              ),
            ],
          ),
          Padding(
            padding: const EdgeInsets.only(right: 8),
            child: Row(
              children: [
                SizedBox(
                  width: 90,
                  child: TextField(
                    controller: _qty,
                    keyboardType: kb,
                    inputFormatters: numPad,
                    onChanged: (v) {
                      widget.line.qty = parseInput(v);
                      widget.onChanged();
                    },
                    decoration: InputDecoration(
                      labelText: '${l.purQty} (${widget.line.unit})',
                      isDense: true,
                    ),
                  ),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: TextField(
                    controller: _price,
                    keyboardType: kb,
                    inputFormatters: numPad,
                    onChanged: (v) {
                      widget.line.price = parseInput(v);
                      widget.onChanged();
                    },
                    decoration: InputDecoration(
                      labelText: l.purPrice,
                      isDense: true,
                    ),
                  ),
                ),
                const SizedBox(width: 10),
                SizedBox(
                  width: 86,
                  child: Text(
                    formatMoney(widget.line.total),
                    textAlign: TextAlign.right,
                    style: const TextStyle(fontWeight: FontWeight.w700),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

// --------------------------------------------------------------- pickers

class _SupplierPicker extends ConsumerStatefulWidget {
  const _SupplierPicker();

  @override
  ConsumerState<_SupplierPicker> createState() => _SupplierPickerState();
}

class _SupplierPickerState extends ConsumerState<_SupplierPicker> {
  final _q = TextEditingController();
  Timer? _debounce;
  List<Salesperson> _list = const [];
  bool _loading = true;
  ApiError? _error;
  int _seq = 0;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _q.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    final my = ++_seq;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final r = await ref
          .read(manageRepositoryProvider)
          .suppliers(_q.text.trim());
      if (my == _seq && mounted) setState(() => _list = r);
    } on ApiError catch (e) {
      if (my == _seq && mounted) setState(() => _error = e);
    } finally {
      if (my == _seq && mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      child: SizedBox(
        height: MediaQuery.sizeOf(context).height * 0.7,
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
              child: TextField(
                controller: _q,
                autofocus: true,
                onChanged: (_) {
                  _debounce?.cancel();
                  _debounce = Timer(const Duration(milliseconds: 300), _load);
                },
                decoration: InputDecoration(
                  hintText: l.purSearchSupplier,
                  prefixIcon: const Icon(Icons.search),
                  isDense: true,
                ),
              ),
            ),
            Expanded(
              child: _error != null
                  ? Center(
                      child: Text(
                        _error!.message(l),
                        style: TextStyle(color: pal.danger),
                      ),
                    )
                  : _loading && _list.isEmpty
                  ? const Center(child: CircularProgressIndicator())
                  : _list.isEmpty
                  ? Center(
                      child: Text(
                        l.purNoSupplier,
                        style: TextStyle(color: pal.textTertiary),
                      ),
                    )
                  : ListView.builder(
                      itemCount: _list.length,
                      itemBuilder: (_, i) => ListTile(
                        dense: true,
                        title: Text(_list[i].name),
                        onTap: () =>
                            Navigator.of(context)
                                .pop((id: _list[i].id, name: _list[i].name)),
                      ),
                    ),
            ),
          ],
        ),
      ),
    );
  }
}

class _PurchaseItemPicker extends ConsumerStatefulWidget {
  const _PurchaseItemPicker();

  @override
  ConsumerState<_PurchaseItemPicker> createState() =>
      _PurchaseItemPickerState();
}

class _PurchaseItemPickerState extends ConsumerState<_PurchaseItemPicker> {
  final _q = TextEditingController();
  Timer? _debounce;
  List<PurchaseItemChoice> _list = const [];
  bool _loading = true;
  ApiError? _error;
  int _seq = 0;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _q.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    final my = ++_seq;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final r = await ref
          .read(manageRepositoryProvider)
          .purchaseItems(_q.text.trim());
      if (my == _seq && mounted) setState(() => _list = r);
    } on ApiError catch (e) {
      if (my == _seq && mounted) setState(() => _error = e);
    } finally {
      if (my == _seq && mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      child: SizedBox(
        height: MediaQuery.sizeOf(context).height * 0.7,
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
              child: TextField(
                controller: _q,
                autofocus: true,
                onChanged: (_) {
                  _debounce?.cancel();
                  _debounce = Timer(const Duration(milliseconds: 300), _load);
                },
                decoration: InputDecoration(
                  hintText: l.purSearchItem,
                  prefixIcon: const Icon(Icons.search),
                  isDense: true,
                ),
              ),
            ),
            Expanded(
              child: _error != null
                  ? Center(
                      child: Text(
                        _error!.message(l),
                        style: TextStyle(color: pal.danger),
                      ),
                    )
                  : _loading && _list.isEmpty
                  ? const Center(child: CircularProgressIndicator())
                  : ListView.builder(
                      itemCount: _list.length,
                      itemBuilder: (_, i) {
                        final c = _list[i];
                        return ListTile(
                          dense: true,
                          title: Text(c.name),
                          subtitle: Text(
                            [
                              if (c.sku.isNotEmpty) c.sku,
                              if (c.unit.isNotEmpty) c.unit,
                              if (c.stock.isNotEmpty) l.itemsStock(c.stock),
                            ].join(' · '),
                            style: TextStyle(
                              fontSize: 12,
                              color: pal.textTertiary,
                            ),
                          ),
                          trailing: Text(formatMoney(c.lastCost)),
                          onTap: () => Navigator.of(context).pop(c),
                        );
                      },
                    ),
            ),
          ],
        ),
      ),
    );
  }
}
