import 'dart:async';

import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/printing/printer_service.dart';
import '../../core/printing/printer_sheet.dart';
import '../../core/printing/recap_layout.dart';
import '../../core/session/session_controller.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'pos_repository.dart';
import 'sales_models.dart';

/// Penjualan kasir ini pada rentang tanggal + jam + menit (default hari ini): daftar nota, laporan per metode bayar, cetak rekap
/// untuk cek fisik, dan detail nota (hanya baca; edit/batal tetap di web).
Future<void> showSalesTodayPage(BuildContext context) =>
    Navigator.of(context)
        .push<void>(MaterialPageRoute(builder: (_) => const _SalesTodayPage()));

class _SalesTodayPage extends ConsumerStatefulWidget {
  const _SalesTodayPage();

  @override
  ConsumerState<_SalesTodayPage> createState() => _SalesTodayPageState();
}

const _kApiFmt = 'yyyy-MM-dd\'T\'HH:mm';

/// Batas dari server (`YYYY-MM-DD` = seharian, `YYYY-MM-DDTHH:MM`) menjadi waktu lokal untuk ditampilkan/diubah.
DateTime? _parseBound(String raw, {required bool end}) {
  final dt = DateTime.tryParse(raw);
  if (dt == null) return null;
  if (raw.contains('T')) return dt;
  return end ? DateTime(dt.year, dt.month, dt.day, 23, 59) : dt;
}

class _SalesTodayPageState extends ConsumerState<_SalesTodayPage> {
  final _search = TextEditingController();
  Timer? _debounce;
  int _seq = 0;
  SalesToday? _data;
  bool _loading = true;
  ApiError? _error;

  /// null = default server (hari ini, zona waktu outlet). Terisi bila kasir memilih rentang sendiri.
  DateTime? _from;
  DateTime? _to;
  String? _rangeError;
  bool _printing = false;
  String? _actionError;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _search.dispose();
    super.dispose();
  }

  String _fmt(DateTime d) => DateFormat(_kApiFmt).format(d);

  Future<void> _load() async {
    final my = ++_seq;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final r = await ref
          .read(posRepositoryProvider)
          .salesToday(
            q: _search.text.trim(),
            from: _from == null ? '' : _fmt(_from!),
            to: _to == null ? '' : _fmt(_to!),
          );
      if (my != _seq || !mounted) return;
      setState(() => _data = r);
    } on ApiError catch (e) {
      if (my == _seq && mounted) setState(() => _error = e);
    } finally {
      if (my == _seq && mounted) setState(() => _loading = false);
    }
  }

  /// Rentang yang sedang ditampilkan (dari jawaban server, atau pilihan kasir).
  DateTime get _shownFrom =>
      _from ??
      _parseBound(_data?.from ?? '', end: false) ??
      DateTime.now().copyWith(hour: 0, minute: 0, second: 0, millisecond: 0);
  DateTime get _shownTo =>
      _to ??
      _parseBound(_data?.to ?? '', end: true) ??
      DateTime.now().copyWith(hour: 23, minute: 59, second: 0, millisecond: 0);

  Future<DateTime?> _pick(DateTime initial) async {
    final d = await showDatePicker(
      context: context,
      initialDate: initial,
      firstDate: DateTime.now().subtract(const Duration(days: 365 * 3)),
      lastDate: DateTime.now().add(const Duration(days: 1)),
    );
    if (d == null || !mounted) return null;
    final t = await showTimePicker(
      context: context,
      initialTime: TimeOfDay.fromDateTime(initial),
      builder: (context, child) => MediaQuery(
        data: MediaQuery.of(context).copyWith(alwaysUse24HourFormat: true),
        child: child!,
      ),
    );
    if (t == null) return null;
    return DateTime(d.year, d.month, d.day, t.hour, t.minute);
  }

  Future<void> _setRange({DateTime? from, DateTime? to}) async {
    final l = AppLocalizations.of(context);
    final f = from ?? _shownFrom;
    final t = to ?? _shownTo;
    String? err;
    if (!t.isAfter(f)) {
      err = l.salesRangeInvalid;
    } else if (t.difference(f).inDays > 62) {
      err = l.salesRangeTooLong;
    }
    setState(() => _rangeError = err);
    if (err != null) return;
    setState(() {
      _from = f;
      _to = t;
    });
    await _load();
  }

  void _resetRange() {
    setState(() {
      _from = null;
      _to = null;
      _rangeError = null;
    });
    _load();
  }

  Future<void> _printRecap() async {
    final l = AppLocalizations.of(context);
    final d = _data;
    if (d == null) return;
    var settings = await ref.read(printerSettingsProvider.future);
    if (!settings.hasPrinter) {
      if (!mounted) return;
      await showPrinterSheet(context);
      settings = await ref.read(printerSettingsProvider.future);
      if (!settings.hasPrinter) return;
    }
    final session = ref.read(sessionProvider);
    final profile = session is SessionSignedIn ? session.profile : null;
    setState(() {
      _printing = true;
      _actionError = null;
    });
    try {
      // Rekap selalu seluruh rentang, walau daftar sedang disaring dengan nomor nota.
      final full = _search.text.trim().isEmpty
          ? d
          : await ref
                .read(posRepositoryProvider)
                .salesToday(from: d.from, to: d.to);
      await ref
          .read(receiptPrinterProvider)
          .printLines(
            recapLines(
              full,
              settings.paper,
              l,
              tenant: profile?.tenant.name ?? '',
              outlet: profile?.outlet.name ?? '',
              cashier: profile?.user.name ?? '',
            ),
            settings,
          );
    } catch (e) {
      if (mounted) {
        setState(
          () => _actionError = e is ApiError
              ? e.message(l)
              : printErrorMessage(l, e),
        );
      }
    } finally {
      if (mounted) setState(() => _printing = false);
    }
  }

  void _onChanged(String _) {
    setState(() {});
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 300), _load);
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final d = _data;
    return Scaffold(
      appBar: AppBar(
        title: Text(l.salesTodayTitle),
        actions: [
          IconButton(
            tooltip: MaterialLocalizations.of(context)
                .refreshIndicatorSemanticLabel,
            icon: const Icon(Icons.refresh),
            onPressed: _loading ? null : _load,
          ),
        ],
      ),
      body: SafeArea(
        child: Column(
          children: [
            _RangeBar(
              from: _shownFrom,
              to: _shownTo,
              custom: _from != null,
              error: _rangeError,
              onPickFrom: () async {
                final v = await _pick(_shownFrom);
                if (v != null) _setRange(from: v);
              },
              onPickTo: () async {
                final v = await _pick(_shownTo);
                if (v != null) _setRange(to: v);
              },
              onToday: _resetRange,
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 4, 16, 8),
              child: TextField(
                controller: _search,
                onChanged: _onChanged,
                textInputAction: TextInputAction.search,
                decoration: InputDecoration(
                  hintText: l.salesTodaySearch,
                  prefixIcon: const Icon(Icons.search),
                  suffixIcon: _search.text.isEmpty
                      ? null
                      : IconButton(
                          icon: const Icon(Icons.close),
                          onPressed: () {
                            _search.clear();
                            _onChanged('');
                          },
                        ),
                  isDense: true,
                ),
              ),
            ),
            if (d != null) _Summary(data: d),
            if (d != null)
              _Actions(
                printing: _printing,
                onPrint: _printRecap,
                error: _actionError,
              ),
            if (_loading && d != null)
              const LinearProgressIndicator(minHeight: 2),
            Expanded(
              child: _error != null
                  ? Center(
                      child: Text(
                        _error!.message(l),
                        style: TextStyle(color: pal.danger),
                      ),
                    )
                  : d == null
                  ? const Center(child: CircularProgressIndicator())
                  : d.rows.isEmpty
                  ? Center(
                      child: Text(
                        l.salesTodayEmpty,
                        style: TextStyle(color: pal.textTertiary),
                      ),
                    )
                  : ListView(
                      children: [
                        _MethodReport(data: d),
                        for (final r in d.rows)
                          _SaleTile(key: ValueKey(r.id), row: r),
                        if (d.truncated)
                          Padding(
                            padding: const EdgeInsets.all(16),
                            child: Text(
                              l.salesTodayTruncated,
                              style: TextStyle(color: pal.warningText),
                            ),
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

class _Summary extends StatelessWidget {
  const _Summary({required this.data});

  final SalesToday data;

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    return Container(
      width: double.infinity,
      margin: const EdgeInsets.symmetric(horizontal: 16),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: pal.primarySoft,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            l.salesTodaySummary('${data.activeCount}', formatMoney(data.total)),
            style: TextStyle(fontWeight: FontWeight.w800, color: pal.primary),
          ),
          if (data.returnsCount > 0 && data.returnsTotal != null) ...[
            const SizedBox(height: 4),
            Text(
              '${l.salesReturnsRow('${data.returnsCount}')}: -${formatMoney(data.returnsTotal!)}'
              '  ·  ${l.salesNetTotal}: ${formatMoney(data.netTotal ?? data.total)}',
              style: const TextStyle(fontSize: 12.5),
            ),
          ],
        ],
      ),
    );
  }
}

/// Filter rentang: tanggal + jam + menit awal dan akhir (shift sering tidak beraturan), plus pintasan "Hari ini".
class _RangeBar extends StatelessWidget {
  const _RangeBar({
    required this.from,
    required this.to,
    required this.custom,
    required this.error,
    required this.onPickFrom,
    required this.onPickTo,
    required this.onToday,
  });

  final DateTime from;
  final DateTime to;
  final bool custom;
  final String? error;
  final VoidCallback onPickFrom;
  final VoidCallback onPickTo;
  final VoidCallback onToday;

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final f = DateFormat('dd/MM/yy HH:mm');
    Widget field(String label, DateTime v, VoidCallback onTap) => Expanded(
      child: OutlinedButton(
        onPressed: onTap,
        style: OutlinedButton.styleFrom(
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
          alignment: Alignment.centerLeft,
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              label,
              style: TextStyle(fontSize: 11, color: pal.textTertiary),
            ),
            Text(
              f.format(v),
              style: const TextStyle(
                fontSize: 13.5,
                fontWeight: FontWeight.w700,
              ),
            ),
          ],
        ),
      ),
    );
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              field(l.salesFrom, from, onPickFrom),
              const SizedBox(width: 8),
              field(l.salesTo, to, onPickTo),
              const SizedBox(width: 8),
              ChoiceChip(
                label: Text(l.salesPresetToday),
                selected: !custom,
                onSelected: (_) => onToday(),
              ),
            ],
          ),
          if (error != null)
            Padding(
              padding: const EdgeInsets.only(top: 4),
              child: Text(
                error!,
                style: TextStyle(color: pal.danger, fontSize: 12.5),
              ),
            ),
        ],
      ),
    );
  }
}

class _Actions extends StatelessWidget {
  const _Actions({
    required this.printing,
    required this.onPrint,
    required this.error,
  });

  final bool printing;
  final VoidCallback onPrint;
  final String? error;

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    Widget spin() => const SizedBox(
      width: 16,
      height: 16,
      child: CircularProgressIndicator(strokeWidth: 2),
    );
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: OutlinedButton.icon(
                  onPressed: printing ? null : onPrint,
                  icon: printing ? spin() : const Icon(Icons.print_outlined),
                  label: Text(l.salesPrintRecap),
                ),
              ),
            ],
          ),
          if (error != null)
            Padding(
              padding: const EdgeInsets.only(top: 4),
              child: Text(
                error!,
                style: TextStyle(color: pal.danger, fontSize: 12.5),
              ),
            ),
        ],
      ),
    );
  }
}

/// Laporan penjualan per metode bayar (tunai, QRIS, ...) + isi laci yang seharusnya ada (cek fisik) + uang lain.
class _MethodReport extends StatelessWidget {
  const _MethodReport({required this.data});

  final SalesToday data;

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    if (data.byMethod.isEmpty && data.drawer.isEmpty) {
      return const SizedBox.shrink();
    }
    Widget line(String k, String v, {bool bold = false, String? sub}) =>
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 2),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      k,
                      style: TextStyle(
                        fontWeight: bold ? FontWeight.w800 : FontWeight.w500,
                      ),
                    ),
                    if (sub != null)
                      Text(
                        sub,
                        style: TextStyle(
                          fontSize: 11.5,
                          color: pal.textTertiary,
                        ),
                      ),
                  ],
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
    Widget title(String t) => Padding(
      padding: const EdgeInsets.only(top: 10, bottom: 2),
      child: Text(
        t,
        style: TextStyle(
          fontSize: 12,
          fontWeight: FontWeight.w700,
          color: pal.textTertiary,
        ),
      ),
    );
    String signed(Decimal v) =>
        '${v > Decimal.zero ? '+' : ''}${formatMoney(v)}';
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 4),
      child: Card(
        margin: EdgeInsets.zero,
        child: ExpansionTile(
          initiallyExpanded: true,
          shape: const Border(),
          collapsedShape: const Border(),
          leading: const Icon(Icons.account_balance_wallet_outlined),
          title: Text(
            l.salesMethodsTitle,
            style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 14.5),
          ),
          childrenPadding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
          expandedCrossAxisAlignment: CrossAxisAlignment.start,
          children: [
            for (final m in data.byMethod)
              line(
                m.name,
                formatMoney(m.amount),
                sub: l.salesMethodNotes('${data.notesFor(m.name)}'),
              ),
            if (data.credit != null && data.credit! > Decimal.zero)
              line(l.salesCreditRow, formatMoney(data.credit!)),
            if (data.surcharge != null && data.surcharge! > Decimal.zero)
              line(l.salesSurchargeRow, formatMoney(data.surcharge!)),
            if (data.flows.isNotEmpty) ...[
              title(l.salesFlowsTitle),
              for (final f in data.flows)
                line('${flowLabel(l, f.source)} · ${f.name}', signed(f.amount)),
            ],
            if (data.drawer.isNotEmpty) ...[
              title(l.salesDrawerTitle),
              for (final m in data.drawer)
                line(m.name, formatMoney(m.amount), bold: m.kind == 'cash'),
            ],
          ],
        ),
      ),
    );
  }
}

class _SaleTile extends ConsumerStatefulWidget {
  const _SaleTile({super.key, required this.row});

  final SaleRow row;

  @override
  ConsumerState<_SaleTile> createState() => _SaleTileState();
}

class _SaleTileState extends ConsumerState<_SaleTile> {
  Future<SaleDetailInfo>? _detail;

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final r = widget.row;
    final pays = [
      ...r.payNames,
      if (r.receivable > Decimal.zero) l.saleCreditBadge,
    ].join(', ');
    return ExpansionTile(
      key: PageStorageKey(r.id),
      onExpansionChanged: (open) {
        if (open && _detail == null) {
          final f = ref.read(posRepositoryProvider).saleDetail(r.id);
          setState(() {
            _detail = f;
          });
        }
      },
      title: Row(
        children: [
          Expanded(
            child: Text(
              r.docNo,
              style: TextStyle(
                fontWeight: FontWeight.w700,
                decoration: r.isVoid ? TextDecoration.lineThrough : null,
              ),
            ),
          ),
          Text(
            formatMoney(r.total),
            style: TextStyle(
              fontWeight: FontWeight.w800,
              color: r.isVoid ? pal.textTertiary : null,
            ),
          ),
        ],
      ),
      subtitle: Wrap(
        spacing: 8,
        children: [
          Text(
            DateFormat('dd/MM HH:mm').format(r.createdAt),
            style: TextStyle(color: pal.textTertiary, fontSize: 12),
          ),
          if (pays.isNotEmpty)
            Text(pays, style: TextStyle(color: pal.textTertiary, fontSize: 12)),
          if (r.isVoid)
            Text(
              l.saleVoidBadge,
              style: TextStyle(
                color: pal.danger,
                fontSize: 12,
                fontWeight: FontWeight.w800,
              ),
            ),
          if (r.returned > Decimal.zero)
            Text(
              l.saleReturnedBadge(formatMoney(r.returned)),
              style: TextStyle(color: pal.warningText, fontSize: 12),
            ),
        ],
      ),
      childrenPadding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
      expandedCrossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (_detail != null)
          FutureBuilder<SaleDetailInfo>(
            future: _detail,
            builder: (context, snap) {
              if (snap.hasError) {
                final e = snap.error;
                return Text(
                  e is ApiError ? e.message(l) : l.errorUnknown,
                  style: TextStyle(color: pal.danger),
                );
              }
              if (!snap.hasData) {
                return const Padding(
                  padding: EdgeInsets.all(8),
                  child: LinearProgressIndicator(minHeight: 2),
                );
              }
              return _DetailBody(d: snap.data!, saleId: r.id);
            },
          ),
      ],
    );
  }
}

class _DetailBody extends StatelessWidget {
  const _DetailBody({required this.d, required this.saleId});

  final SaleDetailInfo d;
  final String saleId;

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    Widget kv(String k, Decimal v, {bool bold = false}) => Padding(
      padding: const EdgeInsets.symmetric(vertical: 1),
      child: Row(
        children: [
          Expanded(
            child: Text(
              k,
              style: TextStyle(fontWeight: bold ? FontWeight.w800 : null),
            ),
          ),
          Text(
            formatMoney(v),
            style: TextStyle(fontWeight: bold ? FontWeight.w800 : null),
          ),
        ],
      ),
    );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          [
            l.saleDetailCashier(d.cashier),
            if (d.member.isNotEmpty) l.saleDetailMember(d.member),
          ].join(' · '),
          style: TextStyle(color: pal.textTertiary, fontSize: 12.5),
        ),
        if (d.voidReason.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 4),
            child: Text(
              l.saleVoidReason(d.voidReason),
              style: TextStyle(color: pal.danger, fontSize: 12.5),
            ),
          ),
        const SizedBox(height: 8),
        for (final ln in d.lines)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 3),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        ln.name,
                        style: const TextStyle(fontWeight: FontWeight.w600),
                      ),
                      Text(
                        '${formatQty(ln.qty)} ${ln.unit} × ${formatMoney(ln.unitPrice)}'
                        '${ln.discount > Decimal.zero ? ' − ${formatMoney(ln.discount)}' : ''}',
                        style: TextStyle(color: pal.textTertiary, fontSize: 12),
                      ),
                    ],
                  ),
                ),
                Text(
                  formatMoney(ln.lineTotal),
                  style: const TextStyle(fontWeight: FontWeight.w600),
                ),
              ],
            ),
          ),
        const Divider(height: 18),
        kv(l.saleSubtotal, d.subtotal),
        if (d.discount > Decimal.zero) kv(l.saleDiscount, d.discount),
        if (d.tax > Decimal.zero) kv(l.saleTax, d.tax),
        if (d.otherCost > Decimal.zero) kv(l.saleOtherCost, d.otherCost),
        kv(l.saleTotal, d.total, bold: true),
        const SizedBox(height: 6),
        for (final p in d.payments) kv(p.name, p.amount),
        if (d.change > Decimal.zero) kv(l.saleChange, d.change),
        if (d.receivable > Decimal.zero) kv(l.saleReceivable, d.receivable),
        const SizedBox(height: 10),
        Center(child: PrintReceiptButton(saleId: saleId, reprint: true)),
      ],
    );
  }
}
