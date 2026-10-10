import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:mobile_scanner/mobile_scanner.dart';

import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';

/// Hasil satu pemindaian: [ok] = barang masuk keranjang; [text] ditampilkan di bawah kamera.
class ScanOutcome {
  const ScanOutcome({required this.ok, required this.text});

  final bool ok;
  final String text;
}

/// Pemindai barcode dengan kamera. Tetap terbuka sampai kasir menekan Selesai, jadi beberapa barang bisa dipindai
/// berurutan. [onCode] mencari kode di server dan memasukkan barang ke keranjang.
Future<void> showScanPage(
  BuildContext context, {
  required Future<ScanOutcome> Function(String code) onCode,
}) => Navigator.of(context).push<void>(
  MaterialPageRoute(
    fullscreenDialog: true,
    builder: (_) => _ScanPage(onCode: onCode),
  ),
);

class _ScanPage extends StatefulWidget {
  const _ScanPage({required this.onCode});

  final Future<ScanOutcome> Function(String code) onCode;

  @override
  State<_ScanPage> createState() => _ScanPageState();
}

class _ScanPageState extends State<_ScanPage> {
  final _controller = MobileScannerController(
    formats: const [
      BarcodeFormat.ean13,
      BarcodeFormat.ean8,
      BarcodeFormat.upcA,
      BarcodeFormat.upcE,
      BarcodeFormat.code128,
      BarcodeFormat.code39,
      BarcodeFormat.code93,
      BarcodeFormat.codabar,
      BarcodeFormat.itf14,
      BarcodeFormat.qrCode,
    ],
    detectionSpeed: DetectionSpeed.noDuplicates,
  );

  bool _busy = false;
  String _lastCode = '';
  DateTime _lastAt = DateTime.fromMillisecondsSinceEpoch(0);
  ScanOutcome? _outcome;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Future<void> _detected(BarcodeCapture capture) async {
    if (_busy) return;
    final code = capture.barcodes
        .map((b) => b.rawValue?.trim() ?? '')
        .firstWhere((v) => v.isNotEmpty, orElse: () => '');
    if (code.isEmpty) return;
    // Kode yang sama tidak diproses ulang selama 2 detik: kamera sering membaca barcode yang sama berkali-kali.
    final now = DateTime.now();
    if (code == _lastCode &&
        now.difference(_lastAt) < const Duration(seconds: 2)) {
      return;
    }
    _busy = true;
    _lastCode = code;
    _lastAt = now;
    try {
      final r = await widget.onCode(code);
      if (!mounted) return;
      if (r.ok) {
        HapticFeedback.mediumImpact();
      } else {
        HapticFeedback.heavyImpact();
      }
      setState(() => _outcome = r);
    } finally {
      _lastAt = DateTime.now();
      _busy = false;
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final o = _outcome;
    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        title: Text(l.scanTitle),
        actions: [
          IconButton(
            tooltip: l.scanTorch,
            icon: const Icon(Icons.flash_on),
            onPressed: _controller.toggleTorch,
          ),
          IconButton(
            tooltip: l.scanSwitchCamera,
            icon: const Icon(Icons.cameraswitch),
            onPressed: _controller.switchCamera,
          ),
        ],
      ),
      body: Column(
        children: [
          Expanded(
            child: Stack(
              fit: StackFit.expand,
              children: [
                MobileScanner(
                  controller: _controller,
                  onDetect: _detected,
                  errorBuilder: (context, error) => Center(
                    child: Padding(
                      padding: const EdgeInsets.all(24),
                      child: Text(
                        error.errorCode ==
                                MobileScannerErrorCode.permissionDenied
                            ? l.scanPermissionDenied
                            : l.scanCameraError,
                        textAlign: TextAlign.center,
                        style: const TextStyle(color: Colors.white),
                      ),
                    ),
                  ),
                ),
                IgnorePointer(
                  child: Center(
                    child: Container(
                      width: 280,
                      height: 160,
                      decoration: BoxDecoration(
                        border: Border.all(color: Colors.white70, width: 2),
                        borderRadius: BorderRadius.circular(16),
                      ),
                    ),
                  ),
                ),
              ],
            ),
          ),
          Container(
            width: double.infinity,
            color: pal.surface,
            padding: const EdgeInsets.fromLTRB(16, 14, 16, 16),
            child: SafeArea(
              top: false,
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Icon(
                        o == null
                            ? Icons.qr_code_scanner
                            : o.ok
                            ? Icons.check_circle
                            : Icons.error_outline,
                        color: o == null
                            ? pal.textTertiary
                            : o.ok
                            ? pal.success
                            : pal.danger,
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: Text(
                          o?.text ?? l.scanHint,
                          style: TextStyle(
                            fontWeight: FontWeight.w600,
                            color: o == null ? pal.textTertiary : null,
                          ),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 12),
                  FilledButton(
                    style: FilledButton.styleFrom(
                      minimumSize: const Size.fromHeight(48),
                    ),
                    onPressed: () => Navigator.of(context).pop(),
                    child: Text(l.scanDone),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}
