import 'dart:async';

import 'package:arus_mobile/core/update/play_update.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

class _FakePlay extends PlayUpdateClient {
  _FakePlay({this.offer, this.fail = false});

  final PlayUpdateOffer? offer;
  final bool fail;
  final done = StreamController<bool>.broadcast();
  bool flexibleStarted = false;
  bool immediateStarted = false;

  @override
  Future<PlayUpdateOffer?> check() async {
    if (fail) throw Exception('no play');
    return offer;
  }

  @override
  Future<bool> startFlexible() async => flexibleStarted = true;

  @override
  Future<bool> performImmediate() async => immediateStarted = true;

  @override
  Stream<bool> get downloaded => done.stream;
}

const _offer = PlayUpdateOffer(
  versionCode: 9,
  priority: 0,
  flexibleAllowed: true,
  immediateAllowed: true,
);

ProviderContainer _box(_FakePlay f) {
  final c = ProviderContainer(
    overrides: [playUpdateClientProvider.overrideWithValue(f)],
  );
  addTearDown(c.dispose);
  return c;
}

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  test(
    'cek otomatis: kartu wajib muncul, diam bila tak ada pembaruan',
    () async {
      final c = _box(_FakePlay(offer: _offer));
      final n = c.read(playUpdateProvider.notifier);
      await n.check();
      expect(c.read(playUpdateProvider).showCard, isTrue);
      expect(c.read(playUpdateProvider).hasUpdate, isTrue);
      expect(c.read(playUpdateProvider).forced, isTrue); // tak ada "Nanti"

      final none = _box(_FakePlay());
      await none.read(playUpdateProvider.notifier).check();
      expect(none.read(playUpdateProvider).showCard, isFalse);
      expect(none.read(playUpdateProvider).hasUpdate, isFalse);
    },
  );

  test('cek manual menampilkan hasil: terbaru dan gagal', () async {
    final c = _box(_FakePlay());
    await c.read(playUpdateProvider.notifier).check(manual: true);
    expect(c.read(playUpdateProvider).phase, PlayUpdatePhase.upToDate);
    expect(c.read(playUpdateProvider).showCard, isTrue);
    c.read(playUpdateProvider.notifier).dismissCard();
    expect(c.read(playUpdateProvider).showCard, isFalse);

    final bad = _box(_FakePlay(fail: true));
    await bad.read(playUpdateProvider.notifier).check(manual: true);
    expect(bad.read(playUpdateProvider).phase, PlayUpdatePhase.failed);
    expect(bad.read(playUpdateProvider).showCard, isTrue);
  });

  test('pembaruan selalu memakai layar penuh Play', () async {
    final f = _FakePlay(
      offer: const PlayUpdateOffer(
        versionCode: 10,
        priority: 0,
        flexibleAllowed: true,
        immediateAllowed: true,
      ),
    );
    final c = _box(f);
    final n = c.read(playUpdateProvider.notifier);
    await n.check();
    await n.start();
    expect(f.immediateStarted, isTrue);
    expect(f.flexibleStarted, isFalse);
  });
}
