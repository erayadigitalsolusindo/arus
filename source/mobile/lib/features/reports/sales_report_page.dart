import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import '../pos/pos_repository.dart';
import 'sales_report_models.dart';

/// Laporan penjualan ringkas untuk admin (izin `sales_reports.view`): omzet, nota, retur, per metode bayar, dan per hari.
/// Hanya baca; laporan lengkap, ekspor, dan filter lain tetap di web.
Future<void> showSalesReportPage(BuildContext context) =>
    Navigator.of(context)
        .push<void>(MaterialPageRoute(builder: (_) => const SalesReportPage()));

enum _Preset { today, week, month, lastMonth, custom }

class SalesReportPage extends ConsumerStatefulWidget {
  const SalesReportPage({super.key});

  @override
  ConsumerState<SalesReportPage> createState() => _SalesReportPageState();
}

class _SalesReportPageState extends ConsumerState<SalesReportPage> {
  static final _api = DateFormat('yyyy-MM-dd');

  _Preset _preset = _Preset.today;
  late DateTimeRange _range = _rangeOf(_Preset.today);
  bool _all = false;
  bool _loading = true;
  ApiError? _error;
  SalesReport? _data;
  int _seq = 0;

  @override
  void initState() {
    super.initState();
    _load();
  }

  static DateTimeRange _rangeOf(_Preset p) {
    final n = DateTime.now();
    final today = DateTime(n.year, n.month, n.day);
    switch (p) {
      case _Preset.week:
        return DateTimeRange(
          start: today.subtract(const Duration(days: 6)),
          end: today,
        );
      case _Preset.month:
        return DateTimeRange(start: DateTime(n.year, n.month), end: today);
      case _Preset.lastMonth:
        return DateTimeRange(
          start: DateTime(n.year, n.month - 1),
          end: DateTime(n.year, n.month, 0),
        );
      case _Preset.today:
      case _Preset.custom:
        return DateTimeRange(start: today, end: today);
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
          .read(posRepositoryProvider)
          .salesReport(
            from: _api.format(_range.start),
            to: _api.format(_range.end),
            allOutlets: _all,
          );
      if (my == _seq && mounted) setState(() => _data = r);
    } on ApiError catch (e) {
      if (my == _seq && mounted) setState(() => _error = e);
    } finally {
      if (my == _seq && mounted) setState(() => _loading = false);
    }
  }

  void _choose(_Preset p) {
    setState(() {
      _preset = p;
      _range = _rangeOf(p);
    });
    _load();
  }

  Future<void> _pickRange() async {
    final now = DateTime.now();
    final r = await showDateRangePicker(
      context: context,
      initialDateRange: _range,
      firstDate: DateTime(now.year - 3),
      lastDate: now,
    );
    if (r == null || !mounted) return;
    setState(() {
      _preset = _Preset.custom;
      _range = r;
    });
    _load();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final d = _data;
    final fmt = DateFormat('dd MMM yyyy', l.localeName);
    final label = _range.start == _range.end
        ? fmt.format(_range.start)
        : '${fmt.format(_range.start)} - ${fmt.format(_range.end)}';
    return Scaffold(
      appBar: AppBar(
        title: Text(l.reportTitle),
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
        child: RefreshIndicator(
          onRefresh: _load,
          child: ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
            children: [
              SingleChildScrollView(
                scrollDirection: Axis.horizontal,
                child: Row(
                  children: [
                    for (final e in {
                      _Preset.today: l.reportToday,
                      _Preset.week: l.report7Days,
                      _Preset.month: l.reportThisMonth,
                      _Preset.lastMonth: l.reportLastMonth,
                    }.entries)
                      Padding(
                        padding: const EdgeInsets.only(right: 8),
                        child: ChoiceChip(
                          label: Text(e.value),
                          selected: _preset == e.key,
                          onSelected: (_) => _choose(e.key),
                        ),
                      ),
                    ChoiceChip(
                      avatar: const Icon(Icons.date_range, size: 18),
                      label: Text(l.reportCustom),
                      selected: _preset == _Preset.custom,
                      onSelected: (_) => _pickRange(),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 6),
              Row(
                children: [
                  Expanded(
                    child: Text(
                      label,
                      style: TextStyle(
                        color: pal.textTertiary,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ),
                  Text(
                    l.reportAllOutlets,
                    style: const TextStyle(fontSize: 13),
                  ),
                  Switch(
                    value: _all,
                    onChanged: (v) {
                      setState(() => _all = v);
                      _load();
                    },
                  ),
                ],
              ),
              if (_loading && d != null)
                const LinearProgressIndicator(minHeight: 2),
              if (_error != null)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 24),
                  child: Text(
                    _error!.message(l),
                    textAlign: TextAlign.center,
                    style: TextStyle(color: pal.danger),
                  ),
                )
              else if (d == null)
                const Padding(
                  padding: EdgeInsets.symmetric(vertical: 48),
                  child: Center(child: CircularProgressIndicator()),
                )
              else if (d.days.isEmpty)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 48),
                  child: Text(
                    l.reportEmpty,
                    textAlign: TextAlign.center,
                    style: TextStyle(color: pal.textTertiary),
                  ),
                )
              else ...[
                _Totals(report: d),
                const SizedBox(height: 16),
                _Section(
                  title: l.reportByMethod,
                  child: _Bars(
                    items: [
                      for (final m in d.byMethod)
                        _BarItem(m.name, m.amount, formatMoney(m.amount)),
                    ],
                  ),
                ),
                const SizedBox(height: 16),
                _Section(
                  title: l.reportByDay,
                  child: _Bars(
                    items: [
                      for (final day in d.days.reversed)
                        _BarItem(
                          DateFormat(
                            'EEE dd/MM',
                            l.localeName,
                          ).format(DateTime.parse(day.date)),
                          day.total,
                          formatMoney(day.total),
                          sub: l.reportDayNotes('${day.saleCount}'),
                        ),
                    ],
                  ),
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

class _Totals extends StatelessWidget {
  const _Totals({required this.report});

  final SalesReport report;

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    Widget stat(String k, String v, {Color? color}) => Expanded(
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: pal.sunken,
          borderRadius: BorderRadius.circular(12),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(k, style: TextStyle(fontSize: 12, color: pal.textTertiary)),
            const SizedBox(height: 2),
            FittedBox(
              fit: BoxFit.scaleDown,
              child: Text(
                v,
                style: TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w800,
                  color: color,
                ),
              ),
            ),
          ],
        ),
      ),
    );
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: pal.primarySoft,
        borderRadius: BorderRadius.circular(16),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            l.reportRevenue,
            style: TextStyle(color: pal.primary, fontWeight: FontWeight.w700),
          ),
          FittedBox(
            fit: BoxFit.scaleDown,
            alignment: Alignment.centerLeft,
            child: Text(
              formatMoney(report.total),
              style: TextStyle(
                fontSize: 32,
                fontWeight: FontWeight.w800,
                color: pal.primary,
              ),
            ),
          ),
          const SizedBox(height: 10),
          Row(
            children: [
              stat(l.reportNotes, '${report.saleCount}'),
              const SizedBox(width: 8),
              stat(
                l.reportVoid,
                '${report.voidCount}',
                color: report.voidCount > 0 ? pal.danger : null,
              ),
              const SizedBox(width: 8),
              stat(l.reportReturns, '${report.returnCount}'),
            ],
          ),
          const SizedBox(height: 8),
          Row(
            children: [
              stat(
                l.reportNet,
                formatMoney(report.netTotal),
                color: pal.success,
              ),
              const SizedBox(width: 8),
              stat(
                l.reportReceivable,
                formatMoney(report.receivable),
                color: report.receivable > Decimal.zero
                    ? pal.warningText
                    : null,
              ),
            ],
          ),
        ],
      ),
    );
  }
}

class _Section extends StatelessWidget {
  const _Section({required this.title, required this.child});

  final String title;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final pal = context.pal;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          title,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w700,
            color: pal.textTertiary,
          ),
        ),
        const SizedBox(height: 8),
        child,
      ],
    );
  }
}

class _BarItem {
  const _BarItem(this.label, this.value, this.text, {this.sub});

  final String label;
  final Decimal value;
  final String text;
  final String? sub;
}

/// Daftar bergaris proporsional terhadap nilai terbesar (grafik batang sederhana tanpa pustaka).
class _Bars extends StatelessWidget {
  const _Bars({required this.items});

  final List<_BarItem> items;

  @override
  Widget build(BuildContext context) {
    final pal = context.pal;
    var max = Decimal.zero;
    for (final i in items) {
      if (i.value > max) max = i.value;
    }
    return Column(
      children: [
        for (final i in items)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 5),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        i.sub == null ? i.label : '${i.label} · ${i.sub}',
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    Text(
                      i.text,
                      style: const TextStyle(fontWeight: FontWeight.w700),
                    ),
                  ],
                ),
                const SizedBox(height: 4),
                ClipRRect(
                  borderRadius: BorderRadius.circular(4),
                  child: LinearProgressIndicator(
                    minHeight: 6,
                    value: max > Decimal.zero
                        ? (i.value / max).toDouble().clamp(0.0, 1.0)
                        : 0,
                    backgroundColor: pal.sunken,
                    color: pal.primary,
                  ),
                ),
              ],
            ),
          ),
      ],
    );
  }
}
