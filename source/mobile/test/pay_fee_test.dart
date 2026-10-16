import 'package:arus_mobile/features/pos/pos_models.dart';
import 'package:decimal/decimal.dart';
import 'package:flutter_test/flutter_test.dart';

Decimal d(String s) => Decimal.parse(s);

void main() {
  // Aturan transaksi kecil (BI): <= batas -> MDR flat menggantikan persen + tetap; selaras dengan server.
  test('feeOf memakai MDR flat pada transaksi kecil', () {
    final m = PayMethodInfo.fromJson({
      'id': 'm',
      'name': 'QRIS',
      'kind': 'ewallet',
      'fee_pct': '0.70',
      'fee_flat': '100.00',
      'small_tx_limit': '100000.00',
      'small_tx_flat': '500.00',
    });
    expect(m.feeOf(d('10000')), d('500'));
    expect(m.feeOf(d('100000')), d('500')); // tepat di batas
    expect(m.feeOf(d('1000000')), d('7100')); // di atas batas: 0,7% + 100
    expect(m.feeLabelFor(d('10000')), isNot(contains('%')));
    expect(m.feeLabelFor(d('1000000')), contains('%'));
    expect(m.hasFee, isTrue);
  });

  test('tanpa aturan transaksi kecil tarif biasa', () {
    final m = PayMethodInfo.fromJson({
      'id': 'm',
      'name': 'Debit',
      'kind': 'debit',
      'fee_pct': '0.70',
      'fee_flat': '0.00',
      'small_tx_limit': '0.00',
      'small_tx_flat': '0.00',
    });
    expect(m.feeOf(d('10000')), d('70'));
    expect(m.hasSmallRule, isFalse);
  });
}
