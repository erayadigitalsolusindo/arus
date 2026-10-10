import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:uuid/uuid.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'approver_fields.dart';
import 'pos_controller.dart';
import 'pos_models.dart';
import 'pos_repository.dart';
import 'shift_dialog.dart';
import 'shift_models.dart';

/// Layar bayar: satu atau beberapa baris (metode + jumlah) → split bayar. Server menentukan total, kembalian,
/// dan menegakkan aturan (non-tunai tak boleh melebihi total, shift wajib). Hasil: nota tersimpan atau null (ditutup).
Future<SaleResult?> showPaySheet(BuildContext context) =>
    showModalBottomSheet<SaleResult>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => const _PaySheet(),
    );

class _PayRow {
  _PayRow({required this.method, String amount = ''})
    : amount = TextEditingController(text: amount);

  PayMethodInfo method;
  final TextEditingController amount;
  final TextEditingController ref = TextEditingController();

  void dispose() {
    amount.dispose();
    ref.dispose();
  }
}

class _PaySheet extends ConsumerStatefulWidget {
  const _PaySheet();

  @override
  ConsumerState<_PaySheet> createState() => _PaySheetState();
}

class _PaySheetState extends ConsumerState<_PaySheet> {
  final List<_PayRow> _rows = [];
  // Kunci idempotensi dipakai ulang saat percobaan diulang (sinyal buruk) selama isi bayar tidak berubah.
  String? _key;
  bool _busy = false;
  ApiError? _error;

  // Kredit: sisa yang belum dibayar menjadi piutang member; baris bayar = DP (boleh kosong).
  bool _credit = false;
  bool _serverOver =
      false; // server menolak karena limit walau pratinjau belum tahu
  Approver? _creditApprover;
  String _creditPin = '';
  final _creditKey = GlobalKey<ApproverFieldsState>();

  @override
  void dispose() {
    for (final r in _rows) {
      r.dispose();
    }
    super.dispose();
  }

  Decimal get _total => ref.read(cartProvider).quote?.total ?? Decimal.zero;

  Decimal get _paid =>
      _rows.fold(Decimal.zero, (a, r) => a + parseInput(r.amount.text));

  Decimal get _cashPaid => _rows
      .where((r) => r.method.isCash)
      .fold(Decimal.zero, (a, r) => a + parseInput(r.amount.text));

  /// Kembalian hanya dari tunai: kelebihan bayar total (semua metode) dikurangi bagian non-tunai tidak boleh > total.
  Decimal get _change {
    final over = _paid - _total;
    if (over <= Decimal.zero) return Decimal.zero;
    return over <= _cashPaid ? over : _cashPaid;
  }

  /// Biaya metode yang ditagihkan ke pelanggan (di luar total nota). Pratinjau; server yang menentukan.
  Decimal get _surcharge => _rows.fold(
    Decimal.zero,
    (a, r) => r.method.feeByCustomer
        ? a + r.method.feeOf(parseInput(r.amount.text))
        : a,
  );

  /// Sisa yang harus dibayar baris [r] (total dikurangi baris lain); dasar uang pas dan nominal cepat.
  Decimal _needFor(_PayRow r) {
    final others = _rows
        .where((x) => x != r)
        .fold(Decimal.zero, (a, x) => a + parseInput(x.amount.text));
    final need = _total - others;
    return need > Decimal.zero ? need : Decimal.zero;
  }

  Decimal get _short => _total > _paid ? _total - _paid : Decimal.zero;

  void _edited() {
    _key = null; // isi berubah → percobaan baru, kunci baru
    setState(() {});
  }

  void _setCredit(bool on) {
    if (_credit == on) return;
    for (final r in _rows) {
      r.dispose();
    }
    _rows.clear();
    _credit = on;
    _serverOver = false;
    _error = null;
    _edited();
  }

  Decimal get _receivable =>
      _credit && _paid < _total ? _total - _paid : Decimal.zero;

  /// Pratinjau; server yang menentukan dan memeriksa limit.
  bool get _overLimit {
    final q = ref.read(cartProvider).quote;
    final c = q?.member == null ? null : q?.credit;
    final r = _receivable;
    if (!_credit || r <= Decimal.zero) return false;
    final preview =
        c != null && c.limit > Decimal.zero && c.outstanding + r > c.limit;
    return preview || _serverOver;
  }

  /// Penyetuju limit kredit hanya diminta bila belum ada penyetuju ubah harga/potongan (dipakai bersama, seperti web).
  bool get _needCreditApproval =>
      _overLimit && ref.read(cartProvider).draft(forPay: true).approval == null;

  bool get _creditApprovalOk =>
      !_needCreditApproval ||
      (_creditApprover != null && RegExp(r'^\d{6}$').hasMatch(_creditPin));

  void _ensureDefaults(List<PayMethodInfo> methods) {
    if (_credit || _rows.isNotEmpty || methods.isEmpty) return;
    final cash = methods.firstWhere(
      (m) => m.isCash,
      orElse: () => methods.first,
    );
    _rows.add(_PayRow(method: cash, amount: decToApi(_total, scale: 2)));
  }

  Future<void> _submit() async {
    final cart = ref.read(cartProvider);
    setState(() {
      _busy = true;
      _error = null;
    });
    final payments = [
      for (final r in _rows)
        if (parseInput(r.amount.text) > Decimal.zero)
          PaymentInput(
            methodId: r.method.id,
            amount: parseInput(r.amount.text),
            refNo: r.ref.text.trim(),
          ),
    ];
    _key ??= const Uuid().v4();
    try {
      SaleResult? result;
      for (var attempt = 0; attempt < 2 && result == null; attempt++) {
        try {
          result = await ref
              .read(posRepositoryProvider)
              .createSale(
                _credit
                    ? cart
                          .draft(forPay: true)
                          .asCredit(
                            _needCreditApproval
                                ? ApprovalGrant(
                                    userId: _creditApprover!.id,
                                    name: _creditApprover!.name,
                                    pin: _creditPin,
                                  )
                                : cart.draft(forPay: true).approval,
                          )
                    : cart.draft(forPay: true),
                payments: payments,
                idempotencyKey: _key!,
              );
        } on ApiError catch (e) {
          // Belum ada shift: tawarkan buka shift lalu ulangi sekali dengan kunci yang sama.
          if (e.code == 'SHIFT_REQUIRED' &&
              attempt == 0 &&
              mounted &&
              await showOpenShiftDialog(context)) {
            continue;
          }
          rethrow;
        }
      }
      if (result == null || !mounted) return;
      ref.read(cartProvider.notifier).clear();
      Navigator.of(context).pop(result);
    } on ApiError catch (e) {
      if (mounted) {
        setState(() {
          _error = e;
          // Melewati limit: minta persetujuan penyetuju; PIN yang ditolak dikosongkan.
          if (e.code == 'CREDIT_LIMIT_EXCEEDED') _serverOver = true;
          if (e.code == 'INVALID_PIN' || e.code == 'CREDIT_LIMIT_EXCEEDED') {
            _creditPin = '';
            _creditKey.currentState?.clearPin();
          }
        });
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final methodsAsync = ref.watch(paymentMethodsProvider);
    final quote = ref.watch(cartProvider.select((c) => c.quote));
    final total = quote?.total ?? Decimal.zero;

    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      child: methodsAsync.when(
        loading: () => const SizedBox(
          height: 220,
          child: Center(child: CircularProgressIndicator()),
        ),
        error: (e, _) => SizedBox(
          height: 220,
          child: Center(
            child: Text(e is ApiError ? e.message(l) : l.errorUnknown),
          ),
        ),
        data: (allMethods) {
          // Deposit hanya bisa dipakai bila nota punya member.
          final hasMember = quote?.member != null;
          final methods = [
            for (final m in allMethods)
              if (!m.isDeposit || hasMember) m,
          ];
          _ensureDefaults(methods);
          final deposit = quote?.member?.deposit ?? Decimal.zero;
          final depositUsed = _rows
              .where((r) => r.method.isDeposit)
              .fold(Decimal.zero, (a, r) => a + parseInput(r.amount.text));
          final depositOver = depositUsed > deposit;
          final enough = _credit
              ? hasMember &&
                    _receivable > Decimal.zero &&
                    _creditApprovalOk &&
                    !depositOver
              : _paid >= total &&
                    total > Decimal.zero &&
                    _rows.isNotEmpty &&
                    !depositOver;
          return Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Flexible(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.fromLTRB(20, 0, 20, 12),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Text(
                        l.payTitle,
                        style: const TextStyle(
                          fontSize: 18,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      const SizedBox(height: 12),
                      Container(
                        padding: const EdgeInsets.all(14),
                        decoration: BoxDecoration(
                          color: pal.primarySoft,
                          borderRadius: BorderRadius.circular(12),
                        ),
                        child: Row(
                          mainAxisAlignment: MainAxisAlignment.spaceBetween,
                          children: [
                            Flexible(
                              child: Text(
                                l.payTotalDue,
                                style: TextStyle(color: pal.textTertiary),
                              ),
                            ),
                            const SizedBox(width: 8),
                            Flexible(
                              child: FittedBox(
                                fit: BoxFit.scaleDown,
                                child: Text(
                                  formatMoney(total),
                                  style: const TextStyle(
                                    fontSize: 22,
                                    fontWeight: FontWeight.w800,
                                  ),
                                ),
                              ),
                            ),
                          ],
                        ),
                      ),
                      if (quote?.member != null) ...[
                        const SizedBox(height: 8),
                        Text(
                          '${ref.watch(cartProvider.select((c) => c.member?.name)) ?? ''} · ${l.memberPointsAndDeposit(quote!.member!.points, formatMoney(deposit))}',
                          style: TextStyle(
                            color: pal.textTertiary,
                            fontSize: 12.5,
                          ),
                        ),
                      ],
                      const SizedBox(height: 12),
                      SegmentedButton<bool>(
                        segments: [
                          ButtonSegment(
                            value: false,
                            label: Text(l.payModePay),
                          ),
                          ButtonSegment(
                            value: true,
                            enabled: hasMember,
                            label: Text(l.payModeCredit),
                          ),
                        ],
                        selected: {_credit},
                        onSelectionChanged: _busy
                            ? null
                            : (s) => _setCredit(s.first),
                      ),
                      if (!hasMember)
                        Padding(
                          padding: const EdgeInsets.only(top: 6),
                          child: Text(
                            l.payCreditNoMember,
                            style: TextStyle(
                              color: pal.textTertiary,
                              fontSize: 12.5,
                            ),
                          ),
                        ),
                      if (_credit) ..._creditPanel(context, quote),
                      const SizedBox(height: 14),
                      for (var i = 0; i < _rows.length; i++) ...[
                        _row(context, i, methods),
                        const SizedBox(height: 10),
                      ],
                      if (_rows.length < methods.length)
                        Align(
                          alignment: Alignment.centerLeft,
                          child: TextButton.icon(
                            onPressed: _busy
                                ? null
                                : () {
                                    final used = _rows
                                        .map((r) => r.method.id)
                                        .toSet();
                                    final next = methods.firstWhere(
                                      (m) => !used.contains(m.id),
                                      orElse: () => methods.first,
                                    );
                                    _rows.add(
                                      _PayRow(
                                        method: next,
                                        amount: decToApi(_short, scale: 2),
                                      ),
                                    );
                                    _edited();
                                  },
                            icon: const Icon(Icons.add, size: 18),
                            label: Text(
                              _credit ? l.payCreditDpAdd : l.payAddMethod,
                            ),
                          ),
                        ),
                      if (depositOver)
                        Padding(
                          padding: const EdgeInsets.only(top: 4),
                          child: Text(
                            l.payDepositOver(formatMoney(deposit)),
                            style: TextStyle(
                              color: pal.danger,
                              fontWeight: FontWeight.w600,
                              fontSize: 12.5,
                            ),
                          ),
                        ),
                      const Divider(height: 24),
                      _kv(l.payPaid, formatMoney(_paid), pal),
                      if (_surcharge > Decimal.zero) ...[
                        _kv(l.paySurcharge, '+${formatMoney(_surcharge)}', pal),
                        _kv(
                          l.payCharged,
                          formatMoney(total + _surcharge),
                          pal,
                          color: pal.danger,
                          bold: true,
                        ),
                      ],
                      if (_credit)
                        _kv(
                          l.payCreditReceivable,
                          formatMoney(_receivable),
                          pal,
                          color: pal.danger,
                          bold: true,
                        )
                      else if (_short > Decimal.zero)
                        _kv(
                          l.payShortBy(formatMoney(_short)),
                          '',
                          pal,
                          color: pal.danger,
                        ),
                      if (_change > Decimal.zero)
                        _kv(
                          l.payChange,
                          formatMoney(_change),
                          pal,
                          color: pal.success,
                          bold: true,
                        ),
                      if (_error != null) ...[
                        const SizedBox(height: 10),
                        Container(
                          padding: const EdgeInsets.all(10),
                          decoration: BoxDecoration(
                            color: pal.dangerSoft,
                            borderRadius: BorderRadius.circular(10),
                          ),
                          child: Text(
                            _errorText(l),
                            style: TextStyle(
                              color: pal.dangerText,
                              fontSize: 13,
                            ),
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 4, 20, 16),
                child: FilledButton(
                  onPressed: _busy || !enough ? null : _submit,
                  child: Text(_busy ? l.payProcessing : l.paySubmit),
                ),
              ),
            ],
          );
        },
      ),
    );
  }

  String _errorText(AppLocalizations l) {
    final e = _error!;
    if (e.code == 'VALIDATION') {
      final c = e.fields['credit'];
      if (c == 'CREDIT_NOT_NEEDED') return l.payCreditNotNeeded;
      if (c == 'MEMBER_REQUIRED') return l.payCreditNoMember;
    }
    return e.message(l);
  }

  /// Ringkasan syarat kredit member + persetujuan bila melewati limit.
  List<Widget> _creditPanel(BuildContext context, Quote? quote) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final c = quote?.credit;
    final r = _receivable;
    Widget kv(String k, String v, {bool bold = false}) => Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Flexible(
            child: Text(
              k,
              style: TextStyle(color: pal.textTertiary, fontSize: 13),
            ),
          ),
          const SizedBox(width: 8),
          Text(
            v,
            style: TextStyle(
              fontSize: 13,
              fontWeight: bold ? FontWeight.w700 : FontWeight.w500,
            ),
          ),
        ],
      ),
    );
    return [
      const SizedBox(height: 10),
      Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: pal.sunken,
          borderRadius: BorderRadius.circular(12),
        ),
        child: Column(
          children: [
            kv(
              l.payCreditDue,
              c != null && c.dueDays > 0
                  ? l.payCreditDueDays(c.dueDays)
                  : l.payCreditNoDue,
            ),
            if (c != null) ...[
              kv(
                l.payCreditLimit,
                c.limit > Decimal.zero
                    ? formatMoney(c.limit)
                    : l.payCreditNoLimit,
              ),
              kv(l.payCreditOutstanding, formatMoney(c.outstanding)),
              kv(l.payCreditAfter, formatMoney(c.outstanding + r), bold: true),
            ],
          ],
        ),
      ),
      // Tetap terpasang (disembunyikan) selama mode kredit, supaya PIN yang diketik tidak hilang saat DP diubah.
      Visibility(
        visible: _overLimit,
        maintainState: true,
        child: Padding(
          padding: const EdgeInsets.only(top: 10),
          child: Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: pal.warningSoft,
              borderRadius: BorderRadius.circular(12),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  l.payCreditOverLimit,
                  style: TextStyle(
                    color: pal.warningText,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                const SizedBox(height: 8),
                if (ref.read(cartProvider).draft(forPay: true).approval != null)
                  Text(
                    l.payCreditSameApprover,
                    style: const TextStyle(fontSize: 12.5),
                  )
                else
                  ApproverFields(
                    key: _creditKey,
                    purpose: 'credit_limit',
                    enabled: !_busy,
                    onChanged: (a, p) => setState(() {
                      _creditApprover = a;
                      _creditPin = p;
                    }),
                  ),
              ],
            ),
          ),
        ),
      ),
    ];
  }

  Widget _kv(
    String k,
    String v,
    AppPalette pal, {
    Color? color,
    bool bold = false,
  }) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 3),
    child: Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Flexible(
          child: Text(k, style: TextStyle(color: color ?? pal.textTertiary)),
        ),
        const SizedBox(width: 8),
        Text(
          v,
          style: TextStyle(
            fontWeight: bold ? FontWeight.w800 : FontWeight.w600,
            color: color,
          ),
        ),
      ],
    ),
  );

  Widget _row(BuildContext context, int i, List<PayMethodInfo> methods) {
    final l = AppLocalizations.of(context);
    final r = _rows[i];
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Expanded(
              flex: 5,
              child: DropdownButtonFormField<String>(
                initialValue: r.method.id,
                isExpanded: true,
                decoration: InputDecoration(labelText: l.payMethod),
                items: [
                  for (final m in methods)
                    DropdownMenuItem(
                      value: m.id,
                      child: Text(
                        m.hasFee
                            ? '${m.name} (${m.feeLabel}${m.feeByCustomer ? ' ↗' : ''})'
                            : m.name,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                ],
                onChanged: _busy
                    ? null
                    : (v) {
                        r.method = methods.firstWhere((m) => m.id == v);
                        _edited();
                      },
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              flex: 5,
              child: TextField(
                controller: r.amount,
                enabled: !_busy,
                keyboardType: const TextInputType.numberWithOptions(
                  decimal: true,
                ),
                inputFormatters: [
                  FilteringTextInputFormatter.allow(RegExp(r'[0-9.,]')),
                ],
                decoration: InputDecoration(labelText: l.payAmount),
                onChanged: (_) => _edited(),
              ),
            ),
            if (_rows.length > 1 || _credit)
              IconButton(
                onPressed: _busy
                    ? null
                    : () {
                        _rows.removeAt(i).dispose();
                        _edited();
                      },
                icon: const Icon(Icons.close),
              ),
          ],
        ),
        if (r.method.hasFee && parseInput(r.amount.text) > Decimal.zero)
          Padding(
            padding: const EdgeInsets.only(top: 6),
            child: Text(
              r.method.feeByCustomer
                  ? l.payFeeCustomer(
                      r.method.feeLabel,
                      formatMoney(r.method.feeOf(parseInput(r.amount.text))),
                      formatMoney(
                        parseInput(r.amount.text) +
                            r.method.feeOf(parseInput(r.amount.text)),
                      ),
                    )
                  : l.payFeeStore(
                      r.method.feeLabel,
                      formatMoney(r.method.feeOf(parseInput(r.amount.text))),
                    ),
              style: TextStyle(color: context.pal.warningText, fontSize: 12),
            ),
          ),
        if (r.method.isCash)
          Padding(
            padding: const EdgeInsets.only(top: 6),
            child: Wrap(
              spacing: 8,
              children: [
                ActionChip(
                  label: Text(l.payExact),
                  onPressed: _busy
                      ? null
                      : () {
                          final others = _rows
                              .where((x) => x != r)
                              .fold(
                                Decimal.zero,
                                (a, x) => a + parseInput(x.amount.text),
                              );
                          final need = _total - others;
                          r.amount.text = decToApi(
                            need > Decimal.zero ? need : Decimal.zero,
                            scale: 2,
                          );
                          _edited();
                        },
                ),
                for (final v in quickCashAmounts(_needFor(r)))
                  ActionChip(
                    label: Text(formatMoney(v, symbol: false)),
                    onPressed: _busy
                        ? null
                        : () {
                            r.amount.text = decToApi(v, scale: 2);
                            _edited();
                          },
                  ),
              ],
            ),
          )
        else if (!r.method.isDeposit)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: TextField(
              controller: r.ref,
              enabled: !_busy,
              decoration: InputDecoration(labelText: l.payRef),
              onChanged: (_) => _edited(),
            ),
          ),
      ],
    );
  }
}

/// Nominal tunai cepat di atas [need]: 50rb dan 100rb ditambah pecahan pembulatan ke atas (5rb, 10rb, 20rb, 50rb,
/// 100rb) yang paling dekat; paling banyak 4 pilihan, naik. Uang pas ditawarkan terpisah.
List<Decimal> quickCashAmounts(Decimal need) {
  if (need <= Decimal.zero) return const [];
  final out = <Decimal>{};
  for (final d in const [5000, 10000, 20000, 50000, 100000]) {
    final unit = Decimal.fromInt(d);
    final up =
        (need / unit).toDecimal(scaleOnInfinitePrecision: 6).ceil() * unit;
    if (up > need) out.add(up);
  }
  for (final fixed in const [50000, 100000]) {
    final v = Decimal.fromInt(fixed);
    if (v > need) out.add(v);
  }
  final sorted = out.toList()..sort();
  return sorted.take(4).toList();
}
