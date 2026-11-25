import 'dart:io';
import 'dart:typed_data';
import 'dart:ui'
    show Canvas, Color, ImageByteFormat, Paint, PictureRecorder, Rect, Size;

import 'package:flutter/material.dart';
import 'package:path_provider/path_provider.dart';
import 'package:qr_flutter/qr_flutter.dart';
import 'package:share_plus/share_plus.dart';

import '../../l10n/gen/app_localizations.dart';

/// PNG berlatar putih dengan tepi (quiet zone): tetap terbaca scanner di WA/Telegram, termasuk tema gelap.
Future<Uint8List> qrPng(String data) async {
  const size = 1024.0, margin = 96.0;
  final rec = PictureRecorder();
  final c = Canvas(rec);
  c.drawRect(
    const Rect.fromLTWH(0, 0, size, size),
    Paint()..color = const Color(0xFFFFFFFF),
  );
  c.translate(margin, margin);
  QrPainter(
    data: data,
    version: QrVersions.auto,
    errorCorrectionLevel: QrErrorCorrectLevel.M,
    eyeStyle: const QrEyeStyle(color: Color(0xFF000000)),
    dataModuleStyle: const QrDataModuleStyle(color: Color(0xFF000000)),
  ).paint(c, const Size(size - 2 * margin, size - 2 * margin));
  final img = await rec.endRecording().toImage(size.toInt(), size.toInt());
  final bytes = await img.toByteData(format: ImageByteFormat.png);
  return bytes!.buffer.asUint8List();
}

/// QR layar penuh: bisa di-zoom (cubit) dan dibagikan sebagai gambar PNG (WA, dll).
class QrViewerPage extends StatelessWidget {
  const QrViewerPage({super.key, required this.data, this.caption});

  final String data;
  final String? caption;

  Future<void> _share() async {
    final f = File('${(await getTemporaryDirectory()).path}/qris.png');
    await f.writeAsBytes(await qrPng(data));
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
          Padding(padding: const EdgeInsets.all(12), child: Text(l.qrZoomHint)),
        ],
      ),
    );
  }
}
