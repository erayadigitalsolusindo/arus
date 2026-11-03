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
  String get textSizeTooltip => 'Ukuran teks';

  @override
  String get textSizeTitle => 'Ukuran teks';

  @override
  String get textSizeSample => 'Contoh teks: Kopi susu Rp 18.000';

  @override
  String get textSizeDecrease => 'Perkecil';

  @override
  String get textSizeIncrease => 'Perbesar';

  @override
  String get textSizeReset => 'Atur ulang';

  @override
  String get textSizeDone => 'Selesai';

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
  String get posConfirm => 'Konfirmasi';

  @override
  String get posPay => 'Bayar';

  @override
  String get posTaxToggle => 'Hitung pajak';

  @override
  String get posClearCart => 'Kosongkan keranjang';

  @override
  String get posClearCartAsk => 'Kosongkan keranjang?';

  @override
  String get posClearCartAskBody =>
      'Semua barang di keranjang akan dihapus dan tidak bisa dikembalikan.';

  @override
  String get posClearCartYes => 'Ya, kosongkan';

  @override
  String get posClearCartNo => 'Batal';

  @override
  String get posAddedToCart => 'Ditambahkan';

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
  String get payEraGenerate => 'Buat QR QRIS';

  @override
  String get payEraRegenerate => 'Buat ulang QR';

  @override
  String get payEraCreating => 'Membuat QR...';

  @override
  String payEraScan(String amount) {
    return 'Minta pelanggan scan dan bayar $amount';
  }

  @override
  String get payEraWaiting =>
      'Menunggu pembayaran... nota tersimpan otomatis setelah lunas.';

  @override
  String get payEraPaid => 'Pembayaran QRIS diterima';

  @override
  String get payEraFailed => 'Pembayaran QRIS gagal. Buat QR baru.';

  @override
  String get payEraExpired => 'QR kedaluwarsa. Buat QR baru.';

  @override
  String get payEraCancelled => 'Pembayaran dibatalkan. Buat QR baru.';

  @override
  String get payEraNeedPaid => 'Buat QR dan tunggu pembayaran diterima.';

  @override
  String get payEraForceShow => 'EraPay tidak dapat dihubungi? Simpan paksa';

  @override
  String get payEraForceHint =>
      'Simpan paksa: nota disimpan & ditandai \"perlu dicek di EraPay\", lalu dicek otomatis. Bila ternyata tidak lunas, nota dibatalkan & stok kembali. Butuh persetujuan Owner/Supervisor.';

  @override
  String get payEraPendingTitle => 'Pembayaran QRIS belum terverifikasi';

  @override
  String get payEraPendingNotice =>
      'Nota ditandai \"perlu dicek di EraPay\" & dicek otomatis. Bila ternyata tidak lunas, nota dibatalkan otomatis & stok dikembalikan.';

  @override
  String get errorEraDisabled => 'Integrasi EraPay belum diaktifkan di server.';

  @override
  String get errorEraNotConfigured =>
      'API key EraPay belum dipasang untuk toko ini. Minta pemilik memasangnya di Metode Pembayaran (web).';

  @override
  String get errorEraRejected => 'EraPay menolak tagihan QRIS ini.';

  @override
  String get errorEraUnavailable => 'EraPay tidak dapat dihubungi, coba lagi.';

  @override
  String get errorEraNotPaid =>
      'Pembayaran QRIS belum lunas, tidak cocok, atau sudah dipakai nota lain.';

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
  String get payItemsTitle => 'Barang yang dibeli';

  @override
  String get paySubmit => 'Transaksi Pembayaran';

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
      'Pilih server ARUS. Untuk uji coba di HP fisik, ketik IP komputer di jaringan yang sama, mis. http://192.168.1.10:8080.';

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
  String get posSalesTodayTooltip => 'Penjualan';

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
  String get salesTodayTitle => 'Penjualan';

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

  @override
  String get pettyMenu => 'Kas kecil';

  @override
  String get pettyTitle => 'Kas kecil kasir';

  @override
  String get pettyBody =>
      'Catat uang yang keluar dari laci di luar penjualan (beli galon, parkir, dll.) atau uang yang ditambahkan ke laci. Ikut dihitung saat tutup shift.';

  @override
  String get pettyOut => 'Kas keluar';

  @override
  String get pettyIn => 'Kas masuk';

  @override
  String get pettyAmount => 'Jumlah';

  @override
  String get pettyCategory => 'Kategori';

  @override
  String get pettyNote => 'Keterangan';

  @override
  String get pettySave => 'Simpan';

  @override
  String get pettySaved => 'Kas kecil dicatat.';

  @override
  String get pettyHistory => 'Catatan shift ini';

  @override
  String get pettyEmpty => 'Belum ada kas kecil di shift ini.';

  @override
  String get pettyVoid => 'Batalkan';

  @override
  String get pettyVoided => 'Dibatalkan';

  @override
  String get pettyVoidConfirm =>
      'Batalkan catatan ini? Uang laci yang seharusnya dihitung ulang.';

  @override
  String pettyApprovedBy(String name) {
    return 'Disetujui $name';
  }

  @override
  String pettyNeedApproval(String limit) {
    return 'Kas keluar di atas $limit butuh persetujuan Owner/Supervisor (PIN).';
  }

  @override
  String get pettyCatOperational => 'Operasional';

  @override
  String get pettyCatMeal => 'Konsumsi';

  @override
  String get pettyCatTransport => 'Parkir/Transport';

  @override
  String get pettyCatSupplies => 'Perlengkapan';

  @override
  String get pettyCatChange => 'Tambah receh';

  @override
  String get pettyCatMisc => 'Lain-lain';

  @override
  String get errorPettyVoided => 'Catatan kas kecil ini sudah dibatalkan.';

  @override
  String get errorNoCashMethod => 'Metode pembayaran tunai belum tersedia.';

  @override
  String get homePettyHint => 'Catat uang keluar/masuk laci';

  @override
  String get updateTitle => 'Pembaruan tersedia';

  @override
  String updateVersions(String current, String latest) {
    return 'Versi terpasang $current → versi baru $latest';
  }

  @override
  String get updateBody => 'Perbarui ARUS ke versi terbaru untuk melanjutkan.';

  @override
  String get updateNow => 'Perbarui sekarang';

  @override
  String get updCheckTitle => 'Cek pembaruan';

  @override
  String get updChecking => 'Memeriksa pembaruan…';

  @override
  String get updUpToDate => 'Sudah versi terbaru';

  @override
  String get updUpToDateBody => 'ARUS yang Anda pakai sudah versi paling baru.';

  @override
  String get updFailedTitle => 'Tidak bisa memeriksa';

  @override
  String get updPlayUnavailable =>
      'Tidak bisa terhubung ke Google Play. Periksa koneksi lalu coba lagi.';

  @override
  String get updPlayBody =>
      'Versi terbaru ARUS tersedia di Google Play. Perbarui untuk mendapat perbaikan dan fitur baru.';

  @override
  String get updForcedBody =>
      'Pembaruan ini wajib dipasang untuk melanjutkan memakai ARUS.';

  @override
  String get updLater => 'Nanti';

  @override
  String get updReadyTitle => 'Pembaruan siap dipasang';

  @override
  String get updReadyBody =>
      'Pembaruan sudah diunduh. Mulai ulang aplikasi untuk memasangnya.';

  @override
  String get updRestart => 'Mulai ulang & pasang';

  @override
  String get updAvailableBadge => 'Versi baru tersedia';

  @override
  String get updateRetry => 'Coba lagi';

  @override
  String get updateDownloading => 'Mengunduh pembaruan…';

  @override
  String get updateInstalling => 'Membuka pemasang…';

  @override
  String get updatePermission =>
      'Izinkan ARUS memasang aplikasi pada layar yang terbuka, lalu kembali ke sini dan ketuk Perbarui sekarang.';

  @override
  String get updateFailed =>
      'Pembaruan gagal diunduh. Periksa koneksi lalu coba lagi.';

  @override
  String appVersion(String version) {
    return 'Versi $version';
  }

  @override
  String get serverPresetProd => 'Produksi';

  @override
  String get serverPresetEmulator => 'Emulator';

  @override
  String get serverPresetCustom => 'Atau ketik alamat lain:';

  @override
  String get posPanelVersion => 'Versi aplikasi';

  @override
  String get printerMenu => 'Printer struk';

  @override
  String get printerTitle => 'Printer struk (Bluetooth)';

  @override
  String get printerHint =>
      'Pasangkan printer lebih dulu di Pengaturan Android > Bluetooth, lalu pilih di sini. Pengaturan ini berlaku untuk HP ini saja.';

  @override
  String get printerRefresh => 'Muat ulang daftar';

  @override
  String get printerNonePaired =>
      'Belum ada perangkat Bluetooth yang dipasangkan. Pasangkan printer di Pengaturan Android, lalu muat ulang.';

  @override
  String get printerPaper => 'Lebar kertas';

  @override
  String get printerPaper58 => '58 mm';

  @override
  String get printerPaper80 => '80 mm';

  @override
  String get printerCut => 'Potong kertas otomatis';

  @override
  String get printerAuto => 'Cetak struk otomatis setelah bayar';

  @override
  String get printerTest => 'Tes printer';

  @override
  String get printerTestSent => 'Halaman uji terkirim.';

  @override
  String get printerNotConfigured =>
      'Printer belum dipilih. Pilih printer Bluetooth dulu.';

  @override
  String get printerPermissionDenied =>
      'Izin Bluetooth ditolak. Izinkan ARUS memakai Bluetooth di pengaturan aplikasi.';

  @override
  String get printerBluetoothOff =>
      'Bluetooth mati. Nyalakan Bluetooth lalu coba lagi.';

  @override
  String get printerConnectFailed =>
      'Tidak dapat terhubung ke printer. Pastikan printer menyala dan berada di dekat HP.';

  @override
  String get printerWriteFailed =>
      'Data gagal dikirim ke printer. Periksa printer lalu coba lagi.';

  @override
  String get printReceipt => 'Cetak struk';

  @override
  String get printReprint => 'Cetak ulang';

  @override
  String get printPrinting => 'Mencetak…';

  @override
  String printFailed(String error) {
    return 'Struk gagal dicetak: $error';
  }

  @override
  String get receiptNo => 'No';

  @override
  String get receiptDate => 'Tgl';

  @override
  String get receiptCashier => 'Kasir';

  @override
  String get receiptMember => 'Member';

  @override
  String get receiptSalesperson => 'Sales';

  @override
  String receiptCopy(int n) {
    return '*** SALINAN KE-$n ***';
  }

  @override
  String get receiptVoid => '*** NOTA DIBATALKAN ***';

  @override
  String get receiptSuperseded => '*** DIGANTI REVISI ***';

  @override
  String get receiptEraPending => '*** PERLU DI CEK DI ERAPAY ***';

  @override
  String get receiptLineDiscount => 'Potongan';

  @override
  String get receiptSubtotal => 'Subtotal';

  @override
  String get receiptDiscount => 'Diskon';

  @override
  String receiptVoucher(String code) {
    return 'Kupon $code';
  }

  @override
  String receiptRedeem(int points) {
    return 'Tukar $points poin';
  }

  @override
  String receiptTaxStore(String pct) {
    return 'Pajak toko $pct%';
  }

  @override
  String receiptTaxGov(String pct) {
    return 'Pajak $pct%';
  }

  @override
  String get receiptOtherCost => 'Biaya lain';

  @override
  String get receiptTotal => 'TOTAL';

  @override
  String get receiptSurcharge => 'Biaya metode';

  @override
  String get receiptCharged => 'Total ditagih';

  @override
  String get receiptReceivable => 'Piutang';

  @override
  String get receiptDue => 'Jatuh tempo';

  @override
  String get receiptChange => 'Kembali';

  @override
  String get receiptSaved => 'Anda hemat';

  @override
  String get receiptPointsEarned => 'Poin didapat';

  @override
  String get receiptTestTitle => 'Halaman uji printer';

  @override
  String get salesFrom => 'Dari';

  @override
  String get salesTo => 'Sampai';

  @override
  String get salesPresetToday => 'Hari ini';

  @override
  String get salesRangeInvalid => 'Waktu akhir harus setelah waktu awal.';

  @override
  String get salesRangeTooLong => 'Rentang maksimal 62 hari.';

  @override
  String get salesPrintRecap => 'Cetak rekap';

  @override
  String get salesMethodsTitle => 'Laporan per metode bayar';

  @override
  String salesMethodNotes(String count) {
    return '$count nota';
  }

  @override
  String get salesDrawerTitle => 'Seharusnya ada (cek fisik)';

  @override
  String get salesFlowsTitle => 'Uang lain lewat kasir ini';

  @override
  String salesReturnsRow(String count) {
    return 'Retur ($count)';
  }

  @override
  String get salesNetTotal => 'Penjualan bersih setelah retur';

  @override
  String get salesCreditRow => 'Piutang';

  @override
  String get salesSurchargeRow => 'Biaya metode (pelanggan)';

  @override
  String get flowReceivablePayment => 'Bayar piutang';

  @override
  String get flowDepositTopup => 'Top-up deposit';

  @override
  String get flowDepositWithdraw => 'Tarik deposit';

  @override
  String get flowSaleReturn => 'Dana kembali retur jual';

  @override
  String get flowPayablePayment => 'Bayar hutang pemasok';

  @override
  String get flowPurchaseReturn => 'Dana kembali retur beli';

  @override
  String get flowSupplierCashout => 'Pencairan kredit pemasok';

  @override
  String get flowPettyIn => 'Kas kecil masuk';

  @override
  String get flowPettyOut => 'Kas kecil keluar';

  @override
  String get recapTitle => 'REKAP PENJUALAN';

  @override
  String get recapCashier => 'Kasir';

  @override
  String get recapPeriod => 'Periode';

  @override
  String get recapCount => 'Jumlah nota';

  @override
  String get recapVoided => 'Nota batal';

  @override
  String get recapRevenue => 'OMZET';

  @override
  String get recapByMethod => 'Penjualan per metode';

  @override
  String get itemAddTooltip => 'Tambah barang';

  @override
  String get itemAddTitle => 'Tambah barang baru';

  @override
  String get itemName => 'Nama barang';

  @override
  String get itemSellPrice => 'Harga jual';

  @override
  String get itemCost => 'Harga modal';

  @override
  String get itemUnit => 'Satuan';

  @override
  String get itemCategory => 'Kategori (opsional)';

  @override
  String get itemCategoryNone => 'Tanpa kategori';

  @override
  String get itemBarcode => 'Barcode (opsional)';

  @override
  String get itemKind => 'Jenis';

  @override
  String get itemKindGoods => 'Barang';

  @override
  String get itemKindService => 'Jasa';

  @override
  String get itemInitialStock => 'Stok awal (opsional)';

  @override
  String get itemAllowNegative => 'Boleh stok minus';

  @override
  String get itemAllowNegativeHint =>
      'Barang tetap bisa dijual walau stok belum diisi; stok jadi minus sampai ada pembelian.';

  @override
  String get itemStockHint =>
      'Tanpa stok awal dan tanpa stok minus, barang belum bisa dijual sampai stok diisi.';

  @override
  String get itemServiceHint => 'Jasa tidak memakai stok.';

  @override
  String get itemSave => 'Simpan';

  @override
  String get itemSaveAndSell => 'Simpan & jual';

  @override
  String itemSaved(String name) {
    return '$name ditambahkan';
  }

  @override
  String get itemRequired => 'Wajib diisi';

  @override
  String get itemInvalidNumber => 'Angka tidak valid';

  @override
  String get itemNoUnits =>
      'Belum ada satuan. Tambahkan satuan di web terlebih dulu.';

  @override
  String get itemScanBarcode => 'Pindai barcode';

  @override
  String get itemScanned => 'Barcode terbaca';

  @override
  String get itemLoadFailed => 'Gagal memuat satuan/kategori.';

  @override
  String get reportTitle => 'Laporan penjualan';

  @override
  String get reportHint => 'Omzet, metode bayar, per hari';

  @override
  String get reportToday => 'Hari ini';

  @override
  String get report7Days => '7 hari';

  @override
  String get reportThisMonth => 'Bulan ini';

  @override
  String get reportLastMonth => 'Bulan lalu';

  @override
  String get reportCustom => 'Pilih tanggal';

  @override
  String get reportAllOutlets => 'Semua cabang';

  @override
  String get reportRevenue => 'Omzet';

  @override
  String get reportNotes => 'Nota';

  @override
  String get reportVoid => 'Batal';

  @override
  String get reportReturns => 'Retur';

  @override
  String get reportNet => 'Bersih setelah retur';

  @override
  String get reportReceivable => 'Piutang';

  @override
  String get reportByMethod => 'Per metode bayar';

  @override
  String get reportByDay => 'Per hari';

  @override
  String get reportEmpty => 'Tidak ada penjualan pada rentang ini.';

  @override
  String reportDayNotes(String count) {
    return '$count nota';
  }

  @override
  String get menuItems => 'Barang';

  @override
  String get menuPurchases => 'Pembelian';

  @override
  String get itemsTitle => 'Barang';

  @override
  String get itemsSearch => 'Cari nama, kode, atau barcode';

  @override
  String get itemsEmpty => 'Belum ada barang.';

  @override
  String get itemsTabActive => 'Aktif';

  @override
  String get itemsTabArchived => 'Diarsipkan';

  @override
  String get itemsEdit => 'Ubah barang';

  @override
  String get itemsArchive => 'Arsipkan';

  @override
  String get itemsRestore => 'Aktifkan kembali';

  @override
  String itemsArchiveAsk(String name) {
    return 'Arsipkan $name? Barang tidak muncul di kasir, riwayat transaksi tetap aman.';
  }

  @override
  String itemsArchived(String name) {
    return '$name diarsipkan';
  }

  @override
  String itemsRestored(String name) {
    return '$name diaktifkan kembali';
  }

  @override
  String itemsUpdated(String name) {
    return '$name diperbarui';
  }

  @override
  String itemsStock(String qty) {
    return 'Stok $qty';
  }

  @override
  String get itemsEditTitle => 'Ubah barang';

  @override
  String get itemsCostNote =>
      'Harga modal berubah lewat pembelian, bukan dari sini.';

  @override
  String get itemsArchivedHint =>
      'Menampilkan maksimal 30 barang diarsipkan; persempit dengan pencarian.';

  @override
  String get commonCancel => 'Batal';

  @override
  String get commonSave => 'Simpan';

  @override
  String get commonRetry => 'Coba lagi';

  @override
  String get purTitle => 'Pembelian';

  @override
  String get purAdd => 'Pembelian baru';

  @override
  String get purCashNote =>
      'Pembelian dari HP selalu tunai dan langsung menambah stok.';

  @override
  String get purSupplier => 'Pemasok';

  @override
  String get purPickSupplier => 'Pilih pemasok';

  @override
  String get purSearchSupplier => 'Cari pemasok';

  @override
  String get purNoSupplier => 'Pemasok tidak ditemukan.';

  @override
  String get purDate => 'Tanggal';

  @override
  String get purInvoiceNo => 'No. faktur pemasok (opsional)';

  @override
  String get purNote => 'Catatan (opsional)';

  @override
  String get purLines => 'Barang dibeli';

  @override
  String get purAddItem => 'Tambah barang';

  @override
  String get purSearchItem => 'Cari barang';

  @override
  String get purQty => 'Qty';

  @override
  String get purPrice => 'Harga beli';

  @override
  String get purSubtotal => 'Subtotal';

  @override
  String get purTax => 'PPN';

  @override
  String get purOtherCost => 'Biaya lain';

  @override
  String get purTotal => 'Total tunai';

  @override
  String get purSave => 'Simpan pembelian';

  @override
  String purSaved(String doc) {
    return 'Pembelian $doc tersimpan, stok bertambah';
  }

  @override
  String get purNoLines => 'Tambah minimal satu barang.';

  @override
  String get purNeedSupplier => 'Pilih pemasok dulu.';

  @override
  String get purBadLine =>
      'Qty harus lebih dari 0 dan harga tidak boleh kosong.';

  @override
  String get purEmpty => 'Belum ada pembelian pada rentang ini.';

  @override
  String purSummary(String count, String total) {
    return '$count pembelian, $total';
  }

  @override
  String get purRange30 => '30 hari';

  @override
  String get purDetailTitle => 'Detail pembelian';

  @override
  String get purEdit => 'Ubah';

  @override
  String get purEditTitle => 'Ubah pembelian';

  @override
  String get purVoid => 'Batalkan';

  @override
  String purVoidAsk(String doc) {
    return 'Batalkan pembelian $doc? Stok akan dikurangi lagi.';
  }

  @override
  String get purReason => 'Alasan (minimal 3 huruf)';

  @override
  String get purVoided => 'Pembelian dibatalkan';

  @override
  String get purEdited => 'Pembelian diperbarui';

  @override
  String get purStatusVoid => 'BATAL';

  @override
  String get purStatusRevised => 'DIREVISI';

  @override
  String purVoidReason(String reason) {
    return 'Alasan batal: $reason';
  }

  @override
  String purBy(String name) {
    return 'Dicatat oleh $name';
  }

  @override
  String get errorPurchaseNotEditable =>
      'Nota ini tidak dapat diubah lagi (sudah dibatalkan atau digantikan revisi).';

  @override
  String get errorPurchaseHasReturns =>
      'Nota ini sudah diretur; batalkan returnya dulu.';

  @override
  String get errorItemUnavailable => 'Barang tidak tersedia untuk pembelian.';

  @override
  String get errorCodeTaken => 'Kode barang sudah dipakai.';

  @override
  String get errorEditWindowClosed => 'Batas waktu edit sudah lewat.';

  @override
  String get errorStockReverse =>
      'Stok tidak cukup untuk membalik nota ini (sebagian sudah terjual).';

  @override
  String get menuSettlement => 'Settlement';

  @override
  String get setTitle => 'Settlement EraPay';

  @override
  String get setCheck => 'Cek ke EraPay';

  @override
  String get setChecking => 'Mengecek ke EraPay...';

  @override
  String get setArus => 'UANG ARUS';

  @override
  String get setEra => 'UANG ERAPAY';

  @override
  String get setArusHint => 'QRIS pada nota aktif';

  @override
  String get setEraHint => 'Tagihan lunas di EraPay';

  @override
  String get setMatchedAll => 'Cocok: uang ARUS sama dengan uang EraPay.';

  @override
  String setDiffMore(String amount) {
    return 'Selisih $amount: ada uang lunas di EraPay yang belum tercatat sebagai nota aktif.';
  }

  @override
  String setDiffLess(String amount) {
    return 'Selisih $amount: ada nota QRIS yang belum lunas di EraPay.';
  }

  @override
  String setCount(String count) {
    return '$count transaksi';
  }

  @override
  String get setFilterAll => 'Semua';

  @override
  String get setFilterAttention => 'Perlu dicek';

  @override
  String get setFilterWaiting => 'Menunggu';

  @override
  String get setFilterMatched => 'Cocok';

  @override
  String get setColArus => 'ARUS';

  @override
  String get setColEra => 'ERAPAY';

  @override
  String get setStatusLabel => 'Status';

  @override
  String get setIdLabel => 'ID EraPay';

  @override
  String get setIdCopied => 'ID disalin';

  @override
  String get setNoId => 'Belum ada ID';

  @override
  String get setStateMatched => 'Cocok';

  @override
  String get setStateMatchedSub => 'Lunas di EraPay dan tercatat di nota';

  @override
  String get setStateWaiting => 'Menunggu';

  @override
  String get setStateWaitingSub => 'EraPay belum memutuskan (QR belum dibayar)';

  @override
  String get setStateOrphan => 'Belum tercatat';

  @override
  String get setStateOrphanSub => 'Lunas di EraPay tetapi belum jadi nota';

  @override
  String get setStateRefund => 'Perlu refund';

  @override
  String get setStateRefundSub => 'Nota dibatalkan, uang masih di EraPay';

  @override
  String get setStateUnpaid => 'Tidak lunas';

  @override
  String get setStateUnpaidSub =>
      'Nota ada tetapi EraPay gagal atau kedaluwarsa';

  @override
  String get setStateClosed => 'Tidak dibayar';

  @override
  String get setStateClosedSub =>
      'Tagihan gagal atau kedaluwarsa, tidak ada uang';

  @override
  String get setEraSucceeded => 'Lunas';

  @override
  String get setEraPending => 'Menunggu';

  @override
  String get setEraFailed => 'Gagal';

  @override
  String get setEraExpired => 'Kedaluwarsa';

  @override
  String get setEraCancelled => 'Dibatalkan';

  @override
  String get setEraUnknown => 'Belum pasti';

  @override
  String get setNotConfigured =>
      'EraPay belum dipasang untuk toko ini. Pasang API key di Metode Pembayaran (web).';

  @override
  String get setEmpty => 'Tidak ada transaksi QRIS EraPay pada rentang ini.';

  @override
  String setRefreshed(String count) {
    return 'Status diperbarui dari EraPay: $count transaksi';
  }

  @override
  String get setTruncated =>
      'Daftar dipotong 300 transaksi terbaru; persempit rentang tanggal.';

  @override
  String get setNote =>
      'Uang EraPay = tagihan berstatus lunas di EraPay. Pencairan ke rekening bank mengikuti jadwal EraPay dan belum tersedia di sini.';
}
