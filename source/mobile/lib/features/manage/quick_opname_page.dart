import 'package:decimal/decimal.dart';
import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/session/session_controller.dart';
import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../l10n/gen/app_localizations.dart';

/// Stok opname cepat (Mode Persediaan): isi stok fisik di rak (bucket display) lalu simpan; stok langsung diganti
/// lewat `POST /stock/counts/quick` (mode replace, izin stock_opname approve). Hanya barang yang diisi yang dikirim.
Future<void> showQuickOpnamePage(BuildContext context) =>
    Navigator.of(context)
        .push<void>(MaterialPageRoute(builder: (_) => const QuickOpnamePage()));

class QuickOpnamePage extends ConsumerStatefulWidget {
  const QuickOpnamePage({super.key});

  @override
  ConsumerState<QuickOpnamePage> createState() => _QuickOpnamePageState();
}

class _Row {
  _Row(Map<String, dynamic> j)
    : id = '${j['id']}',
      name = '${j['name'] ?? ''}',
      sku = '${j['sku'] ?? ''}',
      unit = '${j['unit'] ?? ''}',
      stock = dec(j['display']);

  final String id, name, sku, unit;
  final Decimal stock;
}

class _QuickOpnamePageState extends ConsumerState<QuickOpnamePage> {
  final _q = TextEditingController();
  final _qty =
      <
        String,
        TextEditingController
      >{}; // item id → isian stok fisik; bertahan antar pencarian
  List<_Row> _rows = const [];
  bool _loading = true, _saving = false;
  String? _error;

  Dio get _dio => ref.read(apiClientProvider).dio;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _q.dispose();
    for (final c in _qty.values) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      final r = await _dio.get<Map<String, dynamic>>(
        '/stock/counts/items',
        queryParameters: {
          'limit': 50,
          if (_q.text.trim().isNotEmpty) 'q': _q.text.trim(),
        },
      );
      final rows = (r.data!['data'] as List? ?? const [])
          .map((e) => _Row((e as Map).cast<String, dynamic>()))
          .toList();
      if (mounted)
        setState(() {
          _rows = rows;
          _error = null;
        });
    } on DioException catch (e) {
      if (mounted)
        setState(
          () =>
              _error = ApiError.fromDio(e)
                  .message(AppLocalizations.of(context)),
        );
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _save() async {
    final l = AppLocalizations.of(context);
    final items = [
      for (final e in _qty.entries)
        if (e.value.text.trim().isNotEmpty)
          {'item_id': e.key, 'qty': decToApi(parseInput(e.value.text))},
    ];
    if (items.isEmpty) return;
    setState(() => _saving = true);
    try {
      await _dio.post<void>(
        '/stock/counts/quick',
        data: {'bucket': 'display', 'mode': 'replace', 'items': items},
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(l.opnameDone(items.length))));
      for (final c in _qty.values) {
        c.clear();
      }
      await _load();
    } on DioException catch (e) {
      if (mounted) setState(() => _error = ApiError.fromDio(e).message(l));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(l.opnameTitle)),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 4),
            child: TextField(
              controller: _q,
              textInputAction: TextInputAction.search,
              decoration: InputDecoration(
                hintText: l.itemsSearch,
                prefixIcon: const Icon(Icons.search),
              ),
              onSubmitted: (_) => _load(),
            ),
          ),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Align(
              alignment: Alignment.centerLeft,
              child: Text(l.opnameHint),
            ),
          ),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.all(16),
              child: Text(
                _error!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ),
          Expanded(
            child: _loading
                ? const Center(child: CircularProgressIndicator())
                : ListView.separated(
                    itemCount: _rows.length,
                    separatorBuilder: (_, _) => const Divider(height: 1),
                    itemBuilder: (_, i) {
                      final r = _rows[i];
                      final c = _qty.putIfAbsent(
                        r.id,
                        TextEditingController.new,
                      );
                      return ListTile(
                        title: Text(
                          r.name,
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                        ),
                        subtitle: Text(
                          '${r.sku} · ${l.opnameSystem(formatQty(r.stock), r.unit)}',
                        ),
                        trailing: SizedBox(
                          width: 96,
                          child: TextField(
                            controller: c,
                            textAlign: TextAlign.end,
                            keyboardType: const TextInputType.numberWithOptions(
                              decimal: true,
                            ),
                            inputFormatters: [
                              FilteringTextInputFormatter.allow(
                                RegExp(r'[0-9,]'),
                              ),
                            ],
                            decoration: InputDecoration(
                              hintText: formatQty(r.stock),
                              isDense: true,
                            ),
                          ),
                        ),
                      );
                    },
                  ),
          ),
        ],
      ),
      bottomNavigationBar: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: FilledButton(
            style: FilledButton.styleFrom(
              minimumSize: const Size.fromHeight(48),
            ),
            onPressed: _saving ? null : _save,
            child: Text(l.opnameSave),
          ),
        ),
      ),
    );
  }
}
