import 'package:arus_mobile/core/update/app_update.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('compareVersions', () {
    expect(compareVersions('1.0.1', '1.0.0+1') > 0, isTrue);
    expect(compareVersions('1.0.0+2', '1.0.0+1') > 0, isTrue);
    expect(compareVersions('1.0.0', '1.0.0+5'), 0); // tanpa build di server: nama versi saja
    expect(compareVersions('1.0.0+1', '1.0.0+1'), 0);
    expect(compareVersions('1.2', '1.10.0') < 0, isTrue);
    expect(compareVersions('v2.0.0', '1.9.9+9') > 0, isTrue);
  });
}
