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

  @override
  String get posCartEmptyTitle => 'Keranjang masih kosong';

  @override
  String get posCartEmptyHint => 'Pilih barang untuk mulai berjualan';

  @override
  String get languageTooltip => 'Bahasa';

  @override
  String get languageSystem => 'Ikuti HP';

  @override
  String get outletSwitch => 'Pindah cabang';

  @override
  String get outletSwitchTitle => 'Pilih cabang';

  @override
  String outletSwitched(String name) {
    return 'Sekarang di cabang $name';
  }

  @override
  String get outletApprovalTitle => 'Persetujuan pindah cabang';

  @override
  String outletApprovalBody(String name) {
    return 'Pindah dari kasir ke $name butuh persetujuan Owner/Supervisor. Keranjang aktif akan dikosongkan.';
  }

  @override
  String get outletApprovalNone => 'Belum ada penyetuju di cabang tujuan.';

  @override
  String get outletApprover => 'Penyetuju';

  @override
  String get outletPin => 'PIN penyetuju (6 digit)';

  @override
  String get outletApprovalConfirm => 'Setujui & pindah';

  @override
  String get errorPinRequired => 'Persetujuan Owner/Supervisor (PIN) wajib.';

  @override
  String get errorInvalidPin => 'Penyetuju atau PIN salah.';

  @override
  String get errorPinLocked =>
      'PIN terkunci sementara karena terlalu banyak percobaan. Coba lagi nanti.';

  @override
  String get posShiftCloseTooltip => 'Tutup shift';

  @override
  String get posSalesTodayTooltip => 'Penjualan hari ini';

  @override
  String shiftCloseTitle(String doc) {
    return 'Tutup shift $doc';
  }

  @override
  String get shiftCloseBody =>
      'Hitung uang fisik di laci untuk setiap metode, lalu isi di bawah. Selisih wajib dicatat dan disetujui Owner/Supervisor.';

  @override
  String shiftCloseOpened(String time) {
    return 'Dibuka $time';
  }

  @override
  String shiftCloseSales(String count, String total) {
    return '$count nota, $total';
  }

  @override
  String shiftCloseVoid(String count) {
    return '$count nota batal';
  }

  @override
  String shiftCloseReceivable(String amount) {
    return 'Piutang $amount';
  }

  @override
  String get shiftColExpected => 'Seharusnya';

  @override
  String get shiftColCounted => 'Uang fisik (Rp)';

  @override
  String get shiftColDiff => 'Selisih';

  @override
  String get shiftFillExpected => 'Isi sesuai rekap';

  @override
  String get shiftTotalDiff => 'Total selisih';

  @override
  String get shiftNote => 'Catatan selisih';

  @override
  String get shiftNoteHint => 'Jelaskan penyebab selisih (min. 3 huruf)';

  @override
  String get shiftNeedApproval =>
      'Ada selisih: butuh persetujuan Owner/Supervisor.';

  @override
  String get shiftApprovalNone => 'Belum ada penyetuju di cabang ini.';

  @override
  String get shiftCloseSubmit => 'Tutup shift';

  @override
  String get shiftChangedNotice =>
      'Ada transaksi baru sejak rekap ditampilkan. Rekap sudah dimuat ulang; periksa lagi hitungan Anda.';

  @override
  String shiftClosedTitle(String doc) {
    return 'Shift $doc ditutup';
  }

  @override
  String get shiftClosedBody => 'Rekap dibekukan dan tersimpan di server.';

  @override
  String get shiftTotalExpected => 'Seharusnya';

  @override
  String get shiftTotalCounted => 'Dihitung';

  @override
  String get shiftOpenNew => 'Buka shift baru';

  @override
  String get shiftDone => 'Selesai';

  @override
  String get errorShiftRecapChanged =>
      'Ada transaksi baru sejak rekap ditampilkan. Muat ulang rekap lalu hitung lagi.';

  @override
  String get errorShiftDiffNote =>
      'Selisih wajib disertai catatan dan persetujuan.';

  @override
  String get errorShiftClosed => 'Shift ini sudah ditutup.';

  @override
  String get errorShiftAlreadyOpen => 'Shift Anda di cabang ini sudah terbuka.';

  @override
  String get salesTodayTitle => 'Penjualan hari ini';

  @override
  String salesTodaySummary(String count, String total) {
    return '$count nota, $total';
  }

  @override
  String get salesTodaySearch => 'Cari nomor nota';

  @override
  String get salesTodayEmpty => 'Belum ada penjualan.';

  @override
  String get salesTodayTruncated =>
      'Daftar dipotong; persempit dengan pencarian.';

  @override
  String get saleVoidBadge => 'BATAL';

  @override
  String saleReturnedBadge(String amount) {
    return 'Retur $amount';
  }

  @override
  String get saleCreditBadge => 'Piutang';

  @override
  String saleDetailCashier(String name) {
    return 'Kasir $name';
  }

  @override
  String saleDetailMember(String name) {
    return 'Member $name';
  }

  @override
  String saleVoidReason(String reason) {
    return 'Alasan batal: $reason';
  }

  @override
  String get saleSubtotal => 'Subtotal';

  @override
  String get saleDiscount => 'Potongan';

  @override
  String get saleTax => 'Pajak';

  @override
  String get saleOtherCost => 'Biaya lain';

  @override
  String get saleTotal => 'Total';

  @override
  String get salePaid => 'Dibayar';

  @override
  String get saleChange => 'Kembalian';

  @override
  String get saleReceivable => 'Piutang';

  @override
  String get posScanTooltip => 'Scan barcode dengan kamera';

  @override
  String get scanTitle => 'Scan barang';

  @override
  String get scanTorch => 'Senter';

  @override
  String get scanSwitchCamera => 'Ganti kamera';

  @override
  String get scanHint => 'Arahkan kamera ke barcode barang';

  @override
  String get scanDone => 'Selesai';

  @override
  String get scanPermissionDenied =>
      'Izin kamera ditolak. Aktifkan izin kamera untuk ARUS di pengaturan HP.';

  @override
  String get scanCameraError => 'Kamera tidak bisa dibuka.';

  @override
  String scanAdded(String name) {
    return '$name masuk keranjang';
  }

  @override
  String scanNotFound(String code) {
    return 'Kode $code tidak ditemukan';
  }

  @override
  String scanAmbiguous(String code) {
    return 'Kode $code cocok dengan lebih dari satu barang; cari manual';
  }

  @override
  String get memberChoose => 'Pilih member';

  @override
  String get memberChange => 'Ganti member';

  @override
  String get memberGeneral => 'Pelanggan umum';

  @override
  String get memberSearchHint => 'Cari nama, kode, atau telepon';

  @override
  String get memberSearchEmpty => 'Member tidak ditemukan.';

  @override
  String get memberRemove => 'Lepas member';

  @override
  String memberPoints(int points) {
    return '$points poin';
  }

  @override
  String memberPointsAndDeposit(int points, String deposit) {
    return '$points poin · deposit $deposit';
  }

  @override
  String memberEarn(int points) {
    return '+$points poin';
  }

  @override
  String get memberRedeemTitle => 'Tukar poin';

  @override
  String memberRedeemAvailable(int points, String value) {
    return 'Bisa ditukar maksimal $points poin (senilai $value)';
  }

  @override
  String get memberRedeemField => 'Poin yang ditukar';

  @override
  String get memberRedeemAll => 'Semua';

  @override
  String get memberRedeemNone =>
      'Poin member ini belum bisa ditukar untuk nota ini.';

  @override
  String get memberRedeemReset => 'Batalkan tukar';

  @override
  String get memberRedeemDone => 'Terapkan';

  @override
  String memberRedeemValue(String amount) {
    return 'Potongan $amount';
  }

  @override
  String get errorRedeemInvalid =>
      'Poin yang ditukar melebihi batas. Kurangi jumlahnya.';

  @override
  String payDepositOver(String amount) {
    return 'Deposit melebihi saldo ($amount)';
  }

  @override
  String get adjustTitle => 'Ubah harga / potongan';

  @override
  String get adjustBody =>
      'Butuh persetujuan penyetuju (Owner/Supervisor) dengan PIN-nya.';

  @override
  String get adjustItem => 'Barang';

  @override
  String get adjustListPrice => 'Harga normal';

  @override
  String get adjustTabPrice => 'Ubah harga';

  @override
  String get adjustTabDiscount => 'Potongan';

  @override
  String get adjustNewPrice => 'Harga baru per satuan (Rp)';

  @override
  String get adjustDiscountRp => 'Potongan (Rp)';

  @override
  String get adjustDiscountPct => 'Potongan (%)';

  @override
  String get adjustUnitTotal => 'Total baris';

  @override
  String get adjustUnitPerUnit => 'Per satuan';

  @override
  String adjustPercentPreview(String amount) {
    return 'Potongan $amount per satuan';
  }

  @override
  String get adjustChecking => 'Memeriksa PIN...';

  @override
  String get adjustApply => 'Terapkan';

  @override
  String get adjustReset => 'Kembalikan ke harga normal';

  @override
  String get adjustEditTooltip => 'Ubah harga / potongan';

  @override
  String get adjustBadgePrice => 'Harga diubah';

  @override
  String adjustBadgeDiscount(String amount) {
    return 'Potongan $amount';
  }

  @override
  String adjustApprovedBy(String name) {
    return 'disetujui $name';
  }

  @override
  String get paySurcharge => 'Biaya metode bayar';

  @override
  String get payCharged => 'Ditagih ke pelanggan';

  @override
  String payFeeCustomer(String rate, String fee, String amount) {
    return 'Biaya $rate = $fee, ditagihkan ke pelanggan (pelanggan bayar $amount)';
  }

  @override
  String payFeeStore(String rate, String fee) {
    return 'Biaya $rate = $fee, ditanggung toko';
  }

  @override
  String paySurchargeDone(String surcharge, String charged) {
    return 'Biaya metode $surcharge · ditagih $charged';
  }

  @override
  String get costsTitle => 'Keterangan & biaya lain';

  @override
  String get costsHint =>
      'Biaya lain-lain dengan rincian (maks 20), misalnya ongkir atau packing.';

  @override
  String get costsName => 'Nama biaya';

  @override
  String get costsAmount => 'Jumlah (Rp)';

  @override
  String get costsRemove => 'Hapus';

  @override
  String get costsAdd => 'Tambah biaya';

  @override
  String get costsTotal => 'Total biaya lain';

  @override
  String get costsDone => 'Terapkan';

  @override
  String get costsClear => 'Hapus semua';

  @override
  String get posCostsButton => 'Keterangan & biaya lain';

  @override
  String posTaxStore(String pct) {
    return 'Pajak toko ($pct%)';
  }

  @override
  String posTaxGov(String pct) {
    return 'Pajak negara ($pct%)';
  }

  @override
  String get posCostUnnamed => 'Biaya lain';

  @override
  String get payModePay => 'Bayar';

  @override
  String get payModeCredit => 'Kredit';

  @override
  String get payCreditNoMember =>
      'Kredit hanya untuk member. Pilih member di keranjang dulu.';

  @override
  String get payCreditNotNeeded =>
      'Uang muka sudah menutup total nota; gunakan mode Bayar.';

  @override
  String get payCreditDpAdd => 'Tambah uang muka (DP)';

  @override
  String get payCreditReceivable => 'Piutang (sisa belum dibayar)';

  @override
  String get payCreditDue => 'Jatuh tempo';

  @override
  String payCreditDueDays(int days) {
    return '$days hari setelah nota';
  }

  @override
  String get payCreditNoDue => 'Tanpa jatuh tempo';

  @override
  String get payCreditLimit => 'Limit kredit';

  @override
  String get payCreditNoLimit => 'Tanpa batas';

  @override
  String get payCreditOutstanding => 'Piutang saat ini';

  @override
  String get payCreditAfter => 'Piutang setelah nota ini';

  @override
  String get payCreditOverLimit =>
      'Melewati limit kredit member. Butuh persetujuan penyetuju dengan PIN-nya.';

  @override
  String get payCreditSameApprover =>
      'Penyetuju yang sama (ubah harga/potongan) dipakai untuk limit kredit.';

  @override
  String get errorCreditLimit =>
      'Piutang member melewati limit kredit. Butuh persetujuan penyetuju (PIN).';

  @override
  String paySuccessReceivable(String amount) {
    return 'Piutang $amount';
  }

  @override
  String paySuccessDue(String date) {
    return 'Jatuh tempo $date';
  }

  @override
  String get pendingHoldTitle => 'Tunda nota';

  @override
  String get pendingLabel => 'Keterangan (opsional)';

  @override
  String get pendingLabelHint => 'Nama pelanggan atau ciri-cirinya';

  @override
  String get pendingHold => 'Tunda';

  @override
  String get pendingHoldTooltip => 'Tunda nota';

  @override
  String get pendingListTooltip => 'Nota pending';

  @override
  String pendingFull(int max) {
    return 'Nota pending penuh (maks $max). Buka atau hapus salah satu dulu.';
  }

  @override
  String pendingSaved(int no) {
    return 'Nota ditunda sebagai Pending $no';
  }

  @override
  String pendingOpened(int no) {
    return 'Pending $no dibuka';
  }

  @override
  String pendingOpenedSwapped(int no, int saved) {
    return 'Pending $no dibuka; isi sebelumnya disimpan sebagai Pending $saved';
  }

  @override
  String pendingTitle(int count, int max) {
    return 'Nota pending ($count/$max)';
  }

  @override
  String get pendingHint =>
      'Tersimpan di HP ini, kedaluwarsa 24 jam. Harga dan stok dihitung ulang saat dibuka.';

  @override
  String get pendingEmpty => 'Belum ada nota pending.';

  @override
  String pendingNo(int no) {
    return 'Pending $no';
  }

  @override
  String pendingLines(int count) {
    return '$count baris';
  }

  @override
  String get pendingDelete => 'Hapus';

  @override
  String pendingDeleteAsk(int no) {
    return 'Hapus Pending $no?';
  }

  @override
  String get shortcutsEmpty =>
      'Belum ada pintasan. Ketuk ikon di kanan untuk mengatur.';

  @override
  String get shortcutsManage => 'Atur pintasan barang';

  @override
  String get shortcutsTitle => 'Pintasan barang';

  @override
  String get shortcutsHint =>
      '16 slot milik Anda, sama dengan kasir web. Ketuk slot untuk memasang barang.';

  @override
  String get shortcutsSlotEmpty => 'Kosong — ketuk untuk memasang barang';

  @override
  String get shortcutsInactive => 'Barang nonaktif';

  @override
  String get shortcutsClear => 'Kosongkan slot';

  @override
  String get salespersonLabel => 'Salesman';

  @override
  String get salespersonNone => 'Umum (tanpa salesman)';

  @override
  String get salespersonSearch => 'Cari salesman';

  @override
  String get salespersonEmpty => 'Salesman tidak ditemukan.';

  @override
  String get payQuickAmounts => 'Uang diterima';

  @override
  String get categoryAll => 'Semua';

  @override
  String get noteLabel => 'Keterangan nota';

  @override
  String get noteHint => 'Mis. nama pelanggan, titipan, alamat kirim';

  @override
  String homeGreetMorning(String name) {
    return 'Selamat pagi, $name';
  }

  @override
  String homeGreetNoon(String name) {
    return 'Selamat siang, $name';
  }

  @override
  String homeGreetAfternoon(String name) {
    return 'Selamat sore, $name';
  }

  @override
  String homeGreetNight(String name) {
    return 'Selamat malam, $name';
  }

  @override
  String get homeTodayTitle => 'Penjualan hari ini';

  @override
  String get homeTodayNotes => 'Nota';

  @override
  String get homeShiftLabel => 'Shift';

  @override
  String get homeStallHint => 'Ketuk untuk mulai berjualan';
}
