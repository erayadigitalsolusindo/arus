/// Konfigurasi build. Ganti alamat API saat menjalankan:
///   flutter run --dart-define=API_URL=http://192.168.1.10:8080
/// Bawaan `10.0.2.2` = localhost komputer dari dalam emulator Android. HP fisik harus memakai IP LAN komputer.
/// Rilis wajib https (cleartext hanya diizinkan di manifest debug).
class AppConfig {
  const AppConfig._();

  static const String apiUrl = String.fromEnvironment(
    'API_URL',
    defaultValue: 'http://10.0.2.2:8080',
  );

  /// Penanda klien native: server mengirim/menerima refresh token lewat badan JSON, bukan cookie.
  static const String clientHeader = 'X-Client';
  static const String clientValue = 'mobile';
}
