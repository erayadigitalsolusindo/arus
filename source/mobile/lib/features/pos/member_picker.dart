import 'dart:async';

import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'pos_controller.dart';
import 'pos_models.dart';
import 'pos_repository.dart';

/// Pilih/ganti/lepas member nota. Hasilnya langsung ditulis ke keranjang (`CartController.setMember`).
Future<void> showMemberPicker(BuildContext context) =>
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => const _MemberPicker(),
    );

class _MemberPicker extends ConsumerStatefulWidget {
  const _MemberPicker();

  @override
  ConsumerState<_MemberPicker> createState() => _MemberPickerState();
}

class _MemberPickerState extends ConsumerState<_MemberPicker> {
  final _search = TextEditingController();
  Timer? _debounce;
  int _seq = 0;
  List<MemberInfo> _results = const [];
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
      final r = await ref.read(posRepositoryProvider).memberLookup(q);
      if (my != _seq || !mounted) return;
      setState(() => _results = r);
    } on ApiError catch (e) {
      if (my == _seq && mounted) setState(() => _error = e);
    } finally {
      if (my == _seq && mounted) setState(() => _loading = false);
    }
  }

  void _onChanged(String q) {
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 250), () => _load(q.trim()));
  }

  void _pick(MemberInfo? m) {
    ref.read(cartProvider.notifier).setMember(m);
    Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final current = ref.watch(cartProvider.select((c) => c.member));
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
                textInputAction: TextInputAction.search,
                onChanged: _onChanged,
                decoration: InputDecoration(
                  hintText: l.memberSearchHint,
                  prefixIcon: Icon(Icons.search, color: pal.textTertiary),
                ),
              ),
            ),
            if (current != null)
              ListTile(
                leading: Icon(Icons.person_remove_outlined, color: pal.danger),
                title: Text(
                  l.memberRemove,
                  style: TextStyle(
                    color: pal.danger,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                subtitle: Text(current.name),
                onTap: () => _pick(null),
              ),
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
                        l.memberSearchEmpty,
                        style: TextStyle(color: pal.textTertiary),
                      ),
                    )
                  : ListView.separated(
                      itemCount: _results.length,
                      separatorBuilder: (_, _) =>
                          Divider(height: 1, color: pal.border),
                      itemBuilder: (_, i) {
                        final m = _results[i];
                        return ListTile(
                          leading: CircleAvatar(
                            backgroundColor: pal.primarySoft,
                            child: Icon(Icons.person, color: pal.primary),
                          ),
                          title: Text(
                            m.name,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: const TextStyle(fontWeight: FontWeight.w600),
                          ),
                          subtitle: Text(
                            m.phone.isEmpty ? m.code : '${m.code} · ${m.phone}',
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                          ),
                          trailing: Column(
                            mainAxisAlignment: MainAxisAlignment.center,
                            crossAxisAlignment: CrossAxisAlignment.end,
                            children: [
                              Text(
                                l.memberPoints(m.points),
                                style: const TextStyle(
                                  fontWeight: FontWeight.w600,
                                  fontSize: 12.5,
                                ),
                              ),
                              if (m.level.isNotEmpty)
                                Text(
                                  m.level,
                                  style: TextStyle(
                                    fontSize: 11,
                                    color: pal.textTertiary,
                                  ),
                                ),
                            ],
                          ),
                          selected: current?.id == m.id,
                          onTap: () => _pick(m),
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

/// Dialog tukar poin → potongan nota. Batas = `CartState.redeemCap` (saldo poin dan nilai belanja); server tetap memvalidasi.
Future<void> showRedeemDialog(BuildContext context) =>
    showDialog<void>(context: context, builder: (_) => const _RedeemDialog());

class _RedeemDialog extends ConsumerStatefulWidget {
  const _RedeemDialog();

  @override
  ConsumerState<_RedeemDialog> createState() => _RedeemDialogState();
}

class _RedeemDialogState extends ConsumerState<_RedeemDialog> {
  late final TextEditingController _points;

  @override
  void initState() {
    super.initState();
    final p = ref.read(cartProvider).redeemPoints;
    _points = TextEditingController(text: p > 0 ? '$p' : '');
  }

  @override
  void dispose() {
    _points.dispose();
    super.dispose();
  }

  int get _value => int.tryParse(_points.text) ?? 0;

  void _apply(int v) {
    ref.read(cartProvider.notifier).setRedeem(v);
    Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final cart = ref.watch(cartProvider);
    final member = cart.member;
    final cap = cart.redeemCap;
    final pv = member?.pointValue ?? Decimal.zero;
    final value = _value;
    return AlertDialog(
      title: Text(l.memberRedeemTitle),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (member != null)
            Text(
              member.name,
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
          const SizedBox(height: 6),
          if (cap <= 0 && cart.redeemPoints <= 0)
            Text(
              l.memberRedeemNone,
              style: TextStyle(color: pal.textTertiary, fontSize: 13),
            )
          else ...[
            Text(
              l.memberRedeemAvailable(
                cap,
                formatMoney(pv * Decimal.fromInt(cap)),
              ),
              style: TextStyle(color: pal.textTertiary, fontSize: 13),
            ),
            const SizedBox(height: 10),
            Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _points,
                    autofocus: true,
                    keyboardType: TextInputType.number,
                    inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                    decoration: InputDecoration(labelText: l.memberRedeemField),
                    onChanged: (_) => setState(() {}),
                    onSubmitted: (_) => _apply(value),
                  ),
                ),
                const SizedBox(width: 8),
                OutlinedButton(
                  style: OutlinedButton.styleFrom(
                    minimumSize: const Size(0, 48),
                  ),
                  onPressed: cap <= 0
                      ? null
                      : () => setState(() => _points.text = '$cap'),
                  child: Text(l.memberRedeemAll),
                ),
              ],
            ),
            if (value > 0)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Text(
                  l.memberRedeemValue(formatMoney(pv * Decimal.fromInt(value))),
                  style: TextStyle(
                    color: pal.success,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
          ],
        ],
      ),
      actions: [
        if (cart.redeemPoints > 0)
          TextButton(
            onPressed: () => _apply(0),
            child: Text(l.memberRedeemReset),
          ),
        FilledButton(
          style: FilledButton.styleFrom(minimumSize: const Size(0, 44)),
          onPressed: () => _apply(value),
          child: Text(l.memberRedeemDone),
        ),
      ],
    );
  }
}
