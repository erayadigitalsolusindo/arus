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

  @override
  String get themeSystem => 'Ikut sistem';

  @override
  String get themeLight => 'Terang';

  @override
  String get themeDark => 'Gelap';

  @override
  String themeTooltip(String mode) {
    return 'Tampilan: $mode';
  }

  @override
  String get posTitle => 'Kasir';

  @override
  String get posSearchHint => 'Cari nama, kode, atau barcode';

  @override
  String get posSearchEmpty => 'Barang tidak ditemukan.';

  @override
  String get posCartTitle => 'Keranjang';

  @override
  String get posCartEmpty => 'Keranjang kosong. Pilih barang untuk memulai.';

  @override
  String posItemsCount(int count) {
    return '$count barang';
  }

  @override
  String get posSubtotal => 'Subtotal';

  @override
  String get posDiscount => 'Potongan';

  @override
  String get posTaxLine => 'Pajak';

  @override
  String get posOtherCost => 'Biaya lain';

  @override
  String get posTotal => 'Total';

  @override
  String get posPay => 'Bayar';

  @override
  String get posTaxToggle => 'Hitung pajak';

  @override
  String get posClearCart => 'Kosongkan keranjang';

  @override
  String get posRemoveLine => 'Hapus';

  @override
  String posStock(String qty) {
    return 'Stok $qty';
  }

  @override
  String posStockShort(String qty) {
    return 'Stok kurang (tersedia $qty)';
  }

  @override
  String get posBelowCost => 'Harga di bawah HPP';

  @override
  String get posQuoteFailed => 'Gagal menghitung total.';

  @override
  String get posPanelOutlet => 'Outlet';

  @override
  String get posPanelCashier => 'Kasir';

  @override
  String get posPanelBusiness => 'Bisnis';

  @override
  String get posPanelShift => 'Shift';

  @override
  String get posPanelToggle => 'Info outlet';

  @override
  String get posBackHome => 'Kembali ke beranda';

  @override
  String posShiftOpenedAt(String time) {
    return 'Dibuka $time';
  }

  @override
  String get posShiftNone => 'Belum ada shift';

  @override
  String get shiftOpenTitle => 'Buka shift';

  @override
  String get shiftOpenHint =>
      'Masukkan modal awal kas laci sebelum mulai berjualan.';

  @override
  String get shiftOpeningCash => 'Modal awal (Rp)';

  @override
  String get shiftOpenSubmit => 'Buka shift';

  @override
  String get payTitle => 'Pembayaran';

  @override
  String get payMethod => 'Metode';

  @override
  String get payAmount => 'Jumlah (Rp)';

  @override
  String get payExact => 'Uang pas';

  @override
  String get payAddMethod => 'Tambah metode';

  @override
  String get payRef => 'No. referensi';

  @override
  String get payTotalDue => 'Total tagihan';

  @override
  String get payPaid => 'Dibayar';

  @override
  String get payChange => 'Kembalian';

  @override
  String payShortBy(String amount) {
    return 'Kurang $amount';
  }

  @override
  String get paySubmit => 'Selesaikan pembayaran';

  @override
  String get payProcessing => 'Memproses...';

  @override
  String get paySuccessTitle => 'Transaksi berhasil';

  @override
  String paySuccessDoc(String docNo) {
    return 'Nomor nota $docNo';
  }

  @override
  String get payNewSale => 'Transaksi baru';

  @override
  String payChangeDue(String amount) {
    return 'Kembalian $amount';
  }

  @override
  String get errorShiftRequired =>
      'Buka shift terlebih dahulu sebelum berjualan.';

  @override
  String get errorStockInsufficient =>
      'Stok tidak mencukupi untuk salah satu barang.';

  @override
  String get errorValidation => 'Isian belum valid. Periksa kembali.';

  @override
  String get errorForbidden => 'Anda tidak memiliki izin untuk aksi ini.';

  @override
  String get errorIdempotencyMismatch =>
      'Permintaan bentrok dengan transaksi sebelumnya. Coba lagi.';

  @override
  String get errorMethodInactive => 'Metode pembayaran tidak aktif.';

  @override
  String get errorEditWindow => 'Di luar batas waktu edit.';

  @override
  String get serverTitle => 'Alamat server';

  @override
  String get serverHelp =>
      'Alamat API ARUS. Di HP fisik gunakan IP komputer di jaringan yang sama, mis. http://192.168.1.10:8080. Kosongkan untuk kembali ke bawaan.';

  @override
  String get serverLabel => 'Alamat server';

  @override
  String get serverInvalid =>
      'Harus diawali http:// atau https:// dan berisi alamat.';

  @override
  String get save => 'Simpan';
}
