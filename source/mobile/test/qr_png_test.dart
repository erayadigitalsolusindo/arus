import 'dart:ui' as ui;

import 'package:arus_mobile/features/pos/qr_viewer.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('PNG QR berlatar putih dengan tepi (terbaca di tema gelap WA)', (
    t,
  ) async {
    await t.runAsync(() async {
      final png = await qrPng(
        '00020101021226610014ID.CO.QRIS.WWW0215ID1234567890123',
      );
      final img = await (await ui.instantiateImageCodec(png)).getNextFrame();
      expect(img.image.width, 1024);
      final px = (await img.image.toByteData(
        format: ui.ImageByteFormat.rawRgba,
      ))!;
      int at(int x, int y) => (y * 1024 + x) * 4;
      // pojok = tepi putih opak
      for (final o in [at(2, 2), at(1021, 1021)]) {
        expect(
          [
            px.getUint8(o),
            px.getUint8(o + 1),
            px.getUint8(o + 2),
            px.getUint8(o + 3),
          ],
          [255, 255, 255, 255],
        );
      }
      // modul hitam ada di dalam area QR (pojok kiri atas = finder pattern)
      var dark = 0;
      for (var x = 0; x < 1024; x++) {
        if (px.getUint8(at(x, 300)) < 40) dark++;
      }
      expect(dark, greaterThan(50), reason: 'harus ada modul hitam di baris tengah atas');
    });
  });
}
