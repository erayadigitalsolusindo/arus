import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_error.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'pos_controller.dart';
import 'pos_models.dart';
import 'pos_repository.dart';

/// Pilih salesman nota (opsional; kosong = Umum). Hasilnya langsung ditulis ke keranjang.
Future<void> showSalespersonPicker(BuildContext context) =>
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => const _SalespersonPicker(),
    );

class _SalespersonPicker extends ConsumerStatefulWidget {
  const _SalespersonPicker();

  @override
  ConsumerState<_SalespersonPicker> createState() => _SalespersonPickerState();
}

class _SalespersonPickerState extends ConsumerState<_SalespersonPicker> {
  final _search = TextEditingController();
  Timer? _debounce;
  int _seq = 0;
  List<Salesperson> _results = const [];
  bool _loading = true;
  ApiError? _error;

  @override
  void initState() {
    super.initState();
    _load('');
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _search.dispose();
    super.dispose();
  }

  Future<void> _load(String q) async {
    final my = ++_seq;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final r = await ref.read(posRepositoryProvider).salespeople(q);
      if (my != _seq || !mounted) return;
      setState(() => _results = r);
    } on ApiError catch (e) {
      if (my == _seq && mounted) setState(() => _error = e);
    } finally {
      if (my == _seq && mounted) setState(() => _loading = false);
    }
  }

  void _pick(Salesperson? s) {
    ref.read(cartProvider.notifier).setSalesperson(s);
    Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final current = ref.watch(cartProvider.select((c) => c.salesperson));
    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      child: SizedBox(
        height: MediaQuery.sizeOf(context).height * 0.6,
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
              child: TextField(
                controller: _search,
                onChanged: (q) {
                  _debounce?.cancel();
                  _debounce = Timer(
                    const Duration(milliseconds: 250),
                    () => _load(q.trim()),
                  );
                },
                decoration: InputDecoration(
                  hintText: l.salespersonSearch,
                  prefixIcon: Icon(Icons.search, color: pal.textTertiary),
                ),
              ),
            ),
            ListTile(
              leading: const Icon(Icons.groups_outlined),
              title: Text(l.salespersonNone),
              selected: current == null,
              onTap: () => _pick(null),
            ),
            Divider(height: 1, color: pal.border),
            Expanded(
              child: _error != null
                  ? Center(
                      child: Text(
                        _error!.message(l),
                        style: TextStyle(color: pal.danger),
                      ),
                    )
                  : _loading && _results.isEmpty
                  ? const Center(child: CircularProgressIndicator())
                  : _results.isEmpty
                  ? Center(
                      child: Text(
                        l.salespersonEmpty,
                        style: TextStyle(color: pal.textTertiary),
                      ),
                    )
                  : ListView.separated(
                      itemCount: _results.length,
                      separatorBuilder: (_, _) =>
                          Divider(height: 1, color: pal.border),
                      itemBuilder: (_, i) {
                        final s = _results[i];
                        return ListTile(
                          leading: const Icon(Icons.badge_outlined),
                          title: Text(s.name),
                          selected: current?.id == s.id,
                          onTap: () => _pick(s),
                        );
                      },
                    ),
            ),
          ],
        ),
      ),
    );
  }
}
