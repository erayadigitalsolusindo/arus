import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:decimal/decimal.dart';

import 'package:arus_mobile/app.dart';
import 'package:arus_mobile/features/pos/pay_sheet.dart';
import 'package:arus_mobile/core/session/session_controller.dart';
import 'package:arus_mobile/core/session/token_store.dart';
import 'package:arus_mobile/core/widgets/ocean_background.dart';

import 'test_helpers.dart';

// PNG 1x1 transparan.
final _png = base64Decode(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==',
);

Map<String, dynamic> quoteBody(String total) => {
  'lines': [
    {'unit_price': '12000.00', 'line_total': total},
  ],
  'subtotal': total,
  'discount': '0.00',
  'tax_store': '0.00',
  'tax_gov': '0.00',
  'other_cost': '0.00',
  'total': total,
};

/// Server palsu untuk alur kasir.
FakeAdapter posServer({
  bool shiftOpen = true,
  List<RequestOptions>? sales,
  List<RequestOptions>? switches,
  List<RequestOptions>? images,
  List<RequestOptions>? closes,
  List<RequestOptions>? quotes,
  List<RequestOptions>? shortcuts,
  List<RequestOptions>? searches,
}) {
  return FakeAdapter((o) {
    final key = '${o.method} ${o.path}';
    switch (key) {
      case 'POST /auth/refresh':
        return json(200, sessionBody(refresh: 'r2', access: 'a2'));
      case 'GET /shifts/current':
        return json(200, {
          'shift': shiftOpen
              ? {
                  'id': 's1',
                  'doc_no': 'SH-MAIN-1',
                  'opened_at': '2026-10-10T01:00:00Z',
                  'opening_cash': '0.00',
                }
              : null,
        });
      case 'GET /catalog/lookup/categories/':
        return json(200, {
          'data': searches == null
              ? []
              : [
                  {'id': 'cat1', 'name': 'Minuman'},
                ],
          'total': 1,
        });
      case 'GET /items/search':
        searches?.add(o);
        return json(200, {
          'data': [
            {
              'id': 'i1',
              'sku': 'A1',
              'barcode': '',
              'name': 'Kopi Bubuk',
              'unit': 'PCS',
              'price': '12000.00',
              'stock': {'display': '10.000'},
              'kind': 'goods',
              'main_image_id': 'img1',
            },
          ],
          'exact': [],
          'next_cursor': '',
        });
      case 'GET /items/i1/images/img1/file':
        images?.add(o);
        return ResponseBody.fromBytes(
          _png,
          200,
          headers: {
            Headers.contentTypeHeader: ['image/jpeg'],
          },
        );
      case 'GET /outlets/accessible':
        return json(200, {
          'outlets': [
            {'id': 'o1', 'code': 'MAIN', 'name': 'Pusat'},
            {'id': 'o2', 'code': 'BR2', 'name': 'Cabang Dua'},
          ],
          'current_id': 'o1',
        });
      case 'POST /approvals/check':
        if ((o.data as Map)['pin'] != '123456')
          return json(403, {
            'error': {'code': 'INVALID_PIN', 'message': 'x'},
          });
        return json(200, {'id': 'u9', 'name': 'Sari Owner'});
      case 'GET /approvals/approvers':
        return json(200, {
          'approvers': [
            {'id': 'u9', 'name': 'Sari Owner'},
          ],
        });
      case 'POST /auth/switch-outlet':
        switches?.add(o);
        final b = o.data as Map;
        if (b['pos'] == true && b['approval'] == null) {
          return json(403, {
            'error': {'code': 'PIN_REQUIRED', 'message': 'x'},
          });
        }
        if (b['pos'] == true && (b['approval'] as Map)['pin'] != '123456') {
          return json(403, {
            'error': {'code': 'INVALID_PIN', 'message': 'x'},
          });
        }
        return json(200, {
          ...sessionBody(refresh: '', access: 'a3'),
          'outlet': {'id': 'o2', 'name': 'Cabang Dua', 'code': 'BR2'},
        });
      case 'GET /shifts/s1':
        return json(200, {
          'id': 's1',
          'doc_no': 'SH-MAIN-1',
          'status': 'open',
          'opened_at': '2026-10-10T01:00:00Z',
          'sale_count': 2,
          'void_count': 0,
          'sales_total': '36000.00',
          'receivable_total': '0.00',
          'expected_total': '36000.00',
          'counts': [
            {
              'method_id': 'm1',
              'name': 'Tunai',
              'kind': 'cash',
              'opening': '0.00',
              'sales': '24000.00',
              'flows': '0.00',
              'expected': '24000.00',
            },
            {
              'method_id': 'm2',
              'name': 'QRIS',
              'kind': 'ewallet',
              'opening': '0.00',
              'sales': '12000.00',
              'flows': '0.00',
              'expected': '12000.00',
            },
          ],
        });
      case 'POST /shifts/s1/close':
        closes?.add(o);
        final cb = o.data as Map;
        if (cb['approval'] != null &&
            (cb['approval'] as Map)['pin'] != '123456') {
          return json(403, {
            'error': {'code': 'INVALID_PIN', 'message': 'x'},
          });
        }
        return json(200, {
          'id': 's1',
          'doc_no': 'SH-MAIN-1',
          'status': 'closed',
          'opened_at': '2026-10-10T01:00:00Z',
          'sale_count': 2,
          'void_count': 0,
          'sales_total': '36000.00',
          'receivable_total': '0.00',
          'expected_total': '36000.00',
          'counted_total': '35000.00',
          'diff_total': '-1000.00',
          'counts': [],
        });
      case 'GET /sales/':
        return json(200, {
          'data': [
            {
              'id': 'sale1',
              'doc_no': 'MAIN-261010-0001',
              'status': 'completed',
              'created_at': '2026-10-10T03:00:00Z',
              'total': '12000.00',
              'surcharge': '0',
              'receivable': '0',
              'returned': '0',
              'pays': [
                {'name': 'Tunai'},
              ],
            },
          ],
          'total': '12000.00',
          'by_method': [
            {'name': 'Tunai', 'amount': '12000.00'},
          ],
          'truncated': false,
        });
      case 'GET /sales/sale1':
        return json(200, {
          'doc_no': 'MAIN-261010-0001',
          'status': 'completed',
          'cashier': 'Budi',
          'lines': [
            {
              'name': 'Kopi Bubuk',
              'unit': 'PCS',
              'qty': '1.000',
              'unit_price': '12000.00',
              'discount': '0.00',
              'line_total': '12000.00',
            },
          ],
          'payments': [
            {'method_name': 'Tunai', 'amount': '12000.00'},
          ],
          'subtotal': '12000.00',
          'discount': '0.00',
          'tax_store': '0.00',
          'tax_gov': '0.00',
          'other_cost': '0.00',
          'total': '12000.00',
          'paid': '12000.00',
          'change': '0.00',
          'receivable': '0.00',
        });
      case 'GET /pos/shortcuts/':
        shortcuts?.add(o);
        return json(200, {
          'data': shortcuts == null
              ? []
              : [
                  {
                    'slot': 1,
                    'item_id': 'i1',
                    'sku': 'A1',
                    'name': 'Kopi Bubuk',
                    'unit': 'PCS',
                    'kind': 'goods',
                    'active': true,
                    'price': '12000.00',
                    'main_image_id': null,
                  },
                ],
          'slots': 16,
        });
      case 'PUT /pos/shortcuts/2':
        shortcuts?.add(o);
        return json(200, {'data': [], 'slots': 16});
      case 'GET /catalog/lookup/salespeople/':
        return json(200, {
          'data': [
            {'id': 'sp1', 'name': 'Dewi'},
          ],
          'total': 1,
        });
      case 'GET /members/lookup':
        return json(200, {
          'data': [
            {
              'id': 'mem1',
              'code': 'M001',
              'name': 'Ani Member',
              'phone': '0812',
              'points': 500,
              'level': 'Gold',
              'point_value': '100.00',
            },
          ],
        });
      case 'POST /sales/quote':
        quotes?.add(o);
        final qb = o.data as Map;
        final ln = (qb['lines'] as List).first as Map;
        final qty = int.parse('${ln['qty']}');
        final price = int.parse('${ln['unit_price'] ?? '12000'}');
        final lineDisc = int.parse('${ln['discount'] ?? '0'}');
        final noteDisc = int.parse('${qb['discount'] ?? '0'}');
        final costs = ((qb['other_costs'] as List?) ?? const []).fold<int>(
          0,
          (a, c) => a + int.parse('${(c as Map)['amount']}'),
        );
        final redeem = ((qb['redeem_points'] as num?) ?? 0).toInt();
        final gross = price * qty;
        final disc = lineDisc + noteDisc + redeem * 100;
        final body = {
          'lines': [
            {
              'unit_price': '$price.00',
              'list_price': '12000.00',
              'line_total': '${gross - lineDisc}.00',
              'discount': '$lineDisc.00',
            },
          ],
          'subtotal': '$gross.00',
          'discount': '$disc.00',
          'tax_store': '0.00',
          'tax_gov': '0.00',
          'tax_store_pct': '0',
          'tax_gov_pct': '0',
          'other_cost': '$costs.00',
          'total': '${gross - disc + costs}.00',
          'redeem_amount': '${redeem * 100}.00',
        };
        if (qb['member_id'] == null) return json(200, body);
        // Member: saldo poin/deposit dari server; tukar poin = 100 per poin.
        return json(200, {
          ...body,
          'points_earn': 12,
          'credit': {
            'limit': '50000.00',
            'outstanding': '45000.00',
            'due_days': 14,
          },
          'member': {
            'id': 'mem1',
            'code': 'M001',
            'name': 'Ani Member',
            'points': 500,
            'deposit': '20000.00',
          },
        });
      case 'GET /payment-methods/lookup':
        return json(200, {
          'data': [
            {'id': 'm1', 'name': 'Tunai', 'kind': 'cash'},
            {
              'id': 'm2',
              'name': 'QRIS',
              'kind': 'ewallet',
              'fee_pct': '0.70',
              'fee_flat': '0.00',
              'fee_bearer': 'customer',
            },
            {'id': 'm3', 'name': 'Deposit', 'kind': 'deposit'},
          ],
        });
      case 'POST /sales/':
        sales?.add(o);
        final sb = o.data as Map;
        if (sb['credit'] == true) {
          // Piutang 45.000 + sisa nota melewati limit 50.000 → butuh penyetuju.
          if (sb['approval'] == null)
            return json(403, {
              'error': {'code': 'CREDIT_LIMIT_EXCEEDED', 'message': 'x'},
            });
          final dp = ((sb['payments'] as List?) ?? const []).fold<int>(
            0,
            (a, p) => a + int.parse('${(p as Map)['amount']}'),
          );
          return json(201, {
            'id': 'sale1',
            'doc_no': 'MAIN-261010-0002',
            'total': '12000.00',
            'change': '0.00',
            'receivable': '${12000 - dp}.00',
            'credit': {'due_date': '2026-10-24'},
          });
        }
        return json(201, {
          'id': 'sale1',
          'doc_no': 'MAIN-261010-0001',
          'total': '12000.00',
          'change': '0.00',
        });
    }
    return json(404, {
      'error': {'code': 'NOT_FOUND', 'message': key},
    });
  });
}

Future<void> boot(
  WidgetTester tester,
  FakeAdapter server, {
  Size size = const Size(400, 900),
  bool resetPrefs = true,
}) async {
  // Penyimpanan lokal (keranjang tersimpan) tidak boleh bocor antar test.
  if (resetPrefs) SharedPreferences.setMockInitialValues({});
  tester.platformDispatcher.localesTestValue = const [Locale('id')];
  addTearDown(tester.platformDispatcher.clearLocalesTestValue);
  tester.view.devicePixelRatio = 1;
  tester.view.physicalSize = size;
  addTearDown(tester.view.resetPhysicalSize);
  final tokens = MemoryTokenStore();
  await tokens.writeRefreshToken('r1'); // sesi tersimpan → langsung masuk
  final api = clientWith(server, tokens: tokens);
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        tokenStoreProvider.overrideWithValue(tokens),
        apiClientProvider.overrideWithValue(api),
        oceanAnimateProvider.overrideWithValue(false),
      ],
      child: const ArusApp(),
    ),
  );
  await tester.pump(const Duration(milliseconds: 100));
  await tester.pump(const Duration(milliseconds: 100));
}

Future<void> settle(WidgetTester tester, [int ms = 600]) async {
  await tester.pump(Duration(milliseconds: ms));
  await tester.pump(const Duration(milliseconds: 50));
}

void main() {
  testWidgets('keterangan nota ikut ke server saat bayar', (tester) async {
    final sales = <RequestOptions>[];
    await boot(tester, posServer(sales: sales));
    await tester.tap(find.text('Kasir'));
    await settle(tester);
    await tester.tap(find.text('Kopi Bubuk'));
    await settle(tester);
    await tester.tap(find.byIcon(Icons.shopping_cart_outlined));
    await settle(tester, 400);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Keterangan & biaya lain'));
    await tester.pumpAndSettle();
    await tester.enterText(
      find.widgetWithText(TextField, 'Keterangan nota'),
      'Titip Bu Ani',
    );
    await tester.tap(find.widgetWithText(FilledButton, 'Terapkan'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Titip Bu Ani'), findsOneWidget);
    await tester.tap(find.widgetWithText(FilledButton, 'Bayar').last);
    await settle(tester, 400);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Selesaikan pembayaran'));
    await tester.pumpAndSettle();
    expect((sales.single.data as Map)['note'], 'Titip Bu Ani');
  });

  testWidgets(
    'filter kategori: chip kategori mengirim category_id ke pencarian server',
    (tester) async {
      final searches = <RequestOptions>[];
      await boot(tester, posServer(searches: searches));
      await tester.tap(find.text('Kasir'));
      await settle(tester);
      await settle(tester, 600);
      expect(find.text('Semua'), findsOneWidget);
      expect(searches.last.queryParameters.containsKey('category_id'), isFalse);
      await tester.tap(find.text('Minuman'));
      await settle(tester);
      await tester.pumpAndSettle();
      expect(searches.last.queryParameters['category_id'], 'cat1');
      await tester.tap(find.text('Semua'));
      await settle(tester);
      expect(searches.last.queryParameters.containsKey('category_id'), isFalse);
    },
  );

  test('nominal tunai cepat: pecahan pembulatan di atas total, maks 4', () {
    String f(int need) =>
        quickCashAmounts(Decimal.fromInt(need))
            .map((d) => d.toString())
            .join(',');
    expect(f(12000), '15000,20000,50000,100000');
    expect(f(50000), '60000,100000');
    expect(f(99500), '100000');
    expect(f(0), '');
  });

  testWidgets(
    'pintasan barang: ketuk = masuk keranjang; atur slot dari server',
    (tester) async {
      final calls = <RequestOptions>[];
      await boot(tester, posServer(shortcuts: calls));
      await tester.tap(find.text('Kasir'));
      await settle(tester);
      await settle(tester, 600);

      // Slot 1 tampil di atas kisi; ketuk memasukkan barang ke keranjang.
      expect(
        find.text('Kopi Bubuk'),
        findsNWidgets(2),
      ); // pintasan + kisi katalog
      await tester.tap(find.text('Kopi Bubuk').first);
      await settle(tester);
      expect(tester.widget<Badge>(find.byType(Badge)).isLabelVisible, isTrue);

      // Atur: slot 2 kosong → pilih barang → PUT /pos/shortcuts/2.
      await tester.tap(find.byIcon(Icons.dashboard_customize_outlined));
      await tester.pumpAndSettle();
      expect(find.text('Pintasan barang'), findsOneWidget);
      await tester.tap(find.text('Kosong — ketuk untuk memasang barang').first);
      await tester.pumpAndSettle();
      await tester.tap(find.text('Kopi Bubuk').last);
      await tester.pumpAndSettle();
      final put = calls.firstWhere((c) => c.method == 'PUT');
      expect(put.path, '/pos/shortcuts/2');
      expect(put.data, {'item_id': 'i1'});
    },
  );

  testWidgets('salesman nota: dipilih lalu ikut quote dan bayar', (
    tester,
  ) async {
    final sales = <RequestOptions>[];
    final quotes = <RequestOptions>[];
    await boot(tester, posServer(sales: sales, quotes: quotes));
    await tester.tap(find.text('Kasir'));
    await settle(tester);
    await tester.tap(find.text('Kopi Bubuk'));
    await settle(tester);
    await tester.tap(find.byIcon(Icons.shopping_cart_outlined));
    await settle(tester, 400);
    await tester.pumpAndSettle();

    expect(find.text('Umum (tanpa salesman)'), findsOneWidget);
    await tester.tap(find.text('Umum (tanpa salesman)'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Dewi'));
    await settle(tester);
    await tester.pumpAndSettle();
    expect(quotes.last.data['salesperson_id'], 'sp1');
    expect(find.text('Dewi'), findsOneWidget);

    await tester.tap(find.widgetWithText(FilledButton, 'Bayar').last);
    await settle(tester, 400);
    await tester.pumpAndSettle();
    // Tombol nominal cepat tunai: pecahan di atas total.
    expect(find.text('Uang pas'), findsOneWidget);
    expect(find.text('15.000'), findsOneWidget);
    await tester.tap(find.text('20.000'));
    await tester.pump();
    expect(find.text('Rp 8.000'), findsWidgets); // kembalian dari 20.000
    await tester.tap(find.text('Selesaikan pembayaran'));
    await tester.pumpAndSettle();
    expect((sales.single.data as Map)['salesperson_id'], 'sp1');
  });

  testWidgets(
    'nota pending: tunda dengan keterangan, tersimpan di perangkat, dibuka lagi (dihitung ulang)',
    (tester) async {
      final quotes = <RequestOptions>[];
      await boot(tester, posServer(quotes: quotes));
      await tester.tap(find.text('Kasir'));
      await settle(tester);
      await tester.tap(find.text('Kopi Bubuk'));
      await settle(tester);
      await tester.tap(find.byIcon(Icons.shopping_cart_outlined));
      await settle(tester, 400);
      await tester.pumpAndSettle();

      await tester.tap(find.byIcon(Icons.pause_circle_outline));
      await tester.pumpAndSettle();
      await tester.enterText(
        find.widgetWithText(TextField, 'Keterangan (opsional)'),
        'Bu Ani',
      );
      await tester.tap(find.widgetWithText(FilledButton, 'Tunda'));
      await tester.pumpAndSettle();

      // Tersimpan di perangkat per kasir + cabang; keranjang kosong lagi.
      final prefs = await SharedPreferences.getInstance();
      final stored =
          jsonDecode(prefs.getString('pos.pending.v1.u1.o1')!) as List;
      expect(stored, hasLength(1));
      expect(stored.single['label'], 'Bu Ani');
      expect(
        find.text('Nota ditunda sebagai Pending 1'),
        findsOneWidget,
      ); // lembar keranjang menutup di HP
      expect(
        tester.widget<Badge>(find.byType(Badge)).isLabelVisible,
        isFalse,
      ); // keranjang kosong

      await tester.pump(const Duration(seconds: 6)); // snackbar hilang
      await tester.pumpAndSettle();
      await tester.tap(find.byIcon(Icons.shopping_cart_outlined));
      await settle(tester, 400);
      await tester.pumpAndSettle();
      await tester.tap(find.byIcon(Icons.pending_actions_outlined));
      await tester.pumpAndSettle();
      expect(find.text('Nota pending (1/20)'), findsOneWidget);
      expect(find.text('Pending 1 · Bu Ani'), findsOneWidget);
      expect(find.textContaining('1 baris'), findsOneWidget);
      final before = quotes.length;
      await tester.tap(find.text('Pending 1 · Bu Ani'));
      await settle(tester);
      await tester.pumpAndSettle();

      // Dibuka: isi pulih, harga dihitung ulang server, daftar kosong lagi.
      expect(quotes.length, greaterThan(before));
      expect(prefs.getString('pos.pending.v1.u1.o1'), isNull);
      expect(find.text('Pending 1 dibuka'), findsOneWidget);
      expect(tester.widget<Badge>(find.byType(Badge)).isLabelVisible, isTrue);
      expect(find.text('Rp 12.000'), findsWidgets);
    },
  );

  testWidgets('nota pending kedaluwarsa 24 jam dibuang saat dimuat', (
    tester,
  ) async {
    final item = {
      'id': 'i1',
      'sku': 'A1',
      'barcode': '',
      'name': 'Kopi Bubuk',
      'unit': 'PCS',
      'price': '12000.00',
      'stock': {'display': '10.000'},
      'kind': 'goods',
    };
    Map<String, dynamic> note(int no, Duration age) => {
      'id': 'n$no',
      'no': no,
      'at': DateTime.now().subtract(age).millisecondsSinceEpoch,
      'label': 'Lama $no',
      'total': null,
      'cart': {
        'apply_tax': false,
        'lines': [
          {'item': item, 'qty': '1'},
        ],
      },
    };
    SharedPreferences.setMockInitialValues({
      'pos.pending.v1.u1.o1': jsonEncode([
        note(1, const Duration(hours: 25)),
        note(2, const Duration(hours: 2)),
      ]),
    });
    await boot(tester, posServer(), resetPrefs: false);
    await tester.tap(find.text('Kasir'));
    await settle(tester);
    await tester.tap(find.byIcon(Icons.shopping_cart_outlined));
    await settle(tester, 400);
    await tester.pumpAndSettle();
    await tester.tap(find.byIcon(Icons.pending_actions_outlined));
    await tester.pumpAndSettle();
    expect(find.text('Nota pending (1/20)'), findsOneWidget);
    expect(find.text('Pending 2 · Lama 2'), findsOneWidget);
    expect(find.textContaining('Lama 1'), findsNothing);
  });

  testWidgets(
    'kredit member + DP: melewati limit butuh penyetuju, nota membawa credit & approval',
    (tester) async {
      final sales = <RequestOptions>[];
      await boot(tester, posServer(sales: sales));
      await tester.tap(find.text('Kasir'));
      await settle(tester);
      await tester.tap(find.text('Kopi Bubuk'));
      await settle(tester);
      await tester.tap(find.byIcon(Icons.shopping_cart_outlined));
      await settle(tester, 400);
      await tester.pumpAndSettle();
      await tester.tap(find.text('Pelanggan umum'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Ani Member'));
      await settle(tester);
      await tester.pumpAndSettle();

      await tester.tap(find.widgetWithText(FilledButton, 'Bayar').last);
      await settle(tester, 400);
      await tester.pumpAndSettle();
      await tester.tap(find.text('Kredit'));
      await tester.pumpAndSettle();
      expect(find.text('Jatuh tempo'), findsOneWidget);
      expect(find.text('14 hari setelah nota'), findsOneWidget);
      expect(
        find.text('Rp 57.000'),
        findsOneWidget,
      ); // 45.000 + 12.000 (tanpa DP)
      expect(find.textContaining('Melewati limit kredit'), findsOneWidget);

      // Tanpa PIN penyetuju belum bisa disimpan.
      FilledButton submit() => tester.widget<FilledButton>(
        find.widgetWithText(FilledButton, 'Selesaikan pembayaran'),
      );
      expect(submit().onPressed, isNull);

      // DP 5.000 tunai → sisa piutang 7.000.
      await tester.tap(find.text('Tambah uang muka (DP)'));
      await tester.pumpAndSettle();
      await tester.enterText(
        find.widgetWithText(TextField, 'Jumlah (Rp)'),
        '5000',
      );
      await tester.pump();
      expect(find.text('Rp 7.000'), findsWidgets);

      await tester.enterText(
        find.widgetWithText(TextField, 'PIN penyetuju (6 digit)'),
        '123456',
      );
      await tester.pump();
      await tester.ensureVisible(
        find.widgetWithText(FilledButton, 'Selesaikan pembayaran'),
      );
      expect(submit().onPressed, isNotNull);
      await tester.tap(
        find.widgetWithText(FilledButton, 'Selesaikan pembayaran'),
      );
      await tester.pumpAndSettle();

      final body = sales.single.data as Map;
      expect(body['credit'], true);
      expect(body['member_id'], 'mem1');
      expect(body['approval'], {'user_id': 'u9', 'pin': '123456'});
      expect((body['payments'] as List).single['amount'], '5000');
      expect(find.text('Transaksi berhasil'), findsOneWidget);
      expect(find.text('Piutang Rp 7.000'), findsOneWidget);
      expect(find.text('Jatuh tempo 24 Oct 2026'), findsOneWidget);
    },
  );

  testWidgets('kredit tidak tersedia untuk pelanggan umum', (tester) async {
    await boot(tester, posServer());
    await tester.tap(find.text('Kasir'));
    await settle(tester);
    await tester.tap(find.text('Kopi Bubuk'));
    await settle(tester);
    await tester.tap(find.widgetWithText(FilledButton, 'Bayar').last);
    await settle(tester, 400);
    await tester.pumpAndSettle();
    expect(find.textContaining('Kredit hanya untuk member'), findsOneWidget);
    final seg = tester.widget<SegmentedButton<bool>>(
      find.byType(SegmentedButton<bool>),
    );
    expect(seg.segments.last.enabled, isFalse);
  });

  testWidgets(
    'ubah harga: penyetuju + PIN, quote server, approval ikut saat bayar',
    (tester) async {
      final sales = <RequestOptions>[];
      final quotes = <RequestOptions>[];
      await boot(tester, posServer(sales: sales, quotes: quotes));
      await tester.tap(find.text('Kasir'));
      await settle(tester);
      await tester.tap(find.text('Kopi Bubuk'));
      await settle(tester);
      await tester.tap(find.byIcon(Icons.shopping_cart_outlined));
      await settle(tester, 400);
      await tester.pumpAndSettle();

      await tester.tap(find.byIcon(Icons.edit_outlined));
      await tester.pumpAndSettle();
      expect(find.text('Harga normal'), findsOneWidget);
      // PIN salah ditolak server; sheet tetap terbuka.
      await tester.enterText(
        find.widgetWithText(TextField, 'Harga baru per satuan (Rp)'),
        '10000',
      );
      await tester.enterText(
        find.widgetWithText(TextField, 'PIN penyetuju (6 digit)'),
        '000000',
      );
      await tester.pump();
      await tester.tap(find.widgetWithText(FilledButton, 'Terapkan'));
      await tester.pumpAndSettle();
      expect(find.text('Penyetuju atau PIN salah.'), findsOneWidget);
      expect(
        (quotes.last.data['lines'].first as Map).containsKey('unit_price'),
        isFalse,
      );

      await tester.enterText(
        find.widgetWithText(TextField, 'PIN penyetuju (6 digit)'),
        '123456',
      );
      await tester.pump();
      await tester.tap(find.widgetWithText(FilledButton, 'Terapkan'));
      await settle(tester);
      await tester.pumpAndSettle();
      final line = quotes.last.data['lines'].first as Map;
      expect(line['unit_price'], '10000');
      expect(
        quotes.last.data.containsKey('approval'),
        isFalse,
      ); // quote tidak pernah membawa PIN
      expect(find.textContaining('Harga diubah'), findsOneWidget);
      expect(find.textContaining('disetujui Sari Owner'), findsOneWidget);
      expect(find.text('Rp 10.000'), findsWidgets);

      await tester.tap(find.widgetWithText(FilledButton, 'Bayar').last);
      await settle(tester, 400);
      await tester.pumpAndSettle();
      await tester.tap(find.text('Selesaikan pembayaran'));
      await tester.pumpAndSettle();
      final body = sales.single.data as Map;
      expect((body['lines'] as List).single['unit_price'], '10000');
      expect(body['approval'], {'user_id': 'u9', 'pin': '123456'});
    },
  );

  testWidgets('biaya lain-lain dikirim ke server', (tester) async {
    final quotes = <RequestOptions>[];
    await boot(tester, posServer(quotes: quotes));
    await tester.tap(find.text('Kasir'));
    await settle(tester);
    await tester.tap(find.text('Kopi Bubuk'));
    await settle(tester);
    await tester.tap(find.byIcon(Icons.shopping_cart_outlined));
    await settle(tester, 400);
    await tester.pumpAndSettle();

    await tester.tap(find.text('Keterangan & biaya lain'));
    await tester.pumpAndSettle();
    await tester.enterText(
      find.widgetWithText(TextField, 'Nama biaya'),
      'Ongkir',
    );
    await tester.enterText(
      find.widgetWithText(TextField, 'Jumlah (Rp)'),
      '5000',
    );
    await tester.pump();
    await tester.tap(find.widgetWithText(FilledButton, 'Terapkan'));
    await settle(tester);
    await tester.pumpAndSettle();

    final qb = quotes.last.data as Map;
    expect(qb.containsKey('discount'), isFalse);
    expect(qb.containsKey('note'), isFalse); // keterangan hanya ikut saat bayar
    expect(qb['other_costs'], [
      {'name': 'Ongkir', 'amount': '5000'},
    ]);
    expect(find.text('Rp 17.000'), findsWidgets); // 12.000 + 5.000, dari server
  });

  testWidgets(
    'biaya metode bayar yang ditanggung pelanggan tampil sebelum konfirmasi',
    (tester) async {
      await boot(tester, posServer());
      await tester.tap(find.text('Kasir'));
      await settle(tester);
      await tester.tap(find.text('Kopi Bubuk'));
      await settle(tester);
      await tester.tap(find.widgetWithText(FilledButton, 'Bayar').last);
      await settle(tester, 400);
      await tester.pumpAndSettle();
      await tester.tap(find.text('Tunai').last);
      await tester.pumpAndSettle();
      await tester.tap(find.textContaining('QRIS').last);
      await tester.pumpAndSettle();
      expect(find.textContaining('ditagihkan ke pelanggan'), findsOneWidget);
      expect(find.text('Ditagih ke pelanggan'), findsOneWidget);
      expect(find.text('Rp 12.084'), findsWidgets); // 12.000 + 0,7% = +84
    },
  );

  testWidgets(
    'member: pilih, poin & deposit dari quote, tukar poin, dikirim saat bayar',
    (tester) async {
      final sales = <RequestOptions>[];
      final quotes = <RequestOptions>[];
      await boot(tester, posServer(sales: sales, quotes: quotes));
      await tester.tap(find.text('Kasir'));
      await settle(tester);
      await tester.tap(find.text('Kopi Bubuk'));
      await settle(tester);
      await tester.tap(find.byIcon(Icons.shopping_cart_outlined));
      await settle(tester, 400);
      await tester.pumpAndSettle();

      expect(find.text('Pelanggan umum'), findsOneWidget);
      expect(quotes.last.data.containsKey('member_id'), isFalse);
      await tester.tap(find.text('Pelanggan umum'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Ani Member'));
      await settle(tester);
      await tester.pumpAndSettle();

      expect(quotes.last.data['member_id'], 'mem1');
      expect(find.text('Ani Member [M001]'), findsOneWidget);
      expect(
        find.textContaining('500 poin · deposit Rp 20.000'),
        findsOneWidget,
      );
      expect(find.textContaining('+12 poin'), findsOneWidget);

      // Tukar 100 poin = potongan Rp 10.000 → total Rp 2.000, dihitung server.
      await tester.tap(find.byIcon(Icons.loyalty_outlined));
      await tester.pumpAndSettle();
      expect(
        find.textContaining('maksimal 120 poin'),
        findsOneWidget,
      ); // 12.000 / 100, di bawah saldo 500
      await tester.enterText(
        find.widgetWithText(TextField, 'Poin yang ditukar'),
        '100',
      );
      await tester.pump();
      await tester.tap(find.text('Terapkan'));
      await settle(tester);
      expect(quotes.last.data['redeem_points'], 100);
      expect(find.text('Rp 2.000'), findsWidgets);

      await tester.tap(find.widgetWithText(FilledButton, 'Bayar').last);
      await settle(tester, 400);
      await tester.pumpAndSettle();
      expect(find.textContaining('Ani Member · 500 poin'), findsOneWidget);
      await tester.tap(find.text('Selesaikan pembayaran'));
      await tester.pumpAndSettle();

      final body = sales.single.data as Map;
      expect(body['member_id'], 'mem1');
      expect(body['redeem_points'], 100);
      expect(find.text('Transaksi berhasil'), findsOneWidget);
    },
  );

  testWidgets('deposit tidak ditawarkan di bayar bila nota tanpa member', (
    tester,
  ) async {
    await boot(tester, posServer());
    await tester.tap(find.text('Kasir'));
    await settle(tester);
    await tester.tap(find.text('Kopi Bubuk'));
    await settle(tester);
    await tester.tap(find.widgetWithText(FilledButton, 'Bayar').last);
    await settle(tester, 400);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Tunai').last);
    await tester.pumpAndSettle();
    expect(find.textContaining('QRIS'), findsWidgets);
    expect(find.text('Deposit'), findsNothing);
  });

  testWidgets(
    'tutup shift: selisih butuh catatan + PIN penyetuju, rekap dari server',
    (tester) async {
      final closes = <RequestOptions>[];
      await boot(tester, posServer(closes: closes));
      await tester.tap(find.text('Kasir'));
      await settle(tester);

      await tester.tap(find.byIcon(Icons.menu));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Tutup shift'));
      await tester.pumpAndSettle();
      expect(find.text('Tutup shift SH-MAIN-1'), findsOneWidget);
      expect(find.text('Seharusnya Rp 24.000'), findsOneWidget);

      // Belum terisi → tombol nonaktif.
      FilledButton submit() => tester.widget<FilledButton>(
        find.widgetWithText(FilledButton, 'Tutup shift'),
      );
      expect(submit().onPressed, isNull);

      final fields = find.byType(TextField);
      await tester.enterText(fields.at(0), '24000');
      await tester.enterText(fields.at(1), '11000'); // kurang 1.000
      await tester.pump();
      expect(find.textContaining('butuh persetujuan'), findsOneWidget);
      expect(submit().onPressed, isNull); // catatan + PIN belum ada

      await tester.enterText(
        find.widgetWithText(TextField, 'Catatan selisih'),
        'Uang kembalian kurang',
      );
      await tester.pumpAndSettle();
      await tester.enterText(
        find.widgetWithText(TextField, 'PIN penyetuju (6 digit)'),
        '123456',
      );
      await tester.pump();
      expect(submit().onPressed, isNotNull);

      await tester.ensureVisible(
        find.widgetWithText(FilledButton, 'Tutup shift'),
      );
      await tester.pumpAndSettle();
      await tester.tap(find.widgetWithText(FilledButton, 'Tutup shift'));
      await tester.pumpAndSettle();

      expect(closes, hasLength(1));
      expect(closes.single.headers['Idempotency-Key'], startsWith('shift-'));
      final body = closes.single.data as Map;
      expect(body['counts'], [
        {'method_id': 'm1', 'counted': '24000'},
        {'method_id': 'm2', 'counted': '11000'},
      ]);
      expect(body['note'], 'Uang kembalian kurang');
      expect(body['approval'], {'user_id': 'u9', 'pin': '123456'});
      expect(find.text('Shift SH-MAIN-1 ditutup'), findsOneWidget);
      expect(find.text('Buka shift baru'), findsOneWidget);
    },
  );

  testWidgets('beranda pantai: papan hari ini memuat total, nota, dan shift', (
    tester,
  ) async {
    await boot(tester, posServer());
    await settle(tester);
    // Akordeon tertutup secara default.
    expect(find.text('Penjualan hari ini'), findsOneWidget);
    expect(find.text('Rp 12.000'), findsNothing);
    await tester.tap(find.text('Penjualan hari ini'));
    await tester.pumpAndSettle();
    expect(find.text('Rp 12.000'), findsWidgets);
    expect(find.text('Nota'), findsOneWidget);
    expect(find.text('Shift'), findsOneWidget);
    expect(find.text('Kasir'), findsOneWidget);
  });

  testWidgets('penjualan hari ini: daftar nota dan detail', (tester) async {
    await boot(tester, posServer());
    await tester.tap(find.text('Kasir'));
    await settle(tester);
    await tester.tap(find.byIcon(Icons.menu));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Penjualan hari ini'));
    await tester.pumpAndSettle();
    expect(find.text('MAIN-261010-0001'), findsOneWidget);
    expect(find.text('1 nota, Rp 12.000'), findsOneWidget);
    await tester.tap(find.text('MAIN-261010-0001'));
    await tester.pumpAndSettle();
    expect(find.text('Kopi Bubuk'), findsWidgets);
    expect(find.text('Kasir Budi'), findsOneWidget);
  });

  testWidgets(
    'kasir: panel outlet tertutup, tambah barang, total dari server, bayar tunai',
    (tester) async {
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
      tester
          .state<ScaffoldState>(find.byType(Scaffold).first)
          .closeDrawer(); // tutup laci
      await tester.pumpAndSettle();
      expect(find.text('SHIFT'), findsNothing);

      // Tambah barang → total dihitung server.
      await tester.tap(find.text('Kopi Bubuk'));
      await settle(tester);
      expect(find.text('Rp 12.000'), findsWidgets);

      // Tambah qty → quote ulang, total 24.000.
      await tester.tap(find.byIcon(Icons.shopping_cart_outlined));
      await settle(tester, 400);
      await tester.pumpAndSettle();
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
      expect(
        jsonEncode(body).contains('tenant'),
        isFalse,
      ); // tenant/outlet tidak pernah dikirim klien

      // Transaksi baru mengosongkan keranjang.
      await tester.tap(find.text('Transaksi baru'));
      await settle(tester, 400);
      expect(find.text('—'), findsOneWidget); // total kosong = keranjang kosong
      expect(tester.widget<Badge>(find.byType(Badge)).isLabelVisible, isFalse);
    },
  );

  testWidgets(
    'keranjang yang belum dibayar tersimpan dan pulih saat dibuka lagi',
    (tester) async {
      await boot(tester, posServer());
      await tester.tap(find.text('Kasir'));
      await settle(tester);
      expect(
        find.text('Toko Maju'),
        findsOneWidget,
      ); // judul = nama usaha, bukan "Kasir"/kode cabang
      expect(find.text('MAIN'), findsNothing);
      await tester.tap(find.text('Kopi Bubuk'));
      await settle(tester);
      expect(tester.widget<Badge>(find.byType(Badge)).isLabelVisible, isTrue);

      // Aplikasi dibuka lagi (pohon widget baru, penyimpanan sama): keranjang pulih dan total dihitung ulang server.
      await tester.pumpWidget(const SizedBox());
      await boot(tester, posServer(), resetPrefs: false);
      await tester.tap(find.text('Kasir'));
      await settle(tester, 800);
      expect(tester.widget<Badge>(find.byType(Badge)).isLabelVisible, isTrue);
      await tester.tap(find.byIcon(Icons.shopping_cart_outlined));
      await settle(tester, 400);
      await tester.pumpAndSettle();
      expect(find.text('Rp 12.000'), findsWidgets);
    },
  );

  testWidgets('kasir: tanpa shift terbuka → dialog buka shift', (tester) async {
    await boot(tester, posServer(shiftOpen: false));
    await tester.tap(find.text('Kasir'));
    await settle(tester);
    expect(find.text('Buka shift'), findsWidgets);
    expect(find.text('Modal awal (Rp)'), findsOneWidget);
    await settle(tester, 1000);
  });

  testWidgets('tema gelap dan terang tersedia lewat tombol tema', (
    tester,
  ) async {
    await boot(tester, posServer());
    final app = tester.widget<MaterialApp>(find.byType(MaterialApp));
    expect(app.theme!.brightness, Brightness.light);
    expect(app.darkTheme!.brightness, Brightness.dark);
    await tester.tap(find.byIcon(Icons.brightness_auto)); // sistem → terang
    await tester.pump();
    await tester.tap(find.byIcon(Icons.light_mode_outlined)); // terang → gelap
    await tester.pump();
    expect(
      tester.widget<MaterialApp>(find.byType(MaterialApp)).themeMode,
      ThemeMode.dark,
    );
  });

  testWidgets('katalog memuat gambar barang (thumb) dari API', (tester) async {
    final images = <RequestOptions>[];
    await boot(tester, posServer(images: images));
    await tester.tap(find.text('Kasir'));
    await settle(tester);
    await settle(tester, 1000);
    expect(images, isNotEmpty);
    expect(images.first.queryParameters['size'], 'thumb');
    expect(find.byType(Image), findsWidgets);
  });

  testWidgets('keranjang kosong menampilkan ajakan besar', (tester) async {
    await boot(tester, posServer());
    await tester.tap(find.text('Kasir'));
    await settle(tester);
    await tester.tap(find.byIcon(Icons.shopping_cart_outlined));
    await settle(tester, 400);
    await tester.pumpAndSettle();
    expect(find.text('Keranjang masih kosong'), findsOneWidget);
    expect(find.text('Pilih barang untuk mulai berjualan'), findsOneWidget);
  });

  testWidgets('bahasa bisa diganti ke English dan kembali', (tester) async {
    await boot(tester, posServer());
    expect(find.text('Kasir'), findsOneWidget);
    await tester.tap(find.byIcon(Icons.translate));
    await tester.pumpAndSettle();
    await tester.tap(find.text('English'));
    await tester.pumpAndSettle();
    expect(find.text('Cashier'), findsOneWidget);
    expect(find.text('Kasir'), findsNothing);
  });

  testWidgets('pindah cabang dari beranda tanpa PIN', (tester) async {
    final switches = <RequestOptions>[];
    await boot(tester, posServer(switches: switches));
    await tester.tap(find.text('Pindah cabang'));
    await settle(tester, 400);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Cabang Dua'));
    await settle(tester, 400);
    await tester.pumpAndSettle();
    expect(switches.single.data['pos'], isNull);
    expect(switches.single.data['outlet_id'], 'o2');
    expect(find.textContaining('Cabang Dua'), findsWidgets);
  });

  testWidgets('pindah cabang dari kasir butuh PIN penyetuju', (tester) async {
    final switches = <RequestOptions>[];
    await boot(tester, posServer(switches: switches));
    await tester.tap(find.text('Kasir'));
    await settle(tester);
    await tester.tap(find.text('Kopi Bubuk'));
    await settle(tester);
    await tester.tap(find.byIcon(Icons.menu));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Pindah cabang'));
    await settle(tester, 400);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Cabang Dua'));
    await settle(tester, 400);
    await tester.pumpAndSettle();
    expect(find.text('Persetujuan pindah cabang'), findsOneWidget);

    // PIN salah → pesan galat, dialog tetap terbuka.
    await tester.enterText(
      find.descendant(
        of: find.byType(AlertDialog),
        matching: find.byType(TextField),
      ),
      '000000',
    );
    await tester.pump();
    await tester.tap(find.text('Setujui & pindah'));
    await tester.pumpAndSettle();
    expect(find.text('Penyetuju atau PIN salah.'), findsOneWidget);

    await tester.enterText(
      find.descendant(
        of: find.byType(AlertDialog),
        matching: find.byType(TextField),
      ),
      '123456',
    );
    await tester.pump();
    await tester.tap(find.text('Setujui & pindah'));
    await tester.pumpAndSettle();
    expect(find.text('Persetujuan pindah cabang'), findsNothing);
    final last = switches.last.data as Map;
    expect(last['pos'], true);
    expect(last['approval'], {'user_id': 'u9', 'pin': '123456'});
    expect(find.text('Sekarang di cabang Cabang Dua'), findsOneWidget);
    expect(
      find.text('—'),
      findsOneWidget,
    ); // keranjang cabang tujuan kosong (milik cabang lama tersimpan sendiri)
  });
}
