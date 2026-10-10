import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Test menimpa ini ke false agar tidak ada animasi tak berujung.
final oceanAnimateProvider = Provider<bool>((_) => true);

/// Latar bernuansa lautan: gradasi air, sinar cahaya, hiu berenang bolak-balik, ikan kecil, gelembung naik,
/// dan ombak di dasar. Menyesuaikan mode terang/gelap. Animasi diam bila pengguna mematikan animasi sistem
/// (`MediaQuery.disableAnimations`) atau [animate] false (mis. di test).
class OceanBackground extends StatefulWidget {
  const OceanBackground({super.key, this.child, this.animate = true, this.intensity = 1});

  final Widget? child;
  final bool animate;

  /// 0..1 — mengecilkan jumlah/kontras elemen (mis. di layar kerja agar tidak mengganggu).
  final double intensity;

  @override
  State<OceanBackground> createState() => _OceanBackgroundState();
}

class _OceanBackgroundState extends State<OceanBackground> with SingleTickerProviderStateMixin {
  // Satu putaran = 120 detik; semua gerak dihitung dari fase ini agar mulus saat berulang.
  late final AnimationController _c = AnimationController(vsync: this, duration: const Duration(seconds: 120));
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
          child: CustomPaint(
            painter: _OceanPainter(_c, dark: dark, intensity: widget.intensity.clamp(0, 1).toDouble()),
          ),
        ),
        if (widget.child != null) widget.child!,
      ],
    );
  }
}

class _OceanPainter extends CustomPainter {
  _OceanPainter(this.t, {required this.dark, required this.intensity}) : super(repaint: t);

  final Animation<double> t;
  final bool dark;
  final double intensity;

  static const _period = 120.0; // detik, harus sama dengan durasi controller

  double get _sec => t.value * _period;

  @override
  void paint(Canvas canvas, Size size) {
    if (size.isEmpty) return;
    final s = _sec;
    _water(canvas, size);
    _rays(canvas, size, s);
    _fish(canvas, size, s);
    _shark(canvas, size, s);
    _bubbles(canvas, size, s);
    _waves(canvas, size, s);
  }

  // ---- air ----
  void _water(Canvas canvas, Size size) {
    final colors = dark
        ? const [Color(0xFF04141E), Color(0xFF07304A), Color(0xFF0B5568)]
        : const [Color(0xFFE6F8F7), Color(0xFFB5E4EA), Color(0xFF7CC6D6)];
    final paint = Paint()
      ..shader = LinearGradient(begin: Alignment.topCenter, end: Alignment.bottomCenter, colors: colors).createShader(Offset.zero & size);
    canvas.drawRect(Offset.zero & size, paint);
  }

  // ---- sinar cahaya dari permukaan ----
  void _rays(Canvas canvas, Size size, double s) {
    final base = dark ? 0.07 : 0.16;
    final color = Colors.white;
    for (var i = 0; i < 4; i++) {
      final sway = math.sin(s * 2 * math.pi / (22 + i * 7) + i) * size.width * 0.04;
      final x = size.width * (0.12 + i * 0.26) + sway;
      final w = size.width * (0.10 + 0.03 * (i % 2));
      final path = Path()
        ..moveTo(x, 0)
        ..lineTo(x + w, 0)
        ..lineTo(x + w * 2.2 - size.width * 0.12, size.height * 0.85)
        ..lineTo(x - w * 0.6 - size.width * 0.12, size.height * 0.85)
        ..close();
      final shader = LinearGradient(
        begin: Alignment.topCenter,
        end: Alignment.bottomCenter,
        colors: [color.withValues(alpha: base * intensity), color.withValues(alpha: 0)],
      ).createShader(Rect.fromLTWH(0, 0, size.width, size.height * 0.85));
      canvas.drawPath(path, Paint()..shader = shader);
    }
  }

  // ---- hiu ----
  void _shark(Canvas canvas, Size size, double s) {
    // Bolak-balik: satu lintasan 40 dtk, lalu berbalik arah.
    const leg = 40.0;
    final cycle = s % (leg * 2);
    final goingRight = cycle < leg;
    final p = (goingRight ? cycle : cycle - leg) / leg; // 0..1
    final sharkW = (size.width * 0.52).clamp(150.0, 340.0);
    final span = size.width + sharkW * 2;
    final xLead = -sharkW + span * (goingRight ? p : 1 - p);
    final y = size.height * 0.30 + math.sin(s * 2 * math.pi / 9) * size.height * 0.018;
    final tilt = math.cos(s * 2 * math.pi / 9) * 0.035 * (goingRight ? 1 : -1);

    canvas.save();
    canvas.translate(xLead, y);
    if (!goingRight) canvas.scale(-1, 1);
    canvas.rotate(tilt);
    final swish = math.sin(s * 2 * math.pi / 1.6); // ayunan ekor
    _drawShark(canvas, sharkW, swish);
    canvas.restore();
  }

  void _drawShark(Canvas canvas, double w, double swish) {
    final h = w * 0.34;
    final body = dark ? const Color(0xFF1B3A4B) : const Color(0xFF5E7F93);
    final belly = dark ? const Color(0xFF47697A) : const Color(0xFFD7E6EE);
    final alpha = (0.55 + 0.35 * intensity).clamp(0.0, 1.0);

    // Badan (moncong di kanan x=w, ekor di kiri x=0).
    final tailY = swish * h * 0.18;
    final bodyPath = Path()
      ..moveTo(w, 0)
      ..cubicTo(w * 0.86, -h * 0.55, w * 0.52, -h * 0.62, w * 0.28, -h * 0.20)
      ..cubicTo(w * 0.20, -h * 0.10, w * 0.14, tailY * 0.5, w * 0.08, tailY)
      ..cubicTo(w * 0.14, tailY * 0.6 + h * 0.10, w * 0.22, h * 0.14, w * 0.30, h * 0.24)
      ..cubicTo(w * 0.52, h * 0.58, w * 0.86, h * 0.50, w, 0)
      ..close();
    canvas.drawPath(bodyPath, Paint()..color = body.withValues(alpha: alpha));

    // Perut lebih terang.
    final bellyPath = Path()
      ..moveTo(w * 0.95, h * 0.04)
      ..cubicTo(w * 0.80, h * 0.34, w * 0.52, h * 0.44, w * 0.32, h * 0.20)
      ..cubicTo(w * 0.52, h * 0.30, w * 0.80, h * 0.22, w * 0.95, h * 0.04)
      ..close();
    canvas.drawPath(bellyPath, Paint()..color = belly.withValues(alpha: alpha * 0.9));

    // Sirip punggung.
    final dorsal = Path()
      ..moveTo(w * 0.50, -h * 0.50)
      ..quadraticBezierTo(w * 0.56 + swish * 2, -h * 0.98, w * 0.60, -h * 0.98)
      ..quadraticBezierTo(w * 0.60, -h * 0.74, w * 0.66, -h * 0.50)
      ..close();
    canvas.drawPath(dorsal, Paint()..color = body.withValues(alpha: alpha));

    // Ekor: dua cuping, mengayun.
    final tailX = w * 0.08;
    final tail = Path()
      ..moveTo(tailX + w * 0.04, tailY * 0.9)
      ..quadraticBezierTo(-w * 0.03, tailY - h * 0.45, -w * 0.07, tailY - h * 0.85 + swish * h * 0.1)
      ..quadraticBezierTo(w * 0.02, tailY - h * 0.15, tailX + w * 0.04, tailY * 0.9)
      ..quadraticBezierTo(w * 0.02, tailY + h * 0.18, -w * 0.02, tailY + h * 0.55 + swish * h * 0.08)
      ..quadraticBezierTo(tailX - w * 0.02, tailY + h * 0.10, tailX + w * 0.04, tailY * 0.9)
      ..close();
    canvas.drawPath(tail, Paint()..color = body.withValues(alpha: alpha));

    // Sirip dada.
    final pect = Path()
      ..moveTo(w * 0.62, h * 0.30)
      ..quadraticBezierTo(w * 0.52, h * 0.62 + swish * 3, w * 0.42, h * 0.78)
      ..quadraticBezierTo(w * 0.52, h * 0.46, w * 0.56, h * 0.26)
      ..close();
    canvas.drawPath(pect, Paint()..color = body.withValues(alpha: alpha * 0.95));

    // Insang + mata.
    final gill = Paint()
      ..color = Colors.black.withValues(alpha: dark ? 0.35 : 0.18)
      ..style = PaintingStyle.stroke
      ..strokeWidth = w * 0.006
      ..strokeCap = StrokeCap.round;
    for (var i = 0; i < 3; i++) {
      final gx = w * (0.66 - i * 0.025);
      canvas.drawLine(Offset(gx, -h * 0.12), Offset(gx - w * 0.01, h * 0.12), gill);
    }
    canvas.drawCircle(Offset(w * 0.86, -h * 0.07), w * 0.013, Paint()..color = Colors.white.withValues(alpha: 0.9));
    canvas.drawCircle(Offset(w * 0.862, -h * 0.07), w * 0.0065, Paint()..color = Colors.black.withValues(alpha: 0.85));
  }

  // ---- ikan kecil (gerombolan) ----
  void _fish(Canvas canvas, Size size, double s) {
    if (intensity < 0.35) return;
    final color = dark ? const Color(0xFF5BC8C0) : const Color(0xFF2F9CA8);
    for (var i = 0; i < 6; i++) {
      final dir = i.isEven ? 1.0 : -1.0;
      final speed = 1 / (30.0 + i * 3);
      final phase = (s * speed + i * 0.17) % 1.0;
      final x = dir > 0 ? -40 + (size.width + 80) * phase : size.width + 40 - (size.width + 80) * phase;
      final y = size.height * (0.52 + 0.06 * (i % 3)) + math.sin(s * 0.9 + i * 1.7) * 8;
      final fw = 14.0 + (i % 3) * 3;
      canvas.save();
      canvas.translate(x, y);
      canvas.scale(dir, 1);
      final f = Path()
        ..moveTo(fw, 0)
        ..quadraticBezierTo(fw * 0.5, -fw * 0.42, 0, 0)
        ..quadraticBezierTo(fw * 0.5, fw * 0.42, fw, 0)
        ..close();
      final wag = math.sin(s * 9 + i) * fw * 0.12;
      final tail = Path()
        ..moveTo(0, 0)
        ..lineTo(-fw * 0.42, -fw * 0.32 + wag)
        ..lineTo(-fw * 0.42, fw * 0.32 + wag)
        ..close();
      final paint = Paint()..color = color.withValues(alpha: 0.55 * intensity + 0.1);
      canvas.drawPath(f, paint);
      canvas.drawPath(tail, paint);
      canvas.restore();
    }
  }

  // ---- gelembung ----
  void _bubbles(Canvas canvas, Size size, double s) {
    final count = (22 * intensity).round().clamp(6, 26);
    final fill = Paint()..style = PaintingStyle.fill;
    final stroke = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1;
    for (var i = 0; i < count; i++) {
      final r = _hash(i * 3 + 1); // acak deterministik 0..1
      final r2 = _hash(i * 3 + 2);
      final r3 = _hash(i * 3 + 3);
      final radius = 2.5 + r * 8.5;
      final rise = 1 / (14 + r2 * 22); // siklus per detik
      final phase = (s * rise + r3) % 1.0;
      final y = size.height + radius - (size.height + radius * 2) * phase;
      final x = size.width * (0.04 + 0.92 * r) + math.sin(s * (0.6 + r2) + i) * (6 + 10 * r3);
      final fade = phase < 0.1 ? phase / 0.1 : (phase > 0.9 ? (1 - phase) / 0.1 : 1.0);
      final a = (dark ? 0.5 : 0.7) * fade;
      final c = Offset(x, y);
      fill.shader = RadialGradient(
        colors: [Colors.white.withValues(alpha: 0.02 * a * 10), Colors.white.withValues(alpha: 0.28 * a)],
        stops: const [0.4, 1],
      ).createShader(Rect.fromCircle(center: c, radius: radius));
      canvas.drawCircle(c, radius, fill);
      stroke.color = Colors.white.withValues(alpha: 0.6 * a);
      canvas.drawCircle(c, radius, stroke);
      canvas.drawCircle(c.translate(-radius * 0.35, -radius * 0.35), radius * 0.22, Paint()..color = Colors.white.withValues(alpha: 0.8 * a));
    }
  }

  // ---- ombak di dasar ----
  void _waves(Canvas canvas, Size size, double s) {
    final layers = [
      (dark ? const Color(0xFF0B6B7E) : const Color(0xFF4FB3C4), 0.10, 14.0, 7.0, 0.0),
      (dark ? const Color(0xFF0A5568) : const Color(0xFF3A9DB0), 0.075, 18.0, 9.5, 1.7),
      (dark ? const Color(0xFF083E50) : const Color(0xFF2A8599), 0.05, 22.0, 12.0, 3.1),
    ];
    for (final (color, hFrac, amp, speed, off) in layers) {
      final baseY = size.height * (1 - hFrac);
      final path = Path()..moveTo(0, size.height);
      for (var x = 0.0; x <= size.width; x += 6) {
        final y = baseY + math.sin((x / size.width) * 2 * math.pi * 1.6 + s * (2 * math.pi / speed) + off) * amp * 0.5;
        path.lineTo(x, y);
      }
      path
        ..lineTo(size.width, size.height)
        ..close();
      canvas.drawPath(path, Paint()..color = color.withValues(alpha: 0.55 + 0.25 * intensity));
    }
  }

  /// Acak deterministik 0..1 (tanpa state) agar posisi gelembung konsisten antar frame.
  static double _hash(int n) {
    final x = math.sin(n * 12.9898 + 78.233) * 43758.5453;
    return x - x.floorToDouble();
  }

  @override
  bool shouldRepaint(covariant _OceanPainter old) => old.dark != dark || old.intensity != intensity;
}
