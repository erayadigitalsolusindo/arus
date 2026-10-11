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

  /// Penanda klien native: server mengirim/menerima refresh token lewat badan JSON, bukan cookie.
  static const String clientHeader = 'X-Client';
  static const String clientValue = 'mobile';
}
