import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'approver_fields.dart';
import 'pos_controller.dart';
import 'pos_models.dart';
import 'pos_repository.dart';
import 'shift_models.dart';

/// Ubah harga satuan atau beri potongan satu baris. Keduanya butuh penyetuju + PIN (FR-POS-04); PIN diperiksa
/// dulu ke server, lalu disimpan di memori sampai nota selesai. Harga di bawah HPP tetap ditolak server saat quote.
Future<void> showLineAdjustSheet(
  BuildContext context, {
  required CartLine line,
  required Decimal listPrice,
}) => showModalBottomSheet<void>(
  context: context,
  isScrollControlled: true,
  useSafeArea: true,
  showDragHandle: true,
  builder: (_) => _LineAdjustSheet(line: line, listPrice: listPrice),
);

enum _Mode { price, discount }

enum _Unit { total, perUnit, percent }

class _LineAdjustSheet extends ConsumerStatefulWidget {
  const _LineAdjustSheet({required this.line, required this.listPrice});

  final CartLine line;
  final Decimal listPrice;

  @override
  ConsumerState<_LineAdjustSheet> createState() => _LineAdjustSheetState();
}

class _LineAdjustSheetState extends ConsumerState<_LineAdjustSheet> {
  final _approverKey = GlobalKey<ApproverFieldsState>();
  late final TextEditingController _value;
  _Mode _mode = _Mode.price;
  _Unit _unit = _Unit.total;
  Approver? _approver;
  String _pin = '';
  bool _busy = false;
  ApiError? _error;

  @override
  void initState() {
    super.initState();
    final l = widget.line;
    _value = TextEditingController(
      text: l.overridePrice != null ? decToApi(l.overridePrice!, scale: 2) : '',
    );
  }

  @override
  void dispose() {
    _value.dispose();
    super.dispose();
  }

  Decimal get _amount => parseInput(_value.text);

  bool get _valid {
    if (_approver == null || !RegExp(r'^\d{6}$').hasMatch(_pin)) return false;
    final v = _amount;
    if (_mode == _Mode.price) return _value.text.trim().isNotEmpty;
    if (_unit == _Unit.percent) {
      return v > Decimal.zero && v <= Decimal.fromInt(100);
    }
    return v > Decimal.zero;
  }

  /// Potongan per satuan (%) dihitung dari harga normal, lalu disimpan sebagai rupiah (sama dengan web).
  Decimal get _percentPerUnit =>
      ((widget.listPrice * _amount) / Decimal.fromInt(100))
          .toDecimal(scaleOnInfinitePrecision: 6)
          .round(scale: 2);

  Future<void> _apply() async {
    final l = widget.line;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final ap = await ref
          .read(posRepositoryProvider)
          .checkApproval(_approver!.id, _pin);
      if (!mounted) return;
      final grant = ApprovalGrant(userId: ap.id, name: ap.name, pin: _pin);
      final ctrl = ref.read(cartProvider.notifier);
      if (_mode == _Mode.price) {
        ctrl.adjustLine(
          l.item.id,
          price: _amount,
          disc: l.discount,
          discTotal: l.discountTotal,
          approval: grant,
        );
      } else {
        final disc = _unit == _Unit.percent ? _percentPerUnit : _amount;
        ctrl.adjustLine(
          l.item.id,
          price: l.overridePrice,
          disc: disc,
          discTotal: _unit == _Unit.total,
          approval: grant,
        );
      }
      Navigator.of(context).pop();
    } on ApiError catch (e) {
      if (mounted) {
        setState(() {
          _error = e;
          _pin = '';
        });
        _approverKey.currentState?.clearPin();
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final line = widget.line;
    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      child: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(20, 0, 20, 20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              l.adjustTitle,
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 4),
            Text(
              l.adjustBody,
              style: TextStyle(color: pal.textTertiary, fontSize: 13),
            ),
            const SizedBox(height: 12),
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: pal.sunken,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Column(
                children: [
                  _kv(l.adjustItem, line.item.name),
                  _kv(l.adjustListPrice, formatMoney(widget.listPrice)),
                ],
              ),
            ),
            const SizedBox(height: 12),
            SegmentedButton<_Mode>(
              segments: [
                ButtonSegment(
                  value: _Mode.price,
                  label: Text(l.adjustTabPrice),
                ),
                ButtonSegment(
                  value: _Mode.discount,
                  label: Text(l.adjustTabDiscount),
                ),
              ],
              selected: {_mode},
              onSelectionChanged: _busy
                  ? null
                  : (s) => setState(() {
                      _mode = s.first;
                      _value.text =
                          _mode == _Mode.price && line.overridePrice != null
                          ? decToApi(line.overridePrice!, scale: 2)
                          : '';
                    }),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _value,
              enabled: !_busy,
              autofocus: true,
              keyboardType: const TextInputType.numberWithOptions(
                decimal: true,
              ),
              onChanged: (_) => setState(() {}),
              decoration: InputDecoration(
                labelText: _mode == _Mode.price
                    ? l.adjustNewPrice
                    : (_unit == _Unit.percent
                          ? l.adjustDiscountPct
                          : l.adjustDiscountRp),
              ),
            ),
            if (_mode == _Mode.discount) ...[
              const SizedBox(height: 10),
              SegmentedButton<_Unit>(
                segments: [
                  ButtonSegment(
                    value: _Unit.total,
                    label: Text(l.adjustUnitTotal),
                  ),
                  ButtonSegment(
                    value: _Unit.perUnit,
                    label: Text(l.adjustUnitPerUnit),
                  ),
                  const ButtonSegment(value: _Unit.percent, label: Text('%')),
                ],
                selected: {_unit},
                onSelectionChanged: _busy
                    ? null
                    : (s) => setState(() => _unit = s.first),
              ),
              if (_unit == _Unit.percent &&
                  _amount > Decimal.zero &&
                  _amount <= Decimal.fromInt(100))
                Padding(
                  padding: const EdgeInsets.only(top: 6),
                  child: Text(
                    l.adjustPercentPreview(formatMoney(_percentPerUnit)),
                    style: TextStyle(color: pal.textTertiary, fontSize: 12.5),
                  ),
                ),
            ],
            const SizedBox(height: 12),
            ApproverFields(
              key: _approverKey,
              enabled: !_busy,
              onChanged: (a, p) => setState(() {
                _approver = a;
                _pin = p;
              }),
            ),
            if (_error != null) ...[
              const SizedBox(height: 10),
              Text(_error!.message(l), style: TextStyle(color: pal.danger)),
            ],
            const SizedBox(height: 16),
            FilledButton(
              onPressed: _valid && !_busy ? _apply : null,
              child: Text(_busy ? l.adjustChecking : l.adjustApply),
            ),
            if (line.adjusted) ...[
              const SizedBox(height: 4),
              TextButton(
                onPressed: _busy
                    ? null
                    : () {
                        ref.read(cartProvider.notifier).resetLine(line.item.id);
                        Navigator.of(context).pop();
                      },
                child: Text(l.adjustReset),
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget _kv(String k, String v) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 2),
    child: Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(k),
        const SizedBox(width: 12),
        Flexible(
          child: Text(
            v,
            textAlign: TextAlign.end,
            style: const TextStyle(fontWeight: FontWeight.w600),
          ),
        ),
      ],
    ),
  );
}
