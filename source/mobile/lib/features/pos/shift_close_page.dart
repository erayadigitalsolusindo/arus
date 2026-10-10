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
import 'pos_controller.dart';
import 'pos_repository.dart';
import 'shift_models.dart';

/// Layar tutup shift. Kasir mengisi uang fisik per metode; rekap dan selisih dihitung server. Selisih ≠ 0 wajib
/// catatan + PIN penyetuju. Hasil `true` = kasir memilih membuka shift baru.
Future<bool?> showCloseShiftPage(BuildContext context, String shiftId) =>
    Navigator.of(context).push<bool>(
      MaterialPageRoute(
        fullscreenDialog: true,
        builder: (_) => _CloseShiftPage(shiftId: shiftId),
      ),
    );

class _CloseShiftPage extends ConsumerStatefulWidget {
  const _CloseShiftPage({required this.shiftId});

  final String shiftId;

  @override
  ConsumerState<_CloseShiftPage> createState() => _CloseShiftPageState();
}

class _CloseShiftPageState extends ConsumerState<_CloseShiftPage> {
  ShiftRecap? _recap;
  ShiftRecap? _done;
  final Map<String, TextEditingController> _counted = {};
  final _note = TextEditingController();
  final _pin = TextEditingController();
  List<Approver>? _approvers;
  bool _approversRequested = false;
  String? _approverId;
  bool _busy = false;
  ApiError? _error;
  bool _notice = false;
  // Satu kunci per layar tutup shift: kirim ulang (jaringan putus) tidak menutup dua kali.
  final String _key = 'shift-${const Uuid().v4()}';

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    for (final c in _counted.values) {
      c.dispose();
    }
    _note.dispose();
    _pin.dispose();
    super.dispose();
  }

  Future<void> _load({bool keep = false}) async {
    setState(() => _error = null);
    try {
      final r = await ref
          .read(posRepositoryProvider)
          .shiftRecap(widget.shiftId);
      if (!mounted) return;
      setState(() {
        for (final c in r.counts) {
          _counted.putIfAbsent(c.methodId, () => TextEditingController());
          if (!keep) _counted[c.methodId]!.clear();
        }
        _recap = r;
      });
    } on ApiError catch (e) {
      if (mounted) setState(() => _error = e);
    }
  }

  bool _filled(String id) => (_counted[id]?.text.trim() ?? '').isNotEmpty;

  Decimal _countedOf(String id) => parseInput(_counted[id]?.text ?? '');

  List<ShiftCount> get _rows => _recap?.counts ?? const [];

  bool get _allFilled =>
      _rows.isNotEmpty && _rows.every((c) => _filled(c.methodId));

  Decimal get _diffTotal => _rows
      .where((c) => _filled(c.methodId))
      .fold(Decimal.zero, (a, c) => a + _countedOf(c.methodId) - c.expected);

  Decimal get _diffAbs =>
      _rows.where((c) => _filled(c.methodId)).fold(Decimal.zero, (a, c) {
        final d = _countedOf(c.methodId) - c.expected;
        return a + (d < Decimal.zero ? -d : d);
      });

  bool get _needApproval => _allFilled && _diffAbs != Decimal.zero;

  bool get _valid =>
      _allFilled &&
      (!_needApproval ||
          (_note.text.trim().length >= 3 &&
              _approverId != null &&
              RegExp(r'^\d{6}$').hasMatch(_pin.text)));

  Future<void> _loadApprovers() async {
    if (_approversRequested) return;
    _approversRequested = true;
    try {
      final list = await ref
          .read(posRepositoryProvider)
          .approvers('shift_close');
      if (!mounted) return;
      setState(() {
        _approvers = list;
        if (list.length == 1) _approverId = list.first.id;
      });
    } on ApiError catch (e) {
      if (mounted) {
        setState(() {
          _approvers = const [];
          _error = e;
        });
      }
    }
  }

  void _fillExpected() {
    setState(() {
      for (final c in _rows) {
        _counted[c.methodId]!.text = c.expected < Decimal.zero
            ? '0'
            : decToApi(c.expected, scale: 2);
      }
    });
  }

  Future<void> _submit() async {
    final recap = _recap;
    if (recap == null || !_valid || _busy) return;
    setState(() {
      _busy = true;
      _error = null;
      _notice = false;
    });
    try {
      final res = await ref
          .read(posRepositoryProvider)
          .closeShift(
            recap.id,
            counted: {
              for (final c in _rows)
                c.methodId: decToApi(_countedOf(c.methodId), scale: 2),
            },
            note: _note.text.trim(),
            idempotencyKey: _key,
            approverId: _needApproval ? _approverId : null,
            pin: _needApproval ? _pin.text : null,
          );
      ref.read(shiftProvider.notifier).closed();
      if (mounted) setState(() => _done = res);
    } on ApiError catch (e) {
      if (!mounted) return;
      _pin.clear();
      if (e.code == 'SHIFT_RECAP_CHANGED') {
        await _load(keep: true);
        if (mounted) setState(() => _notice = true);
      } else {
        setState(() => _error = e);
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final done = _done;
    return Scaffold(
      appBar: AppBar(
        title: Text(
          done != null
              ? l.shiftClosedTitle(done.docNo)
              : l.shiftCloseTitle(_recap?.docNo ?? ''),
        ),
      ),
      body: SafeArea(
        child: done != null
            ? _buildDone(context, l, done)
            : _recap == null
            ? _buildLoading(context, l)
            : _buildForm(context, l, _recap!),
      ),
    );
  }

  Widget _buildLoading(BuildContext context, AppLocalizations l) {
    if (_error == null) return const Center(child: CircularProgressIndicator());
    return Center(
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
            OutlinedButton(onPressed: _load, child: const Text('OK')),
          ],
        ),
      ),
    );
  }

  Widget _buildDone(BuildContext context, AppLocalizations l, ShiftRecap d) {
    final pal = context.pal;
    final diff = d.diffTotal ?? Decimal.zero;
    Widget stat(String k, String v, {Color? color}) => Expanded(
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: pal.sunken,
          borderRadius: BorderRadius.circular(12),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(k, style: TextStyle(fontSize: 11, color: pal.textTertiary)),
            const SizedBox(height: 2),
            FittedBox(
              fit: BoxFit.scaleDown,
              child: Text(
                v,
                style: TextStyle(fontWeight: FontWeight.w800, color: color),
              ),
            ),
          ],
        ),
      ),
    );
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Text(l.shiftClosedBody),
        const SizedBox(height: 16),
        Row(
          children: [
            stat(l.shiftTotalExpected, formatMoney(d.expectedTotal)),
            const SizedBox(width: 8),
            stat(
              l.shiftTotalCounted,
              formatMoney(d.countedTotal ?? Decimal.zero),
            ),
            const SizedBox(width: 8),
            stat(
              l.shiftTotalDiff,
              _signed(diff),
              color: diff == Decimal.zero ? null : pal.danger,
            ),
          ],
        ),
        const SizedBox(height: 24),
        FilledButton.icon(
          style: FilledButton.styleFrom(minimumSize: const Size.fromHeight(48)),
          onPressed: () => Navigator.of(context).pop(true),
          icon: const Icon(Icons.lock_open),
          label: Text(l.shiftOpenNew),
        ),
        const SizedBox(height: 8),
        OutlinedButton(
          style: OutlinedButton.styleFrom(
            minimumSize: const Size.fromHeight(48),
          ),
          onPressed: () => Navigator.of(context).pop(false),
          child: Text(l.shiftDone),
        ),
      ],
    );
  }

  String _signed(Decimal d) => (d > Decimal.zero ? '+' : '') + formatMoney(d);

  Widget _buildForm(BuildContext context, AppLocalizations l, ShiftRecap r) {
    final pal = context.pal;
    if (_needApproval) _loadApprovers();
    final diffTotal = _diffTotal;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Text(l.shiftCloseBody, style: TextStyle(color: pal.textTertiary)),
        const SizedBox(height: 10),
        Wrap(
          spacing: 14,
          runSpacing: 2,
          children: [
            Text(
              l.shiftCloseOpened(DateFormat('d MMM HH:mm').format(r.openedAt)),
            ),
            Text(
              l.shiftCloseSales('${r.saleCount}', formatMoney(r.salesTotal)),
            ),
            if (r.voidCount > 0) Text(l.shiftCloseVoid('${r.voidCount}')),
            if (r.receivableTotal > Decimal.zero)
              Text(l.shiftCloseReceivable(formatMoney(r.receivableTotal))),
          ],
        ),
        if (_notice) ...[
          const SizedBox(height: 10),
          _Banner(
            text: l.shiftChangedNotice,
            color: pal.warningSoft,
            fg: pal.warningText,
          ),
        ],
        const SizedBox(height: 12),
        Align(
          alignment: Alignment.centerRight,
          child: TextButton.icon(
            onPressed: _busy ? null : _fillExpected,
            icon: const Icon(Icons.done_all, size: 18),
            label: Text(l.shiftFillExpected),
          ),
        ),
        for (final c in r.counts) _countRow(context, l, c),
        const Divider(height: 24),
        Row(
          children: [
            Expanded(
              child: Text(
                l.shiftTotalDiff,
                style: const TextStyle(fontWeight: FontWeight.w700),
              ),
            ),
            Text(
              _allFilled ? _signed(diffTotal) : '-',
              style: TextStyle(
                fontWeight: FontWeight.w800,
                color: _allFilled && _diffAbs != Decimal.zero
                    ? pal.danger
                    : null,
              ),
            ),
          ],
        ),
        if (_needApproval) ...[
          const SizedBox(height: 14),
          _Banner(
            text: l.shiftNeedApproval,
            color: pal.warningSoft,
            fg: pal.warningText,
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _note,
            enabled: !_busy,
            minLines: 2,
            maxLines: 3,
            onChanged: (_) => setState(() {}),
            decoration: InputDecoration(
              labelText: l.shiftNote,
              hintText: l.shiftNoteHint,
            ),
          ),
          const SizedBox(height: 12),
          if (_approvers == null)
            const Center(child: CircularProgressIndicator())
          else if (_approvers!.isEmpty)
            Text(l.shiftApprovalNone, style: TextStyle(color: pal.warningText))
          else ...[
            DropdownButtonFormField<String>(
              initialValue: _approverId,
              isExpanded: true,
              decoration: InputDecoration(labelText: l.outletApprover),
              items: [
                for (final a in _approvers!)
                  DropdownMenuItem(value: a.id, child: Text(a.name)),
              ],
              onChanged: _busy ? null : (v) => setState(() => _approverId = v),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _pin,
              enabled: !_busy,
              obscureText: true,
              keyboardType: TextInputType.number,
              maxLength: 6,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              onChanged: (_) => setState(() {}),
              decoration: InputDecoration(
                labelText: l.outletPin,
                counterText: '',
              ),
            ),
          ],
        ],
        if (_error != null) ...[
          const SizedBox(height: 12),
          Text(_error!.message(l), style: TextStyle(color: pal.danger)),
        ],
        const SizedBox(height: 20),
        FilledButton(
          style: FilledButton.styleFrom(minimumSize: const Size.fromHeight(50)),
          onPressed: _valid && !_busy ? _submit : null,
          child: _busy
              ? const SizedBox(
                  height: 20,
                  width: 20,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : Text(l.shiftCloseSubmit),
        ),
      ],
    );
  }

  Widget _countRow(BuildContext context, AppLocalizations l, ShiftCount c) {
    final pal = context.pal;
    final filled = _filled(c.methodId);
    final diff = filled ? _countedOf(c.methodId) - c.expected : null;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  c.name,
                  style: const TextStyle(fontWeight: FontWeight.w700),
                ),
              ),
              Text(
                '${l.shiftColExpected} ${formatMoney(c.expected)}',
                style: TextStyle(color: pal.textTertiary, fontSize: 12.5),
              ),
            ],
          ),
          const SizedBox(height: 6),
          Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _counted[c.methodId],
                  enabled: !_busy,
                  keyboardType: const TextInputType.numberWithOptions(
                    decimal: true,
                  ),
                  inputFormatters: [
                    FilteringTextInputFormatter.allow(RegExp(r'[0-9.,]')),
                  ],
                  onChanged: (_) => setState(() {}),
                  decoration: InputDecoration(
                    labelText: l.shiftColCounted,
                    isDense: true,
                  ),
                ),
              ),
              const SizedBox(width: 12),
              SizedBox(
                width: 110,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.end,
                  children: [
                    Text(
                      l.shiftColDiff,
                      style: TextStyle(fontSize: 10.5, color: pal.textTertiary),
                    ),
                    Text(
                      diff == null ? '-' : _signed(diff),
                      style: TextStyle(
                        fontWeight: FontWeight.w700,
                        color: diff == null || diff == Decimal.zero
                            ? null
                            : pal.danger,
                      ),
                    ),
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

class _Banner extends StatelessWidget {
  const _Banner({required this.text, required this.color, required this.fg});

  final String text;
  final Color color;
  final Color fg;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(10),
    decoration: BoxDecoration(
      color: color,
      borderRadius: BorderRadius.circular(10),
    ),
    child: Text(text, style: TextStyle(color: fg, fontSize: 13)),
  );
}
