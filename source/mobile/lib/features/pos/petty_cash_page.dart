import 'package:decimal/decimal.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:uuid/uuid.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'approver_fields.dart';
import 'pos_repository.dart';
import 'shift_models.dart';

/// Kas kecil kasir: uang keluar/masuk laci di luar penjualan pada shift terbuka. Ikut dihitung server di rekap
/// tutup shift. Kas keluar di atas batas outlet butuh PIN penyetuju (`shift_close`).
Future<void> showPettyCashPage(BuildContext context, String shiftId) =>
    Navigator.of(context).push<void>(
      MaterialPageRoute(
        fullscreenDialog: true,
        builder: (_) => _PettyCashPage(shiftId: shiftId),
      ),
    );

class _PettyCashPage extends ConsumerStatefulWidget {
  const _PettyCashPage({required this.shiftId});

  final String shiftId;

  @override
  ConsumerState<_PettyCashPage> createState() => _PettyCashPageState();
}

enum _Cat { operational, meal, transport, supplies, change, misc }

class _PettyCashPageState extends ConsumerState<_PettyCashPage> {
  ShiftRecap? _recap;
  final _amount = TextEditingController();
  final _note = TextEditingController();
  final _approverKey = GlobalKey<ApproverFieldsState>();
  bool _out = true;
  _Cat _cat = _Cat.operational;
  Approver? _approver;
  String _pin = '';
  bool _busy = false;
  bool _saved = false;
  ApiError? _error;
  // Satu kunci per percobaan simpan; dibuat ulang setelah sukses (kirim ulang karena jaringan putus tidak ganda).
  String _key = 'pc-${const Uuid().v4()}';

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _amount.dispose();
    _note.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final r = await ref
          .read(posRepositoryProvider)
          .shiftRecap(widget.shiftId);
      if (mounted) setState(() => _recap = r);
    } on ApiError catch (e) {
      if (mounted) setState(() => _error = e);
    }
  }

  Decimal get _value => parseInput(_amount.text);

  bool get _open => _recap != null && !_recap!.closed;

  bool get _needApproval =>
      _out && _recap != null && _value > _recap!.pettyCashLimit;

  bool get _valid =>
      _open &&
      _value > Decimal.zero &&
      (!_needApproval ||
          (_approver != null && RegExp(r'^\d{6}$').hasMatch(_pin)));

  String _catLabel(AppLocalizations l, _Cat c) => switch (c) {
    _Cat.operational => l.pettyCatOperational,
    _Cat.meal => l.pettyCatMeal,
    _Cat.transport => l.pettyCatTransport,
    _Cat.supplies => l.pettyCatSupplies,
    _Cat.change => l.pettyCatChange,
    _Cat.misc => l.pettyCatMisc,
  };

  Future<void> _submit(AppLocalizations l) async {
    if (!_valid || _busy) return;
    setState(() {
      _busy = true;
      _error = null;
      _saved = false;
    });
    try {
      await ref
          .read(posRepositoryProvider)
          .addPettyCash(
            out: _out,
            amount: decToApi(_value, scale: 2),
            category: _catLabel(l, _cat),
            note: _note.text.trim(),
            idempotencyKey: _key,
            approverId: _needApproval ? _approver!.id : null,
            pin: _needApproval ? _pin : null,
          );
      _key = 'pc-${const Uuid().v4()}';
      _amount.clear();
      _note.clear();
      _approverKey.currentState?.clearPin();
      if (mounted) setState(() => _saved = true);
      await _load();
    } on ApiError catch (e) {
      _approverKey.currentState?.clearPin();
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _void(AppLocalizations l, PettyCash m) async {
    if (_busy) return;
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        content: Text(l.pettyVoidConfirm),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: Text(MaterialLocalizations.of(ctx).cancelButtonLabel),
          ),
          FilledButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: Text(l.pettyVoid),
          ),
        ],
      ),
    );
    if (ok != true) return;
    setState(() {
      _busy = true;
      _error = null;
      _saved = false;
    });
    try {
      await ref.read(posRepositoryProvider).voidPettyCash(m.id);
      await _load();
    } on ApiError catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(l.pettyTitle)),
      body: SafeArea(
        child: _recap == null
            ? (_error == null
                  ? const Center(child: CircularProgressIndicator())
                  : Center(
                      child: Padding(
                        padding: const EdgeInsets.all(24),
                        child: Column(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Text(
                              _error!.message(l),
                              style: TextStyle(color: context.pal.danger),
                              textAlign: TextAlign.center,
                            ),
                            const SizedBox(height: 12),
                            OutlinedButton(
                              onPressed: _load,
                              child: const Text('OK'),
                            ),
                          ],
                        ),
                      ),
                    ))
            : _buildBody(context, l, _recap!),
      ),
    );
  }

  Widget _buildBody(BuildContext context, AppLocalizations l, ShiftRecap r) {
    final pal = context.pal;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Text(l.pettyBody, style: TextStyle(color: pal.textTertiary)),
        const SizedBox(height: 14),
        if (_open) ...[
          SegmentedButton<bool>(
            segments: [
              ButtonSegment(
                value: true,
                icon: const Icon(Icons.remove),
                label: Text(l.pettyOut),
              ),
              ButtonSegment(
                value: false,
                icon: const Icon(Icons.add),
                label: Text(l.pettyIn),
              ),
            ],
            selected: {_out},
            onSelectionChanged: _busy
                ? null
                : (s) => setState(() => _out = s.first),
          ),
          const SizedBox(height: 14),
          TextField(
            controller: _amount,
            enabled: !_busy,
            keyboardType: const TextInputType.numberWithOptions(decimal: true),
            inputFormatters: [
              FilteringTextInputFormatter.allow(RegExp(r'[0-9.,]')),
            ],
            onChanged: (_) => setState(() => _saved = false),
            style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w800),
            decoration: InputDecoration(
              labelText: l.pettyAmount,
              prefixText: 'Rp ',
            ),
          ),
          const SizedBox(height: 14),
          Text(
            l.pettyCategory,
            style: TextStyle(
              fontWeight: FontWeight.w600,
              color: pal.textTertiary,
            ),
          ),
          const SizedBox(height: 6),
          Wrap(
            spacing: 8,
            runSpacing: 4,
            children: [
              for (final c in _Cat.values)
                ChoiceChip(
                  label: Text(_catLabel(l, c)),
                  selected: _cat == c,
                  onSelected: _busy ? null : (_) => setState(() => _cat = c),
                ),
            ],
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _note,
            enabled: !_busy,
            maxLength: 300,
            decoration: InputDecoration(
              labelText: l.pettyNote,
              counterText: '',
            ),
          ),
          if (_needApproval) ...[
            const SizedBox(height: 12),
            Container(
              padding: const EdgeInsets.all(10),
              decoration: BoxDecoration(
                color: pal.warningSoft,
                borderRadius: BorderRadius.circular(10),
              ),
              child: Text(
                l.pettyNeedApproval(formatMoney(r.pettyCashLimit)),
                style: TextStyle(color: pal.warningText, fontSize: 13),
              ),
            ),
            const SizedBox(height: 12),
            ApproverFields(
              key: _approverKey,
              purpose: 'shift_close',
              enabled: !_busy,
              onChanged: (a, pin) =>
                  WidgetsBinding.instance.addPostFrameCallback((_) {
                    if (!mounted) return;
                    if (a?.id != _approver?.id || pin != _pin) {
                      setState(() {
                        _approver = a;
                        _pin = pin;
                      });
                    }
                  }),
            ),
          ],
          if (_saved) ...[
            const SizedBox(height: 10),
            Text(l.pettySaved, style: TextStyle(color: pal.success)),
          ],
          if (_error != null) ...[
            const SizedBox(height: 10),
            Text(_error!.message(l), style: TextStyle(color: pal.danger)),
          ],
          const SizedBox(height: 16),
          FilledButton(
            style: FilledButton.styleFrom(
              minimumSize: const Size.fromHeight(50),
            ),
            onPressed: _valid && !_busy ? () => _submit(l) : null,
            child: _busy
                ? const SizedBox(
                    height: 20,
                    width: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : Text(l.pettySave),
          ),
          const SizedBox(height: 24),
        ] else if (_error != null) ...[
          Text(_error!.message(l), style: TextStyle(color: pal.danger)),
        ],
        Text(
          l.pettyHistory,
          style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15),
        ),
        const SizedBox(height: 6),
        if (r.pettyCash.isEmpty)
          Text(l.pettyEmpty, style: TextStyle(color: pal.textTertiary))
        else
          for (final m in r.pettyCash) _row(context, l, m),
      ],
    );
  }

  Widget _row(BuildContext context, AppLocalizations l, PettyCash m) {
    final pal = context.pal;
    final sub = [
      DateFormat('d MMM HH:mm').format(m.createdAt),
      if (m.approvedBy.isNotEmpty) l.pettyApprovedBy(m.approvedBy),
      if (m.voided) l.pettyVoided,
    ].join(' · ');
    final title = m.note.isEmpty ? m.category : '${m.category} · ${m.note}';
    return Opacity(
      opacity: m.voided ? 0.5 : 1,
      child: ListTile(
        contentPadding: EdgeInsets.zero,
        title: Text(
          title,
          maxLines: 2,
          overflow: TextOverflow.ellipsis,
          style: TextStyle(
            fontWeight: FontWeight.w600,
            decoration: m.voided ? TextDecoration.lineThrough : null,
          ),
        ),
        subtitle: Text(sub, style: TextStyle(color: pal.textTertiary)),
        trailing: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              '${m.out ? '−' : '+'}${formatMoney(m.amount)}',
              style: TextStyle(
                fontWeight: FontWeight.w800,
                color: m.out ? pal.danger : pal.success,
              ),
            ),
            if (_open && !m.voided)
              IconButton(
                tooltip: l.pettyVoid,
                icon: const Icon(Icons.cancel_outlined),
                onPressed: _busy ? null : () => _void(l, m),
              ),
          ],
        ),
      ),
    );
  }
}
