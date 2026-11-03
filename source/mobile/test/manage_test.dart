import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'pos_test.dart' as pt;
import 'test_helpers.dart';

const _admin = {
  'sales_orders': ['view', 'create'],
  'items': ['view', 'create', 'update'],
  'purchase_invoices': ['view', 'create', 'update', 'delete'],
  'sales_reports': ['view'],
};

/// Server palsu: kasir biasa + endpoint kelola barang dan pembelian.
FakeAdapter manageServer({
  Map<String, dynamic>? permissions,
  List<RequestOptions>? calls,
  List<RequestOptions>? settlements,
  bool era = false,
}) {
  final base = pt.posServer(permissions: permissions, era: era);
  return FakeAdapter((o) {
    final key = '${o.method} ${o.path}';
    switch (key) {
      case 'GET /payments/erapay/settlement':
        settlements?.add(o);
        return json(200, {
          'from': '2026-11-03',
          'to': '2026-11-03',
          'configured': true,
          'refreshed': o.queryParameters['refresh'] == true ? 2 : 0,
          'truncated': false,
          'totals': {
            'arus': '20000.00',
            'era': '23000.00',
            'diff': '3000.00',
            'count': 3,
            'matched': 1,
            'waiting': 1,
            'attention': 1,
          },
          'data': [
            {
              'charge_id': 'c1',
              'payment_id': 'pay_ids_0001',
              'gateway': 'ids',
              'status': 'succeeded',
              'state': 'matched',
              'amount': '20000.00',
              'arus_amount': '20000.00',
              'era_amount': '20000.00',
              'created_at': '2026-11-03T03:00:00Z',
              'outlet': 'Pusat',
              'cashier': 'Budi',
              'doc_no': 'MAIN-0001',
              'sale_status': 'completed',
            },
            {
              'charge_id': 'c2',
              'payment_id': 'pay_ids_0002',
              'gateway': 'ids',
              'status': 'succeeded',
              'state': 'orphan',
              'amount': '3000.00',
              'arus_amount': '0.00',
              'era_amount': '3000.00',
              'created_at': '2026-11-03T04:00:00Z',
              'outlet': 'Pusat',
              'cashier': 'Budi',
            },
            {
              'charge_id': 'c3',
              'payment_id': 'pay_ids_0003',
              'gateway': 'ids',
              'status': 'pending',
              'state': 'waiting',
              'amount': '5000.00',
              'arus_amount': '0.00',
              'era_amount': '0.00',
              'created_at': '2026-11-03T05:00:00Z',
              'outlet': 'Pusat',
              'cashier': 'Budi',
            },
          ],
        });
      case 'GET /items/i1':
        return json(200, {
          'id': 'i1',
          'sku': 'A1',
          'barcode': '',
          'origin': '',
          'name': 'Kopi Bubuk',
          'weight_grams': '0',
          'min_stock': '0',
          'sell_price': '12000.00',
          'kind': 'goods',
          'allow_negative_stock': false,
          'sell_below_cost': false,
          'description': '',
          'unit': {'id': 'un1', 'name': 'PCS'},
          'category': null,
          'brand': {'id': 'b1', 'name': 'Merek'},
          'principal': null,
          'supplier': null,
        });
      case 'PUT /items/i1':
      case 'PUT /items/i1/active':
        calls?.add(o);
        return json(200, {'id': 'i1'});
      case 'GET /items/':
        return json(200, {
          'data': [
            {
              'id': 'old1',
              'sku': 'X9',
              'name': 'Teh Lama',
              'unit': 'PCS',
              'price': '5000.00',
            },
          ],
          'total': 1,
        });
      case 'GET /catalog/lookup/suppliers/':
        return json(200, {
          'data': [
            {'id': 's1', 'name': 'PT Sumber Rejeki'},
          ],
          'total': 1,
        });
      case 'GET /purchases/items':
        return json(200, {
          'data': [
            {
              'id': 'i1',
              'sku': 'A1',
              'name': 'Kopi Bubuk',
              'unit': 'PCS',
              'last_cost': '8000.00',
              'stock_total': '10',
            },
          ],
        });
      case 'GET /purchases/':
        return json(200, {
          'data': [
            {
              'id': 'p1',
              'doc_no': 'PB-0001',
              'supplier_name': 'PT Sumber Rejeki',
              'purchase_date': '2026-11-03',
              'payment_type': 'cash',
              'status': 'completed',
              'total': '80000.00',
              'lines': 1,
            },
          ],
          'has_more': false,
          'next_cursor': '',
          'summary': {'count': 1, 'total': '80000.00'},
        });
      case 'GET /purchases/p1':
        return json(200, {
          'id': 'p1',
          'doc_no': 'PB-0001',
          'supplier_id': 's1',
          'supplier_name': 'PT Sumber Rejeki',
          'supplier_invoice_no': '',
          'purchase_date': '2026-11-03',
          'status': 'completed',
          'note': '',
          'subtotal': '80000.00',
          'tax_pct': '0',
          'tax_amount': '0',
          'other_cost': '0',
          'total': '80000.00',
          'created_by': 'Budi',
          'lines': [
            {
              'item_id': 'i1',
              'name': 'Kopi Bubuk',
              'unit': 'PCS',
              'qty_display': '10',
              'qty_warehouse': '0',
              'unit_price': '8000.00',
              'discounts': [],
            },
          ],
          'costs': [],
          'returns': [],
        });
      case 'POST /purchases/':
        calls?.add(o);
        return json(201, {'id': 'p2', 'doc_no': 'PB-0002', 'lines': []});
      case 'POST /purchases/p1/void':
        calls?.add(o);
        return json(200, {'id': 'p1', 'status': 'void'});
    }
    return base.handler(o);
  });
}

void main() {
  testWidgets('beranda: menu compact hanya memuat modul yang diizinkan', (
    tester,
  ) async {
    await pt.boot(tester, manageServer());
    await pt.settle(tester);
    // Kasir biasa: tidak ada Barang, Pembelian, Laporan.
    expect(find.text('Kasir'), findsOneWidget);
    expect(find.text('Barang'), findsNothing);
    expect(find.text('Pembelian'), findsNothing);
    expect(find.text('Laporan penjualan'), findsNothing);
  });

  testWidgets('beranda admin: semua menu muncul dalam ubin kecil', (
    tester,
  ) async {
    await pt.boot(tester, manageServer(permissions: _admin));
    await pt.settle(tester);
    for (final t in ['Kasir', 'Barang', 'Pembelian', 'Laporan penjualan']) {
      expect(find.text(t), findsOneWidget);
    }
    // Ubin kecil, bukan tombol besar.
    expect(tester.getSize(find.text('Barang')).height, lessThan(24));
  });

  testWidgets('hanya izin lihat pembelian: tanpa tombol pembelian baru', (
    tester,
  ) async {
    await pt.boot(
      tester,
      manageServer(
        permissions: {
          'purchase_list': ['view'],
        },
      ),
    );
    await pt.settle(tester);
    await tester.tap(find.text('Pembelian'));
    await tester.pumpAndSettle();
    expect(find.text('PB-0001'), findsOneWidget);
    expect(find.text('Pembelian baru'), findsNothing);
    await tester.tap(find.text('PB-0001'));
    await tester.pumpAndSettle();
    expect(find.text('Batalkan'), findsNothing);
    expect(find.text('Ubah'), findsNothing);
  });

  testWidgets('barang: ubah harga dan arsipkan sesuai izin', (tester) async {
    final calls = <RequestOptions>[];
    await pt.boot(tester, manageServer(permissions: _admin, calls: calls));
    await pt.settle(tester);
    await tester.tap(find.text('Barang'));
    await tester.pumpAndSettle();
    expect(find.text('Kopi Bubuk'), findsOneWidget);

    // Ubah: server mengganti semua field, jadi sku & brand dari barang lama ikut dikirim.
    await tester.tap(find.text('Kopi Bubuk'));
    await tester.pumpAndSettle();
    expect(
      find.text('Harga modal'),
      findsNothing,
    ); // modal hanya berubah lewat pembelian
    await tester.enterText(
      find.widgetWithText(TextField, 'Harga jual'),
      '13500',
    );
    await tester.tap(find.widgetWithText(FilledButton, 'Simpan'));
    await tester.pumpAndSettle();
    final put = calls.firstWhere((o) => o.path == '/items/i1');
    final body = put.data as Map;
    expect(body['sku'], 'A1');
    expect(body['sell_price'], '13500');
    expect(body['brand_id'], 'b1');
    expect(body['unit_id'], 'un1');

    // Arsipkan lewat menu + konfirmasi.
    await tester.tap(find.byIcon(Icons.more_vert).first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Arsipkan').last);
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(FilledButton, 'Arsipkan'));
    await tester.pumpAndSettle();
    final act = calls.firstWhere((o) => o.path == '/items/i1/active');
    expect((act.data as Map)['active'], false);
  });

  testWidgets('barang: hanya izin lihat → tanpa tambah, ubah, arsip', (
    tester,
  ) async {
    await pt.boot(
      tester,
      manageServer(
        permissions: {
          'items': ['view'],
        },
      ),
    );
    await pt.settle(tester);
    await tester.tap(find.text('Barang'));
    await tester.pumpAndSettle();
    expect(find.text('Kopi Bubuk'), findsOneWidget);
    expect(find.byIcon(Icons.add), findsNothing);
    expect(find.byIcon(Icons.more_vert), findsNothing);
    expect(find.text('Diarsipkan'), findsNothing);
  });

  testWidgets('pembelian: buat tunai, kunci idempotensi, lalu batalkan', (
    tester,
  ) async {
    final calls = <RequestOptions>[];
    await pt.boot(tester, manageServer(permissions: _admin, calls: calls));
    await pt.settle(tester);
    await tester.tap(find.text('Pembelian'));
    await tester.pumpAndSettle();
    expect(find.text('1 pembelian, Rp 80.000'), findsOneWidget);

    await tester.tap(find.text('Pembelian baru'));
    await tester.pumpAndSettle();
    // Tanpa pemasok dan barang → ditolak di klien.
    await tester.tap(find.text('Simpan pembelian'));
    await tester.pump();
    expect(find.text('Pilih pemasok dulu.'), findsOneWidget);
    expect(calls, isEmpty);

    await tester.tap(find.text('Pilih pemasok'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('PT Sumber Rejeki'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Tambah barang'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Kopi Bubuk'));
    await tester.pumpAndSettle();
    await tester.enterText(find.widgetWithText(TextField, 'Qty (PCS)'), '5');
    await tester.pump();
    expect(find.text('Rp 40.000'), findsWidgets); // 5 × 8.000
    await tester.tap(find.text('Simpan pembelian'));
    await tester.pumpAndSettle();

    final post = calls.firstWhere((o) => o.method == 'POST');
    expect(post.headers['Idempotency-Key'], isNotEmpty);
    final body = post.data as Map;
    expect(body['payment_type'], 'cash');
    expect(body['supplier_id'], 's1');
    expect(body.containsKey('tenant_id'), isFalse);
    final line = (body['lines'] as List).single as Map;
    expect(line['item_id'], 'i1');
    expect(line['qty_display'], '5');
    expect(line['unit_price'], '8000');
    expect(find.textContaining('PB-0002'), findsWidgets); // snackbar sukses

    // Detail → batalkan dengan alasan.
    await tester.tap(find.text('PB-0001'));
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(OutlinedButton, 'Batalkan'));
    await tester.pumpAndSettle();
    await tester.enterText(
      find.widgetWithText(TextField, 'Alasan (minimal 3 huruf)'),
      'salah input',
    );
    await tester.tap(find.widgetWithText(FilledButton, 'Batalkan'));
    await tester.pumpAndSettle();
    final v = calls.firstWhere((o) => o.path == '/purchases/p1/void');
    expect((v.data as Map)['reason'], 'salah input');
  });

  testWidgets('settlement EraPay: UANG ARUS vs UANG ERAPAY, status, dan ID', (
    tester,
  ) async {
    final calls = <RequestOptions>[];
    await pt.boot(
      tester,
      manageServer(permissions: _admin, era: true, settlements: calls),
    );
    await pt.settle(tester);
    await tester.tap(find.text('Settlement'));
    await tester.pumpAndSettle();
    expect(calls.last.queryParameters['refresh'], isNull);
    expect(find.text('UANG ARUS'), findsOneWidget);
    expect(find.text('UANG ERAPAY'), findsOneWidget);
    expect(find.text('Rp 20.000'), findsWidgets);
    expect(find.text('Rp 23.000'), findsWidgets);
    // Selisih dijelaskan: uang lunas di EraPay yang belum tercatat sebagai nota.
    expect(find.textContaining('Selisih Rp 3.000'), findsOneWidget);
    // Per transaksi: keadaan, status EraPay, dan ID transaksi.
    expect(find.text('Cocok'), findsWidgets);
    expect(find.text('Belum tercatat'), findsOneWidget);
    expect(find.text('pay_ids_0002'), findsOneWidget);
    expect(find.text('MAIN-0001'), findsOneWidget);
    // Saring "Perlu dicek": hanya transaksi bermasalah.
    await tester.tap(find.text('Perlu dicek (1)'));
    await tester.pumpAndSettle();
    expect(find.text('pay_ids_0002'), findsOneWidget);
    expect(find.text('pay_ids_0001'), findsNothing);
    // Cek ke EraPay → permintaan ulang dengan refresh=true.
    await tester.tap(find.byIcon(Icons.sync));
    await tester.pumpAndSettle();
    expect(calls.last.queryParameters['refresh'], true);
    expect(
      find.text('Status diperbarui dari EraPay: 2 transaksi'),
      findsOneWidget,
    );
  });

  testWidgets('menu Settlement hanya untuk admin dengan EraPay terpasang', (
    tester,
  ) async {
    await pt.boot(tester, manageServer(permissions: _admin));
    await pt.settle(tester);
    expect(find.text('Settlement'), findsNothing); // EraPay belum terpasang
  });

  testWidgets('kasir biasa tidak melihat menu Settlement walau EraPay aktif', (
    tester,
  ) async {
    await pt.boot(tester, manageServer(era: true));
    await pt.settle(tester);
    expect(find.text('Settlement'), findsNothing);
  });
}
