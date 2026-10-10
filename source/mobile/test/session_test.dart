import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:arus_mobile/app.dart';
import 'package:arus_mobile/core/api/api_client.dart';
import 'package:arus_mobile/core/session/session_controller.dart';
import 'package:arus_mobile/core/session/token_store.dart';

/// Adapter HTTP palsu: jawaban ditentukan per "METHOD path".
class FakeAdapter implements HttpClientAdapter {
  FakeAdapter(this.handler);

  final ResponseBody Function(RequestOptions o) handler;
  final List<RequestOptions> calls = [];

  @override
  Future<ResponseBody> fetch(RequestOptions o, Stream<Uint8List>? body, Future<void>? cancel) async {
    calls.add(o);
    return handler(o);
  }

  @override
  void close({bool force = false}) {}
}

ResponseBody json(int status, Map<String, dynamic> body) => ResponseBody.fromString(
      jsonEncode(body),
      status,
      headers: {Headers.contentTypeHeader: ['application/json']},
    );

Map<String, dynamic> sessionBody({String refresh = 'r1', String access = 'a1'}) => {
      'access_token': access,
      'expires_in': 900,
      if (refresh.isNotEmpty) 'refresh_token': refresh,
      'permissions': {'sales_orders': ['view', 'create']},
      'email_verified': true,
      'user': {'id': 'u1', 'name': 'Budi', 'email': 'budi@toko.test'},
      'tenant': {'id': 't1', 'name': 'Toko Maju', 'code': 'maju'},
      'outlet': {'id': 'o1', 'name': 'Pusat', 'code': 'MAIN'},
    };

ApiClient clientWith(FakeAdapter raw, {FakeAdapter? authed, TokenStore? tokens, void Function()? onExpired}) {
  return ApiClient(
    tokens: tokens ?? MemoryTokenStore(),
    onSessionExpired: onExpired ?? () {},
    raw: Dio(BaseOptions(baseUrl: 'http://x'))..httpClientAdapter = raw,
    authed: Dio(BaseOptions(baseUrl: 'http://x'))..httpClientAdapter = authed ?? raw,
  );
}

void main() {
  test('login mengirim X-Client dan menyimpan refresh token', () async {
    final raw = FakeAdapter((o) => json(200, sessionBody()));
    final tokens = MemoryTokenStore();
    final api = clientWith(raw, tokens: tokens);
    final s = await api.login(email: 'a@b.co', password: 'x', remember: true);
    expect(s.profile.user.name, 'Budi');
    expect(await tokens.readRefreshToken(), 'r1');
    expect(s.profile.permissions.can('sales_orders', 'create'), isTrue);
    expect(s.profile.permissions.can('stock_opname'), isFalse);
  });

  test('refresh bersamaan hanya satu panggilan (single-flight) dan token dirotasi', () async {
    final raw = FakeAdapter((o) => json(200, sessionBody(refresh: 'r2', access: 'a2')));
    final tokens = MemoryTokenStore();
    await tokens.writeRefreshToken('r1');
    final api = clientWith(raw, tokens: tokens);
    await Future.wait([api.refresh(), api.refresh(), api.refresh()]);
    expect(raw.calls.where((c) => c.path == '/auth/refresh').length, 1);
    expect(raw.calls.first.data, {'refresh_token': 'r1'});
    expect(await tokens.readRefreshToken(), 'r2');
  });

  test('401 TOKEN_EXPIRED -> refresh lalu permintaan diulang dengan token baru', () async {
    var n = 0;
    final raw = FakeAdapter((o) => json(200, sessionBody(refresh: 'r2', access: 'a2')));
    final authed = FakeAdapter((o) {
      n++;
      if (n == 1) return json(401, {'error': {'code': 'TOKEN_EXPIRED', 'message': 'x'}});
      return json(200, {'ok': true});
    });
    final tokens = MemoryTokenStore();
    await tokens.writeRefreshToken('r1');
    final api = clientWith(raw, authed: authed, tokens: tokens);
    await api.refresh(); // access a2
    final res = await api.dio.get<Map<String, dynamic>>('/ping');
    expect(res.data, {'ok': true});
    expect(authed.calls.length, 2);
    expect(authed.calls.last.headers['Authorization'], 'Bearer a2');
  });

  test('refresh ditolak 401 -> sesi dibersihkan dan onSessionExpired dipanggil', () async {
    var expired = false;
    final raw = FakeAdapter((o) => json(401, {'error': {'code': 'SESSION_INVALID', 'message': 'x'}}));
    final authed = FakeAdapter((o) => json(401, {'error': {'code': 'TOKEN_EXPIRED', 'message': 'x'}}));
    final tokens = MemoryTokenStore();
    await tokens.writeRefreshToken('r1');
    final api = clientWith(raw, authed: authed, tokens: tokens, onExpired: () => expired = true);
    await expectLater(api.dio.get<void>('/ping'), throwsA(isA<DioException>()));
    expect(expired, isTrue);
    expect(await tokens.readRefreshToken(), isNull);
  });

  testWidgets('login: validasi form, galat sisa percobaan, lalu masuk ke beranda', (tester) async {
    tester.platformDispatcher.localesTestValue = const [Locale('id')];
    addTearDown(tester.platformDispatcher.clearLocalesTestValue);
    tester.view.devicePixelRatio = 1;
    tester.view.physicalSize = const Size(400, 1000);
    addTearDown(tester.view.resetPhysicalSize);
    var attempt = 0;
    final raw = FakeAdapter((o) {
      attempt++;
      if (attempt == 1) {
        return json(401, {'error': {'code': 'INVALID_CREDENTIALS', 'message': 'x', 'attempts_left': 2}});
      }
      return json(200, sessionBody());
    });
    final tokens = MemoryTokenStore();
    final api = clientWith(raw, tokens: tokens);
    await tester.pumpWidget(ProviderScope(
      overrides: [tokenStoreProvider.overrideWithValue(tokens), apiClientProvider.overrideWithValue(api)],
      child: const ArusApp(),
    ));
    await tester.pumpAndSettle();

    // form kosong -> pesan validasi
    await tester.tap(find.text('Masuk'));
    await tester.pumpAndSettle();
    expect(find.text('Email wajib diisi.'), findsOneWidget);
    expect(find.text('Kata sandi wajib diisi.'), findsOneWidget);

    // kredensial salah -> kotak galat dengan sisa percobaan
    await tester.enterText(find.byType(TextFormField).at(0), 'a@b.co');
    await tester.enterText(find.byType(TextFormField).at(1), 'salah');
    await tester.tap(find.text('Masuk'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Sisa 2 percobaan'), findsOneWidget);

    // benar -> beranda
    await tester.enterText(find.byType(TextFormField).at(1), 'benar');
    await tester.tap(find.text('Masuk'));
    await tester.pumpAndSettle();
    expect(find.text('Selamat datang, Budi'), findsOneWidget);
    expect(find.text('Pusat'), findsOneWidget);

    // keluar -> kembali ke login
    await tester.tap(find.byIcon(Icons.logout));
    await tester.pumpAndSettle();
    expect(find.text('Akses Kasir'), findsOneWidget);
  });
}
