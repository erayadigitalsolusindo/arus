/// Konfigurasi build. Bawaan = server produksi; ganti saat pengembangan:
///   flutter run --dart-define=API_URL=http://192.168.1.10:8080
/// atau pilih/ketik alamat dari layar login (tersimpan di perangkat).
/// Rilis wajib https (cleartext hanya diizinkan di manifest debug).
class AppConfig {
  const AppConfig._();

  static const String productionUrl = 'https://arus-api.erayadigital.co.id';

  /// Localhost komputer dari dalam emulator Android.
  static const String emulatorUrl = 'http://10.0.2.2:8080';

  static const String apiUrl = String.fromEnvironment(
    'API_URL',
    defaultValue: productionUrl,
  );

  /// OAuth client ID bertipe **Web** (sama dengan salah satu GOOGLE_CLIENT_IDS di server): menjadi `aud` ID token Google.
  /// Bukan rahasia (client ID itu publik), jadi boleh ditulis di sini. Isi [googleClientIdDefault] dengan ID dari
  /// Google Cloud Console; kosong = tombol Google disembunyikan. Bisa diganti lewat `--dart-define=GOOGLE_SERVER_CLIENT_ID=...`.
  static const String googleClientIdDefault =
      '452740845962-eerv4ddpk4uhbsglrk49mjmiqatlre5p.apps.googleusercontent.com'; // contoh: 1234567890-xxxx.apps.googleusercontent.com

  static const String googleServerClientId = String.fromEnvironment(
    'GOOGLE_SERVER_CLIENT_ID',
    defaultValue: googleClientIdDefault,
  );

  /// Penanda klien native: server mengirim/menerima refresh token lewat badan JSON, bukan cookie.
  static const String clientHeader = 'X-Client';
  static const String clientValue = 'mobile';
}
