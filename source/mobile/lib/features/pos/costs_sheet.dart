import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/money/money.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'pos_controller.dart';
import 'pos_models.dart';

/// Potongan nota (Rp) dan rincian biaya lain-lain (nama + jumlah, maks 20). Server yang menghitung total;
/// di sini hanya mengisi. Tidak butuh PIN (sama dengan web: hanya ubah harga/potongan baris yang butuh).
Future<void> showCostsSheet(BuildContext context) => showModalBottomSheet<void>(
  context: context,
  isScrollControlled: true,
  useSafeArea: true,
  showDragHandle: true,
  builder: (_) => const _CostsSheet(),
);

class _CostRow {
  _CostRow(String name, String amount)
    : name = TextEditingController(text: name),
      amount = TextEditingController(text: amount);

  final TextEditingController name;
  final TextEditingController amount;

  void dispose() {
    name.dispose();
    amount.dispose();
  }
}

class _CostsSheet extends ConsumerStatefulWidget {
  const _CostsSheet();

  @override
  ConsumerState<_CostsSheet> createState() => _CostsSheetState();
}

class _CostsSheetState extends ConsumerState<_CostsSheet> {
  final List<_CostRow> _rows = [];
  late final TextEditingController _note;

  @override
  void initState() {
    super.initState();
    _note = TextEditingController(text: ref.read(cartProvider).note);
    final c = ref.read(cartProvider);
    for (final e in c.costs) {
      _rows.add(_CostRow(e.name, decToApi(e.amount, scale: 2)));
    }
    if (_rows.isEmpty) _rows.add(_CostRow('', ''));
  }

  @override
  void dispose() {
    _note.dispose();
    for (final r in _rows) {
      r.dispose();
    }
    super.dispose();
  }

  Decimal get _total =>
      _rows.fold(Decimal.zero, (a, r) => a + parseInput(r.amount.text));

  void _apply({bool clear = false}) {
    final ctrl = ref.read(cartProvider.notifier);
    ctrl.setNote(clear ? '' : _note.text.trim());
    if (clear) {
      ctrl.setCosts(const []);
    } else {
      ctrl.setCosts([
        for (final r in _rows)
          CostEntry(
            name: r.name.text.trim(),
            amount: parseInput(r.amount.text),
          ),
      ]);
    }
    Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      child: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(20, 0, 20, 20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              l.costsTitle,
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
            TextField(
              controller: _note,
              maxLength: CartState.maxNote,
              minLines: 1,
              maxLines: 3,
              textCapitalization: TextCapitalization.sentences,
              decoration: InputDecoration(
                labelText: l.noteLabel,
                hintText: l.noteHint,
              ),
            ),
            const SizedBox(height: 12),
            Text(
              l.costsHint,
              style: TextStyle(color: pal.textTertiary, fontSize: 12.5),
            ),
            const SizedBox(height: 8),
            for (var i = 0; i < _rows.length; i++)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: Row(
                  children: [
                    Expanded(
                      flex: 5,
                      child: TextField(
                        controller: _rows[i].name,
                        maxLength: 60,
                        decoration: InputDecoration(
                          labelText: l.costsName,
                          counterText: '',
                        ),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      flex: 4,
                      child: TextField(
                        controller: _rows[i].amount,
                        keyboardType: const TextInputType.numberWithOptions(
                          decimal: true,
                        ),
                        onChanged: (_) => setState(() {}),
                        decoration: InputDecoration(labelText: l.costsAmount),
                      ),
                    ),
                    IconButton(
                      tooltip: l.costsRemove,
                      icon: Icon(Icons.close, color: pal.danger),
                      onPressed: () => setState(() {
                        _rows.removeAt(i).dispose();
                        if (_rows.isEmpty) _rows.add(_CostRow('', ''));
                      }),
                    ),
                  ],
                ),
              ),
            Align(
              alignment: Alignment.centerLeft,
              child: TextButton.icon(
                onPressed: _rows.length >= CartState.maxCosts
                    ? null
                    : () => setState(() => _rows.add(_CostRow('', ''))),
                icon: const Icon(Icons.add, size: 18),
                label: Text(l.costsAdd),
              ),
            ),
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 8),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Text(
                    l.costsTotal,
                    style: const TextStyle(fontWeight: FontWeight.w600),
                  ),
                  Text(
                    formatMoney(_total),
                    style: const TextStyle(fontWeight: FontWeight.w700),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 8),
            FilledButton(onPressed: _apply, child: Text(l.costsDone)),
            TextButton(
              onPressed: () => _apply(clear: true),
              child: Text(l.costsClear),
            ),
          ],
        ),
      ),
    );
  }
}
