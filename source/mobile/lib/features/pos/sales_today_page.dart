import 'dart:async';

import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'pos_repository.dart';
import 'sales_models.dart';

/// Penjualan kasir ini hari ini: daftar nota, total per metode, dan detail nota (hanya baca; edit/batal tetap di web).
Future<void> showSalesTodayPage(BuildContext context) =>
    Navigator.of(context)
        .push<void>(MaterialPageRoute(builder: (_) => const _SalesTodayPage()));

class _SalesTodayPage extends ConsumerStatefulWidget {
  const _SalesTodayPage();

  @override
  ConsumerState<_SalesTodayPage> createState() => _SalesTodayPageState();
}

class _SalesTodayPageState extends ConsumerState<_SalesTodayPage> {
  final _search = TextEditingController();
  Timer? _debounce;
  int _seq = 0;
  SalesToday? _data;
  bool _loading = true;
  ApiError? _error;

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

  Future<void> _load() async {
    final my = ++_seq;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final r = await ref
          .read(posRepositoryProvider)
          .salesToday(q: _search.text.trim());
      if (my != _seq || !mounted) return;
      setState(() => _data = r);
    } on ApiError catch (e) {
      if (my == _seq && mounted) setState(() => _error = e);
    } finally {
      if (my == _seq && mounted) setState(() => _loading = false);
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
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
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
    final active = data.rows.where((r) => !r.isVoid).length;
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
            l.salesTodaySummary('$active', formatMoney(data.total)),
            style: TextStyle(fontWeight: FontWeight.w800, color: pal.primary),
          ),
          if (data.byMethod.isNotEmpty) ...[
            const SizedBox(height: 6),
            Wrap(
              spacing: 14,
              runSpacing: 2,
              children: [
                for (final m in data.byMethod)
                  Text(
                    '${m.name}: ${formatMoney(m.amount)}',
                    style: const TextStyle(fontSize: 12.5),
                  ),
              ],
            ),
          ],
        ],
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
            DateFormat('HH:mm').format(r.createdAt),
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
              return _DetailBody(d: snap.data!);
            },
          ),
      ],
    );
  }
}

class _DetailBody extends StatelessWidget {
  const _DetailBody({required this.d});

  final SaleDetailInfo d;

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
      ],
    );
  }
}
