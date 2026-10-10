import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';

import '../../core/money/money.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'pending_controller.dart';
import 'pos_controller.dart';

/// Tunda keranjang sekarang dengan keterangan (usulan: nama member). Mengembalikan true bila berhasil ditunda.
Future<bool> showHoldDialog(BuildContext context, WidgetRef ref) async {
  final l = AppLocalizations.of(context);
  final cart = ref.read(cartProvider);
  final pending = ref.read(pendingProvider.notifier);
  if (cart.isEmpty) return false;
  if (pending.isFull) {
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(l.pendingFull(maxPending))));
    return false;
  }
  final text = await showDialog<String>(
    context: context,
    builder: (_) => _HoldDialog(initial: cart.member?.name ?? ''),
  );
  if (text == null || !context.mounted) return false;
  final no = pending.hold(text);
  if (no == null) return false;
  ScaffoldMessenger.of(context)
      .showSnackBar(SnackBar(content: Text(l.pendingSaved(no))));
  return true;
}

class _HoldDialog extends StatefulWidget {
  const _HoldDialog({required this.initial});

  final String initial;

  @override
  State<_HoldDialog> createState() => _HoldDialogState();
}

class _HoldDialogState extends State<_HoldDialog> {
  late final TextEditingController _label = TextEditingController(
    text: widget.initial,
  );

  @override
  void dispose() {
    _label.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return AlertDialog(
      title: Text(l.pendingHoldTitle),
      content: TextField(
        controller: _label,
        autofocus: true,
        maxLength: maxPendingLabel,
        textInputAction: TextInputAction.done,
        onSubmitted: (_) => Navigator.of(context).pop(_label.text),
        decoration: InputDecoration(
          labelText: l.pendingLabel,
          hintText: l.pendingLabelHint,
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: Text(MaterialLocalizations.of(context).cancelButtonLabel),
        ),
        FilledButton(
          style: FilledButton.styleFrom(minimumSize: const Size(0, 44)),
          onPressed: () => Navigator.of(context).pop(_label.text),
          child: Text(l.pendingHold),
        ),
      ],
    );
  }
}

/// Daftar nota pending kasir + cabang ini. Mengembalikan true bila sebuah nota dibuka ke keranjang.
Future<bool> showPendingSheet(BuildContext context) async {
  final r = await showModalBottomSheet<bool>(
    context: context,
    isScrollControlled: true,
    useSafeArea: true,
    showDragHandle: true,
    builder: (_) => const _PendingSheet(),
  );
  return r == true;
}

class _PendingSheet extends ConsumerWidget {
  const _PendingSheet();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final list = ref.watch(pendingProvider);
    return SizedBox(
      height: MediaQuery.sizeOf(context).height * 0.65,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 4),
            child: Text(
              l.pendingTitle(list.length, maxPending),
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
            child: Text(
              l.pendingHint,
              style: TextStyle(color: pal.textTertiary, fontSize: 12.5),
            ),
          ),
          Expanded(
            child: list.isEmpty
                ? Center(
                    child: Text(
                      l.pendingEmpty,
                      style: TextStyle(color: pal.textTertiary),
                    ),
                  )
                : ListView.separated(
                    itemCount: list.length,
                    separatorBuilder: (_, _) =>
                        Divider(height: 1, color: pal.border),
                    itemBuilder: (_, i) {
                      final n = list[i];
                      return ListTile(
                        title: Text(
                          n.label.isEmpty
                              ? l.pendingNo(n.no)
                              : '${l.pendingNo(n.no)} · ${n.label}',
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(fontWeight: FontWeight.w600),
                        ),
                        subtitle: Text(
                          [
                            l.pendingLines(n.lineCount),
                            n.cart.member?.name ?? l.memberGeneral,
                            DateFormat('d MMM HH:mm').format(n.at),
                          ].join(' · '),
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                        ),
                        trailing: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            if (n.total != null)
                              Text(
                                formatMoney(n.total!),
                                style: const TextStyle(
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                            IconButton(
                              tooltip: l.pendingDelete,
                              icon: Icon(
                                Icons.delete_outline,
                                color: pal.danger,
                              ),
                              onPressed: () => _confirmDelete(context, ref, n),
                            ),
                          ],
                        ),
                        onTap: () {
                          final swapped = ref
                              .read(pendingProvider.notifier)
                              .open(n);
                          ScaffoldMessenger.of(context).showSnackBar(
                            SnackBar(
                              content: Text(
                                swapped == null
                                    ? l.pendingOpened(n.no)
                                    : l.pendingOpenedSwapped(n.no, swapped),
                              ),
                            ),
                          );
                          Navigator.of(context).pop(true);
                        },
                      );
                    },
                  ),
          ),
        ],
      ),
    );
  }

  Future<void> _confirmDelete(
    BuildContext context,
    WidgetRef ref,
    PendingNote n,
  ) async {
    final l = AppLocalizations.of(context);
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        content: Text(l.pendingDeleteAsk(n.no)),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: Text(MaterialLocalizations.of(ctx).cancelButtonLabel),
          ),
          FilledButton(
            style: FilledButton.styleFrom(minimumSize: const Size(0, 44)),
            onPressed: () => Navigator.of(ctx).pop(true),
            child: Text(l.pendingDelete),
          ),
        ],
      ),
    );
    if (ok == true) ref.read(pendingProvider.notifier).delete(n);
  }
}
