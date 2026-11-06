import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:arus_mobile/app.dart';
import 'package:arus_mobile/core/auth/google_auth.dart';
import 'package:arus_mobile/core/session/session_controller.dart';
import 'package:arus_mobile/core/session/token_store.dart';
import 'package:arus_mobile/core/widgets/ocean_background.dart';
import 'package:arus_mobile/features/auth/google_register_sheet.dart';

import 'test_helpers.dart';

class _FakeGoogle implements GoogleAuth {
  _FakeGoogle(this.token);

  final String? token;

  @override
  bool get enabled => true;

  @override
  Future<String?> idToken() async => token;
}

void main() {
  testWidgets(
    'login Google: belum terdaftar -> lembar daftar -> sesi terbentuk',
    (tester) async {
      tester.platformDispatcher.localesTestValue = const [Locale('id')];
      addTearDown(tester.platformDispatcher.clearLocalesTestValue);
      tester.view.devicePixelRatio = 1;
      tester.view.physicalSize = const Size(400, 1400);
      addTearDown(tester.view.resetPhysicalSize);

      final raw = FakeAdapter((o) {
        if (o.path == '/auth/google/login') {
          return json(404, {
            'error': {'code': 'GOOGLE_NOT_REGISTERED', 'message': 'x'},
          });
        }
        return json(201, sessionBody());
      });
      final tokens = MemoryTokenStore();
      final api = clientWith(raw, tokens: tokens);
      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            tokenStoreProvider.overrideWithValue(tokens),
            apiClientProvider.overrideWithValue(api),
            oceanAnimateProvider.overrideWithValue(false),
            googleAuthProvider.overrideWithValue(_FakeGoogle('id-token-1')),
          ],
          child: const ArusApp(),
        ),
      );
      await tester.pumpAndSettle();

      await tester.tap(find.text('Masuk dengan Google'));
      await tester.pumpAndSettle();
      expect(find.text('Daftar dengan Google'), findsOneWidget);
      expect(raw.calls.first.path, '/auth/google/login');
      expect(raw.calls.first.data, {
        'id_token': 'id-token-1',
        'remember': true,
      });

      // kosong -> validasi
      await tester.tap(find.text('Daftar'));
      await tester.pumpAndSettle();
      expect(find.text('Wajib diisi.'), findsNWidgets(3));
      expect(find.text('Anda harus menyetujui syarat.'), findsOneWidget);

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

      final reg = raw.calls.firstWhere(
        (c) => c.path == '/auth/google/register',
      );
      expect(reg.data, {
        'id_token': 'id-token-1',
        'business_name': 'Toko Maju',
        'outlet_name': 'Pusat',
        'phone': '081234567890',
        'accept_terms': true,
      });
      expect(
        find.textContaining(RegExp(r'^Selamat (pagi|siang|sore|malam), Budi$')),
        findsOneWidget,
      );
    },
  );

  testWidgets('login Google dibatalkan: pesan, tanpa panggilan server', (
    tester,
  ) async {
    tester.platformDispatcher.localesTestValue = const [Locale('id')];
    addTearDown(tester.platformDispatcher.clearLocalesTestValue);
    tester.view.devicePixelRatio = 1;
    tester.view.physicalSize = const Size(400, 1400);
    addTearDown(tester.view.resetPhysicalSize);
    final raw = FakeAdapter((o) => json(200, sessionBody()));
    final tokens = MemoryTokenStore();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          tokenStoreProvider.overrideWithValue(tokens),
          apiClientProvider.overrideWithValue(clientWith(raw, tokens: tokens)),
          oceanAnimateProvider.overrideWithValue(false),
          googleAuthProvider.overrideWithValue(_FakeGoogle(null)),
        ],
        child: const ArusApp(),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('Masuk dengan Google'));
    await tester.pumpAndSettle();
    expect(find.text('Login Google dibatalkan.'), findsOneWidget);
    expect(raw.calls, isEmpty);
  });
}
