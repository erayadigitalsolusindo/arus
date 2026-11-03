import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/session/session_controller.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import '../pos/add_item_sheet.dart';
import '../pos/pos_controller.dart';
import '../pos/pos_models.dart';
import '../pos/pos_repository.dart';
import 'manage_repository.dart';

/// Kelola barang ringkas: cari, tambah, ubah, arsipkan/aktifkan kembali. Tombol mengikuti izin `items`
/// (view/create/update); barang tidak dihapus permanen supaya riwayat transaksi dan laporan tetap utuh.
Future<void> showItemManagePage(BuildContext context) =>
    Navigator.of(context)
        .push<void>(MaterialPageRoute(builder: (_) => const ItemManagePage()));

class ItemManagePage extends ConsumerStatefulWidget {
  const ItemManagePage({super.key});

  @override
  ConsumerState<ItemManagePage> createState() => _ItemManagePageState();
}

class _ItemManagePageState extends ConsumerState<ItemManagePage> {
  final _search = TextEditingController();
  final _scroll = ScrollController();
  Timer? _debounce;
  int _seq = 0;
  bool _archived = false;
  bool _loading = true;
  bool _loadingMore = false;
  String _cursor = '';
  List<PosItem> _items = const [];
  List<ArchivedItem> _old = const [];
  ApiError? _error;

  @override
  void initState() {
    super.initState();
    _scroll.addListener(() {
      if (_scroll.position.pixels > _scroll.position.maxScrollExtent - 300) {
        _more();
      }
    });
    _load();
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _search.dispose();
    _scroll.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    final my = ++_seq;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final q = _search.text.trim();
      if (_archived) {
        final r = await ref.read(manageRepositoryProvider).archivedItems(q);
        if (my == _seq && mounted) {
          setState(() {
            _old = r;
            _cursor = '';
          });
        }
      } else {
        final r = await ref.read(posRepositoryProvider).searchPage(q);
        if (my == _seq && mounted) {
          setState(() {
            _items = r.items;
            _cursor = r.nextCursor;
          });
        }
      }
    } on ApiError catch (e) {
      if (my == _seq && mounted) setState(() => _error = e);
    } finally {
      if (my == _seq && mounted) setState(() => _loading = false);
    }
  }

  Future<void> _more() async {
    if (_archived || _cursor.isEmpty || _loadingMore || _loading) return;
    final my = _seq;
    setState(() => _loadingMore = true);
    try {
      final r = await ref
          .read(posRepositoryProvider)
          .searchPage(_search.text.trim(), cursor: _cursor);
      if (my != _seq || !mounted) return;
      setState(() {
        _items = [..._items, ...r.items];
        _cursor = r.nextCursor;
      });
    } on ApiError {
      // Muat berikutnya dicoba lagi saat digulir; daftar yang ada tetap terlihat.
    } finally {
      if (mounted) setState(() => _loadingMore = false);
    }
  }

  void _onChanged(String _) {
    setState(() {});
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 300), _load);
  }

  void _snack(String text) => ScaffoldMessenger.of(context)
    ..clearSnackBars()
    ..showSnackBar(SnackBar(content: Text(text)));

  Future<void> _add() async {
    final l = AppLocalizations.of(context);
    final r = await showAddItemSheet(context, name: _search.text.trim());
    if (r == null || !mounted) return;
    ref.read(catalogVersionProvider.notifier).bump();
    _snack(l.itemSaved(r.item.name));
    _load();
  }

  Future<void> _edit(String id) async {
    final l = AppLocalizations.of(context);
    final r = await showAddItemSheet(context, editId: id);
    if (r == null || !mounted) return;
    ref.read(catalogVersionProvider.notifier).bump();
    _snack(l.itemsUpdated(r.item.name));
    _load();
  }

  Future<void> _setActive(String id, String name, bool active) async {
    final l = AppLocalizations.of(context);
    if (!active) {
      final ok = await showDialog<bool>(
        context: context,
        builder: (_) => AlertDialog(
          title: Text(l.itemsArchive),
          content: Text(l.itemsArchiveAsk(name)),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(context).pop(false),
              child: Text(l.commonCancel),
            ),
            FilledButton(
              onPressed: () => Navigator.of(context).pop(true),
              child: Text(l.itemsArchive),
            ),
          ],
        ),
      );
      if (ok != true) return;
    }
    try {
      await ref.read(manageRepositoryProvider).setItemActive(id, active);
      if (!mounted) return;
      ref.read(catalogVersionProvider.notifier).bump();
      _snack(active ? l.itemsRestored(name) : l.itemsArchived(name));
      _load();
    } on ApiError catch (e) {
      if (mounted) _snack(e.message(l));
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final session = ref.watch(sessionProvider);
    final perms = session is SessionSignedIn
        ? session.profile.permissions
        : null;
    final canCreate = perms?.can('items', 'create') ?? false;
    final canUpdate = perms?.can('items', 'update') ?? false;
    final empty = _archived ? _old.isEmpty : _items.isEmpty;
    return Scaffold(
      appBar: AppBar(title: Text(l.itemsTitle)),
      floatingActionButton: canCreate
          ? FloatingActionButton.small(
              tooltip: l.itemAddTooltip,
              onPressed: _add,
              child: const Icon(Icons.add),
            )
          : null,
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
              child: TextField(
                controller: _search,
                onChanged: _onChanged,
                textInputAction: TextInputAction.search,
                decoration: InputDecoration(
                  hintText: l.itemsSearch,
                  prefixIcon: const Icon(Icons.search),
                  isDense: true,
                  suffixIcon: _search.text.isEmpty
                      ? null
                      : IconButton(
                          icon: const Icon(Icons.close, size: 18),
                          onPressed: () {
                            _search.clear();
                            _onChanged('');
                          },
                        ),
                ),
              ),
            ),
            if (canUpdate)
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
                child: SizedBox(
                  width: double.infinity,
                  child: SegmentedButton<bool>(
                    showSelectedIcon: false,
                    style: const ButtonStyle(
                      visualDensity: VisualDensity.compact,
                    ),
                    segments: [
                      ButtonSegment(
                        value: false,
                        label: Text(l.itemsTabActive),
                      ),
                      ButtonSegment(
                        value: true,
                        label: Text(l.itemsTabArchived),
                      ),
                    ],
                    selected: {_archived},
                    onSelectionChanged: (s) {
                      setState(() => _archived = s.first);
                      _load();
                    },
                  ),
                ),
              ),
            if (_loading && !empty) const LinearProgressIndicator(minHeight: 2),
            Expanded(
              child: RefreshIndicator(
                onRefresh: _load,
                child: _error != null
                    ? _Msg(text: _error!.message(l), color: pal.danger)
                    : empty
                    ? (_loading
                          ? const Center(child: CircularProgressIndicator())
                          : _Msg(text: l.itemsEmpty, color: pal.textTertiary))
                    : ListView.separated(
                        controller: _scroll,
                        physics: const AlwaysScrollableScrollPhysics(),
                        padding: const EdgeInsets.only(bottom: 80),
                        itemCount: _archived
                            ? _old.length + 1
                            : _items.length + (_cursor.isNotEmpty ? 1 : 0),
                        separatorBuilder: (_, _) =>
                            Divider(height: 1, color: pal.border),
                        itemBuilder: (_, i) {
                          if (_archived) {
                            if (i == _old.length) {
                              return Padding(
                                padding: const EdgeInsets.all(16),
                                child: Text(
                                  l.itemsArchivedHint,
                                  style: TextStyle(
                                    fontSize: 12,
                                    color: pal.textTertiary,
                                  ),
                                ),
                              );
                            }
                            final it = _old[i];
                            return _Tile(
                              name: it.name,
                              sub: [
                                if (it.sku.isNotEmpty) it.sku,
                                if (it.unit.isNotEmpty) it.unit,
                              ].join(' · '),
                              price: formatMoney(it.price),
                              muted: true,
                              onTap: canUpdate
                                  ? () => _setActive(it.id, it.name, true)
                                  : null,
                              menu: canUpdate
                                  ? [
                                      _Act(
                                        l.itemsRestore,
                                        Icons.unarchive_outlined,
                                        () => _setActive(it.id, it.name, true),
                                      ),
                                    ]
                                  : const [],
                            );
                          }
                          if (i == _items.length) {
                            return const Padding(
                              padding: EdgeInsets.all(16),
                              child: Center(
                                child: SizedBox(
                                  width: 20,
                                  height: 20,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                  ),
                                ),
                              ),
                            );
                          }
                          final it = _items[i];
                          return _Tile(
                            name: it.name,
                            sub: [
                              if (it.sku.isNotEmpty) it.sku,
                              if (it.unit.isNotEmpty) it.unit,
                              if (it.isGoods)
                                l.itemsStock(formatQty(it.stockDisplay)),
                            ].join(' · '),
                            price: formatMoney(it.price),
                            onTap: canUpdate ? () => _edit(it.id) : null,
                            menu: canUpdate
                                ? [
                                    _Act(
                                      l.itemsEdit,
                                      Icons.edit_outlined,
                                      () => _edit(it.id),
                                    ),
                                    _Act(
                                      l.itemsArchive,
                                      Icons.archive_outlined,
                                      () => _setActive(it.id, it.name, false),
                                    ),
                                  ]
                                : const [],
                          );
                        },
                      ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Act {
  const _Act(this.label, this.icon, this.run);

  final String label;
  final IconData icon;
  final VoidCallback run;
}

class _Tile extends StatelessWidget {
  const _Tile({
    required this.name,
    required this.sub,
    required this.price,
    required this.menu,
    this.onTap,
    this.muted = false,
  });

  final String name;
  final String sub;
  final String price;
  final List<_Act> menu;
  final VoidCallback? onTap;
  final bool muted;

  @override
  Widget build(BuildContext context) {
    final pal = context.pal;
    return ListTile(
      dense: true,
      onTap: onTap,
      title: Text(
        name,
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: TextStyle(
          fontWeight: FontWeight.w600,
          color: muted ? pal.textTertiary : null,
        ),
      ),
      subtitle: sub.isEmpty
          ? null
          : Text(
              sub,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(fontSize: 12, color: pal.textTertiary),
            ),
      trailing: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(price, style: const TextStyle(fontWeight: FontWeight.w700)),
          if (menu.isNotEmpty)
            PopupMenuButton<int>(
              padding: EdgeInsets.zero,
              icon: const Icon(Icons.more_vert, size: 20),
              onSelected: (i) => menu[i].run(),
              itemBuilder: (_) => [
                for (var i = 0; i < menu.length; i++)
                  PopupMenuItem(
                    value: i,
                    child: Row(
                      children: [
                        Icon(menu[i].icon, size: 18),
                        const SizedBox(width: 10),
                        Text(menu[i].label),
                      ],
                    ),
                  ),
              ],
            ),
        ],
      ),
    );
  }
}

class _Msg extends StatelessWidget {
  const _Msg({required this.text, required this.color});

  final String text;
  final Color color;

  @override
  Widget build(BuildContext context) => ListView(
    physics: const AlwaysScrollableScrollPhysics(),
    children: [
      Padding(
        padding: const EdgeInsets.all(48),
        child: Text(
          text,
          textAlign: TextAlign.center,
          style: TextStyle(color: color),
        ),
      ),
    ],
  );
}
