import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:mobile_scanner/mobile_scanner.dart';
import 'package:qr_flutter/qr_flutter.dart';

import '../../l10n/gen/app_localizations.dart';
import '../api/api_error.dart';
import '../session/session_controller.dart';
import '../theme/app_theme.dart';

/// QRIS statis toko di QRISGate (EraPay). [active] = jalur pembayaran toko sekarang ke QRISGate; selain itu kartunya
/// disembunyikan. Gagal memuat (bukan Owner, belum tertaut, EraPay tak terjangkau) juga berarti tidak aktif.
class QrisToko {
  const QrisToko({
    this.active = false,
    this.hasQris = false,
    this.name = '',
    this.payload = '',
  });

  final bool active;
  final bool hasQris;
  final String name;
  final String payload;
}

final qrisTokoProvider = FutureProvider.autoDispose<QrisToko>((ref) async {
  try {
    final r = await ref
        .read(apiClientProvider)
        .dio
        .get<Map<String, dynamic>>('/ecosystem/erapay/qris');
    final j = r.data ?? const {};
    return QrisToko(
      active: j['active'] == true,
      hasQris: j['has_qris'] == true,
      name: '${j['qris_name'] ?? ''}',
      payload: '${j['qris_payload'] ?? ''}',
    );
  } on DioException {
    return const QrisToko();
  }
});

/// Kartu di dialog pengaturan; kosong bila jalur pembayaran toko bukan QRISGate.
class QrisTokoTile extends ConsumerWidget {
  const QrisTokoTile({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final q = ref.watch(qrisTokoProvider).value;
    if (q == null || !q.active) return const SizedBox.shrink();
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Material(
        color: pal.primarySoft,
        borderRadius: BorderRadius.circular(12),
        child: ListTile(
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(12),
          ),
          leading: Icon(Icons.qr_code_2, color: pal.primary),
          title: Text(
            l.qrisTokoTitle,
            style: const TextStyle(fontWeight: FontWeight.w700),
          ),
          subtitle: Text(
            q.hasQris ? q.name : l.qrisTokoNone,
            style: TextStyle(fontSize: 12, color: pal.textTertiary),
          ),
          onTap: () => showDialog<void>(
            context: context,
            builder: (_) => _QrisDialog(initial: q),
          ),
        ),
      ),
    );
  }
}

class _QrisDialog extends ConsumerStatefulWidget {
  const _QrisDialog({required this.initial});

  final QrisToko initial;

  @override
  ConsumerState<_QrisDialog> createState() => _QrisDialogState();
}

class _QrisDialogState extends ConsumerState<_QrisDialog> {
  late QrisToko _q = widget.initial;
  bool _busy = false;

  Future<void> _replace(String payload) async {
    final l = AppLocalizations.of(context);
    final messenger = ScaffoldMessenger.of(context);
    final ok = await showDialog<bool>(
      context: context,
      builder: (c) => AlertDialog(
        title: Text(l.qrisTokoConfirmTitle),
        content: Text(l.qrisTokoConfirmBody),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(c, false),
            child: Text(l.commonCancel),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(c, true),
            child: Text(l.qrisTokoConfirmYes),
          ),
        ],
      ),
    );
    if (ok != true) return;
    setState(() => _busy = true);
    try {
      await ref
          .read(apiClientProvider)
          .dio
          .put<Map<String, dynamic>>(
            '/ecosystem/erapay/qris',
            data: {'payload': payload},
          );
      ref.invalidate(qrisTokoProvider);
      final fresh = await ref.read(qrisTokoProvider.future);
      if (!mounted) return;
      setState(() => _q = fresh);
      messenger.showSnackBar(SnackBar(content: Text(l.qrisTokoSaved)));
    } on DioException catch (e) {
      messenger.showSnackBar(
        SnackBar(content: Text(ApiError.fromDio(e).message(l))),
      );
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _scan() async {
    final code = await Navigator.of(context).push<String>(
      MaterialPageRoute(
        fullscreenDialog: true,
        builder: (_) => const _QrisScanPage(),
      ),
    );
    if (code != null && code.isNotEmpty) await _replace(code);
  }

  Future<void> _paste() async {
    final l = AppLocalizations.of(context);
    final c = TextEditingController(
      text: (await Clipboard.getData(Clipboard.kTextPlain))?.text?.trim() ?? '',
    );
    if (!mounted) return;
    final text = await showDialog<String>(
      context: context,
      builder: (d) => AlertDialog(
        title: Text(l.qrisTokoPasteTitle),
        content: TextField(
          controller: c,
          maxLines: 4,
          autofocus: true,
          decoration: InputDecoration(hintText: l.qrisTokoPasteHint),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(d),
            child: Text(l.commonCancel),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(d, c.text.trim()),
            child: Text(l.commonSave),
          ),
        ],
      ),
    );
    c.dispose();
    if (text != null && text.isNotEmpty) await _replace(text);
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    return AlertDialog(
      scrollable: true,
      title: Text(l.qrisTokoTitle),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (_q.hasQris) ...[
            Container(
              padding: const EdgeInsets.all(10),
              decoration: BoxDecoration(
                color:
                    Colors.white, // QR harus kontras tinggi di tema gelap juga
                borderRadius: BorderRadius.circular(10),
              ),
              child: QrImageView(
                data: _q.payload,
                size: 220,
                backgroundColor: Colors.white,
                errorCorrectionLevel: QrErrorCorrectLevel.M,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              _q.name,
              textAlign: TextAlign.center,
              style: const TextStyle(fontWeight: FontWeight.w700),
            ),
          ] else
            Text(l.qrisTokoNone, textAlign: TextAlign.center),
          const SizedBox(height: 8),
          Text(
            l.qrisTokoHint,
            textAlign: TextAlign.center,
            style: TextStyle(fontSize: 12, color: pal.textTertiary),
          ),
          const SizedBox(height: 12),
          FilledButton.icon(
            onPressed: _busy ? null : _scan,
            icon: const Icon(Icons.qr_code_scanner),
            label: Text(l.qrisTokoScan),
          ),
          TextButton.icon(
            onPressed: _busy ? null : _paste,
            icon: const Icon(Icons.content_paste),
            label: Text(l.qrisTokoPaste),
          ),
          if (_busy) const LinearProgressIndicator(),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(l.textSizeDone),
        ),
      ],
    );
  }
}

/// Pemindai satu QR: menutup dengan teks QRIS begitu terbaca. Validasi ada di server (CRC, QRIS statis, tanpa nominal).
class _QrisScanPage extends StatefulWidget {
  const _QrisScanPage();

  @override
  State<_QrisScanPage> createState() => _QrisScanPageState();
}

class _QrisScanPageState extends State<_QrisScanPage> {
  final _controller = MobileScannerController(
    formats: const [BarcodeFormat.qrCode],
    detectionSpeed: DetectionSpeed.noDuplicates,
  );
  bool _done = false;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(l.qrisTokoScan)),
      body: MobileScanner(
        controller: _controller,
        onDetect: (capture) {
          if (_done) return;
          final code = capture.barcodes
              .map((b) => b.rawValue?.trim() ?? '')
              .firstWhere((v) => v.isNotEmpty, orElse: () => '');
          if (code.isEmpty) return;
          _done = true;
          Navigator.pop(context, code);
        },
        errorBuilder: (context, error) => Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Text(
              error.errorCode == MobileScannerErrorCode.permissionDenied
                  ? l.scanPermissionDenied
                  : l.scanCameraError,
              textAlign: TextAlign.center,
            ),
          ),
        ),
      ),
    );
  }
}
