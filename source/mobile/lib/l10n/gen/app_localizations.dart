import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:intl/intl.dart' as intl;

import 'app_localizations_en.dart';
import 'app_localizations_id.dart';
import 'app_localizations_ja.dart';

// ignore_for_file: type=lint

/// Callers can lookup localized strings with an instance of AppLocalizations
/// returned by `AppLocalizations.of(context)`.
///
/// Applications need to include `AppLocalizations.delegate()` in their app's
/// `localizationDelegates` list, and the locales they support in the app's
/// `supportedLocales` list. For example:
///
/// ```dart
/// import 'gen/app_localizations.dart';
///
/// return MaterialApp(
///   localizationsDelegates: AppLocalizations.localizationsDelegates,
///   supportedLocales: AppLocalizations.supportedLocales,
///   home: MyApplicationHome(),
/// );
/// ```
///
/// ## Update pubspec.yaml
///
/// Please make sure to update your pubspec.yaml to include the following
/// packages:
///
/// ```yaml
/// dependencies:
///   # Internationalization support.
///   flutter_localizations:
///     sdk: flutter
///   intl: any # Use the pinned version from flutter_localizations
///
///   # Rest of dependencies
/// ```
///
/// ## iOS Applications
///
/// iOS applications define key application metadata, including supported
/// locales, in an Info.plist file that is built into the application bundle.
/// To configure the locales supported by your app, you’ll need to edit this
/// file.
///
/// First, open your project’s ios/Runner.xcworkspace Xcode workspace file.
/// Then, in the Project Navigator, open the Info.plist file under the Runner
/// project’s Runner folder.
///
/// Next, select the Information Property List item, select Add Item from the
/// Editor menu, then select Localizations from the pop-up menu.
///
/// Select and expand the newly-created Localizations item then, for each
/// locale your application supports, add a new item and select the locale
/// you wish to add from the pop-up menu in the Value field. This list should
/// be consistent with the languages listed in the AppLocalizations.supportedLocales
/// property.
abstract class AppLocalizations {
  AppLocalizations(String locale)
    : localeName = intl.Intl.canonicalizedLocale(locale.toString());

  final String localeName;

  static AppLocalizations of(BuildContext context) {
    return Localizations.of<AppLocalizations>(context, AppLocalizations)!;
  }

  static const LocalizationsDelegate<AppLocalizations> delegate =
      _AppLocalizationsDelegate();

  /// A list of this localizations delegate along with the default localizations
  /// delegates.
  ///
  /// Returns a list of localizations delegates containing this delegate along with
  /// GlobalMaterialLocalizations.delegate, GlobalCupertinoLocalizations.delegate,
  /// and GlobalWidgetsLocalizations.delegate.
  ///
  /// Additional delegates can be added by appending to this list in
  /// MaterialApp. This list does not have to be used at all if a custom list
  /// of delegates is preferred or required.
  static const List<LocalizationsDelegate<dynamic>> localizationsDelegates =
      <LocalizationsDelegate<dynamic>>[
        delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
      ];

  /// A list of this localizations delegate's supported locales.
  static const List<Locale> supportedLocales = <Locale>[
    Locale('en'),
    Locale('id'),
    Locale('ja'),
  ];

  /// No description provided for @appName.
  ///
  /// In id, this message translates to:
  /// **'ARUS'**
  String get appName;

  /// No description provided for @loginTitleLine1.
  ///
  /// In id, this message translates to:
  /// **'Akses Kasir'**
  String get loginTitleLine1;

  /// No description provided for @loginTitleLine2.
  ///
  /// In id, this message translates to:
  /// **'ARUS (Aciraba Upgrade System)'**
  String get loginTitleLine2;

  /// No description provided for @loginSubtitle.
  ///
  /// In id, this message translates to:
  /// **'Masuk untuk melanjutkan ke ruang kerja Anda'**
  String get loginSubtitle;

  /// No description provided for @loginEmail.
  ///
  /// In id, this message translates to:
  /// **'Alamat email'**
  String get loginEmail;

  /// No description provided for @loginEmailHint.
  ///
  /// In id, this message translates to:
  /// **'anda@perusahaan.com'**
  String get loginEmailHint;

  /// No description provided for @loginPassword.
  ///
  /// In id, this message translates to:
  /// **'Kata sandi'**
  String get loginPassword;

  /// No description provided for @loginShowPassword.
  ///
  /// In id, this message translates to:
  /// **'Tampilkan kata sandi'**
  String get loginShowPassword;

  /// No description provided for @loginHidePassword.
  ///
  /// In id, this message translates to:
  /// **'Sembunyikan kata sandi'**
  String get loginHidePassword;

  /// No description provided for @loginRemember.
  ///
  /// In id, this message translates to:
  /// **'Tetap masuk'**
  String get loginRemember;

  /// No description provided for @loginSubmit.
  ///
  /// In id, this message translates to:
  /// **'Masuk'**
  String get loginSubmit;

  /// No description provided for @loginEmailRequired.
  ///
  /// In id, this message translates to:
  /// **'Email wajib diisi.'**
  String get loginEmailRequired;

  /// No description provided for @loginEmailInvalid.
  ///
  /// In id, this message translates to:
  /// **'Format email tidak valid.'**
  String get loginEmailInvalid;

  /// No description provided for @loginPasswordRequired.
  ///
  /// In id, this message translates to:
  /// **'Kata sandi wajib diisi.'**
  String get loginPasswordRequired;

  /// No description provided for @loginServer.
  ///
  /// In id, this message translates to:
  /// **'Server'**
  String get loginServer;

  /// No description provided for @logout.
  ///
  /// In id, this message translates to:
  /// **'Keluar'**
  String get logout;

  /// No description provided for @homeWelcome.
  ///
  /// In id, this message translates to:
  /// **'Selamat datang, {name}'**
  String homeWelcome(String name);

  /// No description provided for @homeOutlet.
  ///
  /// In id, this message translates to:
  /// **'Outlet aktif'**
  String get homeOutlet;

  /// No description provided for @homeTenant.
  ///
  /// In id, this message translates to:
  /// **'Bisnis'**
  String get homeTenant;

  /// No description provided for @homeComingSoon.
  ///
  /// In id, this message translates to:
  /// **'Layar kasir sedang disiapkan.'**
  String get homeComingSoon;

  /// No description provided for @errorNetwork.
  ///
  /// In id, this message translates to:
  /// **'Tidak dapat terhubung ke server.'**
  String get errorNetwork;

  /// No description provided for @errorTimeout.
  ///
  /// In id, this message translates to:
  /// **'Server tidak menjawab. Coba lagi.'**
  String get errorTimeout;

  /// No description provided for @errorUnknown.
  ///
  /// In id, this message translates to:
  /// **'Terjadi kesalahan. Coba lagi.'**
  String get errorUnknown;

  /// No description provided for @errorInvalidCredentials.
  ///
  /// In id, this message translates to:
  /// **'Email atau password salah.'**
  String get errorInvalidCredentials;

  /// No description provided for @errorInvalidCredentialsLeft.
  ///
  /// In id, this message translates to:
  /// **'Email atau password salah. Sisa {count} percobaan sebelum akun dikunci sementara.'**
  String errorInvalidCredentialsLeft(int count);

  /// No description provided for @errorAccountDisabled.
  ///
  /// In id, this message translates to:
  /// **'Akun Anda dinonaktifkan. Hubungi administrator.'**
  String get errorAccountDisabled;

  /// No description provided for @errorNoOutlet.
  ///
  /// In id, this message translates to:
  /// **'Akun Anda belum memiliki outlet aktif. Hubungi administrator.'**
  String get errorNoOutlet;

  /// No description provided for @errorAccountLocked.
  ///
  /// In id, this message translates to:
  /// **'Terlalu banyak percobaan gagal. Coba lagi dalam {minutes} menit.'**
  String errorAccountLocked(int minutes);

  /// No description provided for @errorRateLimited.
  ///
  /// In id, this message translates to:
  /// **'Terlalu banyak percobaan. Coba lagi beberapa saat lagi.'**
  String get errorRateLimited;

  /// No description provided for @errorUnavailable.
  ///
  /// In id, this message translates to:
  /// **'Layanan sementara tidak tersedia. Coba lagi nanti.'**
  String get errorUnavailable;

  /// No description provided for @errorInternal.
  ///
  /// In id, this message translates to:
  /// **'Terjadi kesalahan pada server.'**
  String get errorInternal;

  /// No description provided for @errorSessionInvalid.
  ///
  /// In id, this message translates to:
  /// **'Sesi berakhir. Silakan masuk kembali.'**
  String get errorSessionInvalid;

  /// No description provided for @themeSystem.
  ///
  /// In id, this message translates to:
  /// **'Ikut sistem'**
  String get themeSystem;

  /// No description provided for @themeLight.
  ///
  /// In id, this message translates to:
  /// **'Terang'**
  String get themeLight;

  /// No description provided for @themeDark.
  ///
  /// In id, this message translates to:
  /// **'Gelap'**
  String get themeDark;

  /// No description provided for @textSizeTooltip.
  ///
  /// In id, this message translates to:
  /// **'Ukuran teks'**
  String get textSizeTooltip;

  /// No description provided for @textSizeTitle.
  ///
  /// In id, this message translates to:
  /// **'Ukuran teks'**
  String get textSizeTitle;

  /// No description provided for @textSizeSample.
  ///
  /// In id, this message translates to:
  /// **'Contoh teks: Kopi susu Rp 18.000'**
  String get textSizeSample;

  /// No description provided for @textSizeDecrease.
  ///
  /// In id, this message translates to:
  /// **'Perkecil'**
  String get textSizeDecrease;

  /// No description provided for @textSizeIncrease.
  ///
  /// In id, this message translates to:
  /// **'Perbesar'**
  String get textSizeIncrease;

  /// No description provided for @textSizeReset.
  ///
  /// In id, this message translates to:
  /// **'Atur ulang'**
  String get textSizeReset;

  /// No description provided for @textSizeDone.
  ///
  /// In id, this message translates to:
  /// **'Selesai'**
  String get textSizeDone;

  /// No description provided for @themeTooltip.
  ///
  /// In id, this message translates to:
  /// **'Tampilan: {mode}'**
  String themeTooltip(String mode);

  /// No description provided for @posTitle.
  ///
  /// In id, this message translates to:
  /// **'Kasir'**
  String get posTitle;

  /// No description provided for @posSearchHint.
  ///
  /// In id, this message translates to:
  /// **'Cari nama, kode, atau barcode'**
  String get posSearchHint;

  /// No description provided for @posSearchEmpty.
  ///
  /// In id, this message translates to:
  /// **'Barang tidak ditemukan.'**
  String get posSearchEmpty;

  /// No description provided for @posCartTitle.
  ///
  /// In id, this message translates to:
  /// **'Keranjang'**
  String get posCartTitle;

  /// No description provided for @posItemsCount.
  ///
  /// In id, this message translates to:
  /// **'{count} barang'**
  String posItemsCount(int count);

  /// No description provided for @posSubtotal.
  ///
  /// In id, this message translates to:
  /// **'Subtotal'**
  String get posSubtotal;

  /// No description provided for @posDiscount.
  ///
  /// In id, this message translates to:
  /// **'Potongan'**
  String get posDiscount;

  /// No description provided for @posTaxLine.
  ///
  /// In id, this message translates to:
  /// **'Pajak'**
  String get posTaxLine;

  /// No description provided for @posOtherCost.
  ///
  /// In id, this message translates to:
  /// **'Biaya lain'**
  String get posOtherCost;

  /// No description provided for @posTotal.
  ///
  /// In id, this message translates to:
  /// **'Total'**
  String get posTotal;

  /// No description provided for @posConfirm.
  ///
  /// In id, this message translates to:
  /// **'Konfirmasi'**
  String get posConfirm;

  /// No description provided for @posPay.
  ///
  /// In id, this message translates to:
  /// **'Bayar'**
  String get posPay;

  /// No description provided for @posTaxToggle.
  ///
  /// In id, this message translates to:
  /// **'Hitung pajak'**
  String get posTaxToggle;

  /// No description provided for @posClearCart.
  ///
  /// In id, this message translates to:
  /// **'Kosongkan keranjang'**
  String get posClearCart;

  /// No description provided for @posClearCartAsk.
  ///
  /// In id, this message translates to:
  /// **'Kosongkan keranjang?'**
  String get posClearCartAsk;

  /// No description provided for @posClearCartAskBody.
  ///
  /// In id, this message translates to:
  /// **'Semua barang di keranjang akan dihapus dan tidak bisa dikembalikan.'**
  String get posClearCartAskBody;

  /// No description provided for @posClearCartYes.
  ///
  /// In id, this message translates to:
  /// **'Ya, kosongkan'**
  String get posClearCartYes;

  /// No description provided for @posClearCartNo.
  ///
  /// In id, this message translates to:
  /// **'Batal'**
  String get posClearCartNo;

  /// No description provided for @posAddedToCart.
  ///
  /// In id, this message translates to:
  /// **'Ditambahkan'**
  String get posAddedToCart;

  /// No description provided for @posRemoveLine.
  ///
  /// In id, this message translates to:
  /// **'Hapus'**
  String get posRemoveLine;

  /// No description provided for @posStock.
  ///
  /// In id, this message translates to:
  /// **'Stok {qty}'**
  String posStock(String qty);

  /// No description provided for @posStockShort.
  ///
  /// In id, this message translates to:
  /// **'Stok kurang (tersedia {qty})'**
  String posStockShort(String qty);

  /// No description provided for @posBelowCost.
  ///
  /// In id, this message translates to:
  /// **'Harga di bawah HPP'**
  String get posBelowCost;

  /// No description provided for @posQuoteFailed.
  ///
  /// In id, this message translates to:
  /// **'Gagal menghitung total.'**
  String get posQuoteFailed;

  /// No description provided for @posPanelOutlet.
  ///
  /// In id, this message translates to:
  /// **'Outlet'**
  String get posPanelOutlet;

  /// No description provided for @posPanelCashier.
  ///
  /// In id, this message translates to:
  /// **'Kasir'**
  String get posPanelCashier;

  /// No description provided for @posPanelBusiness.
  ///
  /// In id, this message translates to:
  /// **'Bisnis'**
  String get posPanelBusiness;

  /// No description provided for @posPanelShift.
  ///
  /// In id, this message translates to:
  /// **'Shift'**
  String get posPanelShift;

  /// No description provided for @posPanelToggle.
  ///
  /// In id, this message translates to:
  /// **'Info outlet'**
  String get posPanelToggle;

  /// No description provided for @posBackHome.
  ///
  /// In id, this message translates to:
  /// **'Kembali ke beranda'**
  String get posBackHome;

  /// No description provided for @posShiftOpenedAt.
  ///
  /// In id, this message translates to:
  /// **'Dibuka {time}'**
  String posShiftOpenedAt(String time);

  /// No description provided for @posShiftNone.
  ///
  /// In id, this message translates to:
  /// **'Belum ada shift'**
  String get posShiftNone;

  /// No description provided for @shiftOpenTitle.
  ///
  /// In id, this message translates to:
  /// **'Buka shift'**
  String get shiftOpenTitle;

  /// No description provided for @shiftOpenHint.
  ///
  /// In id, this message translates to:
  /// **'Masukkan modal awal kas laci sebelum mulai berjualan.'**
  String get shiftOpenHint;

  /// No description provided for @shiftOpeningCash.
  ///
  /// In id, this message translates to:
  /// **'Modal awal (Rp)'**
  String get shiftOpeningCash;

  /// No description provided for @shiftOpenSubmit.
  ///
  /// In id, this message translates to:
  /// **'Buka shift'**
  String get shiftOpenSubmit;

  /// No description provided for @payTitle.
  ///
  /// In id, this message translates to:
  /// **'Pembayaran'**
  String get payTitle;

  /// No description provided for @payMethod.
  ///
  /// In id, this message translates to:
  /// **'Metode'**
  String get payMethod;

  /// No description provided for @payAmount.
  ///
  /// In id, this message translates to:
  /// **'Jumlah (Rp)'**
  String get payAmount;

  /// No description provided for @payExact.
  ///
  /// In id, this message translates to:
  /// **'Uang pas'**
  String get payExact;

  /// No description provided for @payAddMethod.
  ///
  /// In id, this message translates to:
  /// **'Tambah metode'**
  String get payAddMethod;

  /// No description provided for @payRef.
  ///
  /// In id, this message translates to:
  /// **'No. referensi'**
  String get payRef;

  /// No description provided for @payEraGenerate.
  ///
  /// In id, this message translates to:
  /// **'Buat QR QRIS'**
  String get payEraGenerate;

  /// No description provided for @payEraRegenerate.
  ///
  /// In id, this message translates to:
  /// **'Buat ulang QR'**
  String get payEraRegenerate;

  /// No description provided for @payEraCreating.
  ///
  /// In id, this message translates to:
  /// **'Membuat QR...'**
  String get payEraCreating;

  /// No description provided for @payEraScan.
  ///
  /// In id, this message translates to:
  /// **'Minta pelanggan scan dan bayar {amount}'**
  String payEraScan(String amount);

  /// No description provided for @payEraWaiting.
  ///
  /// In id, this message translates to:
  /// **'Menunggu pembayaran... nota tersimpan otomatis setelah lunas.'**
  String get payEraWaiting;

  /// No description provided for @payEraPaid.
  ///
  /// In id, this message translates to:
  /// **'Pembayaran QRIS diterima'**
  String get payEraPaid;

  /// No description provided for @payEraFailed.
  ///
  /// In id, this message translates to:
  /// **'Pembayaran QRIS gagal. Buat QR baru.'**
  String get payEraFailed;

  /// No description provided for @payEraExpired.
  ///
  /// In id, this message translates to:
  /// **'QR kedaluwarsa. Buat QR baru.'**
  String get payEraExpired;

  /// No description provided for @payEraCancelled.
  ///
  /// In id, this message translates to:
  /// **'Pembayaran dibatalkan. Buat QR baru.'**
  String get payEraCancelled;

  /// No description provided for @payEraNeedPaid.
  ///
  /// In id, this message translates to:
  /// **'Buat QR dan tunggu pembayaran diterima.'**
  String get payEraNeedPaid;

  /// No description provided for @payEraForceShow.
  ///
  /// In id, this message translates to:
  /// **'EraPay tidak dapat dihubungi? Simpan paksa'**
  String get payEraForceShow;

  /// No description provided for @payEraForceHint.
  ///
  /// In id, this message translates to:
  /// **'Simpan paksa: nota disimpan & ditandai \"perlu dicek di EraPay\", lalu dicek otomatis. Bila ternyata tidak lunas, nota dibatalkan & stok kembali. Butuh persetujuan Owner/Supervisor.'**
  String get payEraForceHint;

  /// No description provided for @payEraPendingTitle.
  ///
  /// In id, this message translates to:
  /// **'Pembayaran QRIS belum terverifikasi'**
  String get payEraPendingTitle;

  /// No description provided for @payEraPendingNotice.
  ///
  /// In id, this message translates to:
  /// **'Nota ditandai \"perlu dicek di EraPay\" & dicek otomatis. Bila ternyata tidak lunas, nota dibatalkan otomatis & stok dikembalikan.'**
  String get payEraPendingNotice;

  /// No description provided for @errorEraDisabled.
  ///
  /// In id, this message translates to:
  /// **'Integrasi EraPay belum diaktifkan di server.'**
  String get errorEraDisabled;

  /// No description provided for @errorEraNotConfigured.
  ///
  /// In id, this message translates to:
  /// **'API key EraPay belum dipasang untuk toko ini. Minta pemilik memasangnya di Metode Pembayaran (web).'**
  String get errorEraNotConfigured;

  /// No description provided for @errorEraRejected.
  ///
  /// In id, this message translates to:
  /// **'EraPay menolak tagihan QRIS ini.'**
  String get errorEraRejected;

  /// No description provided for @errorEraUnavailable.
  ///
  /// In id, this message translates to:
  /// **'EraPay tidak dapat dihubungi, coba lagi.'**
  String get errorEraUnavailable;

  /// No description provided for @errorEraNotPaid.
  ///
  /// In id, this message translates to:
  /// **'Pembayaran QRIS belum lunas, tidak cocok, atau sudah dipakai nota lain.'**
  String get errorEraNotPaid;

  /// No description provided for @payTotalDue.
  ///
  /// In id, this message translates to:
  /// **'Total tagihan'**
  String get payTotalDue;

  /// No description provided for @payPaid.
  ///
  /// In id, this message translates to:
  /// **'Dibayar'**
  String get payPaid;

  /// No description provided for @payChange.
  ///
  /// In id, this message translates to:
  /// **'Kembalian'**
  String get payChange;

  /// No description provided for @payShortBy.
  ///
  /// In id, this message translates to:
  /// **'Kurang {amount}'**
  String payShortBy(String amount);

  /// No description provided for @payItemsTitle.
  ///
  /// In id, this message translates to:
  /// **'Barang yang dibeli'**
  String get payItemsTitle;

  /// No description provided for @paySubmit.
  ///
  /// In id, this message translates to:
  /// **'Transaksi Pembayaran'**
  String get paySubmit;

  /// No description provided for @payProcessing.
  ///
  /// In id, this message translates to:
  /// **'Memproses...'**
  String get payProcessing;

  /// No description provided for @paySuccessTitle.
  ///
  /// In id, this message translates to:
  /// **'Transaksi berhasil'**
  String get paySuccessTitle;

  /// No description provided for @paySuccessDoc.
  ///
  /// In id, this message translates to:
  /// **'Nomor nota {docNo}'**
  String paySuccessDoc(String docNo);

  /// No description provided for @payNewSale.
  ///
  /// In id, this message translates to:
  /// **'Transaksi baru'**
  String get payNewSale;

  /// No description provided for @payChangeDue.
  ///
  /// In id, this message translates to:
  /// **'Kembalian {amount}'**
  String payChangeDue(String amount);

  /// No description provided for @errorShiftRequired.
  ///
  /// In id, this message translates to:
  /// **'Buka shift terlebih dahulu sebelum berjualan.'**
  String get errorShiftRequired;

  /// No description provided for @errorStockInsufficient.
  ///
  /// In id, this message translates to:
  /// **'Stok tidak mencukupi untuk salah satu barang.'**
  String get errorStockInsufficient;

  /// No description provided for @errorValidation.
  ///
  /// In id, this message translates to:
  /// **'Isian belum valid. Periksa kembali.'**
  String get errorValidation;

  /// No description provided for @errorForbidden.
  ///
  /// In id, this message translates to:
  /// **'Anda tidak memiliki izin untuk aksi ini.'**
  String get errorForbidden;

  /// No description provided for @errorIdempotencyMismatch.
  ///
  /// In id, this message translates to:
  /// **'Permintaan bentrok dengan transaksi sebelumnya. Coba lagi.'**
  String get errorIdempotencyMismatch;

  /// No description provided for @errorMethodInactive.
  ///
  /// In id, this message translates to:
  /// **'Metode pembayaran tidak aktif.'**
  String get errorMethodInactive;

  /// No description provided for @errorEditWindow.
  ///
  /// In id, this message translates to:
  /// **'Di luar batas waktu edit.'**
  String get errorEditWindow;

  /// No description provided for @serverTitle.
  ///
  /// In id, this message translates to:
  /// **'Alamat server'**
  String get serverTitle;

  /// No description provided for @serverHelp.
  ///
  /// In id, this message translates to:
  /// **'Pilih server ARUS. Untuk uji coba di HP fisik, ketik IP komputer di jaringan yang sama, mis. http://192.168.1.10:8080.'**
  String get serverHelp;

  /// No description provided for @serverLabel.
  ///
  /// In id, this message translates to:
  /// **'Alamat server'**
  String get serverLabel;

  /// No description provided for @serverInvalid.
  ///
  /// In id, this message translates to:
  /// **'Harus diawali http:// atau https:// dan berisi alamat.'**
  String get serverInvalid;

  /// No description provided for @save.
  ///
  /// In id, this message translates to:
  /// **'Simpan'**
  String get save;

  /// No description provided for @posCartEmptyTitle.
  ///
  /// In id, this message translates to:
  /// **'Keranjang masih kosong'**
  String get posCartEmptyTitle;

  /// No description provided for @posCartEmptyHint.
  ///
  /// In id, this message translates to:
  /// **'Pilih barang untuk mulai berjualan'**
  String get posCartEmptyHint;

  /// No description provided for @languageTooltip.
  ///
  /// In id, this message translates to:
  /// **'Bahasa'**
  String get languageTooltip;

  /// No description provided for @languageSystem.
  ///
  /// In id, this message translates to:
  /// **'Ikuti HP'**
  String get languageSystem;

  /// No description provided for @outletSwitch.
  ///
  /// In id, this message translates to:
  /// **'Pindah cabang'**
  String get outletSwitch;

  /// No description provided for @outletSwitchTitle.
  ///
  /// In id, this message translates to:
  /// **'Pilih cabang'**
  String get outletSwitchTitle;

  /// No description provided for @outletSwitched.
  ///
  /// In id, this message translates to:
  /// **'Sekarang di cabang {name}'**
  String outletSwitched(String name);

  /// No description provided for @outletApprovalTitle.
  ///
  /// In id, this message translates to:
  /// **'Persetujuan pindah cabang'**
  String get outletApprovalTitle;

  /// No description provided for @outletApprovalBody.
  ///
  /// In id, this message translates to:
  /// **'Pindah dari kasir ke {name} butuh persetujuan Owner/Supervisor. Keranjang aktif akan dikosongkan.'**
  String outletApprovalBody(String name);

  /// No description provided for @outletApprovalNone.
  ///
  /// In id, this message translates to:
  /// **'Belum ada penyetuju di cabang tujuan.'**
  String get outletApprovalNone;

  /// No description provided for @outletApprover.
  ///
  /// In id, this message translates to:
  /// **'Penyetuju'**
  String get outletApprover;

  /// No description provided for @outletPin.
  ///
  /// In id, this message translates to:
  /// **'PIN penyetuju (6 digit)'**
  String get outletPin;

  /// No description provided for @outletApprovalConfirm.
  ///
  /// In id, this message translates to:
  /// **'Setujui & pindah'**
  String get outletApprovalConfirm;

  /// No description provided for @errorPinRequired.
  ///
  /// In id, this message translates to:
  /// **'Persetujuan Owner/Supervisor (PIN) wajib.'**
  String get errorPinRequired;

  /// No description provided for @errorInvalidPin.
  ///
  /// In id, this message translates to:
  /// **'Penyetuju atau PIN salah.'**
  String get errorInvalidPin;

  /// No description provided for @errorPinLocked.
  ///
  /// In id, this message translates to:
  /// **'PIN terkunci sementara karena terlalu banyak percobaan. Coba lagi nanti.'**
  String get errorPinLocked;

  /// No description provided for @posShiftCloseTooltip.
  ///
  /// In id, this message translates to:
  /// **'Tutup shift'**
  String get posShiftCloseTooltip;

  /// No description provided for @posSalesTodayTooltip.
  ///
  /// In id, this message translates to:
  /// **'Penjualan'**
  String get posSalesTodayTooltip;

  /// No description provided for @shiftCloseTitle.
  ///
  /// In id, this message translates to:
  /// **'Tutup shift {doc}'**
  String shiftCloseTitle(String doc);

  /// No description provided for @shiftCloseBody.
  ///
  /// In id, this message translates to:
  /// **'Hitung uang fisik di laci untuk setiap metode, lalu isi di bawah. Selisih wajib dicatat dan disetujui Owner/Supervisor.'**
  String get shiftCloseBody;

  /// No description provided for @shiftCloseOpened.
  ///
  /// In id, this message translates to:
  /// **'Dibuka {time}'**
  String shiftCloseOpened(String time);

  /// No description provided for @shiftCloseSales.
  ///
  /// In id, this message translates to:
  /// **'{count} nota, {total}'**
  String shiftCloseSales(String count, String total);

  /// No description provided for @shiftCloseVoid.
  ///
  /// In id, this message translates to:
  /// **'{count} nota batal'**
  String shiftCloseVoid(String count);

  /// No description provided for @shiftCloseReceivable.
  ///
  /// In id, this message translates to:
  /// **'Piutang {amount}'**
  String shiftCloseReceivable(String amount);

  /// No description provided for @shiftColExpected.
  ///
  /// In id, this message translates to:
  /// **'Seharusnya'**
  String get shiftColExpected;

  /// No description provided for @shiftColCounted.
  ///
  /// In id, this message translates to:
  /// **'Uang fisik (Rp)'**
  String get shiftColCounted;

  /// No description provided for @shiftColDiff.
  ///
  /// In id, this message translates to:
  /// **'Selisih'**
  String get shiftColDiff;

  /// No description provided for @shiftFillExpected.
  ///
  /// In id, this message translates to:
  /// **'Isi sesuai rekap'**
  String get shiftFillExpected;

  /// No description provided for @shiftTotalDiff.
  ///
  /// In id, this message translates to:
  /// **'Total selisih'**
  String get shiftTotalDiff;

  /// No description provided for @shiftNote.
  ///
  /// In id, this message translates to:
  /// **'Catatan selisih'**
  String get shiftNote;

  /// No description provided for @shiftNoteHint.
  ///
  /// In id, this message translates to:
  /// **'Jelaskan penyebab selisih (min. 3 huruf)'**
  String get shiftNoteHint;

  /// No description provided for @shiftNeedApproval.
  ///
  /// In id, this message translates to:
  /// **'Ada selisih: butuh persetujuan Owner/Supervisor.'**
  String get shiftNeedApproval;

  /// No description provided for @shiftApprovalNone.
  ///
  /// In id, this message translates to:
  /// **'Belum ada penyetuju di cabang ini.'**
  String get shiftApprovalNone;

  /// No description provided for @shiftCloseSubmit.
  ///
  /// In id, this message translates to:
  /// **'Tutup shift'**
  String get shiftCloseSubmit;

  /// No description provided for @shiftChangedNotice.
  ///
  /// In id, this message translates to:
  /// **'Ada transaksi baru sejak rekap ditampilkan. Rekap sudah dimuat ulang; periksa lagi hitungan Anda.'**
  String get shiftChangedNotice;

  /// No description provided for @shiftClosedTitle.
  ///
  /// In id, this message translates to:
  /// **'Shift {doc} ditutup'**
  String shiftClosedTitle(String doc);

  /// No description provided for @shiftClosedBody.
  ///
  /// In id, this message translates to:
  /// **'Rekap dibekukan dan tersimpan di server.'**
  String get shiftClosedBody;

  /// No description provided for @shiftTotalExpected.
  ///
  /// In id, this message translates to:
  /// **'Seharusnya'**
  String get shiftTotalExpected;

  /// No description provided for @shiftTotalCounted.
  ///
  /// In id, this message translates to:
  /// **'Dihitung'**
  String get shiftTotalCounted;

  /// No description provided for @shiftOpenNew.
  ///
  /// In id, this message translates to:
  /// **'Buka shift baru'**
  String get shiftOpenNew;

  /// No description provided for @shiftDone.
  ///
  /// In id, this message translates to:
  /// **'Selesai'**
  String get shiftDone;

  /// No description provided for @errorShiftRecapChanged.
  ///
  /// In id, this message translates to:
  /// **'Ada transaksi baru sejak rekap ditampilkan. Muat ulang rekap lalu hitung lagi.'**
  String get errorShiftRecapChanged;

  /// No description provided for @errorShiftDiffNote.
  ///
  /// In id, this message translates to:
  /// **'Selisih wajib disertai catatan dan persetujuan.'**
  String get errorShiftDiffNote;

  /// No description provided for @errorShiftClosed.
  ///
  /// In id, this message translates to:
  /// **'Shift ini sudah ditutup.'**
  String get errorShiftClosed;

  /// No description provided for @errorShiftAlreadyOpen.
  ///
  /// In id, this message translates to:
  /// **'Shift Anda di cabang ini sudah terbuka.'**
  String get errorShiftAlreadyOpen;

  /// No description provided for @salesTodayTitle.
  ///
  /// In id, this message translates to:
  /// **'Penjualan'**
  String get salesTodayTitle;

  /// No description provided for @salesTodaySummary.
  ///
  /// In id, this message translates to:
  /// **'{count} nota, {total}'**
  String salesTodaySummary(String count, String total);

  /// No description provided for @salesTodaySearch.
  ///
  /// In id, this message translates to:
  /// **'Cari nomor nota'**
  String get salesTodaySearch;

  /// No description provided for @salesTodayEmpty.
  ///
  /// In id, this message translates to:
  /// **'Belum ada penjualan.'**
  String get salesTodayEmpty;

  /// No description provided for @salesTodayTruncated.
  ///
  /// In id, this message translates to:
  /// **'Daftar dipotong; persempit dengan pencarian.'**
  String get salesTodayTruncated;

  /// No description provided for @saleVoidBadge.
  ///
  /// In id, this message translates to:
  /// **'BATAL'**
  String get saleVoidBadge;

  /// No description provided for @saleReturnedBadge.
  ///
  /// In id, this message translates to:
  /// **'Retur {amount}'**
  String saleReturnedBadge(String amount);

  /// No description provided for @saleCreditBadge.
  ///
  /// In id, this message translates to:
  /// **'Piutang'**
  String get saleCreditBadge;

  /// No description provided for @saleDetailCashier.
  ///
  /// In id, this message translates to:
  /// **'Kasir {name}'**
  String saleDetailCashier(String name);

  /// No description provided for @saleDetailMember.
  ///
  /// In id, this message translates to:
  /// **'Member {name}'**
  String saleDetailMember(String name);

  /// No description provided for @saleVoidReason.
  ///
  /// In id, this message translates to:
  /// **'Alasan batal: {reason}'**
  String saleVoidReason(String reason);

  /// No description provided for @saleSubtotal.
  ///
  /// In id, this message translates to:
  /// **'Subtotal'**
  String get saleSubtotal;

  /// No description provided for @saleDiscount.
  ///
  /// In id, this message translates to:
  /// **'Potongan'**
  String get saleDiscount;

  /// No description provided for @saleTax.
  ///
  /// In id, this message translates to:
  /// **'Pajak'**
  String get saleTax;

  /// No description provided for @saleOtherCost.
  ///
  /// In id, this message translates to:
  /// **'Biaya lain'**
  String get saleOtherCost;

  /// No description provided for @saleTotal.
  ///
  /// In id, this message translates to:
  /// **'Total'**
  String get saleTotal;

  /// No description provided for @salePaid.
  ///
  /// In id, this message translates to:
  /// **'Dibayar'**
  String get salePaid;

  /// No description provided for @saleChange.
  ///
  /// In id, this message translates to:
  /// **'Kembalian'**
  String get saleChange;

  /// No description provided for @saleReceivable.
  ///
  /// In id, this message translates to:
  /// **'Piutang'**
  String get saleReceivable;

  /// No description provided for @posScanTooltip.
  ///
  /// In id, this message translates to:
  /// **'Scan barcode dengan kamera'**
  String get posScanTooltip;

  /// No description provided for @scanTitle.
  ///
  /// In id, this message translates to:
  /// **'Scan barang'**
  String get scanTitle;

  /// No description provided for @scanTorch.
  ///
  /// In id, this message translates to:
  /// **'Senter'**
  String get scanTorch;

  /// No description provided for @scanSwitchCamera.
  ///
  /// In id, this message translates to:
  /// **'Ganti kamera'**
  String get scanSwitchCamera;

  /// No description provided for @scanHint.
  ///
  /// In id, this message translates to:
  /// **'Arahkan kamera ke barcode barang'**
  String get scanHint;

  /// No description provided for @scanDone.
  ///
  /// In id, this message translates to:
  /// **'Selesai'**
  String get scanDone;

  /// No description provided for @scanPermissionDenied.
  ///
  /// In id, this message translates to:
  /// **'Izin kamera ditolak. Aktifkan izin kamera untuk ARUS di pengaturan HP.'**
  String get scanPermissionDenied;

  /// No description provided for @scanCameraError.
  ///
  /// In id, this message translates to:
  /// **'Kamera tidak bisa dibuka.'**
  String get scanCameraError;

  /// No description provided for @scanAdded.
  ///
  /// In id, this message translates to:
  /// **'{name} masuk keranjang'**
  String scanAdded(String name);

  /// No description provided for @scanNotFound.
  ///
  /// In id, this message translates to:
  /// **'Kode {code} tidak ditemukan'**
  String scanNotFound(String code);

  /// No description provided for @scanAmbiguous.
  ///
  /// In id, this message translates to:
  /// **'Kode {code} cocok dengan lebih dari satu barang; cari manual'**
  String scanAmbiguous(String code);

  /// No description provided for @memberChoose.
  ///
  /// In id, this message translates to:
  /// **'Pilih member'**
  String get memberChoose;

  /// No description provided for @memberChange.
  ///
  /// In id, this message translates to:
  /// **'Ganti member'**
  String get memberChange;

  /// No description provided for @memberGeneral.
  ///
  /// In id, this message translates to:
  /// **'Pelanggan umum'**
  String get memberGeneral;

  /// No description provided for @memberSearchHint.
  ///
  /// In id, this message translates to:
  /// **'Cari nama, kode, atau telepon'**
  String get memberSearchHint;

  /// No description provided for @memberSearchEmpty.
  ///
  /// In id, this message translates to:
  /// **'Member tidak ditemukan.'**
  String get memberSearchEmpty;

  /// No description provided for @memberRemove.
  ///
  /// In id, this message translates to:
  /// **'Lepas member'**
  String get memberRemove;

  /// No description provided for @memberPoints.
  ///
  /// In id, this message translates to:
  /// **'{points} poin'**
  String memberPoints(int points);

  /// No description provided for @memberPointsAndDeposit.
  ///
  /// In id, this message translates to:
  /// **'{points} poin · deposit {deposit}'**
  String memberPointsAndDeposit(int points, String deposit);

  /// No description provided for @memberEarn.
  ///
  /// In id, this message translates to:
  /// **'+{points} poin'**
  String memberEarn(int points);

  /// No description provided for @memberRedeemTitle.
  ///
  /// In id, this message translates to:
  /// **'Tukar poin'**
  String get memberRedeemTitle;

  /// No description provided for @memberRedeemAvailable.
  ///
  /// In id, this message translates to:
  /// **'Bisa ditukar maksimal {points} poin (senilai {value})'**
  String memberRedeemAvailable(int points, String value);

  /// No description provided for @memberRedeemField.
  ///
  /// In id, this message translates to:
  /// **'Poin yang ditukar'**
  String get memberRedeemField;

  /// No description provided for @memberRedeemAll.
  ///
  /// In id, this message translates to:
  /// **'Semua'**
  String get memberRedeemAll;

  /// No description provided for @memberRedeemNone.
  ///
  /// In id, this message translates to:
  /// **'Poin member ini belum bisa ditukar untuk nota ini.'**
  String get memberRedeemNone;

  /// No description provided for @memberRedeemReset.
  ///
  /// In id, this message translates to:
  /// **'Batalkan tukar'**
  String get memberRedeemReset;

  /// No description provided for @memberRedeemDone.
  ///
  /// In id, this message translates to:
  /// **'Terapkan'**
  String get memberRedeemDone;

  /// No description provided for @memberRedeemValue.
  ///
  /// In id, this message translates to:
  /// **'Potongan {amount}'**
  String memberRedeemValue(String amount);

  /// No description provided for @errorRedeemInvalid.
  ///
  /// In id, this message translates to:
  /// **'Poin yang ditukar melebihi batas. Kurangi jumlahnya.'**
  String get errorRedeemInvalid;

  /// No description provided for @payDepositOver.
  ///
  /// In id, this message translates to:
  /// **'Deposit melebihi saldo ({amount})'**
  String payDepositOver(String amount);

  /// No description provided for @adjustTitle.
  ///
  /// In id, this message translates to:
  /// **'Ubah harga / potongan'**
  String get adjustTitle;

  /// No description provided for @adjustBody.
  ///
  /// In id, this message translates to:
  /// **'Butuh persetujuan penyetuju (Owner/Supervisor) dengan PIN-nya.'**
  String get adjustBody;

  /// No description provided for @adjustItem.
  ///
  /// In id, this message translates to:
  /// **'Barang'**
  String get adjustItem;

  /// No description provided for @adjustListPrice.
  ///
  /// In id, this message translates to:
  /// **'Harga normal'**
  String get adjustListPrice;

  /// No description provided for @adjustTabPrice.
  ///
  /// In id, this message translates to:
  /// **'Ubah harga'**
  String get adjustTabPrice;

  /// No description provided for @adjustTabDiscount.
  ///
  /// In id, this message translates to:
  /// **'Potongan'**
  String get adjustTabDiscount;

  /// No description provided for @adjustNewPrice.
  ///
  /// In id, this message translates to:
  /// **'Harga baru per satuan (Rp)'**
  String get adjustNewPrice;

  /// No description provided for @adjustDiscountRp.
  ///
  /// In id, this message translates to:
  /// **'Potongan (Rp)'**
  String get adjustDiscountRp;

  /// No description provided for @adjustDiscountPct.
  ///
  /// In id, this message translates to:
  /// **'Potongan (%)'**
  String get adjustDiscountPct;

  /// No description provided for @adjustUnitTotal.
  ///
  /// In id, this message translates to:
  /// **'Total baris'**
  String get adjustUnitTotal;

  /// No description provided for @adjustUnitPerUnit.
  ///
  /// In id, this message translates to:
  /// **'Per satuan'**
  String get adjustUnitPerUnit;

  /// No description provided for @adjustPercentPreview.
  ///
  /// In id, this message translates to:
  /// **'Potongan {amount} per satuan'**
  String adjustPercentPreview(String amount);

  /// No description provided for @adjustChecking.
  ///
  /// In id, this message translates to:
  /// **'Memeriksa PIN...'**
  String get adjustChecking;

  /// No description provided for @adjustApply.
  ///
  /// In id, this message translates to:
  /// **'Terapkan'**
  String get adjustApply;

  /// No description provided for @adjustReset.
  ///
  /// In id, this message translates to:
  /// **'Kembalikan ke harga normal'**
  String get adjustReset;

  /// No description provided for @adjustEditTooltip.
  ///
  /// In id, this message translates to:
  /// **'Ubah harga / potongan'**
  String get adjustEditTooltip;

  /// No description provided for @adjustBadgePrice.
  ///
  /// In id, this message translates to:
  /// **'Harga diubah'**
  String get adjustBadgePrice;

  /// No description provided for @adjustBadgeDiscount.
  ///
  /// In id, this message translates to:
  /// **'Potongan {amount}'**
  String adjustBadgeDiscount(String amount);

  /// No description provided for @adjustApprovedBy.
  ///
  /// In id, this message translates to:
  /// **'disetujui {name}'**
  String adjustApprovedBy(String name);

  /// No description provided for @paySurcharge.
  ///
  /// In id, this message translates to:
  /// **'Biaya metode bayar'**
  String get paySurcharge;

  /// No description provided for @payCharged.
  ///
  /// In id, this message translates to:
  /// **'Ditagih ke pelanggan'**
  String get payCharged;

  /// No description provided for @payFeeCustomer.
  ///
  /// In id, this message translates to:
  /// **'Biaya {rate} = {fee}, ditagihkan ke pelanggan (pelanggan bayar {amount})'**
  String payFeeCustomer(String rate, String fee, String amount);

  /// No description provided for @payFeeStore.
  ///
  /// In id, this message translates to:
  /// **'Biaya {rate} = {fee}, ditanggung toko'**
  String payFeeStore(String rate, String fee);

  /// No description provided for @paySurchargeDone.
  ///
  /// In id, this message translates to:
  /// **'Biaya metode {surcharge} · ditagih {charged}'**
  String paySurchargeDone(String surcharge, String charged);

  /// No description provided for @costsTitle.
  ///
  /// In id, this message translates to:
  /// **'Keterangan & biaya lain'**
  String get costsTitle;

  /// No description provided for @costsHint.
  ///
  /// In id, this message translates to:
  /// **'Biaya lain-lain dengan rincian (maks 20), misalnya ongkir atau packing.'**
  String get costsHint;

  /// No description provided for @costsName.
  ///
  /// In id, this message translates to:
  /// **'Nama biaya'**
  String get costsName;

  /// No description provided for @costsAmount.
  ///
  /// In id, this message translates to:
  /// **'Jumlah (Rp)'**
  String get costsAmount;

  /// No description provided for @costsRemove.
  ///
  /// In id, this message translates to:
  /// **'Hapus'**
  String get costsRemove;

  /// No description provided for @costsAdd.
  ///
  /// In id, this message translates to:
  /// **'Tambah biaya'**
  String get costsAdd;

  /// No description provided for @costsTotal.
  ///
  /// In id, this message translates to:
  /// **'Total biaya lain'**
  String get costsTotal;

  /// No description provided for @costsDone.
  ///
  /// In id, this message translates to:
  /// **'Terapkan'**
  String get costsDone;

  /// No description provided for @costsClear.
  ///
  /// In id, this message translates to:
  /// **'Hapus semua'**
  String get costsClear;

  /// No description provided for @posCostsButton.
  ///
  /// In id, this message translates to:
  /// **'Keterangan & biaya lain'**
  String get posCostsButton;

  /// No description provided for @posTaxStore.
  ///
  /// In id, this message translates to:
  /// **'Pajak toko ({pct}%)'**
  String posTaxStore(String pct);

  /// No description provided for @posTaxGov.
  ///
  /// In id, this message translates to:
  /// **'Pajak negara ({pct}%)'**
  String posTaxGov(String pct);

  /// No description provided for @posCostUnnamed.
  ///
  /// In id, this message translates to:
  /// **'Biaya lain'**
  String get posCostUnnamed;

  /// No description provided for @payModePay.
  ///
  /// In id, this message translates to:
  /// **'Bayar'**
  String get payModePay;

  /// No description provided for @payModeCredit.
  ///
  /// In id, this message translates to:
  /// **'Kredit'**
  String get payModeCredit;

  /// No description provided for @payCreditNoMember.
  ///
  /// In id, this message translates to:
  /// **'Kredit hanya untuk member. Pilih member di keranjang dulu.'**
  String get payCreditNoMember;

  /// No description provided for @payCreditNotNeeded.
  ///
  /// In id, this message translates to:
  /// **'Uang muka sudah menutup total nota; gunakan mode Bayar.'**
  String get payCreditNotNeeded;

  /// No description provided for @payCreditDpAdd.
  ///
  /// In id, this message translates to:
  /// **'Tambah uang muka (DP)'**
  String get payCreditDpAdd;

  /// No description provided for @payCreditReceivable.
  ///
  /// In id, this message translates to:
  /// **'Piutang (sisa belum dibayar)'**
  String get payCreditReceivable;

  /// No description provided for @payCreditDue.
  ///
  /// In id, this message translates to:
  /// **'Jatuh tempo'**
  String get payCreditDue;

  /// No description provided for @payCreditDueDays.
  ///
  /// In id, this message translates to:
  /// **'{days} hari setelah nota'**
  String payCreditDueDays(int days);

  /// No description provided for @payCreditNoDue.
  ///
  /// In id, this message translates to:
  /// **'Tanpa jatuh tempo'**
  String get payCreditNoDue;

  /// No description provided for @payCreditLimit.
  ///
  /// In id, this message translates to:
  /// **'Limit kredit'**
  String get payCreditLimit;

  /// No description provided for @payCreditNoLimit.
  ///
  /// In id, this message translates to:
  /// **'Tanpa batas'**
  String get payCreditNoLimit;

  /// No description provided for @payCreditOutstanding.
  ///
  /// In id, this message translates to:
  /// **'Piutang saat ini'**
  String get payCreditOutstanding;

  /// No description provided for @payCreditAfter.
  ///
  /// In id, this message translates to:
  /// **'Piutang setelah nota ini'**
  String get payCreditAfter;

  /// No description provided for @payCreditOverLimit.
  ///
  /// In id, this message translates to:
  /// **'Melewati limit kredit member. Butuh persetujuan penyetuju dengan PIN-nya.'**
  String get payCreditOverLimit;

  /// No description provided for @payCreditSameApprover.
  ///
  /// In id, this message translates to:
  /// **'Penyetuju yang sama (ubah harga/potongan) dipakai untuk limit kredit.'**
  String get payCreditSameApprover;

  /// No description provided for @errorCreditLimit.
  ///
  /// In id, this message translates to:
  /// **'Piutang member melewati limit kredit. Butuh persetujuan penyetuju (PIN).'**
  String get errorCreditLimit;

  /// No description provided for @paySuccessReceivable.
  ///
  /// In id, this message translates to:
  /// **'Piutang {amount}'**
  String paySuccessReceivable(String amount);

  /// No description provided for @paySuccessDue.
  ///
  /// In id, this message translates to:
  /// **'Jatuh tempo {date}'**
  String paySuccessDue(String date);

  /// No description provided for @pendingHoldTitle.
  ///
  /// In id, this message translates to:
  /// **'Tunda nota'**
  String get pendingHoldTitle;

  /// No description provided for @pendingLabel.
  ///
  /// In id, this message translates to:
  /// **'Keterangan (opsional)'**
  String get pendingLabel;

  /// No description provided for @pendingLabelHint.
  ///
  /// In id, this message translates to:
  /// **'Nama pelanggan atau ciri-cirinya'**
  String get pendingLabelHint;

  /// No description provided for @pendingHold.
  ///
  /// In id, this message translates to:
  /// **'Tunda'**
  String get pendingHold;

  /// No description provided for @pendingHoldTooltip.
  ///
  /// In id, this message translates to:
  /// **'Tunda nota'**
  String get pendingHoldTooltip;

  /// No description provided for @pendingListTooltip.
  ///
  /// In id, this message translates to:
  /// **'Nota pending'**
  String get pendingListTooltip;

  /// No description provided for @pendingFull.
  ///
  /// In id, this message translates to:
  /// **'Nota pending penuh (maks {max}). Buka atau hapus salah satu dulu.'**
  String pendingFull(int max);

  /// No description provided for @pendingSaved.
  ///
  /// In id, this message translates to:
  /// **'Nota ditunda sebagai Pending {no}'**
  String pendingSaved(int no);

  /// No description provided for @pendingOpened.
  ///
  /// In id, this message translates to:
  /// **'Pending {no} dibuka'**
  String pendingOpened(int no);

  /// No description provided for @pendingOpenedSwapped.
  ///
  /// In id, this message translates to:
  /// **'Pending {no} dibuka; isi sebelumnya disimpan sebagai Pending {saved}'**
  String pendingOpenedSwapped(int no, int saved);

  /// No description provided for @pendingTitle.
  ///
  /// In id, this message translates to:
  /// **'Nota pending ({count}/{max})'**
  String pendingTitle(int count, int max);

  /// No description provided for @pendingHint.
  ///
  /// In id, this message translates to:
  /// **'Tersimpan di HP ini, kedaluwarsa 24 jam. Harga dan stok dihitung ulang saat dibuka.'**
  String get pendingHint;

  /// No description provided for @pendingEmpty.
  ///
  /// In id, this message translates to:
  /// **'Belum ada nota pending.'**
  String get pendingEmpty;

  /// No description provided for @pendingNo.
  ///
  /// In id, this message translates to:
  /// **'Pending {no}'**
  String pendingNo(int no);

  /// No description provided for @pendingLines.
  ///
  /// In id, this message translates to:
  /// **'{count} baris'**
  String pendingLines(int count);

  /// No description provided for @pendingDelete.
  ///
  /// In id, this message translates to:
  /// **'Hapus'**
  String get pendingDelete;

  /// No description provided for @pendingDeleteAsk.
  ///
  /// In id, this message translates to:
  /// **'Hapus Pending {no}?'**
  String pendingDeleteAsk(int no);

  /// No description provided for @shortcutsEmpty.
  ///
  /// In id, this message translates to:
  /// **'Belum ada pintasan. Ketuk ikon di kanan untuk mengatur.'**
  String get shortcutsEmpty;

  /// No description provided for @shortcutsManage.
  ///
  /// In id, this message translates to:
  /// **'Atur pintasan barang'**
  String get shortcutsManage;

  /// No description provided for @shortcutsTitle.
  ///
  /// In id, this message translates to:
  /// **'Pintasan barang'**
  String get shortcutsTitle;

  /// No description provided for @shortcutsHint.
  ///
  /// In id, this message translates to:
  /// **'16 slot milik Anda, sama dengan kasir web. Ketuk slot untuk memasang barang.'**
  String get shortcutsHint;

  /// No description provided for @shortcutsSlotEmpty.
  ///
  /// In id, this message translates to:
  /// **'Kosong — ketuk untuk memasang barang'**
  String get shortcutsSlotEmpty;

  /// No description provided for @shortcutsInactive.
  ///
  /// In id, this message translates to:
  /// **'Barang nonaktif'**
  String get shortcutsInactive;

  /// No description provided for @shortcutsClear.
  ///
  /// In id, this message translates to:
  /// **'Kosongkan slot'**
  String get shortcutsClear;

  /// No description provided for @salespersonLabel.
  ///
  /// In id, this message translates to:
  /// **'Salesman'**
  String get salespersonLabel;

  /// No description provided for @salespersonNone.
  ///
  /// In id, this message translates to:
  /// **'Umum (tanpa salesman)'**
  String get salespersonNone;

  /// No description provided for @salespersonSearch.
  ///
  /// In id, this message translates to:
  /// **'Cari salesman'**
  String get salespersonSearch;

  /// No description provided for @salespersonEmpty.
  ///
  /// In id, this message translates to:
  /// **'Salesman tidak ditemukan.'**
  String get salespersonEmpty;

  /// No description provided for @payQuickAmounts.
  ///
  /// In id, this message translates to:
  /// **'Uang diterima'**
  String get payQuickAmounts;

  /// No description provided for @categoryAll.
  ///
  /// In id, this message translates to:
  /// **'Semua'**
  String get categoryAll;

  /// No description provided for @noteLabel.
  ///
  /// In id, this message translates to:
  /// **'Keterangan nota'**
  String get noteLabel;

  /// No description provided for @noteHint.
  ///
  /// In id, this message translates to:
  /// **'Mis. nama pelanggan, titipan, alamat kirim'**
  String get noteHint;

  /// No description provided for @homeGreetMorning.
  ///
  /// In id, this message translates to:
  /// **'Selamat pagi, {name}'**
  String homeGreetMorning(String name);

  /// No description provided for @homeGreetNoon.
  ///
  /// In id, this message translates to:
  /// **'Selamat siang, {name}'**
  String homeGreetNoon(String name);

  /// No description provided for @homeGreetAfternoon.
  ///
  /// In id, this message translates to:
  /// **'Selamat sore, {name}'**
  String homeGreetAfternoon(String name);

  /// No description provided for @homeGreetNight.
  ///
  /// In id, this message translates to:
  /// **'Selamat malam, {name}'**
  String homeGreetNight(String name);

  /// No description provided for @homeTodayTitle.
  ///
  /// In id, this message translates to:
  /// **'Penjualan hari ini'**
  String get homeTodayTitle;

  /// No description provided for @homeTodayNotes.
  ///
  /// In id, this message translates to:
  /// **'Nota'**
  String get homeTodayNotes;

  /// No description provided for @homeShiftLabel.
  ///
  /// In id, this message translates to:
  /// **'Shift'**
  String get homeShiftLabel;

  /// No description provided for @homeStallHint.
  ///
  /// In id, this message translates to:
  /// **'Ketuk untuk mulai berjualan'**
  String get homeStallHint;

  /// No description provided for @pettyMenu.
  ///
  /// In id, this message translates to:
  /// **'Kas kecil'**
  String get pettyMenu;

  /// No description provided for @pettyTitle.
  ///
  /// In id, this message translates to:
  /// **'Kas kecil kasir'**
  String get pettyTitle;

  /// No description provided for @pettyBody.
  ///
  /// In id, this message translates to:
  /// **'Catat uang yang keluar dari laci di luar penjualan (beli galon, parkir, dll.) atau uang yang ditambahkan ke laci. Ikut dihitung saat tutup shift.'**
  String get pettyBody;

  /// No description provided for @pettyOut.
  ///
  /// In id, this message translates to:
  /// **'Kas keluar'**
  String get pettyOut;

  /// No description provided for @pettyIn.
  ///
  /// In id, this message translates to:
  /// **'Kas masuk'**
  String get pettyIn;

  /// No description provided for @pettyAmount.
  ///
  /// In id, this message translates to:
  /// **'Jumlah'**
  String get pettyAmount;

  /// No description provided for @pettyCategory.
  ///
  /// In id, this message translates to:
  /// **'Kategori'**
  String get pettyCategory;

  /// No description provided for @pettyNote.
  ///
  /// In id, this message translates to:
  /// **'Keterangan'**
  String get pettyNote;

  /// No description provided for @pettySave.
  ///
  /// In id, this message translates to:
  /// **'Simpan'**
  String get pettySave;

  /// No description provided for @pettySaved.
  ///
  /// In id, this message translates to:
  /// **'Kas kecil dicatat.'**
  String get pettySaved;

  /// No description provided for @pettyHistory.
  ///
  /// In id, this message translates to:
  /// **'Catatan shift ini'**
  String get pettyHistory;

  /// No description provided for @pettyEmpty.
  ///
  /// In id, this message translates to:
  /// **'Belum ada kas kecil di shift ini.'**
  String get pettyEmpty;

  /// No description provided for @pettyVoid.
  ///
  /// In id, this message translates to:
  /// **'Batalkan'**
  String get pettyVoid;

  /// No description provided for @pettyVoided.
  ///
  /// In id, this message translates to:
  /// **'Dibatalkan'**
  String get pettyVoided;

  /// No description provided for @pettyVoidConfirm.
  ///
  /// In id, this message translates to:
  /// **'Batalkan catatan ini? Uang laci yang seharusnya dihitung ulang.'**
  String get pettyVoidConfirm;

  /// No description provided for @pettyApprovedBy.
  ///
  /// In id, this message translates to:
  /// **'Disetujui {name}'**
  String pettyApprovedBy(String name);

  /// No description provided for @pettyNeedApproval.
  ///
  /// In id, this message translates to:
  /// **'Kas keluar di atas {limit} butuh persetujuan Owner/Supervisor (PIN).'**
  String pettyNeedApproval(String limit);

  /// No description provided for @pettyCatOperational.
  ///
  /// In id, this message translates to:
  /// **'Operasional'**
  String get pettyCatOperational;

  /// No description provided for @pettyCatMeal.
  ///
  /// In id, this message translates to:
  /// **'Konsumsi'**
  String get pettyCatMeal;

  /// No description provided for @pettyCatTransport.
  ///
  /// In id, this message translates to:
  /// **'Parkir/Transport'**
  String get pettyCatTransport;

  /// No description provided for @pettyCatSupplies.
  ///
  /// In id, this message translates to:
  /// **'Perlengkapan'**
  String get pettyCatSupplies;

  /// No description provided for @pettyCatChange.
  ///
  /// In id, this message translates to:
  /// **'Tambah receh'**
  String get pettyCatChange;

  /// No description provided for @pettyCatMisc.
  ///
  /// In id, this message translates to:
  /// **'Lain-lain'**
  String get pettyCatMisc;

  /// No description provided for @errorPettyVoided.
  ///
  /// In id, this message translates to:
  /// **'Catatan kas kecil ini sudah dibatalkan.'**
  String get errorPettyVoided;

  /// No description provided for @errorNoCashMethod.
  ///
  /// In id, this message translates to:
  /// **'Metode pembayaran tunai belum tersedia.'**
  String get errorNoCashMethod;

  /// No description provided for @homePettyHint.
  ///
  /// In id, this message translates to:
  /// **'Catat uang keluar/masuk laci'**
  String get homePettyHint;

  /// No description provided for @updateTitle.
  ///
  /// In id, this message translates to:
  /// **'Pembaruan tersedia'**
  String get updateTitle;

  /// No description provided for @updateVersions.
  ///
  /// In id, this message translates to:
  /// **'Versi terpasang {current} → versi baru {latest}'**
  String updateVersions(String current, String latest);

  /// No description provided for @updateBody.
  ///
  /// In id, this message translates to:
  /// **'Perbarui ARUS ke versi terbaru untuk melanjutkan.'**
  String get updateBody;

  /// No description provided for @updateNow.
  ///
  /// In id, this message translates to:
  /// **'Perbarui sekarang'**
  String get updateNow;

  /// No description provided for @updCheckTitle.
  ///
  /// In id, this message translates to:
  /// **'Cek pembaruan'**
  String get updCheckTitle;

  /// No description provided for @updChecking.
  ///
  /// In id, this message translates to:
  /// **'Memeriksa pembaruan…'**
  String get updChecking;

  /// No description provided for @updUpToDate.
  ///
  /// In id, this message translates to:
  /// **'Sudah versi terbaru'**
  String get updUpToDate;

  /// No description provided for @updUpToDateBody.
  ///
  /// In id, this message translates to:
  /// **'ARUS yang Anda pakai sudah versi paling baru.'**
  String get updUpToDateBody;

  /// No description provided for @updFailedTitle.
  ///
  /// In id, this message translates to:
  /// **'Tidak bisa memeriksa'**
  String get updFailedTitle;

  /// No description provided for @updPlayUnavailable.
  ///
  /// In id, this message translates to:
  /// **'Tidak bisa terhubung ke Google Play. Periksa koneksi lalu coba lagi.'**
  String get updPlayUnavailable;

  /// No description provided for @updPlayBody.
  ///
  /// In id, this message translates to:
  /// **'Versi terbaru ARUS tersedia di Google Play. Perbarui untuk mendapat perbaikan dan fitur baru.'**
  String get updPlayBody;

  /// No description provided for @updForcedBody.
  ///
  /// In id, this message translates to:
  /// **'Pembaruan ini wajib dipasang untuk melanjutkan memakai ARUS.'**
  String get updForcedBody;

  /// No description provided for @updLater.
  ///
  /// In id, this message translates to:
  /// **'Nanti'**
  String get updLater;

  /// No description provided for @updReadyTitle.
  ///
  /// In id, this message translates to:
  /// **'Pembaruan siap dipasang'**
  String get updReadyTitle;

  /// No description provided for @updReadyBody.
  ///
  /// In id, this message translates to:
  /// **'Pembaruan sudah diunduh. Mulai ulang aplikasi untuk memasangnya.'**
  String get updReadyBody;

  /// No description provided for @updRestart.
  ///
  /// In id, this message translates to:
  /// **'Mulai ulang & pasang'**
  String get updRestart;

  /// No description provided for @updAvailableBadge.
  ///
  /// In id, this message translates to:
  /// **'Versi baru tersedia'**
  String get updAvailableBadge;

  /// No description provided for @updateRetry.
  ///
  /// In id, this message translates to:
  /// **'Coba lagi'**
  String get updateRetry;

  /// No description provided for @updateDownloading.
  ///
  /// In id, this message translates to:
  /// **'Mengunduh pembaruan…'**
  String get updateDownloading;

  /// No description provided for @updateInstalling.
  ///
  /// In id, this message translates to:
  /// **'Membuka pemasang…'**
  String get updateInstalling;

  /// No description provided for @updatePermission.
  ///
  /// In id, this message translates to:
  /// **'Izinkan ARUS memasang aplikasi pada layar yang terbuka, lalu kembali ke sini dan ketuk Perbarui sekarang.'**
  String get updatePermission;

  /// No description provided for @updateFailed.
  ///
  /// In id, this message translates to:
  /// **'Pembaruan gagal diunduh. Periksa koneksi lalu coba lagi.'**
  String get updateFailed;

  /// No description provided for @appVersion.
  ///
  /// In id, this message translates to:
  /// **'Versi {version}'**
  String appVersion(String version);

  /// No description provided for @serverPresetProd.
  ///
  /// In id, this message translates to:
  /// **'Produksi'**
  String get serverPresetProd;

  /// No description provided for @serverPresetEmulator.
  ///
  /// In id, this message translates to:
  /// **'Emulator'**
  String get serverPresetEmulator;

  /// No description provided for @serverPresetCustom.
  ///
  /// In id, this message translates to:
  /// **'Atau ketik alamat lain:'**
  String get serverPresetCustom;

  /// No description provided for @posPanelVersion.
  ///
  /// In id, this message translates to:
  /// **'Versi aplikasi'**
  String get posPanelVersion;

  /// No description provided for @printerMenu.
  ///
  /// In id, this message translates to:
  /// **'Printer struk'**
  String get printerMenu;

  /// No description provided for @printerTitle.
  ///
  /// In id, this message translates to:
  /// **'Printer struk (Bluetooth)'**
  String get printerTitle;

  /// No description provided for @printerHint.
  ///
  /// In id, this message translates to:
  /// **'Pasangkan printer lebih dulu di Pengaturan Android > Bluetooth, lalu pilih di sini. Pengaturan ini berlaku untuk HP ini saja.'**
  String get printerHint;

  /// No description provided for @printerRefresh.
  ///
  /// In id, this message translates to:
  /// **'Muat ulang daftar'**
  String get printerRefresh;

  /// No description provided for @printerNonePaired.
  ///
  /// In id, this message translates to:
  /// **'Belum ada perangkat Bluetooth yang dipasangkan. Pasangkan printer di Pengaturan Android, lalu muat ulang.'**
  String get printerNonePaired;

  /// No description provided for @printerPaper.
  ///
  /// In id, this message translates to:
  /// **'Lebar kertas'**
  String get printerPaper;

  /// No description provided for @printerPaper58.
  ///
  /// In id, this message translates to:
  /// **'58 mm'**
  String get printerPaper58;

  /// No description provided for @printerPaper80.
  ///
  /// In id, this message translates to:
  /// **'80 mm'**
  String get printerPaper80;

  /// No description provided for @printerCut.
  ///
  /// In id, this message translates to:
  /// **'Potong kertas otomatis'**
  String get printerCut;

  /// No description provided for @printerAuto.
  ///
  /// In id, this message translates to:
  /// **'Cetak struk otomatis setelah bayar'**
  String get printerAuto;

  /// No description provided for @printerTest.
  ///
  /// In id, this message translates to:
  /// **'Tes printer'**
  String get printerTest;

  /// No description provided for @printerTestSent.
  ///
  /// In id, this message translates to:
  /// **'Halaman uji terkirim.'**
  String get printerTestSent;

  /// No description provided for @printerNotConfigured.
  ///
  /// In id, this message translates to:
  /// **'Printer belum dipilih. Pilih printer Bluetooth dulu.'**
  String get printerNotConfigured;

  /// No description provided for @printerPermissionDenied.
  ///
  /// In id, this message translates to:
  /// **'Izin Bluetooth ditolak. Izinkan ARUS memakai Bluetooth di pengaturan aplikasi.'**
  String get printerPermissionDenied;

  /// No description provided for @printerBluetoothOff.
  ///
  /// In id, this message translates to:
  /// **'Bluetooth mati. Nyalakan Bluetooth lalu coba lagi.'**
  String get printerBluetoothOff;

  /// No description provided for @printerConnectFailed.
  ///
  /// In id, this message translates to:
  /// **'Tidak dapat terhubung ke printer. Pastikan printer menyala dan berada di dekat HP.'**
  String get printerConnectFailed;

  /// No description provided for @printerWriteFailed.
  ///
  /// In id, this message translates to:
  /// **'Data gagal dikirim ke printer. Periksa printer lalu coba lagi.'**
  String get printerWriteFailed;

  /// No description provided for @printReceipt.
  ///
  /// In id, this message translates to:
  /// **'Cetak struk'**
  String get printReceipt;

  /// No description provided for @printReprint.
  ///
  /// In id, this message translates to:
  /// **'Cetak ulang'**
  String get printReprint;

  /// No description provided for @printPrinting.
  ///
  /// In id, this message translates to:
  /// **'Mencetak…'**
  String get printPrinting;

  /// No description provided for @printFailed.
  ///
  /// In id, this message translates to:
  /// **'Struk gagal dicetak: {error}'**
  String printFailed(String error);

  /// No description provided for @receiptNo.
  ///
  /// In id, this message translates to:
  /// **'No'**
  String get receiptNo;

  /// No description provided for @receiptDate.
  ///
  /// In id, this message translates to:
  /// **'Tgl'**
  String get receiptDate;

  /// No description provided for @receiptCashier.
  ///
  /// In id, this message translates to:
  /// **'Kasir'**
  String get receiptCashier;

  /// No description provided for @receiptMember.
  ///
  /// In id, this message translates to:
  /// **'Member'**
  String get receiptMember;

  /// No description provided for @receiptSalesperson.
  ///
  /// In id, this message translates to:
  /// **'Sales'**
  String get receiptSalesperson;

  /// No description provided for @receiptCopy.
  ///
  /// In id, this message translates to:
  /// **'*** SALINAN KE-{n} ***'**
  String receiptCopy(int n);

  /// No description provided for @receiptVoid.
  ///
  /// In id, this message translates to:
  /// **'*** NOTA DIBATALKAN ***'**
  String get receiptVoid;

  /// No description provided for @receiptSuperseded.
  ///
  /// In id, this message translates to:
  /// **'*** DIGANTI REVISI ***'**
  String get receiptSuperseded;

  /// No description provided for @receiptEraPending.
  ///
  /// In id, this message translates to:
  /// **'*** PERLU DI CEK DI ERAPAY ***'**
  String get receiptEraPending;

  /// No description provided for @receiptLineDiscount.
  ///
  /// In id, this message translates to:
  /// **'Potongan'**
  String get receiptLineDiscount;

  /// No description provided for @receiptSubtotal.
  ///
  /// In id, this message translates to:
  /// **'Subtotal'**
  String get receiptSubtotal;

  /// No description provided for @receiptDiscount.
  ///
  /// In id, this message translates to:
  /// **'Diskon'**
  String get receiptDiscount;

  /// No description provided for @receiptVoucher.
  ///
  /// In id, this message translates to:
  /// **'Kupon {code}'**
  String receiptVoucher(String code);

  /// No description provided for @receiptRedeem.
  ///
  /// In id, this message translates to:
  /// **'Tukar {points} poin'**
  String receiptRedeem(int points);

  /// No description provided for @receiptTaxStore.
  ///
  /// In id, this message translates to:
  /// **'Pajak toko {pct}%'**
  String receiptTaxStore(String pct);

  /// No description provided for @receiptTaxGov.
  ///
  /// In id, this message translates to:
  /// **'Pajak {pct}%'**
  String receiptTaxGov(String pct);

  /// No description provided for @receiptOtherCost.
  ///
  /// In id, this message translates to:
  /// **'Biaya lain'**
  String get receiptOtherCost;

  /// No description provided for @receiptTotal.
  ///
  /// In id, this message translates to:
  /// **'TOTAL'**
  String get receiptTotal;

  /// No description provided for @receiptSurcharge.
  ///
  /// In id, this message translates to:
  /// **'Biaya metode'**
  String get receiptSurcharge;

  /// No description provided for @receiptCharged.
  ///
  /// In id, this message translates to:
  /// **'Total ditagih'**
  String get receiptCharged;

  /// No description provided for @receiptReceivable.
  ///
  /// In id, this message translates to:
  /// **'Piutang'**
  String get receiptReceivable;

  /// No description provided for @receiptDue.
  ///
  /// In id, this message translates to:
  /// **'Jatuh tempo'**
  String get receiptDue;

  /// No description provided for @receiptChange.
  ///
  /// In id, this message translates to:
  /// **'Kembali'**
  String get receiptChange;

  /// No description provided for @receiptSaved.
  ///
  /// In id, this message translates to:
  /// **'Anda hemat'**
  String get receiptSaved;

  /// No description provided for @receiptPointsEarned.
  ///
  /// In id, this message translates to:
  /// **'Poin didapat'**
  String get receiptPointsEarned;

  /// No description provided for @receiptTestTitle.
  ///
  /// In id, this message translates to:
  /// **'Halaman uji printer'**
  String get receiptTestTitle;

  /// No description provided for @salesFrom.
  ///
  /// In id, this message translates to:
  /// **'Dari'**
  String get salesFrom;

  /// No description provided for @salesTo.
  ///
  /// In id, this message translates to:
  /// **'Sampai'**
  String get salesTo;

  /// No description provided for @salesPresetToday.
  ///
  /// In id, this message translates to:
  /// **'Hari ini'**
  String get salesPresetToday;

  /// No description provided for @salesRangeInvalid.
  ///
  /// In id, this message translates to:
  /// **'Waktu akhir harus setelah waktu awal.'**
  String get salesRangeInvalid;

  /// No description provided for @salesRangeTooLong.
  ///
  /// In id, this message translates to:
  /// **'Rentang maksimal 62 hari.'**
  String get salesRangeTooLong;

  /// No description provided for @salesPrintRecap.
  ///
  /// In id, this message translates to:
  /// **'Cetak rekap'**
  String get salesPrintRecap;

  /// No description provided for @salesMethodsTitle.
  ///
  /// In id, this message translates to:
  /// **'Laporan per metode bayar'**
  String get salesMethodsTitle;

  /// No description provided for @salesMethodNotes.
  ///
  /// In id, this message translates to:
  /// **'{count} nota'**
  String salesMethodNotes(String count);

  /// No description provided for @salesDrawerTitle.
  ///
  /// In id, this message translates to:
  /// **'Seharusnya ada (cek fisik)'**
  String get salesDrawerTitle;

  /// No description provided for @salesFlowsTitle.
  ///
  /// In id, this message translates to:
  /// **'Uang lain lewat kasir ini'**
  String get salesFlowsTitle;

  /// No description provided for @salesReturnsRow.
  ///
  /// In id, this message translates to:
  /// **'Retur ({count})'**
  String salesReturnsRow(String count);

  /// No description provided for @salesNetTotal.
  ///
  /// In id, this message translates to:
  /// **'Penjualan bersih setelah retur'**
  String get salesNetTotal;

  /// No description provided for @salesCreditRow.
  ///
  /// In id, this message translates to:
  /// **'Piutang'**
  String get salesCreditRow;

  /// No description provided for @salesSurchargeRow.
  ///
  /// In id, this message translates to:
  /// **'Biaya metode (pelanggan)'**
  String get salesSurchargeRow;

  /// No description provided for @flowReceivablePayment.
  ///
  /// In id, this message translates to:
  /// **'Bayar piutang'**
  String get flowReceivablePayment;

  /// No description provided for @flowDepositTopup.
  ///
  /// In id, this message translates to:
  /// **'Top-up deposit'**
  String get flowDepositTopup;

  /// No description provided for @flowDepositWithdraw.
  ///
  /// In id, this message translates to:
  /// **'Tarik deposit'**
  String get flowDepositWithdraw;

  /// No description provided for @flowSaleReturn.
  ///
  /// In id, this message translates to:
  /// **'Dana kembali retur jual'**
  String get flowSaleReturn;

  /// No description provided for @flowPayablePayment.
  ///
  /// In id, this message translates to:
  /// **'Bayar hutang pemasok'**
  String get flowPayablePayment;

  /// No description provided for @flowPurchaseReturn.
  ///
  /// In id, this message translates to:
  /// **'Dana kembali retur beli'**
  String get flowPurchaseReturn;

  /// No description provided for @flowSupplierCashout.
  ///
  /// In id, this message translates to:
  /// **'Pencairan kredit pemasok'**
  String get flowSupplierCashout;

  /// No description provided for @flowPettyIn.
  ///
  /// In id, this message translates to:
  /// **'Kas kecil masuk'**
  String get flowPettyIn;

  /// No description provided for @flowPettyOut.
  ///
  /// In id, this message translates to:
  /// **'Kas kecil keluar'**
  String get flowPettyOut;

  /// No description provided for @recapTitle.
  ///
  /// In id, this message translates to:
  /// **'REKAP PENJUALAN'**
  String get recapTitle;

  /// No description provided for @recapCashier.
  ///
  /// In id, this message translates to:
  /// **'Kasir'**
  String get recapCashier;

  /// No description provided for @recapPeriod.
  ///
  /// In id, this message translates to:
  /// **'Periode'**
  String get recapPeriod;

  /// No description provided for @recapCount.
  ///
  /// In id, this message translates to:
  /// **'Jumlah nota'**
  String get recapCount;

  /// No description provided for @recapVoided.
  ///
  /// In id, this message translates to:
  /// **'Nota batal'**
  String get recapVoided;

  /// No description provided for @recapRevenue.
  ///
  /// In id, this message translates to:
  /// **'OMZET'**
  String get recapRevenue;

  /// No description provided for @recapByMethod.
  ///
  /// In id, this message translates to:
  /// **'Penjualan per metode'**
  String get recapByMethod;

  /// No description provided for @itemAddTooltip.
  ///
  /// In id, this message translates to:
  /// **'Tambah barang'**
  String get itemAddTooltip;

  /// No description provided for @itemAddTitle.
  ///
  /// In id, this message translates to:
  /// **'Tambah barang baru'**
  String get itemAddTitle;

  /// No description provided for @itemName.
  ///
  /// In id, this message translates to:
  /// **'Nama barang'**
  String get itemName;

  /// No description provided for @itemSellPrice.
  ///
  /// In id, this message translates to:
  /// **'Harga jual'**
  String get itemSellPrice;

  /// No description provided for @itemCost.
  ///
  /// In id, this message translates to:
  /// **'Harga modal'**
  String get itemCost;

  /// No description provided for @itemUnit.
  ///
  /// In id, this message translates to:
  /// **'Satuan'**
  String get itemUnit;

  /// No description provided for @itemCategory.
  ///
  /// In id, this message translates to:
  /// **'Kategori (opsional)'**
  String get itemCategory;

  /// No description provided for @itemCategoryNone.
  ///
  /// In id, this message translates to:
  /// **'Tanpa kategori'**
  String get itemCategoryNone;

  /// No description provided for @itemBarcode.
  ///
  /// In id, this message translates to:
  /// **'Barcode (opsional)'**
  String get itemBarcode;

  /// No description provided for @itemKind.
  ///
  /// In id, this message translates to:
  /// **'Jenis'**
  String get itemKind;

  /// No description provided for @itemKindGoods.
  ///
  /// In id, this message translates to:
  /// **'Barang'**
  String get itemKindGoods;

  /// No description provided for @itemKindService.
  ///
  /// In id, this message translates to:
  /// **'Jasa'**
  String get itemKindService;

  /// No description provided for @itemInitialStock.
  ///
  /// In id, this message translates to:
  /// **'Stok awal (opsional)'**
  String get itemInitialStock;

  /// No description provided for @itemAllowNegative.
  ///
  /// In id, this message translates to:
  /// **'Boleh stok minus'**
  String get itemAllowNegative;

  /// No description provided for @itemAllowNegativeHint.
  ///
  /// In id, this message translates to:
  /// **'Barang tetap bisa dijual walau stok belum diisi; stok jadi minus sampai ada pembelian.'**
  String get itemAllowNegativeHint;

  /// No description provided for @itemStockHint.
  ///
  /// In id, this message translates to:
  /// **'Tanpa stok awal dan tanpa stok minus, barang belum bisa dijual sampai stok diisi.'**
  String get itemStockHint;

  /// No description provided for @itemServiceHint.
  ///
  /// In id, this message translates to:
  /// **'Jasa tidak memakai stok.'**
  String get itemServiceHint;

  /// No description provided for @itemSave.
  ///
  /// In id, this message translates to:
  /// **'Simpan'**
  String get itemSave;

  /// No description provided for @itemSaveAndSell.
  ///
  /// In id, this message translates to:
  /// **'Simpan & jual'**
  String get itemSaveAndSell;

  /// No description provided for @itemSaved.
  ///
  /// In id, this message translates to:
  /// **'{name} ditambahkan'**
  String itemSaved(String name);

  /// No description provided for @itemRequired.
  ///
  /// In id, this message translates to:
  /// **'Wajib diisi'**
  String get itemRequired;

  /// No description provided for @itemInvalidNumber.
  ///
  /// In id, this message translates to:
  /// **'Angka tidak valid'**
  String get itemInvalidNumber;

  /// No description provided for @itemNoUnits.
  ///
  /// In id, this message translates to:
  /// **'Belum ada satuan. Tambahkan satuan di web terlebih dulu.'**
  String get itemNoUnits;

  /// No description provided for @itemScanBarcode.
  ///
  /// In id, this message translates to:
  /// **'Pindai barcode'**
  String get itemScanBarcode;

  /// No description provided for @itemScanned.
  ///
  /// In id, this message translates to:
  /// **'Barcode terbaca'**
  String get itemScanned;

  /// No description provided for @itemLoadFailed.
  ///
  /// In id, this message translates to:
  /// **'Gagal memuat satuan/kategori.'**
  String get itemLoadFailed;

  /// No description provided for @reportTitle.
  ///
  /// In id, this message translates to:
  /// **'Laporan penjualan'**
  String get reportTitle;

  /// No description provided for @reportHint.
  ///
  /// In id, this message translates to:
  /// **'Omzet, metode bayar, per hari'**
  String get reportHint;

  /// No description provided for @reportToday.
  ///
  /// In id, this message translates to:
  /// **'Hari ini'**
  String get reportToday;

  /// No description provided for @report7Days.
  ///
  /// In id, this message translates to:
  /// **'7 hari'**
  String get report7Days;

  /// No description provided for @reportThisMonth.
  ///
  /// In id, this message translates to:
  /// **'Bulan ini'**
  String get reportThisMonth;

  /// No description provided for @reportLastMonth.
  ///
  /// In id, this message translates to:
  /// **'Bulan lalu'**
  String get reportLastMonth;

  /// No description provided for @reportCustom.
  ///
  /// In id, this message translates to:
  /// **'Pilih tanggal'**
  String get reportCustom;

  /// No description provided for @reportAllOutlets.
  ///
  /// In id, this message translates to:
  /// **'Semua cabang'**
  String get reportAllOutlets;

  /// No description provided for @reportRevenue.
  ///
  /// In id, this message translates to:
  /// **'Omzet'**
  String get reportRevenue;

  /// No description provided for @reportNotes.
  ///
  /// In id, this message translates to:
  /// **'Nota'**
  String get reportNotes;

  /// No description provided for @reportVoid.
  ///
  /// In id, this message translates to:
  /// **'Batal'**
  String get reportVoid;

  /// No description provided for @reportReturns.
  ///
  /// In id, this message translates to:
  /// **'Retur'**
  String get reportReturns;

  /// No description provided for @reportNet.
  ///
  /// In id, this message translates to:
  /// **'Bersih setelah retur'**
  String get reportNet;

  /// No description provided for @reportReceivable.
  ///
  /// In id, this message translates to:
  /// **'Piutang'**
  String get reportReceivable;

  /// No description provided for @reportByMethod.
  ///
  /// In id, this message translates to:
  /// **'Per metode bayar'**
  String get reportByMethod;

  /// No description provided for @reportByDay.
  ///
  /// In id, this message translates to:
  /// **'Per hari'**
  String get reportByDay;

  /// No description provided for @reportEmpty.
  ///
  /// In id, this message translates to:
  /// **'Tidak ada penjualan pada rentang ini.'**
  String get reportEmpty;

  /// No description provided for @reportDayNotes.
  ///
  /// In id, this message translates to:
  /// **'{count} nota'**
  String reportDayNotes(String count);

  /// No description provided for @menuItems.
  ///
  /// In id, this message translates to:
  /// **'Barang'**
  String get menuItems;

  /// No description provided for @menuPurchases.
  ///
  /// In id, this message translates to:
  /// **'Pembelian'**
  String get menuPurchases;

  /// No description provided for @itemsTitle.
  ///
  /// In id, this message translates to:
  /// **'Barang'**
  String get itemsTitle;

  /// No description provided for @itemsSearch.
  ///
  /// In id, this message translates to:
  /// **'Cari nama, kode, atau barcode'**
  String get itemsSearch;

  /// No description provided for @itemsEmpty.
  ///
  /// In id, this message translates to:
  /// **'Belum ada barang.'**
  String get itemsEmpty;

  /// No description provided for @itemsTabActive.
  ///
  /// In id, this message translates to:
  /// **'Aktif'**
  String get itemsTabActive;

  /// No description provided for @itemsTabArchived.
  ///
  /// In id, this message translates to:
  /// **'Diarsipkan'**
  String get itemsTabArchived;

  /// No description provided for @itemsEdit.
  ///
  /// In id, this message translates to:
  /// **'Ubah barang'**
  String get itemsEdit;

  /// No description provided for @itemsArchive.
  ///
  /// In id, this message translates to:
  /// **'Arsipkan'**
  String get itemsArchive;

  /// No description provided for @itemsRestore.
  ///
  /// In id, this message translates to:
  /// **'Aktifkan kembali'**
  String get itemsRestore;

  /// No description provided for @itemsArchiveAsk.
  ///
  /// In id, this message translates to:
  /// **'Arsipkan {name}? Barang tidak muncul di kasir, riwayat transaksi tetap aman.'**
  String itemsArchiveAsk(String name);

  /// No description provided for @itemsArchived.
  ///
  /// In id, this message translates to:
  /// **'{name} diarsipkan'**
  String itemsArchived(String name);

  /// No description provided for @itemsRestored.
  ///
  /// In id, this message translates to:
  /// **'{name} diaktifkan kembali'**
  String itemsRestored(String name);

  /// No description provided for @itemsUpdated.
  ///
  /// In id, this message translates to:
  /// **'{name} diperbarui'**
  String itemsUpdated(String name);

  /// No description provided for @itemsStock.
  ///
  /// In id, this message translates to:
  /// **'Stok {qty}'**
  String itemsStock(String qty);

  /// No description provided for @itemsEditTitle.
  ///
  /// In id, this message translates to:
  /// **'Ubah barang'**
  String get itemsEditTitle;

  /// No description provided for @itemsCostNote.
  ///
  /// In id, this message translates to:
  /// **'Harga modal berubah lewat pembelian, bukan dari sini.'**
  String get itemsCostNote;

  /// No description provided for @itemsArchivedHint.
  ///
  /// In id, this message translates to:
  /// **'Menampilkan maksimal 30 barang diarsipkan; persempit dengan pencarian.'**
  String get itemsArchivedHint;

  /// No description provided for @commonCancel.
  ///
  /// In id, this message translates to:
  /// **'Batal'**
  String get commonCancel;

  /// No description provided for @commonSave.
  ///
  /// In id, this message translates to:
  /// **'Simpan'**
  String get commonSave;

  /// No description provided for @commonRetry.
  ///
  /// In id, this message translates to:
  /// **'Coba lagi'**
  String get commonRetry;

  /// No description provided for @purTitle.
  ///
  /// In id, this message translates to:
  /// **'Pembelian'**
  String get purTitle;

  /// No description provided for @purAdd.
  ///
  /// In id, this message translates to:
  /// **'Pembelian baru'**
  String get purAdd;

  /// No description provided for @purCashNote.
  ///
  /// In id, this message translates to:
  /// **'Pembelian dari HP selalu tunai dan langsung menambah stok.'**
  String get purCashNote;

  /// No description provided for @purSupplier.
  ///
  /// In id, this message translates to:
  /// **'Pemasok'**
  String get purSupplier;

  /// No description provided for @purPickSupplier.
  ///
  /// In id, this message translates to:
  /// **'Pilih pemasok'**
  String get purPickSupplier;

  /// No description provided for @purSearchSupplier.
  ///
  /// In id, this message translates to:
  /// **'Cari pemasok'**
  String get purSearchSupplier;

  /// No description provided for @purNoSupplier.
  ///
  /// In id, this message translates to:
  /// **'Pemasok tidak ditemukan.'**
  String get purNoSupplier;

  /// No description provided for @purDate.
  ///
  /// In id, this message translates to:
  /// **'Tanggal'**
  String get purDate;

  /// No description provided for @purInvoiceNo.
  ///
  /// In id, this message translates to:
  /// **'No. faktur pemasok (opsional)'**
  String get purInvoiceNo;

  /// No description provided for @purNote.
  ///
  /// In id, this message translates to:
  /// **'Catatan (opsional)'**
  String get purNote;

  /// No description provided for @purLines.
  ///
  /// In id, this message translates to:
  /// **'Barang dibeli'**
  String get purLines;

  /// No description provided for @purAddItem.
  ///
  /// In id, this message translates to:
  /// **'Tambah barang'**
  String get purAddItem;

  /// No description provided for @purSearchItem.
  ///
  /// In id, this message translates to:
  /// **'Cari barang'**
  String get purSearchItem;

  /// No description provided for @purQty.
  ///
  /// In id, this message translates to:
  /// **'Qty'**
  String get purQty;

  /// No description provided for @purPrice.
  ///
  /// In id, this message translates to:
  /// **'Harga beli'**
  String get purPrice;

  /// No description provided for @purSubtotal.
  ///
  /// In id, this message translates to:
  /// **'Subtotal'**
  String get purSubtotal;

  /// No description provided for @purTax.
  ///
  /// In id, this message translates to:
  /// **'PPN'**
  String get purTax;

  /// No description provided for @purOtherCost.
  ///
  /// In id, this message translates to:
  /// **'Biaya lain'**
  String get purOtherCost;

  /// No description provided for @purTotal.
  ///
  /// In id, this message translates to:
  /// **'Total tunai'**
  String get purTotal;

  /// No description provided for @purSave.
  ///
  /// In id, this message translates to:
  /// **'Simpan pembelian'**
  String get purSave;

  /// No description provided for @purSaved.
  ///
  /// In id, this message translates to:
  /// **'Pembelian {doc} tersimpan, stok bertambah'**
  String purSaved(String doc);

  /// No description provided for @purNoLines.
  ///
  /// In id, this message translates to:
  /// **'Tambah minimal satu barang.'**
  String get purNoLines;

  /// No description provided for @purNeedSupplier.
  ///
  /// In id, this message translates to:
  /// **'Pilih pemasok dulu.'**
  String get purNeedSupplier;

  /// No description provided for @purBadLine.
  ///
  /// In id, this message translates to:
  /// **'Qty harus lebih dari 0 dan harga tidak boleh kosong.'**
  String get purBadLine;

  /// No description provided for @purEmpty.
  ///
  /// In id, this message translates to:
  /// **'Belum ada pembelian pada rentang ini.'**
  String get purEmpty;

  /// No description provided for @purSummary.
  ///
  /// In id, this message translates to:
  /// **'{count} pembelian, {total}'**
  String purSummary(String count, String total);

  /// No description provided for @purRange30.
  ///
  /// In id, this message translates to:
  /// **'30 hari'**
  String get purRange30;

  /// No description provided for @purDetailTitle.
  ///
  /// In id, this message translates to:
  /// **'Detail pembelian'**
  String get purDetailTitle;

  /// No description provided for @purEdit.
  ///
  /// In id, this message translates to:
  /// **'Ubah'**
  String get purEdit;

  /// No description provided for @purEditTitle.
  ///
  /// In id, this message translates to:
  /// **'Ubah pembelian'**
  String get purEditTitle;

  /// No description provided for @purVoid.
  ///
  /// In id, this message translates to:
  /// **'Batalkan'**
  String get purVoid;

  /// No description provided for @purVoidAsk.
  ///
  /// In id, this message translates to:
  /// **'Batalkan pembelian {doc}? Stok akan dikurangi lagi.'**
  String purVoidAsk(String doc);

  /// No description provided for @purReason.
  ///
  /// In id, this message translates to:
  /// **'Alasan (minimal 3 huruf)'**
  String get purReason;

  /// No description provided for @purVoided.
  ///
  /// In id, this message translates to:
  /// **'Pembelian dibatalkan'**
  String get purVoided;

  /// No description provided for @purEdited.
  ///
  /// In id, this message translates to:
  /// **'Pembelian diperbarui'**
  String get purEdited;

  /// No description provided for @purStatusVoid.
  ///
  /// In id, this message translates to:
  /// **'BATAL'**
  String get purStatusVoid;

  /// No description provided for @purStatusRevised.
  ///
  /// In id, this message translates to:
  /// **'DIREVISI'**
  String get purStatusRevised;

  /// No description provided for @purVoidReason.
  ///
  /// In id, this message translates to:
  /// **'Alasan batal: {reason}'**
  String purVoidReason(String reason);

  /// No description provided for @purBy.
  ///
  /// In id, this message translates to:
  /// **'Dicatat oleh {name}'**
  String purBy(String name);

  /// No description provided for @errorPurchaseNotEditable.
  ///
  /// In id, this message translates to:
  /// **'Nota ini tidak dapat diubah lagi (sudah dibatalkan atau digantikan revisi).'**
  String get errorPurchaseNotEditable;

  /// No description provided for @errorPurchaseHasReturns.
  ///
  /// In id, this message translates to:
  /// **'Nota ini sudah diretur; batalkan returnya dulu.'**
  String get errorPurchaseHasReturns;

  /// No description provided for @errorItemUnavailable.
  ///
  /// In id, this message translates to:
  /// **'Barang tidak tersedia untuk pembelian.'**
  String get errorItemUnavailable;

  /// No description provided for @errorCodeTaken.
  ///
  /// In id, this message translates to:
  /// **'Kode barang sudah dipakai.'**
  String get errorCodeTaken;

  /// No description provided for @errorEditWindowClosed.
  ///
  /// In id, this message translates to:
  /// **'Batas waktu edit sudah lewat.'**
  String get errorEditWindowClosed;

  /// No description provided for @errorStockReverse.
  ///
  /// In id, this message translates to:
  /// **'Stok tidak cukup untuk membalik nota ini (sebagian sudah terjual).'**
  String get errorStockReverse;

  /// No description provided for @menuSettlement.
  ///
  /// In id, this message translates to:
  /// **'Settlement'**
  String get menuSettlement;

  /// No description provided for @setTitle.
  ///
  /// In id, this message translates to:
  /// **'Settlement EraPay'**
  String get setTitle;

  /// No description provided for @setCheck.
  ///
  /// In id, this message translates to:
  /// **'Cek ke EraPay'**
  String get setCheck;

  /// No description provided for @setChecking.
  ///
  /// In id, this message translates to:
  /// **'Mengecek ke EraPay...'**
  String get setChecking;

  /// No description provided for @setArus.
  ///
  /// In id, this message translates to:
  /// **'UANG ARUS'**
  String get setArus;

  /// No description provided for @setEra.
  ///
  /// In id, this message translates to:
  /// **'UANG ERAPAY'**
  String get setEra;

  /// No description provided for @setArusHint.
  ///
  /// In id, this message translates to:
  /// **'QRIS pada nota aktif'**
  String get setArusHint;

  /// No description provided for @setEraHint.
  ///
  /// In id, this message translates to:
  /// **'Tagihan lunas di EraPay'**
  String get setEraHint;

  /// No description provided for @setMatchedAll.
  ///
  /// In id, this message translates to:
  /// **'Cocok: uang ARUS sama dengan uang EraPay.'**
  String get setMatchedAll;

  /// No description provided for @setDiffMore.
  ///
  /// In id, this message translates to:
  /// **'Selisih {amount}: ada uang lunas di EraPay yang belum tercatat sebagai nota aktif.'**
  String setDiffMore(String amount);

  /// No description provided for @setDiffLess.
  ///
  /// In id, this message translates to:
  /// **'Selisih {amount}: ada nota QRIS yang belum lunas di EraPay.'**
  String setDiffLess(String amount);

  /// No description provided for @setCount.
  ///
  /// In id, this message translates to:
  /// **'{count} transaksi'**
  String setCount(String count);

  /// No description provided for @setFilterAll.
  ///
  /// In id, this message translates to:
  /// **'Semua'**
  String get setFilterAll;

  /// No description provided for @setFilterAttention.
  ///
  /// In id, this message translates to:
  /// **'Perlu dicek'**
  String get setFilterAttention;

  /// No description provided for @setFilterWaiting.
  ///
  /// In id, this message translates to:
  /// **'Menunggu'**
  String get setFilterWaiting;

  /// No description provided for @setFilterMatched.
  ///
  /// In id, this message translates to:
  /// **'Cocok'**
  String get setFilterMatched;

  /// No description provided for @setColArus.
  ///
  /// In id, this message translates to:
  /// **'ARUS'**
  String get setColArus;

  /// No description provided for @setColEra.
  ///
  /// In id, this message translates to:
  /// **'ERAPAY'**
  String get setColEra;

  /// No description provided for @setStatusLabel.
  ///
  /// In id, this message translates to:
  /// **'Status'**
  String get setStatusLabel;

  /// No description provided for @setIdLabel.
  ///
  /// In id, this message translates to:
  /// **'ID EraPay'**
  String get setIdLabel;

  /// No description provided for @setIdCopied.
  ///
  /// In id, this message translates to:
  /// **'ID disalin'**
  String get setIdCopied;

  /// No description provided for @setNoId.
  ///
  /// In id, this message translates to:
  /// **'Belum ada ID'**
  String get setNoId;

  /// No description provided for @setStateMatched.
  ///
  /// In id, this message translates to:
  /// **'Cocok'**
  String get setStateMatched;

  /// No description provided for @setStateMatchedSub.
  ///
  /// In id, this message translates to:
  /// **'Lunas di EraPay dan tercatat di nota'**
  String get setStateMatchedSub;

  /// No description provided for @setStateWaiting.
  ///
  /// In id, this message translates to:
  /// **'Menunggu'**
  String get setStateWaiting;

  /// No description provided for @setStateWaitingSub.
  ///
  /// In id, this message translates to:
  /// **'EraPay belum memutuskan (QR belum dibayar)'**
  String get setStateWaitingSub;

  /// No description provided for @setStateOrphan.
  ///
  /// In id, this message translates to:
  /// **'Belum tercatat'**
  String get setStateOrphan;

  /// No description provided for @setStateOrphanSub.
  ///
  /// In id, this message translates to:
  /// **'Lunas di EraPay tetapi belum jadi nota'**
  String get setStateOrphanSub;

  /// No description provided for @setStateRefund.
  ///
  /// In id, this message translates to:
  /// **'Perlu refund'**
  String get setStateRefund;

  /// No description provided for @setStateRefundSub.
  ///
  /// In id, this message translates to:
  /// **'Nota dibatalkan, uang masih di EraPay'**
  String get setStateRefundSub;

  /// No description provided for @setStateUnpaid.
  ///
  /// In id, this message translates to:
  /// **'Tidak lunas'**
  String get setStateUnpaid;

  /// No description provided for @setStateUnpaidSub.
  ///
  /// In id, this message translates to:
  /// **'Nota ada tetapi EraPay gagal atau kedaluwarsa'**
  String get setStateUnpaidSub;

  /// No description provided for @setStateClosed.
  ///
  /// In id, this message translates to:
  /// **'Tidak dibayar'**
  String get setStateClosed;

  /// No description provided for @setStateClosedSub.
  ///
  /// In id, this message translates to:
  /// **'Tagihan gagal atau kedaluwarsa, tidak ada uang'**
  String get setStateClosedSub;

  /// No description provided for @setEraSucceeded.
  ///
  /// In id, this message translates to:
  /// **'Lunas'**
  String get setEraSucceeded;

  /// No description provided for @setEraPending.
  ///
  /// In id, this message translates to:
  /// **'Menunggu'**
  String get setEraPending;

  /// No description provided for @setEraFailed.
  ///
  /// In id, this message translates to:
  /// **'Gagal'**
  String get setEraFailed;

  /// No description provided for @setEraExpired.
  ///
  /// In id, this message translates to:
  /// **'Kedaluwarsa'**
  String get setEraExpired;

  /// No description provided for @setEraCancelled.
  ///
  /// In id, this message translates to:
  /// **'Dibatalkan'**
  String get setEraCancelled;

  /// No description provided for @setEraUnknown.
  ///
  /// In id, this message translates to:
  /// **'Belum pasti'**
  String get setEraUnknown;

  /// No description provided for @setNotConfigured.
  ///
  /// In id, this message translates to:
  /// **'EraPay belum dipasang untuk toko ini. Pasang API key di Metode Pembayaran (web).'**
  String get setNotConfigured;

  /// No description provided for @setEmpty.
  ///
  /// In id, this message translates to:
  /// **'Tidak ada transaksi QRIS EraPay pada rentang ini.'**
  String get setEmpty;

  /// No description provided for @setRefreshed.
  ///
  /// In id, this message translates to:
  /// **'Status diperbarui dari EraPay: {count} transaksi'**
  String setRefreshed(String count);

  /// No description provided for @setTruncated.
  ///
  /// In id, this message translates to:
  /// **'Daftar dipotong 300 transaksi terbaru; persempit rentang tanggal.'**
  String get setTruncated;

  /// No description provided for @setNote.
  ///
  /// In id, this message translates to:
  /// **'Uang EraPay = tagihan berstatus lunas di EraPay. Pencairan ke rekening bank mengikuti jadwal EraPay dan belum tersedia di sini.'**
  String get setNote;
}

class _AppLocalizationsDelegate
    extends LocalizationsDelegate<AppLocalizations> {
  const _AppLocalizationsDelegate();

  @override
  Future<AppLocalizations> load(Locale locale) {
    return SynchronousFuture<AppLocalizations>(lookupAppLocalizations(locale));
  }

  @override
  bool isSupported(Locale locale) =>
      <String>['en', 'id', 'ja'].contains(locale.languageCode);

  @override
  bool shouldReload(_AppLocalizationsDelegate old) => false;
}

AppLocalizations lookupAppLocalizations(Locale locale) {
  // Lookup logic when only language code is specified.
  switch (locale.languageCode) {
    case 'en':
      return AppLocalizationsEn();
    case 'id':
      return AppLocalizationsId();
    case 'ja':
      return AppLocalizationsJa();
  }

  throw FlutterError(
    'AppLocalizations.delegate failed to load unsupported locale "$locale". This is likely '
    'an issue with the localizations generation tool. Please file an issue '
    'on GitHub with a reproducible sample app and the gen-l10n configuration '
    'that was used.',
  );
}
