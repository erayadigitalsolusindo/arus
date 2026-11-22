import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../core/money/money.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'item_thumb.dart';
import 'pos_models.dart';

/// Dialog di tengah layar untuk input jumlah setelah barang di-tap di kasir. Mengembalikan qty (> 0) atau null bila ditutup.
Future<Decimal?> showQtyDialog(BuildContext context, PosItem item) =>
    showDialog<Decimal>(
      context: context,
      builder: (_) => _QtySheet(item: item),
    );

class _QtySheet extends StatefulWidget {
  const _QtySheet({required this.item});

  final PosItem item;

  @override
  State<_QtySheet> createState() => _QtySheetState();
}

class _QtySheetState extends State<_QtySheet> {
  final _c = TextEditingController(text: '1');
  final _focus = FocusNode();

  @override
  void initState() {
    super.initState();
    // Langsung terpilih semua: kasir tinggal mengetik angka baru.
    _focus.addListener(() {
      if (_focus.hasFocus) {
        _c.selection = TextSelection(
          baseOffset: 0,
          extentOffset: _c.text.length,
        );
      }
    });
  }

  @override
  void dispose() {
    _c.dispose();
    _focus.dispose();
    super.dispose();
  }

  Decimal get _qty =>
      _c.text.trim().isEmpty ? Decimal.zero : parseInput(_c.text);

  void _set(Decimal v) {
    HapticFeedback.selectionClick();
    final t = formatQty(v < Decimal.zero ? Decimal.zero : v);
    setState(
      () => _c.value = TextEditingValue(
        text: t,
        selection: TextSelection.collapsed(offset: t.length),
      ),
    );
  }

  void _submit() {
    if (_qty > Decimal.zero) Navigator.of(context).pop(_qty);
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final item = widget.item;
    final qty = _qty;
    final ok = qty > Decimal.zero;
    return Dialog(
      backgroundColor: pal.surface,
      insetPadding: const EdgeInsets.symmetric(horizontal: 40, vertical: 24),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 300),
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(14),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(
                children: [
                  ClipRRect(
                    borderRadius: BorderRadius.circular(10),
                    child: SizedBox(
                      width: 48,
                      height: 48,
                      child: ItemThumb(
                        itemId: item.id,
                        imageId: item.mainImageId,
                        name: item.name,
                      ),
                    ),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          item.name,
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(
                            fontSize: 14,
                            fontWeight: FontWeight.w700,
                            height: 1.2,
                          ),
                        ),
                        const SizedBox(height: 2),
                        Text(
                          item.unit.isEmpty
                              ? formatMoney(item.price)
                              : '${formatMoney(item.price)} / ${item.unit}',
                          style: TextStyle(
                            fontSize: 12.5,
                            fontWeight: FontWeight.w600,
                            color: pal.accent,
                          ),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 12),
              Row(
                children: [
                  _RoundBtn(
                    icon: Icons.remove,
                    onTap: qty > Decimal.one
                        ? () => _set(qty - Decimal.one)
                        : null,
                  ),
                  Expanded(
                    child: TextField(
                      controller: _c,
                      focusNode: _focus,
                      autofocus: true,
                      textAlign: TextAlign.center,
                      keyboardType: const TextInputType.numberWithOptions(
                        decimal: true,
                      ),
                      inputFormatters: [
                        FilteringTextInputFormatter.allow(RegExp(r'[0-9.,]')),
                      ],
                      textInputAction: TextInputAction.done,
                      onChanged: (_) => setState(() {}),
                      onSubmitted: (_) => _submit(),
                      style: TextStyle(
                        fontSize: 26,
                        fontWeight: FontWeight.w800,
                        color: pal.primary,
                      ),
                      decoration: InputDecoration(
                        isDense: true,
                        filled: true,
                        fillColor: pal.primarySoft,
                        contentPadding: const EdgeInsets.symmetric(vertical: 6),
                        border: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(10),
                          borderSide: BorderSide.none,
                        ),
                        enabledBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(10),
                          borderSide: BorderSide.none,
                        ),
                        focusedBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(10),
                          borderSide: BorderSide(
                            color: pal.primary,
                            width: 1.5,
                          ),
                        ),
                      ),
                    ),
                  ),
                  _RoundBtn(
                    icon: Icons.add,
                    onTap: () => _set(qty + Decimal.one),
                  ),
                ],
              ),
              const SizedBox(height: 12),
              FilledButton(
                style: FilledButton.styleFrom(
                  minimumSize: const Size(0, 42),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(10),
                  ),
                ),
                onPressed: ok ? _submit : null,
                child: FittedBox(
                  fit: BoxFit.scaleDown,
                  child: Text(
                    ok
                        ? '${l.posQtyAdd} · ${formatMoney(item.price * qty)}'
                        : l.posQtyAdd,
                    style: const TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _RoundBtn extends StatelessWidget {
  const _RoundBtn({required this.icon, required this.onTap});

  final IconData icon;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final pal = context.pal;
    return Material(
      color: pal.primarySoft,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(10)),
      child: InkWell(
        borderRadius: BorderRadius.circular(10),
        onTap: onTap,
        child: SizedBox(
          width: 36,
          height: 36,
          child: Icon(
            icon,
            size: 20,
            color: onTap == null ? pal.textTertiary : pal.primary,
          ),
        ),
      ),
    );
  }
}
