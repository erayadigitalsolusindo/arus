import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_error.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import 'approver_fields.dart';
import 'pos_repository.dart';
import 'shift_models.dart';

/// Batalkan nota: alasan + persetujuan Owner/Supervisor (PIN). Mengembalikan true bila nota berhasil dibatalkan.
Future<bool> showVoidSaleDialog(
  BuildContext context, {
  required String saleId,
  required String docNo,
}) async =>
    await showDialog<bool>(
      context: context,
      barrierDismissible: false,
      builder: (_) => _VoidSaleDialog(saleId: saleId, docNo: docNo),
    ) ??
    false;

class _VoidSaleDialog extends ConsumerStatefulWidget {
  const _VoidSaleDialog({required this.saleId, required this.docNo});

  final String saleId;
  final String docNo;

  @override
  ConsumerState<_VoidSaleDialog> createState() => _VoidSaleDialogState();
}

class _VoidSaleDialogState extends ConsumerState<_VoidSaleDialog> {
  final _reason = TextEditingController();
  final _approverKey = GlobalKey<ApproverFieldsState>();
  Approver? _approver;
  String _pin = '';
  bool _busy = false;
  ApiError? _error;

  @override
  void dispose() {
    _reason.dispose();
    super.dispose();
  }

  // Server meminta alasan 3–200 karakter dan PIN 6 digit.
  bool get _ready =>
      !_busy &&
      _reason.text.trim().length >= 3 &&
      _approver != null &&
      _pin.length == 6;

  Future<void> _submit() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref.read(posRepositoryProvider).voidSale(
        widget.saleId,
        reason: _reason.text.trim(),
        approverId: _approver!.id,
        pin: _pin,
      );
      if (mounted) Navigator.pop(context, true);
    } on ApiError catch (e) {
      if (!mounted) return;
      if (e.code == 'INVALID_PIN' || e.code == 'PIN_REQUIRED') {
        _approverKey.currentState?.clearPin();
      }
      setState(() {
        _error = e;
        _busy = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return AlertDialog(
      title: Text(l.saleVoidTitle(widget.docNo)),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            TextField(
              controller: _reason,
              enabled: !_busy,
              maxLength: 200,
              minLines: 1,
              maxLines: 3,
              onChanged: (_) => setState(() {}),
              decoration: InputDecoration(labelText: l.saleVoidReasonLabel),
            ),
            ApproverFields(
              key: _approverKey,
              purpose: 'sale_edit',
              enabled: !_busy,
              onChanged: (a, pin) => WidgetsBinding.instance.addPostFrameCallback(
                (_) {
                  if (mounted) {
                    setState(() {
                      _approver = a;
                      _pin = pin;
                    });
                  }
                },
              ),
            ),
            if (_error != null)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Text(
                  _error!.message(l),
                  style: TextStyle(color: context.pal.danger),
                ),
              ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: _busy ? null : () => Navigator.pop(context, false),
          child: Text(l.commonCancel),
        ),
        FilledButton(
          style: FilledButton.styleFrom(backgroundColor: context.pal.danger),
          onPressed: _ready ? _submit : null,
          child: Text(l.saleVoidConfirm),
        ),
      ],
    );
  }
}
