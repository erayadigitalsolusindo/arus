import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Penyimpanan refresh token di Keystore Android. Access token TIDAK disimpan (hanya di memori).
abstract class TokenStore {
  Future<String?> readRefreshToken();
  Future<void> writeRefreshToken(String token);
  Future<void> clear();
}

class SecureTokenStore implements TokenStore {
  SecureTokenStore([FlutterSecureStorage? storage]) : _s = storage ?? const FlutterSecureStorage();

  static const _key = 'arus.refresh_token';
  final FlutterSecureStorage _s;

  @override
  Future<String?> readRefreshToken() => _s.read(key: _key);

  @override
  Future<void> writeRefreshToken(String token) => _s.write(key: _key, value: token);

  @override
  Future<void> clear() => _s.delete(key: _key);
}

/// Untuk test.
class MemoryTokenStore implements TokenStore {
  String? _token;

  @override
  Future<String?> readRefreshToken() async => _token;

  @override
  Future<void> writeRefreshToken(String token) async => _token = token;

  @override
  Future<void> clear() async => _token = null;
}
