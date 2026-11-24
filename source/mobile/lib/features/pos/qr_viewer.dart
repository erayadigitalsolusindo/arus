import 'dart:io';
import 'dart:ui' show ImageByteFormat;

import 'package:flutter/material.dart';
import 'package:path_provider/path_provider.dart';
import 'package:qr_flutter/qr_flutter.dart';
import 'package:share_plus/share_plus.dart';

import '../../l10n/gen/app_localizations.dart';

/// QR layar penuh: bisa di-zoom (cubit) dan dibagikan sebagai gambar PNG (WA, dll).
class QrViewerPage extends StatelessWidget {
  const QrViewerPage({super.key, required this.data, this.caption});

  final String data;
  final String? caption;

  Future<void> _share() async {
    final img = await QrPainter(
      data: data,
      version: QrVersions.auto,
      errorCorrectionLevel: QrErrorCorrectLevel.M,
      gapless: false,
    ).toImageData(1024, format: ImageByteFormat.png);
    if (img == null) return;
    // shortcut: PNG tanpa margin putih (quiet zone) tambahan, upgrade bila scanner WA gagal membaca.
    final f = File('${(await getTemporaryDirectory()).path}/qris.png');
    await f.writeAsBytes(img.buffer.asUint8List());
    await SharePlus.instance.share(
      ShareParams(files: [XFile(f.path)], text: caption),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return Scaffold(
      appBar: AppBar(
        actions: [
          IconButton(
            tooltip: l.qrShare,
            icon: const Icon(Icons.share),
            onPressed: _share,
          ),
        ],
      ),
      body: Column(
        children: [
          Expanded(
            child: InteractiveViewer(
              maxScale: 6,
              child: Center(
                child: Container(
                  color: Colors.white,
                  padding: const EdgeInsets.all(16),
                  margin: const EdgeInsets.all(16),
                  child: QrImageView(
                    data: data,
                    backgroundColor: Colors.white,
                    errorCorrectionLevel: QrErrorCorrectLevel.M,
                  ),
                ),
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(12),
            child: Text(l.qrZoomHint),
          ),
        ],
      ),
    );
  }
}
