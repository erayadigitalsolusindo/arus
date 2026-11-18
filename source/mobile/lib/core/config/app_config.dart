/// Konfigurasi build. Bawaan = server produksi; ganti saat pengembangan:
///   flutter run --dart-define=API_URL=http://192.168.1.10:8080
/// atau pilih/ketik alamat dari layar login (tersimpan di perangkat).
/// Rilis wajib https (cleartext hanya diizinkan di manifest debug).
class AppConfig {
  const AppConfig._();

  static const String productionUrl = 'https://arus-api.erayadigital.co.id';

  /// Backend dev di WSL, dilihat dari emulator (LDPlayer di Windows); alamat host WSL bisa berubah setelah restart.
  static const String emulatorUrl = 'http://172.21.192.1:8080';

  static const String apiUrl = String.fromEnvironment(
    'API_URL',
    defaultValue: productionUrl,
  );

  /// OAuth client ID bertipe **Web** (sama dengan salah satu GOOGLE_CLIENT_IDS di server): menjadi `aud` ID token Google.
  /// Bukan rahasia (client ID itu publik), jadi boleh ditulis di sini. Isi [googleClientIdDefault] dengan ID dari
  /// Google Cloud Console; kosong = tombol Google disembunyikan. Bisa diganti lewat `--dart-define=GOOGLE_SERVER_CLIENT_ID=...`.
  static const String googleClientIdDefault =
      '452740845962-ilhsknjm9viukk3vmbo70qoqc0vql11e.apps.googleusercontent.com'; // contoh: 1234567890-xxxx.apps.googleusercontent.com

  static const String googleServerClientId = String.fromEnvironment(
    'GOOGLE_SERVER_CLIENT_ID',
    defaultValue: googleClientIdDefault,
  );

  /// SSO Eraya (OIDC + PKCE): issuer & client ID publik, harus sama dengan ERAYA_ISSUER/ERAYA_CLIENT_ID di server.
  /// Daftarkan [erayaRedirectUri] sebagai redirect URI pada client itu di Eraya. Kosong = tombol disembunyikan.
  static const String erayaIssuer = String.fromEnvironment(
    'ERAYA_ISSUER',
    defaultValue: 'https://erayadigital.co.id',
  );
  static const String erayaClientId = String.fromEnvironment(
    'ERAYA_CLIENT_ID',
    defaultValue: 'c_16ca4457ba364862efe4bf72',
  );
  static const String erayaCallbackScheme = 'id.co.erayadigital.arus';
  static const String erayaRedirectUri =
      '$erayaCallbackScheme://eraya/callback';

  /// Penanda klien native: server mengirim/menerima refresh token lewat badan JSON, bukan cookie.
  static const String clientHeader = 'X-Client';
  static const String clientValue = 'mobile';
}
