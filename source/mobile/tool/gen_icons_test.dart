// Pembuat ikon Android dari assets/images/logo_tanpa_text-no-bg.svg (tanpa alat luar):
//   flutter test tool/gen_icons_test.dart
// Menulis mipmap-*/ic_launcher.png (ikon lama, latar putih membulat) dan ic_launcher_foreground.png (adaptive, latar transparan).
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter_svg/flutter_svg.dart';
import 'package:flutter_test/flutter_test.dart';

const _res = 'android/app/src/main/res';
const _legacy = {'mdpi': 48, 'hdpi': 72, 'xhdpi': 96, 'xxhdpi': 144, 'xxxhdpi': 192};

Future<ui.Image> _crop(ui.Image src, ui.Rect r, int side, {double fill = 1, ui.Color? bg, double radius = 0}) async {
  final rec = ui.PictureRecorder();
  final c = ui.Canvas(rec);
  final s = side.toDouble();
  if (bg != null) {
    c.drawRRect(ui.RRect.fromRectXY(ui.Rect.fromLTWH(0, 0, s, s), s * radius, s * radius), ui.Paint()..color = bg);
  }
  final scale = s * fill / (r.width > r.height ? r.width : r.height);
  final dw = r.width * scale, dh = r.height * scale;
  c.drawImageRect(
    src,
    r,
    ui.Rect.fromLTWH((s - dw) / 2, (s - dh) / 2, dw, dh),
    ui.Paint()..filterQuality = ui.FilterQuality.high,
  );
  return rec.endRecording().toImage(side, side);
}

Future<void> _save(ui.Image img, String path) async {
  final b = (await img.toByteData(format: ui.ImageByteFormat.png))!;
  File(path).writeAsBytesSync(b.buffer.asUint8List());
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test('generate launcher icons', () async {
    final svg = File('assets/images/logo_tanpa_text-no-bg.svg').readAsStringSync();
    final info = await vg.loadPicture(SvgStringLoader(svg), null);
    final full = await info.picture.toImage(1774, 887);
    final px = (await full.toByteData(format: ui.ImageByteFormat.rawRgba))!.buffer.asUint8List();
    var x0 = full.width, y0 = full.height, x1 = 0, y1 = 0;
    for (var y = 0; y < full.height; y++) {
      for (var x = 0; x < full.width; x++) {
        if (px[(y * full.width + x) * 4 + 3] > 8) {
          if (x < x0) x0 = x;
          if (x > x1) x1 = x;
          if (y < y0) y0 = y;
          if (y > y1) y1 = y;
        }
      }
    }
    final box = ui.Rect.fromLTRB(x0.toDouble(), y0.toDouble(), x1 + 1.0, y1 + 1.0);
    // ignore: avoid_print
    print('bbox logo: $box');
    for (final e in _legacy.entries) {
      await _save(
        await _crop(full, box, e.value, fill: 0.72, bg: const ui.Color(0xFFFFFFFF), radius: 0.22),
        '$_res/mipmap-${e.key}/ic_launcher.png',
      );
      // Adaptive: kanvas 108dp, logo di zona aman (~58%).
      final fg = (e.value * 108 / 48).round();
      await _save(await _crop(full, box, fg, fill: 0.58), '$_res/mipmap-${e.key}/ic_launcher_foreground.png');
    }
  });
}
