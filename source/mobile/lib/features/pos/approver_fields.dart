import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_error.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'pos_repository.dart';
import 'shift_models.dart';

/// Pilih penyetuju + isi PIN-nya. Dipakai ubah harga/potongan dan melewati limit kredit.
/// [purpose] null = penyetuju ubah harga; `credit_limit` = penyetuju limit kredit. PIN hanya dilaporkan lewat
/// [onChanged]; pemeriksaannya (POST /approvals/check) dilakukan pemanggil.
class ApproverFields extends ConsumerStatefulWidget {
  const ApproverFields({
    super.key,
    required this.onChanged,
    this.purpose,
    this.enabled = true,
  });

  final String? purpose;
  final bool enabled;
  final void Function(Approver? approver, String pin) onChanged;

  @override
  ConsumerState<ApproverFields> createState() => ApproverFieldsState();
}

class ApproverFieldsState extends ConsumerState<ApproverFields> {
  final _pin = TextEditingController();
  List<Approver>? _list;
  ApiError? _error;
  String? _id;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _pin.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final list = await ref
          .read(posRepositoryProvider)
          .approvers(widget.purpose);
      if (!mounted) return;
      setState(() {
        _list = list;
        if (list.length == 1) _id = list.first.id;
      });
      _report();
    } on ApiError catch (e) {
      if (mounted) {
        setState(() {
          _list = const [];
          _error = e;
        });
      }
    }
  }

  Approver? get _approver {
    for (final a in _list ?? const <Approver>[]) {
      if (a.id == _id) return a;
    }
    return null;
  }

  void _report() => widget.onChanged(_approver, _pin.text);

  /// Dipanggil pemanggil setelah PIN ditolak.
  void clearPin() {
    _pin.clear();
    _report();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final list = _list;
    if (list == null) {
      return const Padding(
        padding: EdgeInsets.all(12),
        child: Center(child: CircularProgressIndicator()),
      );
    }
    if (list.isEmpty) {
      return Text(
        _error?.message(l) ?? l.shiftApprovalNone,
        style: TextStyle(color: pal.warningText),
      );
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        DropdownButtonFormField<String>(
          initialValue: _id,
          isExpanded: true,
          decoration: InputDecoration(labelText: l.outletApprover),
          items: [
            for (final a in list)
              DropdownMenuItem(value: a.id, child: Text(a.name)),
          ],
          onChanged: widget.enabled
              ? (v) {
                  setState(() => _id = v);
                  _report();
                }
              : null,
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _pin,
          enabled: widget.enabled,
          obscureText: true,
          keyboardType: TextInputType.number,
          maxLength: 6,
          inputFormatters: [FilteringTextInputFormatter.digitsOnly],
          onChanged: (_) => _report(),
          decoration: InputDecoration(labelText: l.outletPin, counterText: ''),
        ),
      ],
    );
  }
}
