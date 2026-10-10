import 'dart:math' as math;

import 'package:flutter/material.dart';

/// Latar langit untuk beranda, gunung dan laut di bawahnya.
/// Siang: matahari, awan putih, camar. Malam: bulan, bintang berkelip, meteor, awan gelap.
/// Animasi diam bila [animate] false (test) atau pengguna mematikan animasi sistem.
class BeachBackground extends StatefulWidget {
  const BeachBackground({super.key, this.child, this.animate = true});

  final Widget? child;
  final bool animate;

  @override
  State<BeachBackground> createState() => _BeachBackgroundState();
}

class _BeachBackgroundState extends State<BeachBackground>
    with SingleTickerProviderStateMixin {
  // Satu putaran = 120 detik; semua gerak dihitung dari fase ini agar mulus saat berulang.
  late final AnimationController _c = AnimationController(
    vsync: this,
    duration: const Duration(seconds: 120),
  );
  bool _running = false;

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  void _sync(bool shouldRun) {
    if (shouldRun == _running) return;
    _running = shouldRun;
    if (shouldRun) {
      _c.repeat();
    } else {
      _c.stop();
    }
  }

  @override
  Widget build(BuildContext context) {
    final dark = Theme.of(context).brightness == Brightness.dark;
    final reduce = MediaQuery.maybeDisableAnimationsOf(context) ?? false;
    _sync(widget.animate && !reduce);
    return Stack(
      fit: StackFit.expand,
      children: [
        RepaintBoundary(
          child: CustomPaint(painter: _SkyPainter(_c, dark: dark)),
        ),
        if (widget.child != null) widget.child!,
      ],
    );
  }
}

class _SkyPainter extends CustomPainter {
  _SkyPainter(this.t, {required this.dark}) : super(repaint: t);

  final Animation<double> t;
  final bool dark;

  static const _period = 120.0; // detik, harus sama dengan durasi controller
  static const _horizon = 0.66; // garis kaki gunung / permukaan laut

  @override
  void paint(Canvas canvas, Size size) {
    if (size.isEmpty) return;
    final s = t.value * _period;
    _sky(canvas, size);
    if (dark) {
      _stars(canvas, size, s);
      _meteors(canvas, size, s);
      _moon(canvas, size, s);
    } else {
      _sun(canvas, size, s);
    }
    _clouds(canvas, size, s);
    if (!dark) _gulls(canvas, size, s);
    _mountains(canvas, size);
    _sea(canvas, size, s);
  }

  // ---- langit ----
  void _sky(Canvas canvas, Size size) {
    final colors = dark
        ? const [Color(0xFF040A1C), Color(0xFF0E1B3D), Color(0xFF2A2F5C)]
        : const [Color(0xFF4FB0EA), Color(0xFF9BD6F5), Color(0xFFE3F4FB)];
    final rect = Rect.fromLTWH(0, 0, size.width, size.height * _horizon);
    canvas.drawRect(
      rect,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: colors,
        ).createShader(rect),
    );
  }

  // ---- siang ----
  void _sun(Canvas canvas, Size size, double s) {
    final c = Offset(size.width * 0.74, size.height * 0.17);
    final r = size.width * 0.10;
    canvas.drawCircle(
      c,
      r * 3.2,
      Paint()
        ..shader = RadialGradient(
          colors: [
            const Color(0xFFFFE08A).withValues(alpha: 0.65),
            const Color(0xFFFFE08A).withValues(alpha: 0),
          ],
        ).createShader(Rect.fromCircle(center: c, radius: r * 3.2)),
    );
    // Sinar berputar pelan.
    final ray = Paint()
      ..color = const Color(0xFFFFE9A8).withValues(alpha: 0.55)
      ..strokeWidth = 3
      ..strokeCap = StrokeCap.round;
    for (var i = 0; i < 14; i++) {
      final a = i * 2 * math.pi / 14 + s * 0.05;
      final inner = r * 1.25;
      final outer = r * (1.6 + 0.2 * math.sin(s * 1.5 + i));
      canvas.drawLine(
        c + Offset(math.cos(a), math.sin(a)) * inner,
        c + Offset(math.cos(a), math.sin(a)) * outer,
        ray,
      );
    }
    canvas.drawCircle(c, r, Paint()..color = const Color(0xFFFFC93C));
    canvas.drawCircle(
      c.translate(-r * 0.2, -r * 0.2),
      r * 0.7,
      Paint()..color = const Color(0xFFFFD968),
    );
  }

  void _gulls(Canvas canvas, Size size, double s) {
    final p = Paint()
      ..color = const Color(0xFF3B4A5A)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2
      ..strokeCap = StrokeCap.round;
    for (var i = 0; i < 3; i++) {
      final span = size.width + 80;
      final phase = (s / (34.0 + i * 8) + i * 0.27) % 1.0;
      final x = -40 + span * phase;
      final y = size.height * (0.27 + 0.04 * i) + math.sin(s * 0.7 + i * 2) * 8;
      final flap = math.sin(s * 5 + i * 1.9) * 5;
      final wing = 9.0 + i * 1.5;
      canvas.drawPath(
        Path()
          ..moveTo(x - wing, y - flap)
          ..quadraticBezierTo(x - wing * 0.4, y - 5 + flap * 0.4, x, y)
          ..quadraticBezierTo(
            x + wing * 0.4,
            y - 5 + flap * 0.4,
            x + wing,
            y - flap,
          ),
        p,
      );
    }
  }

  // ---- malam ----
  void _stars(Canvas canvas, Size size, double s) {
    final p = Paint();
    for (var i = 0; i < 70; i++) {
      final x = size.width * _hash(i * 3 + 1);
      final y = size.height * _horizon * 0.95 * _hash(i * 3 + 2);
      final tw = 0.5 + 0.5 * math.sin(s * (0.8 + _hash(i * 3 + 3) * 2) + i);
      final r = 0.6 + _hash(i * 7 + 5) * 1.3;
      p.color = Colors.white.withValues(alpha: 0.25 + 0.7 * tw);
      canvas.drawCircle(Offset(x, y), r, p);
      if (r > 1.4) {
        // Bintang besar berbentuk tanda tambah kecil.
        final l = Paint()
          ..color = Colors.white.withValues(alpha: 0.35 * tw)
          ..strokeWidth = 1;
        canvas.drawLine(Offset(x - 4, y), Offset(x + 4, y), l);
        canvas.drawLine(Offset(x, y - 4), Offset(x, y + 4), l);
      }
    }
  }

  void _meteors(Canvas canvas, Size size, double s) {
    // Tiga slot, tiap slot jatuh sekali per siklus ~14–19 detik, selama ~1,2 detik.
    for (var k = 0; k < 3; k++) {
      final cycle = 14.0 + k * 2.5;
      final local = (s + k * 6.3) % cycle;
      const dur = 1.2;
      if (local > dur) continue;
      final p = local / dur;
      final seed = ((s + k * 6.3) / cycle).floor() + k * 11;
      final sx = size.width * (0.25 + 0.7 * _hash(seed * 2 + 1));
      final sy = size.height * (0.02 + 0.22 * _hash(seed * 2 + 2));
      final dir = Offset(-0.8, 0.6);
      final len = size.width * 0.38;
      final head = Offset(sx, sy) + dir * (len * p * 1.6);
      final tail = head - dir * (len * 0.35);
      final fade = math.sin(p * math.pi);
      final paint = Paint()
        ..shader = LinearGradient(
          colors: [
            Colors.white.withValues(alpha: 0),
            Colors.white.withValues(alpha: 0.95 * fade),
          ],
        ).createShader(Rect.fromPoints(tail, head))
        ..strokeWidth = 2.4
        ..strokeCap = StrokeCap.round;
      canvas.drawLine(tail, head, paint);
      canvas.drawCircle(
        head,
        2.6,
        Paint()..color = Colors.white.withValues(alpha: fade),
      );
    }
  }

  void _moon(Canvas canvas, Size size, double s) {
    final c = Offset(size.width * 0.76, size.height * 0.16);
    final r = size.width * 0.075;
    canvas.drawCircle(
      c,
      r * 3,
      Paint()
        ..shader = RadialGradient(
          colors: [
            const Color(0xFFFFF2C4).withValues(alpha: 0.28),
            const Color(0xFFFFF2C4).withValues(alpha: 0),
          ],
        ).createShader(Rect.fromCircle(center: c, radius: r * 3)),
    );
    canvas.drawCircle(c, r, Paint()..color = const Color(0xFFFFF6DA));
    // Kawah.
    final crater = Paint()..color = const Color(0xFFE6D9AE);
    canvas.drawCircle(c.translate(-r * 0.3, -r * 0.2), r * 0.18, crater);
    canvas.drawCircle(c.translate(r * 0.25, r * 0.3), r * 0.25, crater);
    canvas.drawCircle(c.translate(r * 0.3, -r * 0.35), r * 0.1, crater);
  }

  // ---- awan ----
  void _clouds(Canvas canvas, Size size, double s) {
    final count = dark ? 4 : 3;
    for (var i = 0; i < count; i++) {
      final w = size.width * (0.34 + 0.08 * (i % 3));
      final span = size.width + w * 2;
      final phase = (s / (60.0 + i * 22) + i * 0.29) % 1.0;
      final x = -w + span * phase;
      final y = size.height * (0.09 + 0.09 * i);
      final color = dark
          ? Color.lerp(
              const Color(0xFF05070F),
              const Color(0xFF1B2140),
              (i % 2) * 0.6,
            )!
          : Colors.white;
      _cloud(
        canvas,
        Offset(x, y),
        w,
        color.withValues(alpha: dark ? 0.9 : 0.9),
      );
    }
  }

  void _cloud(Canvas canvas, Offset o, double w, Color color) {
    final h = w * 0.24;
    final p = Paint()..color = color;
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        Rect.fromLTWH(o.dx, o.dy + h * 0.55, w, h),
        Radius.circular(h / 2),
      ),
      p,
    );
    canvas.drawCircle(Offset(o.dx + w * 0.30, o.dy + h * 0.6), h * 0.75, p);
    canvas.drawCircle(Offset(o.dx + w * 0.52, o.dy + h * 0.38), h * 1.0, p);
    canvas.drawCircle(Offset(o.dx + w * 0.74, o.dy + h * 0.62), h * 0.7, p);
  }

  // ---- gunung ----
  void _mountains(Canvas canvas, Size size) {
    final base = size.height * _horizon;
    void ridge(List<(double, double)> pts, Color color) {
      final path = Path()..moveTo(0, base + 2);
      for (final (fx, fy) in pts) {
        path.lineTo(size.width * fx, base - size.height * fy);
      }
      path
        ..lineTo(size.width, base + 2)
        ..close();
      canvas.drawPath(path, Paint()..color = color);
    }

    // Lapisan belakang (jauh, pucat) lalu depan (gelap).
    ridge(const [
      (0.0, 0.07),
      (0.18, 0.15),
      (0.32, 0.08),
      (0.52, 0.19),
      (0.72, 0.09),
      (0.9, 0.16),
      (1.0, 0.08),
    ], dark ? const Color(0xFF1A2447) : const Color(0xFF8DB5CF));
    ridge(const [
      (0.0, 0.12),
      (0.14, 0.05),
      (0.3, 0.14),
      (0.46, 0.04),
      (0.68, 0.17),
      (0.84, 0.06),
      (1.0, 0.13),
    ], dark ? const Color(0xFF0B1230) : const Color(0xFF5C8EA8));
    // Puncak bersalju/berembun di gunung depan paling tinggi.
    final cap = Path()
      ..moveTo(size.width * 0.62, base - size.height * 0.128)
      ..lineTo(size.width * 0.68, base - size.height * 0.17)
      ..lineTo(size.width * 0.74, base - size.height * 0.128)
      ..lineTo(size.width * 0.70, base - size.height * 0.138)
      ..lineTo(size.width * 0.68, base - size.height * 0.122)
      ..lineTo(size.width * 0.66, base - size.height * 0.138)
      ..close();
    canvas.drawPath(
      cap,
      Paint()..color = Colors.white.withValues(alpha: dark ? 0.55 : 0.9),
    );
  }

  // ---- laut ----
  void _sea(Canvas canvas, Size size, double s) {
    final top = size.height * _horizon;
    final rect = Rect.fromLTRB(0, top, size.width, size.height);
    canvas.drawRect(
      rect,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: dark
              ? const [Color(0xFF0C2347), Color(0xFF050C22)]
              : const [Color(0xFF58BCE0), Color(0xFF1C84B8)],
        ).createShader(rect),
    );
    // Pantulan matahari / bulan.
    final glint = Paint()
      ..color = (dark ? const Color(0xFFFFF2C4) : Colors.white).withValues(
        alpha: dark ? 0.35 : 0.5,
      )
      ..strokeWidth = 2.5
      ..strokeCap = StrokeCap.round;
    final gx = size.width * (dark ? 0.76 : 0.74);
    for (var i = 0; i < 7; i++) {
      final y = top + (size.height - top) * (0.05 + i * 0.07);
      final w = size.width * (0.03 + 0.012 * i);
      final x = gx + math.sin(s * 0.9 + i * 1.3) * size.width * 0.025;
      canvas.drawLine(Offset(x - w, y), Offset(x + w, y), glint);
    }
    // Ombak berlapis.
    for (var k = 0; k < 3; k++) {
      final y0 = top + (size.height - top) * (0.22 + k * 0.28);
      final path = Path()..moveTo(0, y0);
      for (var x = 0.0; x <= size.width; x += 8) {
        path.lineTo(
          x,
          y0 +
              math.sin(x / size.width * 2 * math.pi * (2 + k) + s * (1 + k)) *
                  (3 + k * 1.5),
        );
      }
      canvas.drawPath(
        path,
        Paint()
          ..color = Colors.white.withValues(alpha: dark ? 0.10 : 0.30)
          ..style = PaintingStyle.stroke
          ..strokeWidth = 1.6,
      );
    }
  }

  /// Acak deterministik 0..1 (tanpa state) agar posisi konsisten antar frame.
  static double _hash(int n) {
    final x = math.sin(n * 12.9898 + 78.233) * 43758.5453;
    return x - x.floorToDouble();
  }

  @override
  bool shouldRepaint(covariant _SkyPainter old) => old.dark != dark;
}
