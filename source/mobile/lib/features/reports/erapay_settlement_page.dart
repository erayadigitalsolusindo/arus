import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import '../pos/pos_repository.dart';
import 'erapay_settlement_models.dart';

/// Settlement EraPay (izin `sales_reports.view`): uang menurut ARUS vs uang menurut EraPay per transaksi QRIS, lengkap
/// dengan status dan ID transaksi EraPay. Hanya baca; "Cek ke EraPay" menanyakan ulang status tagihan yang masih menunggu.
Future<void> showEraSettlementPage(BuildContext context) => Navigator.of(
  context,
).push<void>(MaterialPageRoute(builder: (_) => const EraSettlementPage()));

enum _Preset { today, week, month, custom }

enum _Filter { all, attention, waiting, matched }

class EraSettlementPage extends ConsumerStatefulWidget {
  const EraSettlementPage({super.key});

  @override
  ConsumerState<EraSettlementPage> createState() => _EraSettlementPageState();
}

class _EraSettlementPageState extends ConsumerState<EraSettlementPage> {
  static final _api = DateFormat('yyyy-MM-dd');

  _Preset _preset = _Preset.today;
  DateTimeRange _range = _rangeOf(_Preset.today);
  _Filter _filter = _Filter.all;
  bool _all = false;
  bool _loading = true;
  ApiError? _error;
  EraSettlementReport? _data;
  int _seq = 0;
  String? _notice;

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
      case _Preset.today:
      case _Preset.custom:
        return DateTimeRange(start: today, end: today);
    }
  }

  Future<void> _load({bool refresh = false}) async {
    final my = ++_seq;
    setState(() {
      _loading = true;
      _error = null;
      if (!refresh) _notice = null;
    });
    try {
      final r = await ref
          .read(posRepositoryProvider)
          .eraSettlement(
            from: _api.format(_range.start),
            to: _api.format(_range.end),
            allOutlets: _all,
            refresh: refresh,
          );
      if (my == _seq && mounted) {
        final l = AppLocalizations.of(context);
        setState(() {
          _data = r;
          if (refresh) _notice = l.setRefreshed('${r.refreshed}');
        });
      }
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
      firstDate: DateTime(now.year - 1),
      lastDate: now,
    );
    if (r == null || !mounted) return;
    setState(() {
      _preset = _Preset.custom;
      _range = r;
    });
    _load();
  }

  bool _show(SettlementRow r) => switch (_filter) {
    _Filter.all => true,
    _Filter.attention => r.needsAttention,
    _Filter.waiting => r.state == 'waiting',
    _Filter.matched => r.state == 'matched',
  };

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final d = _data;
    final fmt = DateFormat('dd MMM yyyy', l.localeName);
    final label = _range.start == _range.end
        ? fmt.format(_range.start)
        : '${fmt.format(_range.start)} - ${fmt.format(_range.end)}';
    final rows = d == null
        ? const <SettlementRow>[]
        : d.rows.where(_show).toList();
    return Scaffold(
      appBar: AppBar(
        title: Text(l.setTitle),
        actions: [
          IconButton(
            tooltip: l.setCheck,
            icon: _loading
                ? const SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.sync),
            onPressed: _loading ? null : () => _load(refresh: true),
          ),
        ],
      ),
      body: SafeArea(
        child: RefreshIndicator(
          onRefresh: () => _load(refresh: true),
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
                    style: const TextStyle(fontSize: 14),
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
              if (_notice != null)
                Padding(
                  padding: const EdgeInsets.only(bottom: 6),
                  child: Text(
                    _notice!,
                    style: TextStyle(color: pal.success, fontSize: 13.5),
                  ),
                ),
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
              else if (!d.configured)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 32),
                  child: Text(
                    l.setNotConfigured,
                    textAlign: TextAlign.center,
                    style: TextStyle(color: pal.textTertiary, fontSize: 15),
                  ),
                )
              else ...[
                _Summary(report: d),
                const SizedBox(height: 12),
                SingleChildScrollView(
                  scrollDirection: Axis.horizontal,
                  child: Row(
                    children: [
                      for (final e in {
                        _Filter.all: '${l.setFilterAll} (${d.count})',
                        _Filter.attention:
                            '${l.setFilterAttention} (${d.attention})',
                        _Filter.waiting: '${l.setFilterWaiting} (${d.waiting})',
                        _Filter.matched: '${l.setFilterMatched} (${d.matched})',
                      }.entries)
                        Padding(
                          padding: const EdgeInsets.only(right: 8),
                          child: ChoiceChip(
                            label: Text(e.value),
                            selected: _filter == e.key,
                            onSelected: (_) => setState(() => _filter = e.key),
                          ),
                        ),
                    ],
                  ),
                ),
                const SizedBox(height: 8),
                if (rows.isEmpty)
                  Padding(
                    padding: const EdgeInsets.symmetric(vertical: 32),
                    child: Text(
                      l.setEmpty,
                      textAlign: TextAlign.center,
                      style: TextStyle(color: pal.textTertiary, fontSize: 15),
                    ),
                  )
                else
                  for (final r in rows) _RowCard(row: r),
                if (d.truncated)
                  Padding(
                    padding: const EdgeInsets.only(top: 8),
                    child: Text(
                      l.setTruncated,
                      style: TextStyle(color: pal.warningText, fontSize: 13.5),
                    ),
                  ),
                const SizedBox(height: 12),
                Text(
                  l.setNote,
                  style: TextStyle(fontSize: 12.5, color: pal.textTertiary),
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

class _Summary extends StatelessWidget {
  const _Summary({required this.report});

  final EraSettlementReport report;

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    Widget box(String title, Decimal v, String hint, Color color) => Expanded(
      child: Container(
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(
          color: color.withValues(alpha: 0.10),
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: color.withValues(alpha: 0.35)),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              title,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w800,
                letterSpacing: 0.4,
                color: color,
              ),
            ),
            const SizedBox(height: 4),
            FittedBox(
              fit: BoxFit.scaleDown,
              alignment: Alignment.centerLeft,
              child: Text(
                formatMoney(v),
                style: const TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                ),
              ),
            ),
            const SizedBox(height: 2),
            Text(hint, style: TextStyle(fontSize: 12, color: pal.textTertiary)),
          ],
        ),
      ),
    );
    final diff = report.diff;
    final zero = diff == Decimal.zero;
    final money = formatMoney(diff < Decimal.zero ? -diff : diff);
    final msg = zero
        ? l.setMatchedAll
        : diff > Decimal.zero
        ? l.setDiffMore(money)
        : l.setDiffLess(money);
    final color = zero ? pal.success : pal.warningText;
    return Column(
      children: [
        Row(
          children: [
            box(l.setArus, report.arus, l.setArusHint, pal.primary),
            const SizedBox(width: 10),
            box(l.setEra, report.era, l.setEraHint, pal.accent),
          ],
        ),
        const SizedBox(height: 10),
        Container(
          width: double.infinity,
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(
            color: color.withValues(alpha: 0.12),
            borderRadius: BorderRadius.circular(12),
          ),
          child: Row(
            children: [
              Icon(
                zero ? Icons.check_circle_outline : Icons.error_outline,
                color: color,
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  msg,
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: color,
                  ),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _RowCard extends StatelessWidget {
  const _RowCard({required this.row});

  final SettlementRow row;

  ({String title, String sub, Color color, IconData icon}) _state(
    AppLocalizations l,
    AppPalette pal,
  ) => switch (row.state) {
    'matched' => (
      title: l.setStateMatched,
      sub: l.setStateMatchedSub,
      color: pal.success,
      icon: Icons.check_circle,
    ),
    'waiting' => (
      title: l.setStateWaiting,
      sub: l.setStateWaitingSub,
      color: pal.primary,
      icon: Icons.schedule,
    ),
    'orphan' => (
      title: l.setStateOrphan,
      sub: l.setStateOrphanSub,
      color: pal.warningText,
      icon: Icons.help_outline,
    ),
    'refund' => (
      title: l.setStateRefund,
      sub: l.setStateRefundSub,
      color: pal.danger,
      icon: Icons.undo,
    ),
    'unpaid' => (
      title: l.setStateUnpaid,
      sub: l.setStateUnpaidSub,
      color: pal.danger,
      icon: Icons.cancel_outlined,
    ),
    _ => (
      title: l.setStateClosed,
      sub: l.setStateClosedSub,
      color: pal.textTertiary,
      icon: Icons.remove_circle_outline,
    ),
  };

  String _eraStatus(AppLocalizations l) => switch (row.status) {
    'succeeded' => l.setEraSucceeded,
    'pending' => l.setEraPending,
    'failed' => l.setEraFailed,
    'expired' => l.setEraExpired,
    'cancelled' => l.setEraCancelled,
    _ => l.setEraUnknown,
  };

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final st = _state(l, pal);
    Widget money(String col, Decimal v, {Color? color}) => Expanded(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            col,
            style: TextStyle(
              fontSize: 11.5,
              fontWeight: FontWeight.w800,
              letterSpacing: 0.4,
              color: pal.textTertiary,
            ),
          ),
          FittedBox(
            fit: BoxFit.scaleDown,
            alignment: Alignment.centerLeft,
            child: Text(
              formatMoney(v),
              style: TextStyle(
                fontSize: 17,
                fontWeight: FontWeight.w800,
                color: color,
              ),
            ),
          ),
        ],
      ),
    );
    // Garis aksen kiri berwarna status: border berwarna tidak seragam tak bisa dipadu radius, jadi memakai Row + klip.
    return Container(
      margin: const EdgeInsets.only(top: 8),
      clipBehavior: Clip.antiAlias,
      decoration: BoxDecoration(
        color: pal.surface,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: pal.border),
      ),
      child: IntrinsicHeight(
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Container(width: 4, color: st.color),
            Expanded(
              child: Padding(
                padding: const EdgeInsets.all(12),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Icon(st.icon, size: 20, color: st.color),
                        const SizedBox(width: 8),
                        Expanded(
                          child: Text(
                            st.title,
                            style: TextStyle(
                              fontSize: 16,
                              fontWeight: FontWeight.w800,
                              color: st.color,
                            ),
                          ),
                        ),
                        Text(
                          DateFormat('dd/MM HH:mm').format(row.createdAt),
                          style: TextStyle(
                            fontSize: 13,
                            color: pal.textTertiary,
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 2),
                    Text(
                      st.sub,
                      style: TextStyle(fontSize: 13, color: pal.textTertiary),
                    ),
                    const SizedBox(height: 10),
                    Row(
                      children: [
                        money(l.setColArus, row.arusAmount),
                        money(l.setColEra, row.eraAmount, color: pal.accent),
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                l.setStatusLabel.toUpperCase(),
                                style: TextStyle(
                                  fontSize: 11.5,
                                  fontWeight: FontWeight.w800,
                                  letterSpacing: 0.4,
                                  color: pal.textTertiary,
                                ),
                              ),
                              Text(
                                _eraStatus(l),
                                style: const TextStyle(
                                  fontSize: 15,
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                            ],
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 10),
                    Wrap(
                      spacing: 12,
                      runSpacing: 2,
                      children: [
                        if (row.docNo.isNotEmpty)
                          _Meta(Icons.receipt_long_outlined, row.docNo),
                        if (row.cashier.isNotEmpty)
                          _Meta(Icons.person_outline, row.cashier),
                        if (row.outlet.isNotEmpty)
                          _Meta(Icons.storefront_outlined, row.outlet),
                        if (row.gateway.isNotEmpty)
                          _Meta(Icons.hub_outlined, row.gateway.toUpperCase()),
                      ],
                    ),
                    const SizedBox(height: 6),
                    InkWell(
                      borderRadius: BorderRadius.circular(6),
                      onTap: row.paymentId.isEmpty
                          ? null
                          : () async {
                              await Clipboard.setData(
                                ClipboardData(text: row.paymentId),
                              );
                              if (!context.mounted) return;
                              ScaffoldMessenger.of(context)
                                ..clearSnackBars()
                                ..showSnackBar(
                                  SnackBar(content: Text(l.setIdCopied)),
                                );
                            },
                      child: Padding(
                        padding: const EdgeInsets.symmetric(vertical: 2),
                        child: Row(
                          children: [
                            Text(
                              '${l.setIdLabel}: ',
                              style: TextStyle(
                                fontSize: 12.5,
                                color: pal.textTertiary,
                              ),
                            ),
                            Expanded(
                              child: Text(
                                row.paymentId.isEmpty
                                    ? l.setNoId
                                    : row.paymentId,
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: const TextStyle(
                                  fontSize: 12.5,
                                  fontFamily: 'monospace',
                                  fontWeight: FontWeight.w600,
                                ),
                              ),
                            ),
                            if (row.paymentId.isNotEmpty)
                              Icon(
                                Icons.copy,
                                size: 14,
                                color: pal.textTertiary,
                              ),
                          ],
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Meta extends StatelessWidget {
  const _Meta(this.icon, this.text);

  final IconData icon;
  final String text;

  @override
  Widget build(BuildContext context) {
    final pal = context.pal;
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 14, color: pal.textTertiary),
        const SizedBox(width: 4),
        Text(text, style: TextStyle(fontSize: 13, color: pal.textTertiary)),
      ],
    );
  }
}
