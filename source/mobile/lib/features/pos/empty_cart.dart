import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/theme/app_theme.dart';
import '../../core/widgets/ocean_background.dart';
import '../../l10n/gen/app_localizations.dart';

/// Keranjang kosong yang "hidup": keranjang melayang pelan, cincin berdenyut, gelembung naik.
/// Diam bila animasi sistem dimatikan atau [oceanAnimateProvider] false (test).
class EmptyCart extends ConsumerStatefulWidget {
  const EmptyCart({super.key});

  @override
  ConsumerState<EmptyCart> createState() => _EmptyCartState();
}

class _EmptyCartState extends ConsumerState<EmptyCart>
    with SingleTickerProviderStateMixin {
  late final AnimationController _c = AnimationController(
    vsync: this,
    duration: const Duration(seconds: 4),
  );
  bool _running = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final on =
        ref.read(oceanAnimateProvider) &&
        !MediaQuery.disableAnimationsOf(context);
    if (on && !_running) {
      _c.repeat();
    } else if (!on && _running) {
      _c.stop();
    }
    _running = on;
  }

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            SizedBox(
              width: 170,
              height: 170,
              child: AnimatedBuilder(
                animation: _c,
                builder: (_, _) {
                  final t = _c.value;
                  final bob = math.sin(t * 2 * math.pi) * 6;
                  return Stack(
                    alignment: Alignment.center,
                    children: [
                      for (final phase in const [0.0, 0.5])
                        _ring((t + phase) % 1, pal.primary),
                      for (var i = 0; i < 5; i++) _bubble(i, t, pal.primary),
                      Transform.translate(
                        offset: Offset(0, bob),
                        child: Container(
                          width: 92,
                          height: 92,
                          decoration: BoxDecoration(
                            color: pal.primarySoft,
                            shape: BoxShape.circle,
                          ),
                          child: Icon(
                            Icons.shopping_cart_outlined,
                            size: 48,
                            color: pal.primary,
                          ),
                        ),
                      ),
                    ],
                  );
                },
              ),
            ),
            const SizedBox(height: 12),
            Text(
              l.posCartEmptyTitle,
              textAlign: TextAlign.center,
              style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 6),
            Text(
              l.posCartEmptyHint,
              textAlign: TextAlign.center,
              style: TextStyle(fontSize: 16, color: pal.textTertiary),
            ),
          ],
        ),
      ),
    );
  }

  Widget _ring(double p, Color color) => Container(
    width: 92 + 70 * p,
    height: 92 + 70 * p,
    decoration: BoxDecoration(
      shape: BoxShape.circle,
      border: Border.all(
        color: color.withValues(alpha: 0.28 * (1 - p)),
        width: 2,
      ),
    ),
  );

  Widget _bubble(int i, double t, Color color) {
    final p = (t + i / 5) % 1;
    final x = math.sin((i * 2.3) + p * 4) * 4 + (i - 2) * 26;
    final size = 6.0 + (i % 3) * 2;
    return Positioned(
      bottom: 20 + p * 120,
      left: 85 + x - size / 2,
      child: Opacity(
        opacity: (1 - p) * 0.5,
        child: Container(
          width: size,
          height: size,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            border: Border.all(color: color, width: 1.5),
          ),
        ),
      ),
    );
  }
}
