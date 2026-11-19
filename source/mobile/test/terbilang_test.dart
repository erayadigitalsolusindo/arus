import 'package:flutter_test/flutter_test.dart';
import 'package:arus_mobile/core/pay_voice.dart';

void main() {
  test('terbilang', () {
    expect(terbilang(125000), 'seratus dua puluh lima ribu');
    expect(terbilang(1500000), 'satu juta lima ratus ribu');
  });
}
