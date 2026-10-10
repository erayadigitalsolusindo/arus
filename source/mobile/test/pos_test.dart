import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:arus_mobile/app.dart';
import 'package:arus_mobile/core/session/session_controller.dart';
import 'package:arus_mobile/core/session/token_store.dart';
import 'package:arus_mobile/core/widgets/ocean_background.dart';
import 'test_helpers.dart';

Map<String, dynamic> quoteBody(String total) => {
      'lines': [
        {'unit_price': '12000.00', 'line_total': total}
      ],
      'subtotal': total,
      'discount': '0.00',
      'tax_store': '0.00',
      'tax_gov': '0.00',
      'other_cost': '0.00',
      'total': total,
    };

/// Server palsu untuk alur kasir.
FakeAdapter posServer({bool shiftOpen = true, List<RequestOptions>? sales}) {
  return FakeAdapter((o) {
    final key = '${o.method} ${o.path}';
    switch (key) {
      case 'POST /auth/refresh':
        return json(200, sessionBody(refresh: 'r2', access: 'a2'));
      case 'GET /shifts/current':
        return json(200, {
          'shift': shiftOpen ? {'id': 's1', 'doc_no': 'SH-MAIN-1', 'opened_at': '2026-10-10T01:00:00Z', 'opening_cash': '0.00'} : null
        });
      case 'GET /items/search':
        return json(200, {
          'data': [
            {'id': 'i1', 'sku': 'A1', 'barcode': '', 'name': 'Kopi Bubuk', 'unit': 'PCS', 'price': '12000.00', 'stock': {'display': '10.000'}, 'kind': 'goods'}
          ],
          'exact': [],
          'next_cursor': ''
        });
      case 'POST /sales/quote':
        final qty = ((o.data as Map)['lines'] as List).first['qty'];
        return json(200, quoteBody(qty == '2' ? '24000.00' : '12000.00'));
      case 'GET /payment-methods/lookup':
        return json(200, {
          'data': [
            {'id': 'm1', 'name': 'Tunai', 'kind': 'cash'},
            {'id': 'm2', 'name': 'QRIS', 'kind': 'ewallet'}
          ]
        });
      case 'POST /sales/':
        sales?.add(o);
        return json(201, {'id': 'sale1', 'doc_no': 'MAIN-261010-0001', 'total': '12000.00', 'change': '0.00'});
    }
    return json(404, {
      'error': {'code': 'NOT_FOUND', 'message': key}
    });
  });
}

Future<void> boot(WidgetTester tester, FakeAdapter server, {Size size = const Size(400, 900)}) async {
  tester.platformDispatcher.localesTestValue = const [Locale('id')];
  addTearDown(tester.platformDispatcher.clearLocalesTestValue);
  tester.view.devicePixelRatio = 1;
  tester.view.physicalSize = size;
  addTearDown(tester.view.resetPhysicalSize);
  final tokens = MemoryTokenStore();
  await tokens.writeRefreshToken('r1'); // sesi tersimpan → langsung masuk
  final api = clientWith(server, tokens: tokens);
  await tester.pumpWidget(ProviderScope(
    overrides: [tokenStoreProvider.overrideWithValue(tokens), apiClientProvider.overrideWithValue(api), oceanAnimateProvider.overrideWithValue(false)],
    child: const ArusApp(),
  ));
  await tester.pump(const Duration(milliseconds: 100));
  await tester.pump(const Duration(milliseconds: 100));
}

Future<void> settle(WidgetTester tester, [int ms = 600]) async {
  await tester.pump(Duration(milliseconds: ms));
  await tester.pump(const Duration(milliseconds: 50));
}

void main() {
  testWidgets('kasir: panel outlet tertutup, tambah barang, total dari server, bayar tunai', (tester) async {
    final sales = <RequestOptions>[];
    await boot(tester, posServer(sales: sales));

    // Beranda → Kasir
    expect(find.text('Kasir'), findsOneWidget);
    await tester.tap(find.text('Kasir'));
    await settle(tester);

    // Panel info outlet ("MAIN") tertutup sendiri: isinya belum ada di layar.
    expect(find.text('SHIFT'), findsNothing);
    expect(find.text('Kopi Bubuk'), findsOneWidget);

    // Buka panel lewat tombol menu.
    await tester.tap(find.byIcon(Icons.menu));
    await settle(tester, 400);
    expect(find.text('SHIFT'), findsOneWidget);
    expect(find.textContaining('SH-MAIN-1'), findsOneWidget);
    tester.state<ScaffoldState>(find.byType(Scaffold).first).closeDrawer(); // tutup laci
    await tester.pumpAndSettle();
    expect(find.text('SHIFT'), findsNothing);

    // Tambah barang → total dihitung server.
    await tester.tap(find.text('Kopi Bubuk'));
    await settle(tester);
    expect(find.text('Rp 12.000'), findsWidgets);

    // Tambah qty → quote ulang, total 24.000.
    await tester.tap(find.byIcon(Icons.shopping_cart_outlined));
    await settle(tester, 400);
    await tester.tap(find.byIcon(Icons.add).last);
    await settle(tester);
    expect(find.text('Rp 24.000'), findsWidgets);

    // Kembali ke 1 lalu bayar.
    await tester.tap(find.byIcon(Icons.remove).last);
    await settle(tester);
    await tester.tap(find.widgetWithText(FilledButton, 'Bayar').last);
    await settle(tester, 400);
    await tester.pumpAndSettle();
    expect(find.text('Pembayaran'), findsOneWidget);
    await tester.tap(find.text('Selesaikan pembayaran'));
    await tester.pumpAndSettle();

    expect(find.text('Transaksi berhasil'), findsOneWidget);
    expect(find.text('Nomor nota MAIN-261010-0001'), findsOneWidget);
    expect(sales, hasLength(1));
    expect(sales.single.headers['Idempotency-Key'], isNotEmpty);
    final body = sales.single.data as Map;
    expect((body['lines'] as List).single, {'item_id': 'i1', 'qty': '1'});
    expect((body['payments'] as List).single['method_id'], 'm1');
    expect((body['payments'] as List).single['amount'], '12000');
    expect(jsonEncode(body).contains('tenant'), isFalse); // tenant/outlet tidak pernah dikirim klien

    // Transaksi baru mengosongkan keranjang.
    await tester.tap(find.text('Transaksi baru'));
    await settle(tester, 400);
    expect(find.text('—'), findsOneWidget); // total kosong = keranjang kosong
    expect(tester.widget<Badge>(find.byType(Badge)).isLabelVisible, isFalse);
  });

  testWidgets('kasir: tanpa shift terbuka → dialog buka shift', (tester) async {
    await boot(tester, posServer(shiftOpen: false));
    await tester.tap(find.text('Kasir'));
    await settle(tester);
    expect(find.text('Buka shift'), findsWidgets);
    expect(find.text('Modal awal (Rp)'), findsOneWidget);
  });

  testWidgets('tema gelap dan terang tersedia lewat tombol tema', (tester) async {
    await boot(tester, posServer());
    final app = tester.widget<MaterialApp>(find.byType(MaterialApp));
    expect(app.theme!.brightness, Brightness.light);
    expect(app.darkTheme!.brightness, Brightness.dark);
    await tester.tap(find.byIcon(Icons.brightness_auto)); // sistem → terang
    await tester.pump();
    await tester.tap(find.byIcon(Icons.light_mode_outlined)); // terang → gelap
    await tester.pump();
    expect(tester.widget<MaterialApp>(find.byType(MaterialApp)).themeMode, ThemeMode.dark);
  });
}
