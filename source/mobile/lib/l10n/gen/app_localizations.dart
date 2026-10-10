import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:intl/intl.dart' as intl;

import 'app_localizations_en.dart';
import 'app_localizations_id.dart';

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

  /// No description provided for @posCartEmpty.
  ///
  /// In id, this message translates to:
  /// **'Keranjang kosong. Pilih barang untuk memulai.'**
  String get posCartEmpty;

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

  /// No description provided for @paySubmit.
  ///
  /// In id, this message translates to:
  /// **'Selesaikan pembayaran'**
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
  /// **'Alamat API ARUS. Di HP fisik gunakan IP komputer di jaringan yang sama, mis. http://192.168.1.10:8080. Kosongkan untuk kembali ke bawaan.'**
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
      <String>['en', 'id'].contains(locale.languageCode);

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
  }

  throw FlutterError(
    'AppLocalizations.delegate failed to load unsupported locale "$locale". This is likely '
    'an issue with the localizations generation tool. Please file an issue '
    'on GitHub with a reproducible sample app and the gen-l10n configuration '
    'that was used.',
  );
}
