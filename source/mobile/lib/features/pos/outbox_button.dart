import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/money/money.dart';
import '../../l10n/gen/app_localizations.dart';
import 'outbox.dart';

/// Lencana jumlah nota offline yang menunggu sinkron; ketuk untuk melihat daftar. Hilang bila antrean kosong.
class OutboxButton extends ConsumerWidget {
  const OutboxButton({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final list = ref.watch(outboxProvider);
    if (list.isEmpty) return const SizedBox.shrink();
    final l = AppLocalizations.of(context);
    final bad = list.any((s) => s.failed);
    return IconButton(
      tooltip: l.outboxTitle,
      icon: Badge(
        label: Text('${list.length}'),
        child: Icon(bad ? Icons.sync_problem : Icons.cloud_upload_outlined),
      ),
      onPressed: () => showModalBottomSheet<void>(
        context: context,
        isScrollControlled: true,
        builder: (_) => const _OutboxSheet(),
      ),
    );
  }
}

class _OutboxSheet extends ConsumerWidget {
  const _OutboxSheet();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final list = ref.watch(outboxProvider);
    final box = ref.read(outboxProvider.notifier);
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(l.outboxTitle, style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 8),
            Flexible(
              child: ListView(
                shrinkWrap: true,
                children: [
                  for (final s in list)
                    ListTile(
                      contentPadding: EdgeInsets.zero,
                      title: Text('${s.docNo} · ${formatMoney(dec(s.total))}'),
                      subtitle: Text(
                        s.failed
                            ? l.outboxRejected(s.error!)
                            : '${s.at.hour.toString().padLeft(2, '0')}:${s.at.minute.toString().padLeft(2, '0')}',
                      ),
                      trailing: s.failed
                          ? IconButton(
                              tooltip: l.outboxDelete,
                              icon: const Icon(Icons.delete_outline),
                              onPressed: () => box.remove(s.key),
                            )
                          : null,
                    ),
                ],
              ),
            ),
            const SizedBox(height: 8),
            FilledButton.icon(
              icon: const Icon(Icons.sync),
              label: Text(
                list.any((s) => s.failed) ? l.outboxRetry : l.outboxSendNow,
              ),
              onPressed: () => box.flush(retryFailed: true),
            ),
          ],
        ),
      ),
    );
  }
}
