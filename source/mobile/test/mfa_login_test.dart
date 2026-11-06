import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:arus_mobile/app.dart';
import 'package:arus_mobile/core/auth/google_auth.dart';
import 'package:arus_mobile/core/session/session_controller.dart';
import 'package:arus_mobile/core/session/token_store.dart';
import 'package:arus_mobile/core/widgets/ocean_background.dart';

import 'test_helpers.dart';

class _FakeGoogle implements GoogleAuth {
  @override
  bool get enabled => true;

  @override
  Future<String?> idToken() async => 'id-token-1';
}

void main() {
  testWidgets('login ber-2FA: tantangan -> kode salah -> kode benar -> sesi', (
    tester,
  ) async {
    tester.platformDispatcher.localesTestValue = const [Locale('id')];
    addTearDown(tester.platformDispatcher.clearLocalesTestValue);
    tester.view.devicePixelRatio = 1;
    tester.view.physicalSize = const Size(400, 1400);
    addTearDown(tester.view.resetPhysicalSize);

    final raw = FakeAdapter((o) {
      switch (o.path) {
        case '/auth/google/login':
          return json(200, {'mfa_required': true, 'mfa_token': 'tantangan-1'});
        case '/auth/login/mfa':
          final code = (o.data as Map)['code'];
          if (code != '123456') {
            return json(401, {
              'error': {
                'code': 'MFA_CODE_INVALID',
                'message': 'x',
                'attempts_left': 4,
              },
            });
          }
          return json(200, sessionBody());
      }
      return json(500, {
        'error': {'code': 'UNKNOWN', 'message': 'x'},
      });
    });
    final tokens = MemoryTokenStore();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          tokenStoreProvider.overrideWithValue(tokens),
          apiClientProvider.overrideWithValue(clientWith(raw, tokens: tokens)),
          oceanAnimateProvider.overrideWithValue(false),
          googleAuthProvider.overrideWithValue(_FakeGoogle()),
        ],
        child: const ArusApp(),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('Masuk dengan Google'));
    await tester.pumpAndSettle();
    expect(find.text('Verifikasi dua langkah'), findsOneWidget);

    final field = find.descendant(
      of: find.byType(AlertDialog),
      matching: find.byType(TextField),
    );
    await tester.enterText(field, '000000');
    await tester.tap(find.text('Verifikasi'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Kode verifikasi salah'), findsOneWidget);
    expect(raw.calls.last.data, {'mfa_token': 'tantangan-1', 'code': '000000'});

    await tester.enterText(field, '123456');
    await tester.tap(find.text('Verifikasi'));
    await tester.pumpAndSettle();
    expect(
      find.textContaining(RegExp(r'^Selamat (pagi|siang|sore|malam), Budi$')),
      findsOneWidget,
    );
  });
}
