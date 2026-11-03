import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import '../manage/manage_repository.dart';
import 'pos_models.dart';
import 'pos_repository.dart';
import 'scan_page.dart';

/// Form tambah/ubah barang sederhana dari HP ([AddItemSheet.editId] != null = ubah). Mengembalikan barang baru bila disimpan; [AddItemResult.sellNow] = langsung
/// masuk keranjang. Harga jual dan modal wajib supaya laporan laba/stok tetap benar; SKU dibuat otomatis oleh server.
class AddItemResult {
  const AddItemResult(this.item, {required this.sellNow});

  final PosItem item;
  final bool sellNow;
}

Future<AddItemResult?> showAddItemSheet(
  BuildContext context, {
  String name = '',
  String? editId,
}) => showModalBottomSheet<AddItemResult>(
  context: context,
  isScrollControlled: true,
  useSafeArea: true,
  showDragHandle: true,
  builder: (_) => AddItemSheet(initialName: name, editId: editId),
);

class AddItemSheet extends ConsumerStatefulWidget {
  const AddItemSheet({super.key, this.initialName = '', this.editId});

  final String initialName;

  /// Id barang yang diubah; null = barang baru.
  final String? editId;

  @override
  ConsumerState<AddItemSheet> createState() => _AddItemSheetState();
}

class _AddItemSheetState extends ConsumerState<AddItemSheet> {
  late final _name = TextEditingController(text: widget.initialName);
  final _sell = TextEditingController();
  final _cost = TextEditingController();
  final _barcode = TextEditingController();
  final _stock = TextEditingController();

  List<Salesperson> _units = const [];
  List<Salesperson> _categories = const [];
  String? _unitId;
  String? _categoryId;
  bool _goods = true;
  bool _negative = false;
  bool _openingOpen = false;
  bool _loading = true;
  bool _loadFailed = false;
  bool _busy = false;
  bool _submitted = false;
  ApiError? _error;
  Map<String, dynamic>? _raw; // barang yang sedang diubah (dasar body PUT)

  bool get _edit => widget.editId != null;

  @override
  void initState() {
    super.initState();
    _loadLookups();
  }

  @override
  void dispose() {
    for (final c in [_name, _sell, _cost, _barcode, _stock]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _loadLookups() async {
    final repo = ref.read(posRepositoryProvider);
    try {
      final r = await Future.wait([
        repo.units(),
        repo.categories(),
        repo.openingOpen(),
      ]);
      if (!mounted) return;
      final units = r[0] as List<Salesperson>;
      if (_edit) {
        final raw = await ref
            .read(manageRepositoryProvider)
            .getItem(widget.editId!);
        _raw = raw;
        _name.text = '${raw['name'] ?? ''}';
        _sell.text = decToApi(dec(raw['sell_price']), scale: 2);
        _barcode.text = '${raw['barcode'] ?? ''}';
        _goods = raw['kind'] != 'service';
        _negative = raw['allow_negative_stock'] == true;
        _categoryId = (raw['category'] as Map?)?['id'] as String?;
        _unitId = (raw['unit'] as Map?)?['id'] as String?;
      }
      if (!mounted) return;
      setState(() {
        _units = units;
        _categories = r[1] as List<Salesperson>;
        _openingOpen = !_edit && (r[2] as bool);
        _unitId ??= units.isEmpty ? null : units.first.id;
        _loading = false;
      });
    } on ApiError catch (e) {
      if (mounted) {
        setState(() {
          _error = e;
          _loadFailed = true;
          _loading = false;
        });
      }
    }
  }

  Decimal? _num(TextEditingController c) {
    final t = c.text.trim();
    if (t.isEmpty) return null;
    final d = parseInput(t);
    return d < Decimal.zero ? null : d;
  }

  String? _moneyError(AppLocalizations l, TextEditingController c) {
    if (!_submitted) return null;
    if (c.text.trim().isEmpty) return l.itemRequired;
    return _num(c) == null ? l.itemInvalidNumber : null;
  }

  Future<void> _scan() async {
    final l = AppLocalizations.of(context);
    await showScanPage(
      context,
      onCode: (code) async {
        _barcode.text = code;
        if (mounted) setState(() {});
        Navigator.of(context).maybePop();
        return ScanOutcome(ok: true, text: l.itemScanned);
      },
    );
  }

  Future<void> _save({required bool sellNow}) async {
    setState(() => _submitted = true);
    final sell = _num(_sell);
    final cost = _edit ? Decimal.zero : _num(_cost);
    if (_name.text.trim().isEmpty ||
        sell == null ||
        cost == null ||
        _unitId == null) {
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      if (_edit) {
        final raw = _raw!;
        // Server mengganti semua field barang: field yang tidak diubah dibawa dari barang yang ada.
        String refId(String k) => '${(raw[k] as Map?)?['id'] ?? ''}';
        await ref.read(manageRepositoryProvider).updateItem(widget.editId!, {
          'sku': raw['sku'],
          'barcode': _barcode.text.trim(),
          'origin': raw['origin'] ?? '',
          'name': _name.text.trim(),
          'weight_grams': raw['weight_grams'] ?? '0',
          'min_stock': raw['min_stock'] ?? '0',
          'sell_price': decToApi(sell, scale: 2),
          'unit_id': _unitId,
          'category_id': _categoryId ?? '',
          'brand_id': refId('brand'),
          'principal_id': refId('principal'),
          'supplier_id': refId('supplier'),
          'kind': raw['kind'] ?? 'goods',
          'allow_negative_stock': _goods && _negative,
          'sell_below_cost': raw['sell_below_cost'] == true,
          'description': raw['description'] ?? '',
        });
        if (mounted) {
          Navigator.of(context).pop(
            AddItemResult(
              PosItem(
                id: widget.editId!,
                sku: '${raw['sku'] ?? ''}',
                barcode: _barcode.text.trim(),
                name: _name.text.trim(),
                unit: '',
                price: sell,
                stockDisplay: Decimal.zero,
                isGoods: _goods,
              ),
              sellNow: false,
            ),
          );
        }
        return;
      }
      final item = await ref
          .read(posRepositoryProvider)
          .createItem(
            name: _name.text.trim(),
            sellPrice: sell,
            cost: cost,
            unitId: _unitId!,
            categoryId: _categoryId,
            barcode: _barcode.text.trim(),
            isGoods: _goods,
            allowNegativeStock: _negative,
            openingQty: _openingOpen ? _num(_stock) : null,
          );
      if (mounted) {
        Navigator.of(context).pop(AddItemResult(item, sellNow: sellNow));
      }
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
    const numPad = TextInputType.numberWithOptions(decimal: true);
    final numOnly = [FilteringTextInputFormatter.allow(RegExp(r'[0-9.,]'))];
    final noUnits = !_loading && !_loadFailed && _units.isEmpty;
    // Tombol simpan menempel di bawah (di luar area gulir) supaya selalu terlihat walau formnya panjang.
    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Flexible(
            child: SingleChildScrollView(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Text(
                    _edit ? l.itemsEditTitle : l.itemAddTitle,
                    style: const TextStyle(
                      fontSize: 18,
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                  const SizedBox(height: 14),
                  if (_loading)
                    const Padding(
                      padding: EdgeInsets.symmetric(vertical: 40),
                      child: Center(child: CircularProgressIndicator()),
                    )
                  else ...[
                    TextField(
                      controller: _name,
                      textCapitalization: TextCapitalization.words,
                      textInputAction: TextInputAction.next,
                      maxLength: 120,
                      decoration: InputDecoration(
                        labelText: l.itemName,
                        counterText: '',
                        errorText: _submitted && _name.text.trim().isEmpty
                            ? l.itemRequired
                            : null,
                      ),
                    ),
                    const SizedBox(height: 12),
                    SegmentedButton<bool>(
                      segments: [
                        ButtonSegment(
                          value: true,
                          icon: const Icon(Icons.inventory_2_outlined),
                          label: Text(l.itemKindGoods),
                        ),
                        ButtonSegment(
                          value: false,
                          icon: const Icon(Icons.design_services_outlined),
                          label: Text(l.itemKindService),
                        ),
                      ],
                      selected: {_goods},
                      showSelectedIcon: false,
                      onSelectionChanged: _edit
                          ? null
                          : (s) => setState(() => _goods = s.first),
                    ),
                    const SizedBox(height: 12),
                    Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Expanded(
                          child: TextField(
                            controller: _sell,
                            keyboardType: numPad,
                            inputFormatters: numOnly,
                            textInputAction: TextInputAction.next,
                            decoration: InputDecoration(
                              labelText: l.itemSellPrice,
                              errorText: _moneyError(l, _sell),
                            ),
                          ),
                        ),
                        if (!_edit) ...[
                          const SizedBox(width: 12),
                          Expanded(
                            child: TextField(
                              controller: _cost,
                              keyboardType: numPad,
                              inputFormatters: numOnly,
                              textInputAction: TextInputAction.next,
                              decoration: InputDecoration(
                                labelText: l.itemCost,
                                errorText: _moneyError(l, _cost),
                              ),
                            ),
                          ),
                        ],
                      ],
                    ),
                    if (_edit)
                      Padding(
                        padding: const EdgeInsets.only(top: 6),
                        child: Text(
                          l.itemsCostNote,
                          style: TextStyle(
                            fontSize: 12,
                            color: pal.textTertiary,
                          ),
                        ),
                      ),
                    const SizedBox(height: 12),
                    if (noUnits)
                      Text(l.itemNoUnits, style: TextStyle(color: pal.danger))
                    else
                      DropdownButtonFormField<String>(
                        initialValue: _unitId,
                        isExpanded: true,
                        decoration: InputDecoration(labelText: l.itemUnit),
                        items: [
                          for (final u in _units)
                            DropdownMenuItem(value: u.id, child: Text(u.name)),
                        ],
                        onChanged: (v) => setState(() => _unitId = v),
                      ),
                    if (_categories.isNotEmpty) ...[
                      const SizedBox(height: 12),
                      DropdownButtonFormField<String?>(
                        initialValue: _categoryId,
                        isExpanded: true,
                        decoration: InputDecoration(labelText: l.itemCategory),
                        items: [
                          DropdownMenuItem<String?>(
                            value: null,
                            child: Text(l.itemCategoryNone),
                          ),
                          for (final c in _categories)
                            DropdownMenuItem<String?>(
                              value: c.id,
                              child: Text(c.name),
                            ),
                        ],
                        onChanged: (v) => setState(() => _categoryId = v),
                      ),
                    ],
                    const SizedBox(height: 12),
                    TextField(
                      controller: _barcode,
                      textInputAction: TextInputAction.next,
                      maxLength: 64,
                      decoration: InputDecoration(
                        labelText: l.itemBarcode,
                        counterText: '',
                        suffixIcon: IconButton(
                          tooltip: l.itemScanBarcode,
                          icon: const Icon(Icons.qr_code_scanner),
                          onPressed: _scan,
                        ),
                      ),
                    ),
                    const SizedBox(height: 12),
                    if (_goods) ...[
                      if (_openingOpen) ...[
                        TextField(
                          controller: _stock,
                          keyboardType: numPad,
                          inputFormatters: numOnly,
                          decoration: InputDecoration(
                            labelText: l.itemInitialStock,
                          ),
                        ),
                        const SizedBox(height: 4),
                      ],
                      SwitchListTile(
                        contentPadding: EdgeInsets.zero,
                        title: Text(l.itemAllowNegative),
                        subtitle: Text(
                          l.itemAllowNegativeHint,
                          style: TextStyle(
                            fontSize: 12,
                            color: pal.textTertiary,
                          ),
                        ),
                        value: _negative,
                        onChanged: (v) => setState(() => _negative = v),
                      ),
                      if (!_edit &&
                          !_negative &&
                          (!_openingOpen ||
                              (_num(_stock) ?? Decimal.zero) <= Decimal.zero))
                        Padding(
                          padding: const EdgeInsets.only(bottom: 4),
                          child: Text(
                            l.itemStockHint,
                            style: TextStyle(
                              fontSize: 12,
                              color: pal.warningText,
                            ),
                          ),
                        ),
                    ] else
                      Text(
                        l.itemServiceHint,
                        style: TextStyle(fontSize: 12, color: pal.textTertiary),
                      ),
                    if (_error != null)
                      Padding(
                        padding: const EdgeInsets.only(top: 8),
                        child: Text(
                          _error!.fields.isNotEmpty
                              ? '${_error!.message(l)} (${_error!.fields.keys.join(', ')})'
                              : _error!.message(l),
                          style: TextStyle(color: pal.danger),
                        ),
                      ),
                  ],
                ],
              ),
            ),
          ),
          if (!_loading)
            SafeArea(
              top: false,
              child: Padding(
                padding: const EdgeInsets.fromLTRB(20, 4, 20, 12),
                child: _edit
                    ? SizedBox(
                        width: double.infinity,
                        child: FilledButton(
                          onPressed: _busy || noUnits || _loadFailed
                              ? null
                              : () => _save(sellNow: false),
                          child: _busy
                              ? const SizedBox(
                                  width: 18,
                                  height: 18,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                    color: Colors.white,
                                  ),
                                )
                              : Text(l.commonSave),
                        ),
                      )
                    : Row(
                        children: [
                          Expanded(
                            child: OutlinedButton(
                              onPressed: _busy || noUnits || _loadFailed
                                  ? null
                                  : () => _save(sellNow: false),
                              child: Text(l.itemSave),
                            ),
                          ),
                          const SizedBox(width: 12),
                          Expanded(
                            child: FilledButton(
                              onPressed: _busy || noUnits || _loadFailed
                                  ? null
                                  : () => _save(sellNow: true),
                              child: _busy
                                  ? const SizedBox(
                                      width: 18,
                                      height: 18,
                                      child: CircularProgressIndicator(
                                        strokeWidth: 2,
                                        color: Colors.white,
                                      ),
                                    )
                                  : Text(l.itemSaveAndSell),
                            ),
                          ),
                        ],
                      ),
              ),
            ),
        ],
      ),
    );
  }
}
