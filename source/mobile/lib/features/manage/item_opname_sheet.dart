import 'package:decimal/decimal.dart';
import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/session/session_controller.dart';
import '../../l10n/gen/app_localizations.dart';

/// Stok opname satu barang (Mode Persediaan): isi stok fisik di rak (bucket display), stok langsung diganti lewat
/// `POST /stock/counts/quick` (mode replace, izin stock_opname approve). true = stok berubah.
Future<bool> showItemOpnameSheet(
  BuildContext context, {
  required String itemId,
  required String name,
  required String unit,
  required Decimal stock,
}) async =>
    await showModalBottomSheet<bool>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      builder: (_) =>
          _OpnameSheet(itemId: itemId, name: name, unit: unit, stock: stock),
    ) ??
    false;

class _OpnameSheet extends ConsumerStatefulWidget {
  const _OpnameSheet({
    required this.itemId,
    required this.name,
    required this.unit,
    required this.stock,
  });

  final String itemId, name, unit;
  final Decimal stock;

  @override
  ConsumerState<_OpnameSheet> createState() => _OpnameSheetState();
}

class _OpnameSheetState extends ConsumerState<_OpnameSheet> {
  final _qty = TextEditingController();
  bool _saving = false;
  String? _error;

  @override
  void dispose() {
    _qty.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final l = AppLocalizations.of(context);
    if (_qty.text.trim().isEmpty) return;
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      await ref
          .read(apiClientProvider)
          .dio
          .post<void>(
            '/stock/counts/quick',
            data: {
              'bucket': 'display',
              'mode': 'replace',
              'items': [
                {
                  'item_id': widget.itemId,
                  'qty': decToApi(parseInput(_qty.text)),
                },
              ],
            },
          );
      if (mounted) Navigator.of(context).pop(true);
    } on DioException catch (e) {
      if (mounted) setState(() => _error = ApiError.fromDio(e).message(l));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return Padding(
      padding: EdgeInsets.fromLTRB(
        20,
        8,
        20,
        16 + MediaQuery.viewInsetsOf(context).bottom,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            l.itemsOpname,
            style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 4),
          Text(widget.name, maxLines: 2, overflow: TextOverflow.ellipsis),
          Text(l.opnameSystem(formatQty(widget.stock), widget.unit)),
          const SizedBox(height: 12),
          TextField(
            controller: _qty,
            autofocus: true,
            textAlign: TextAlign.end,
            keyboardType: const TextInputType.numberWithOptions(decimal: true),
            inputFormatters: [
              FilteringTextInputFormatter.allow(RegExp(r'[0-9,]')),
            ],
            decoration: InputDecoration(
              hintText: formatQty(widget.stock),
              suffixText: widget.unit,
            ),
            onChanged: (_) => setState(() {}),
            onSubmitted: (_) => _save(),
          ),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(
                _error!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ),
          const SizedBox(height: 16),
          FilledButton(
            style: FilledButton.styleFrom(
              minimumSize: const Size.fromHeight(48),
            ),
            onPressed: _saving || _qty.text.trim().isEmpty ? null : _save,
            child: Text(l.opnameSave),
          ),
        ],
      ),
    );
  }
}
