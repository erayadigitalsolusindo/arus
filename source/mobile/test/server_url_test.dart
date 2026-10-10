import 'package:flutter_test/flutter_test.dart';

import 'package:arus_mobile/core/config/server_url.dart';

void main() {
  test('normalizeServerUrl merapikan dan menolak alamat tidak sah', () {
    expect(
      normalizeServerUrl(' http://192.168.18.51:8080/ '),
      'http://192.168.18.51:8080',
    );
    expect(
      normalizeServerUrl('https://api.contoh.id//'),
      'https://api.contoh.id',
    );
    expect(normalizeServerUrl('192.168.18.51:8080'), isNull);
    expect(normalizeServerUrl('ftp://x.test'), isNull);
    expect(normalizeServerUrl('http://'), isNull);
    expect(normalizeServerUrl(''), isNull);
  });
}
