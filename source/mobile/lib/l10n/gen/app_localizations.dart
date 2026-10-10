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
  /// **'Penjualan hari ini'**
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
  /// **'Penjualan hari ini'**
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
