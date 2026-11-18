import 'package:decimal/decimal.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:arus_mobile/core/api/api_error.dart';
import 'package:arus_mobile/core/offline/offline_box.dart';
import 'package:arus_mobile/features/pos/outbox.dart';
import 'package:arus_mobile/features/pos/pos_models.dart';
import 'package:arus_mobile/features/pos/pos_repository.dart';

import 'test_helpers.dart';

final _item = PosItem(
  id: 'i1',
  sku: 'A1',
  barcode: '',
  name: 'Teh',
  unit: 'pcs',
  price: Decimal.fromInt(5000),
  stockDisplay: Decimal.fromInt(10),
  isGoods: true,
);

SaleDraft _draft() => SaleDraft(
  lines: [CartLine(item: _item, qty: Decimal.fromInt(2))],
  applyTax: false,
);

FakeAdapter _down() => FakeAdapter(
  (o) => throw DioException(
    requestOptions: o,
    type: DioExceptionType.connectionError,
  ),
);

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  test(
    'nota tunai offline masuk antrean; kirim ulang memakai kunci yang sama',
    () async {
      const box = OfflineBox('u.o');
      await box.write('methods', [
        {'id': 'm1', 'name': 'Tunai', 'kind': 'cash'},
        {'id': 'm2', 'name': 'QRIS', 'kind': 'ewallet'},
      ]);
      final queued = <QueuedSale>[];
      final repo = PosRepository(
        clientWith(_down()),
        offlineBox: box,
        enqueue: (q) async => queued.add(q),
      );

      final r = await repo.createSale(
        _draft(),
        payments: [
          PaymentInput(methodId: 'm1', amount: Decimal.fromInt(10000)),
        ],
        idempotencyKey: 'abcdef12-key',
      );
      expect(r.offline, isTrue);
      expect(r.total, Decimal.fromInt(10000));
      expect(queued.single.key, 'abcdef12-key');
      expect(queued.single.body['offline'], true);
      expect(queued.single.body['occurred_at'], isNotNull);

      // QRIS tidak boleh offline: galat jaringan dikembalikan apa adanya, tidak masuk antrean.
      queued.clear();
      await expectLater(
        repo.createSale(
          _draft(),
          payments: [
            PaymentInput(methodId: 'm2', amount: Decimal.fromInt(10000)),
          ],
          idempotencyKey: 'abcdef12-key2',
        ),
        throwsA(isA<ApiError>().having((e) => e.isNetwork, 'isNetwork', true)),
      );
      expect(queued, isEmpty);

      // Jaringan pulih: kirim ulang dengan Idempotency-Key yang sama.
      final up = FakeAdapter(
        (o) => json(201, {
          'id': 's1',
          'doc_no': 'MAIN-261118-0001',
          'total': '10000.00',
          'change': '0.00',
          'surcharge': '0.00',
          'receivable': '0.00',
        }),
      );
      final sent = await PosRepository(clientWith(up), offlineBox: box)
          .sendQueued(
            QueuedSale(
              key: 'abcdef12-key',
              body: r.offline ? {'offline': true} : {},
              at: DateTime.now(),
              total: '10000.00',
            ),
          );
      expect(sent.docNo, 'MAIN-261118-0001');
      expect(up.calls.single.headers['Idempotency-Key'], 'abcdef12-key');
    },
  );

  test('pencarian offline memakai cache katalog', () async {
    const box = OfflineBox('u.o');
    final okAdapter = FakeAdapter(
      (o) => json(200, {
        'data': [_item.toJson()],
        'next_cursor': '',
      }),
    );
    final on = PosRepository(clientWith(okAdapter), offlineBox: box);
    await on.searchPage('teh');
    await Future<void>.delayed(
      const Duration(seconds: 4),
    ); // tunggu simpan ke disk

    final off = PosRepository(clientWith(_down()), offlineBox: box);
    final r = await off.searchPage('teh');
    expect(r.items.single.id, 'i1');
    expect((await off.searchPage('zzz')).items, isEmpty);
  });
}
