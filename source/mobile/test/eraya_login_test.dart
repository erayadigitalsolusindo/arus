import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:arus_mobile/app.dart';
import 'package:arus_mobile/core/auth/eraya_auth.dart';
import 'package:arus_mobile/core/session/session_controller.dart';
import 'package:arus_mobile/core/session/token_store.dart';
import 'package:arus_mobile/core/widgets/ocean_background.dart';
import 'package:arus_mobile/features/auth/google_register_sheet.dart';

import 'test_helpers.dart';

class _FakeEraya implements ErayaAuth {
  _FakeEraya(this.grant);

  final ErayaGrant? grant;

  @override
  bool get enabled => true;

  @override
  Future<ErayaGrant?> authorize() async => grant;
}

Future<void> _pump(
  WidgetTester tester,
  FakeAdapter raw,
  ErayaGrant? grant,
) async {
  tester.platformDispatcher.localesTestValue = const [Locale('id')];
  addTearDown(tester.platformDispatcher.clearLocalesTestValue);
  tester.view.devicePixelRatio = 1;
  tester.view.physicalSize = const Size(400, 1400);
  addTearDown(tester.view.resetPhysicalSize);
  final tokens = MemoryTokenStore();
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        tokenStoreProvider.overrideWithValue(tokens),
        apiClientProvider.overrideWithValue(clientWith(raw, tokens: tokens)),
        oceanAnimateProvider.overrideWithValue(false),
        erayaAuthProvider.overrideWithValue(_FakeEraya(grant)),
      ],
      child: const ArusApp(),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  testWidgets(
    'login Eraya: tukar code -> login; belum terdaftar -> lembar daftar',
    (tester) async {
      final raw = FakeAdapter((o) {
        if (o.path == '/auth/eraya/exchange') {
          return json(200, {'id_token': 'eraya-id-token'});
        }
        if (o.path == '/auth/eraya/login') {
          return json(404, {
            'error': {'code': 'ERAYA_NOT_REGISTERED', 'message': 'x'},
          });
        }
        return json(201, sessionBody());
      });
      await _pump(tester, raw, ErayaGrant('code-1', 'v' * 64));

      await tester.tap(find.text('Masuk dengan Eraya'));
      await tester.pumpAndSettle();
      expect(raw.calls[0].path, '/auth/eraya/exchange');
      expect(raw.calls[0].data, {
        'code': 'code-1',
        'code_verifier': 'v' * 64,
        'redirect_uri': 'id.co.erayadigital.arus://eraya/callback',
      });
      expect(raw.calls[1].path, '/auth/eraya/login');
      expect(find.text('Daftar dengan Eraya'), findsOneWidget);

      final fields = find.descendant(
        of: find.byType(GoogleRegisterSheet),
        matching: find.byType(TextFormField),
      );
      await tester.enterText(fields.at(0), 'Toko Maju');
      await tester.enterText(fields.at(1), 'Pusat');
      await tester.enterText(fields.at(2), '081234567890');
      await tester.tap(find.byType(CheckboxListTile));
      await tester.tap(find.text('Daftar'));
      await tester.pumpAndSettle();
      final reg = raw.calls.firstWhere((c) => c.path == '/auth/eraya/register');
      expect((reg.data as Map)['id_token'], 'eraya-id-token');
      expect(
        find.textContaining(RegExp(r'^Selamat (pagi|siang|sore|malam), Budi$')),
        findsOneWidget,
      );
    },
  );

  testWidgets('login Eraya dibatalkan: pesan, tanpa panggilan server', (
    tester,
  ) async {
    final raw = FakeAdapter((o) => json(200, sessionBody()));
    await _pump(tester, raw, null);
    await tester.tap(find.text('Masuk dengan Eraya'));
    await tester.pumpAndSettle();
    expect(find.text('Login Eraya dibatalkan.'), findsOneWidget);
    expect(raw.calls, isEmpty);
  });
}
