import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'item_thumb.dart';
import 'pos_controller.dart';
import 'pos_models.dart';
import 'pos_repository.dart';

/// Pintasan barang milik kasir ini (16 slot, tersimpan di server). Cabang tidak membedakan slot.
final shortcutsProvider =
    AsyncNotifierProvider<ShortcutsController, List<Shortcut>>(
      ShortcutsController.new,
    );

class ShortcutsController extends AsyncNotifier<List<Shortcut>> {
  @override
  Future<List<Shortcut>> build() => ref.read(posRepositoryProvider).shortcuts();

  Future<void> set(int slot, String itemId) async => state = AsyncData(
    await ref.read(posRepositoryProvider).setShortcut(slot, itemId),
  );

  Future<void> clear(int slot) async => state = AsyncData(
    await ref.read(posRepositoryProvider).clearShortcut(slot),
  );
}

/// Baris pintasan di atas kisi barang: ketuk = masuk keranjang; tombol atur membuka 16 slot.
class ShortcutsStrip extends ConsumerWidget {
  const ShortcutsStrip({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final list = ref.watch(shortcutsProvider).asData?.value ?? const [];
    return SizedBox(
      height: 84,
      child: Row(
        children: [
          Expanded(
            child: list.isEmpty
                ? Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 14),
                    child: Align(
                      alignment: Alignment.centerLeft,
                      child: Text(
                        l.shortcutsEmpty,
                        style: TextStyle(
                          color: pal.textTertiary,
                          fontSize: 12.5,
                        ),
                      ),
                    ),
                  )
                : ListView.separated(
                    scrollDirection: Axis.horizontal,
                    padding: const EdgeInsets.fromLTRB(12, 4, 4, 4),
                    itemCount: list.length,
                    separatorBuilder: (_, _) => const SizedBox(width: 8),
                    itemBuilder: (_, i) => _ShortcutTile(shortcut: list[i]),
                  ),
          ),
          IconButton(
            tooltip: l.shortcutsManage,
            icon: const Icon(Icons.dashboard_customize_outlined),
            onPressed: () => showShortcutsSheet(context),
          ),
        ],
      ),
    );
  }
}

class _ShortcutTile extends ConsumerWidget {
  const _ShortcutTile({required this.shortcut});

  final Shortcut shortcut;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final pal = context.pal;
    final s = shortcut;
    return Opacity(
      opacity: s.active ? 1 : 0.45,
      child: Material(
        color: pal.surface,
        borderRadius: BorderRadius.circular(12),
        child: InkWell(
          borderRadius: BorderRadius.circular(12),
          onTap: s.active
              ? () => ref.read(cartProvider.notifier).add(s.item)
              : null,
          child: Container(
            width: 150,
            padding: const EdgeInsets.all(6),
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: pal.border),
            ),
            child: Row(
              children: [
                ClipRRect(
                  borderRadius: BorderRadius.circular(8),
                  child: SizedBox(
                    width: 42,
                    height: 42,
                    child: ItemThumb(
                      itemId: s.item.id,
                      imageId: s.item.mainImageId,
                    ),
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Column(
                    mainAxisAlignment: MainAxisAlignment.center,
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        s.item.name,
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontSize: 11.5,
                          fontWeight: FontWeight.w600,
                          height: 1.15,
                        ),
                      ),
                      Text(
                        formatMoney(s.item.price),
                        style: TextStyle(
                          fontSize: 11.5,
                          fontWeight: FontWeight.w800,
                          color: pal.accent,
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

Future<void> showShortcutsSheet(BuildContext context) =>
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => const _ShortcutsSheet(),
    );

class _ShortcutsSheet extends ConsumerStatefulWidget {
  const _ShortcutsSheet();

  @override
  ConsumerState<_ShortcutsSheet> createState() => _ShortcutsSheetState();
}

class _ShortcutsSheetState extends ConsumerState<_ShortcutsSheet> {
  ApiError? _error;
  bool _busy = false;

  Future<void> _run(Future<void> Function() fn) async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await fn();
    } on ApiError catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _assign(int slot) async {
    final item = await showModalBottomSheet<PosItem>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => const _ItemPicker(),
    );
    if (item == null || !mounted) return;
    await _run(() => ref.read(shortcutsProvider.notifier).set(slot, item.id));
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final list = ref.watch(shortcutsProvider).asData?.value ?? const [];
    final bySlot = {for (final s in list) s.slot: s};
    return SizedBox(
      height: MediaQuery.sizeOf(context).height * 0.7,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 4),
            child: Text(
              l.shortcutsTitle,
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
            child: Text(
              l.shortcutsHint,
              style: TextStyle(color: pal.textTertiary, fontSize: 12.5),
            ),
          ),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
              child: Text(
                _error!.message(l),
                style: TextStyle(color: pal.danger),
              ),
            ),
          Expanded(
            child: ListView.separated(
              itemCount: shortcutSlots,
              separatorBuilder: (_, _) => Divider(height: 1, color: pal.border),
              itemBuilder: (_, i) {
                final slot = i + 1;
                final s = bySlot[slot];
                return ListTile(
                  enabled: !_busy,
                  leading: CircleAvatar(
                    radius: 15,
                    backgroundColor: pal.primarySoft,
                    child: Text(
                      '$slot',
                      style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w700,
                        color: pal.primary,
                      ),
                    ),
                  ),
                  title: Text(
                    s?.item.name ?? l.shortcutsSlotEmpty,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontWeight: FontWeight.w600,
                      color: s == null ? pal.textTertiary : null,
                    ),
                  ),
                  subtitle: s == null
                      ? null
                      : Text(
                          s.active
                              ? formatMoney(s.item.price)
                              : l.shortcutsInactive,
                        ),
                  trailing: s == null
                      ? const Icon(Icons.add)
                      : IconButton(
                          tooltip: l.shortcutsClear,
                          icon: Icon(Icons.close, color: pal.danger),
                          onPressed: _busy
                              ? null
                              : () => _run(
                                  () => ref
                                      .read(shortcutsProvider.notifier)
                                      .clear(slot),
                                ),
                        ),
                  onTap: _busy ? null : () => _assign(slot),
                );
              },
            ),
          ),
        ],
      ),
    );
  }
}

/// Cari barang untuk dipasang ke slot (pencarian kasir yang sama dengan katalog).
class _ItemPicker extends ConsumerStatefulWidget {
  const _ItemPicker();

  @override
  ConsumerState<_ItemPicker> createState() => _ItemPickerState();
}

class _ItemPickerState extends ConsumerState<_ItemPicker> {
  final _search = TextEditingController();
  Timer? _debounce;
  int _seq = 0;
  List<PosItem> _items = const [];
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
      final r = await ref.read(posRepositoryProvider).search(q);
      if (my != _seq || !mounted) return;
      setState(() => _items = r);
    } on ApiError catch (e) {
      if (my == _seq && mounted) setState(() => _error = e);
    } finally {
      if (my == _seq && mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      child: SizedBox(
        height: MediaQuery.sizeOf(context).height * 0.7,
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
              child: TextField(
                controller: _search,
                autofocus: true,
                onChanged: (q) {
                  _debounce?.cancel();
                  _debounce = Timer(
                    const Duration(milliseconds: 300),
                    () => _load(q.trim()),
                  );
                },
                decoration: InputDecoration(
                  hintText: l.posSearchHint,
                  prefixIcon: Icon(Icons.search, color: pal.textTertiary),
                ),
              ),
            ),
            Expanded(
              child: _error != null
                  ? Center(
                      child: Text(
                        _error!.message(l),
                        style: TextStyle(color: pal.danger),
                      ),
                    )
                  : _loading && _items.isEmpty
                  ? const Center(child: CircularProgressIndicator())
                  : _items.isEmpty
                  ? Center(
                      child: Text(
                        l.posSearchEmpty,
                        style: TextStyle(color: pal.textTertiary),
                      ),
                    )
                  : ListView.separated(
                      itemCount: _items.length,
                      separatorBuilder: (_, _) =>
                          Divider(height: 1, color: pal.border),
                      itemBuilder: (_, i) => ListTile(
                        title: Text(
                          _items[i].name,
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                        ),
                        subtitle: Text(formatMoney(_items[i].price)),
                        onTap: () => Navigator.of(context).pop(_items[i]),
                      ),
                    ),
            ),
          ],
        ),
      ),
    );
  }
}
