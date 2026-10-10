import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/gen/app_localizations.dart';
import '../api/api_error.dart';
import '../session/session_controller.dart';
import '../theme/app_theme.dart';

class OutletInfo {
  const OutletInfo({required this.id, required this.code, required this.name});

  final String id;
  final String code;
  final String name;

  factory OutletInfo.fromJson(Map<String, dynamic> j) => OutletInfo(
    id: '${j['id']}',
    code: '${j['code'] ?? ''}',
    name: '${j['name'] ?? ''}',
  );
}

class ApproverInfo {
  const ApproverInfo({required this.id, required this.name});

  final String id;
  final String name;

  factory ApproverInfo.fromJson(Map<String, dynamic> j) =>
      ApproverInfo(id: '${j['id']}', name: '${j['name'] ?? ''}');
}

/// Daftar outlet aktif yang boleh dipilih pengguna (`GET /outlets/accessible`).
final accessibleOutletsProvider = FutureProvider.autoDispose<List<OutletInfo>>((
  ref,
) async {
  try {
    final r = await ref
        .read(apiClientProvider)
        .dio
        .get<Map<String, dynamic>>('/outlets/accessible');
    return (r.data!['outlets'] as List? ?? const [])
        .map((e) => OutletInfo.fromJson((e as Map).cast<String, dynamic>()))
        .toList();
  } on DioException catch (e) {
    throw ApiError.fromDio(e);
  }
});

/// Penyetuju pindah outlet di outlet tujuan (`GET /approvals/approvers?for=outlet_switch`).
final _approversProvider = FutureProvider.autoDispose
    .family<List<ApproverInfo>, String>((ref, outletId) async {
      try {
        final r = await ref
            .read(apiClientProvider)
            .dio
            .get<Map<String, dynamic>>(
              '/approvals/approvers',
              queryParameters: {'for': 'outlet_switch', 'outlet_id': outletId},
            );
        return (r.data!['approvers'] as List? ?? const [])
            .map(
              (e) => ApproverInfo.fromJson((e as Map).cast<String, dynamic>()),
            )
            .toList();
      } on DioException catch (e) {
        throw ApiError.fromDio(e);
      }
    });

/// Lembar pilih cabang. [pos] = dipanggil dari layar kasir: server meminta PIN penyetuju (kecuali pelaku sendiri
/// penyetuju), jadi dialog PIN muncul hanya bila server menjawab `PIN_REQUIRED`. [onSwitched] dipanggil setelah
/// berhasil (mis. kosongkan keranjang, muat ulang katalog).
Future<void> showOutletSwitcher(
  BuildContext context, {
  bool pos = false,
  VoidCallback? onSwitched,
}) async {
  final picked = await showModalBottomSheet<OutletInfo>(
    context: context,
    showDragHandle: true,
    isScrollControlled: true,
    useSafeArea: true,
    builder: (_) => const _OutletSheet(),
  );
  if (picked == null || !context.mounted) return;
  await _switchTo(context, picked, pos: pos, onSwitched: onSwitched);
}

Future<void> _switchTo(
  BuildContext context,
  OutletInfo target, {
  required bool pos,
  VoidCallback? onSwitched,
}) async {
  final container = ProviderScope.containerOf(context);
  final session = container.read(sessionProvider.notifier);
  final l = AppLocalizations.of(context);
  final messenger = ScaffoldMessenger.of(context);
  try {
    await session.switchOutlet(target.id, pos: pos);
  } on ApiError catch (e) {
    if (e.code == 'PIN_REQUIRED' && pos && context.mounted) {
      final ok = await showDialog<bool>(
        context: context,
        barrierDismissible: false,
        builder: (_) => _ApprovalDialog(target: target),
      );
      if (ok == true) {
        onSwitched?.call();
        messenger.showSnackBar(
          SnackBar(content: Text(l.outletSwitched(target.name))),
        );
      }
      return;
    }
    messenger.showSnackBar(SnackBar(content: Text(e.message(l))));
    return;
  }
  onSwitched?.call();
  messenger.showSnackBar(
    SnackBar(content: Text(l.outletSwitched(target.name))),
  );
}

class _OutletSheet extends ConsumerWidget {
  const _OutletSheet();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final session = ref.watch(sessionProvider);
    final current = session is SessionSignedIn ? session.profile.outlet.id : '';
    final list = ref.watch(accessibleOutletsProvider);
    return ConstrainedBox(
      constraints: BoxConstraints(
        maxHeight: MediaQuery.sizeOf(context).height * 0.7,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
            child: Text(
              l.outletSwitchTitle,
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w800),
            ),
          ),
          Flexible(
            child: list.when(
              loading: () => const Padding(
                padding: EdgeInsets.all(32),
                child: Center(child: CircularProgressIndicator()),
              ),
              error: (e, _) => Padding(
                padding: const EdgeInsets.all(24),
                child: Text(
                  e is ApiError ? e.message(l) : l.errorUnknown,
                  style: TextStyle(color: pal.danger),
                ),
              ),
              data: (outlets) => ListView(
                shrinkWrap: true,
                children: [
                  for (final o in outlets)
                    ListTile(
                      leading: Icon(
                        Icons.storefront,
                        color: o.id == current ? pal.primary : null,
                      ),
                      title: Text(
                        o.name,
                        style: const TextStyle(fontWeight: FontWeight.w600),
                      ),
                      subtitle: Text(o.code),
                      trailing: o.id == current
                          ? Icon(Icons.check_circle, color: pal.primary)
                          : null,
                      onTap: o.id == current
                          ? null
                          : () => Navigator.of(context).pop(o),
                    ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 12),
        ],
      ),
    );
  }
}

/// Dialog persetujuan PIN Owner/Supervisor untuk pindah cabang dari kasir. Mengirim sendiri dan menutup `true` bila berhasil.
class _ApprovalDialog extends ConsumerStatefulWidget {
  const _ApprovalDialog({required this.target});

  final OutletInfo target;

  @override
  ConsumerState<_ApprovalDialog> createState() => _ApprovalDialogState();
}

class _ApprovalDialogState extends ConsumerState<_ApprovalDialog> {
  final _pin = TextEditingController();
  String? _approverId;
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _pin.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final l = AppLocalizations.of(context);
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref
          .read(sessionProvider.notifier)
          .switchOutlet(
            widget.target.id,
            pos: true,
            approvalUserId: _approverId,
            pin: _pin.text,
          );
      if (mounted) Navigator.of(context).pop(true);
    } on ApiError catch (e) {
      if (!mounted) return;
      _pin.clear();
      setState(() {
        _busy = false;
        _error = e.message(l);
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final approvers = ref.watch(_approversProvider(widget.target.id));
    final list = approvers.asData?.value ?? const <ApproverInfo>[];
    if (_approverId == null && list.length == 1) _approverId = list.first.id;
    final valid = _approverId != null && RegExp(r'^\d{6}$').hasMatch(_pin.text);
    return AlertDialog(
      title: Text(l.outletApprovalTitle),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(l.outletApprovalBody(widget.target.name)),
            const SizedBox(height: 14),
            if (approvers.isLoading)
              const Center(child: CircularProgressIndicator())
            else if (approvers.hasError)
              Text(
                approvers.error is ApiError
                    ? (approvers.error as ApiError).message(l)
                    : l.errorUnknown,
                style: TextStyle(color: pal.danger),
              )
            else if (list.isEmpty)
              Text(
                l.outletApprovalNone,
                style: TextStyle(color: pal.warningText),
              )
            else
              DropdownButtonFormField<String>(
                initialValue: _approverId,
                isExpanded: true,
                decoration: InputDecoration(labelText: l.outletApprover),
                items: [
                  for (final a in list)
                    DropdownMenuItem(value: a.id, child: Text(a.name)),
                ],
                onChanged: _busy
                    ? null
                    : (v) => setState(() => _approverId = v),
              ),
            const SizedBox(height: 12),
            TextField(
              controller: _pin,
              enabled: !_busy,
              obscureText: true,
              keyboardType: TextInputType.number,
              maxLength: 6,
              onChanged: (_) => setState(() {}),
              decoration: InputDecoration(
                labelText: l.outletPin,
                counterText: '',
              ),
            ),
            if (_error != null) ...[
              const SizedBox(height: 8),
              Text(_error!, style: TextStyle(color: pal.danger)),
            ],
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: _busy ? null : () => Navigator.of(context).pop(false),
          child: Text(MaterialLocalizations.of(context).cancelButtonLabel),
        ),
        FilledButton(
          onPressed: valid && !_busy ? _submit : null,
          child: Text(l.outletApprovalConfirm),
        ),
      ],
    );
  }
}
