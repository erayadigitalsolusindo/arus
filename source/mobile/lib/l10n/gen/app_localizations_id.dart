// ignore: unused_import
import 'package:intl/intl.dart' as intl;

import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for Indonesian (`id`).
class AppLocalizationsId extends AppLocalizations {
  AppLocalizationsId([String locale = 'id']) : super(locale);

  @override
  String get appName => 'ARUS';

  @override
  String get loginTitleLine1 => 'Akses Kasir';

  @override
  String get loginTitleLine2 => 'ARUS (Aciraba Upgrade System)';

  @override
  String get loginSubtitle => 'Masuk untuk melanjutkan ke ruang kerja Anda';

  @override
  String get loginEmail => 'Alamat email';

  @override
  String get loginEmailHint => 'anda@perusahaan.com';

  @override
  String get loginPassword => 'Kata sandi';

  @override
  String get loginShowPassword => 'Tampilkan kata sandi';

  @override
  String get loginHidePassword => 'Sembunyikan kata sandi';

  @override
  String get loginRemember => 'Tetap masuk';

  @override
  String get loginSubmit => 'Masuk';

  @override
  String get loginEmailRequired => 'Email wajib diisi.';

  @override
  String get loginEmailInvalid => 'Format email tidak valid.';

  @override
  String get loginPasswordRequired => 'Kata sandi wajib diisi.';

  @override
  String get loginServer => 'Server';

  @override
  String get logout => 'Keluar';

  @override
  String homeWelcome(String name) {
    return 'Selamat datang, $name';
  }

  @override
  String get homeOutlet => 'Outlet aktif';

  @override
  String get homeTenant => 'Bisnis';

  @override
  String get homeComingSoon => 'Layar kasir sedang disiapkan.';

  @override
  String get errorNetwork => 'Tidak dapat terhubung ke server.';

  @override
  String get errorTimeout => 'Server tidak menjawab. Coba lagi.';

  @override
  String get errorUnknown => 'Terjadi kesalahan. Coba lagi.';

  @override
  String get errorInvalidCredentials => 'Email atau password salah.';

  @override
  String errorInvalidCredentialsLeft(int count) {
    return 'Email atau password salah. Sisa $count percobaan sebelum akun dikunci sementara.';
  }

  @override
  String get errorAccountDisabled =>
      'Akun Anda dinonaktifkan. Hubungi administrator.';

  @override
  String get errorNoOutlet =>
      'Akun Anda belum memiliki outlet aktif. Hubungi administrator.';

  @override
  String errorAccountLocked(int minutes) {
    return 'Terlalu banyak percobaan gagal. Coba lagi dalam $minutes menit.';
  }

  @override
  String get errorRateLimited =>
      'Terlalu banyak percobaan. Coba lagi beberapa saat lagi.';

  @override
  String get errorUnavailable =>
      'Layanan sementara tidak tersedia. Coba lagi nanti.';

  @override
  String get errorInternal => 'Terjadi kesalahan pada server.';

  @override
  String get errorSessionInvalid => 'Sesi berakhir. Silakan masuk kembali.';
}
