import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:uuid/uuid.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'pos_controller.dart';
import 'pos_models.dart';
import 'pos_repository.dart';
import 'shift_dialog.dart';

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

  Decimal get _short => _total > _paid ? _total - _paid : Decimal.zero;

  void _edited() {
    _key = null; // isi berubah → percobaan baru, kunci baru
    setState(() {});
  }

  void _ensureDefaults(List<PayMethodInfo> methods) {
    if (_rows.isNotEmpty || methods.isEmpty) return;
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
                cart.lines,
                applyTax: cart.applyTax,
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
      if (mounted) setState(() => _error = e);
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
        data: (methods) {
          _ensureDefaults(methods);
          final enough =
              _paid >= total && total > Decimal.zero && _rows.isNotEmpty;
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
                            label: Text(l.payAddMethod),
                          ),
                        ),
                      const Divider(height: 24),
                      _kv(l.payPaid, formatMoney(_paid), pal),
                      if (_short > Decimal.zero)
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
                            _error!.message(l),
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
        Text(k, style: TextStyle(color: color ?? pal.textTertiary)),
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
                      child: Text(m.name, overflow: TextOverflow.ellipsis),
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
            if (_rows.length > 1)
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
                for (final v in const [50000, 100000])
                  ActionChip(
                    label: Text(formatMoney(Decimal.fromInt(v), symbol: false)),
                    onPressed: _busy
                        ? null
                        : () {
                            r.amount.text = '$v';
                            _edited();
                          },
                  ),
              ],
            ),
          )
        else
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
