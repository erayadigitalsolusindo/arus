import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:print_bluetooth_thermal/print_bluetooth_thermal.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../l10n/gen/app_localizations.dart';
import '../api/api_error.dart';
import '../session/session_controller.dart';
import 'escpos.dart';
import 'receipt_layout.dart';
import 'receipt_model.dart';

/// Pengaturan printer — per perangkat (HP kasir), bukan per akun: tiap HP bisa punya printer berbeda.
@immutable
class PrinterSettings {
  const PrinterSettings({
    this.address = '',
    this.name = '',
    this.paper = Paper.mm58,
    this.cut = true,
    this.auto = false,
  });

  /// Alamat MAC printer Bluetooth yang sudah dipasangkan di Pengaturan Android; kosong = belum dipilih.
  final String address;
  final String name;
  final Paper paper;
  final bool cut;

  /// Cetak otomatis setelah bayar.
  final bool auto;

  bool get hasPrinter => address.isNotEmpty;

  PrinterSettings copyWith({
    String? address,
    String? name,
    Paper? paper,
    bool? cut,
    bool? auto,
  }) => PrinterSettings(
    address: address ?? this.address,
    name: name ?? this.name,
    paper: paper ?? this.paper,
    cut: cut ?? this.cut,
    auto: auto ?? this.auto,
  );
}

final printerSettingsProvider =
    AsyncNotifierProvider<PrinterSettingsController, PrinterSettings>(
      PrinterSettingsController.new,
    );

class PrinterSettingsController extends AsyncNotifier<PrinterSettings> {
  static const _kAddress = 'print.address';
  static const _kName = 'print.name';
  static const _kPaper = 'print.paper';
  static const _kCut = 'print.cut';
  static const _kAuto = 'print.auto';

  @override
  Future<PrinterSettings> build() async {
    try {
      final p = await SharedPreferences.getInstance();
      return PrinterSettings(
        address: p.getString(_kAddress) ?? '',
        name: p.getString(_kName) ?? '',
        paper: p.getInt(_kPaper) == 80 ? Paper.mm80 : Paper.mm58,
        cut: p.getBool(_kCut) ?? true,
        auto: p.getBool(_kAuto) ?? false,
      );
    } catch (_) {
      return const PrinterSettings();
    }
  }

  Future<void> save(PrinterSettings s) async {
    state = AsyncData(s);
    try {
      final p = await SharedPreferences.getInstance();
      await p.setString(_kAddress, s.address);
      await p.setString(_kName, s.name);
      await p.setInt(_kPaper, s.paper.mm);
      await p.setBool(_kCut, s.cut);
      await p.setBool(_kAuto, s.auto);
    } catch (_) {
      // Penyimpanan tidak tersedia: pengaturan berlaku sampai aplikasi ditutup.
    }
  }
}

enum PrinterFailure { notConfigured, permission, bluetoothOff, connect, write }

/// Galat cetak yang bisa ditampilkan ke kasir (kode → teks lewat [printErrorMessage]).
class PrinterException implements Exception {
  PrinterException(this.failure);

  final PrinterFailure failure;
}

/// Pesan galat cetak untuk kasir.
String printErrorMessage(AppLocalizations l, Object e) {
  if (e is PrinterException) {
    return switch (e.failure) {
      PrinterFailure.notConfigured => l.printerNotConfigured,
      PrinterFailure.permission => l.printerPermissionDenied,
      PrinterFailure.bluetoothOff => l.printerBluetoothOff,
      PrinterFailure.connect => l.printerConnectFailed,
      PrinterFailure.write => l.printerWriteFailed,
    };
  }
  if (e is ApiError) return l.printFailed(e.message(l));
  if (e is PlatformException) {
    return l.printFailed(e.message ?? e.code);
  }
  return l.printFailed('$e');
}

final receiptPrinterProvider = Provider<ReceiptPrinter>(ReceiptPrinter.new);

/// Cetak struk ke printer thermal Bluetooth (Classic SPP). Satu koneksi dipertahankan antar cetakan dan dibuka ulang
/// bila putus (printer tidur / keluar jangkauan).
class ReceiptPrinter {
  ReceiptPrinter(this._ref);

  final Ref _ref;
  String? _connected;

  /// Izin Bluetooth + Bluetooth menyala. Dipanggil sebelum membaca daftar printer atau mencetak.
  Future<void> ensureReady() async {
    // Plugin print_bluetooth_thermal menolak SEMUA panggilan di Android 12+ bila BLUETOOTH_CONNECT atau
    // BLUETOOTH_SCAN belum diberikan; getter ini meminta keduanya (dialog "Perangkat di sekitar") bila belum.
    if (!await PrintBluetoothThermal.isPermissionBluetoothGranted) {
      throw PrinterException(PrinterFailure.permission);
    }
    if (!await PrintBluetoothThermal.bluetoothEnabled) {
      throw PrinterException(PrinterFailure.bluetoothOff);
    }
  }

  /// Printer yang sudah dipasangkan di Pengaturan Android (pairing tidak dilakukan dari aplikasi).
  Future<List<BluetoothInfo>> paired() async {
    await ensureReady();
    return PrintBluetoothThermal.pairedBluetooths;
  }

  Future<void> _connect(String address) async {
    if (_connected == address && await PrintBluetoothThermal.connectionStatus) {
      return;
    }
    await PrintBluetoothThermal.disconnect;
    _connected = null;
    final ok = await PrintBluetoothThermal.connect(macPrinterAddress: address);
    if (!ok) throw PrinterException(PrinterFailure.connect);
    _connected = address;
    // Printer murah belum siap menerima data persis setelah soket SPP terbuka.
    await Future<void>.delayed(const Duration(milliseconds: 500));
  }

  Future<void> _write(String address, Uint8List data) async {
    // Plugin menambah satu LF di depan tiap panggilan writeBytes dan memecah sendiri per 16 KB, jadi struk
    // (teks saja, biasanya < 4 KB) dikirim sekali jalan. Hanya data besar yang dipecah.
    const chunk = 8192;
    for (var i = 0; i < data.length; i += chunk) {
      final end = i + chunk > data.length ? data.length : i + chunk;
      // Harus List<int> biasa: Uint8List tiba di Android sebagai ByteArray, dan plugin (`as? List<Int>`) diam-diam
      // mengembalikan false tanpa mengirim apa pun.
      final ok = await PrintBluetoothThermal.writeBytes(
        data.sublist(i, end).toList(),
      );
      if (!ok) throw PrinterException(PrinterFailure.write);
      if (end < data.length) {
        await Future<void>.delayed(const Duration(milliseconds: 100));
      }
    }
  }

  /// Kirim byte mentah ke printer terpilih; satu kali coba ulang dengan koneksi baru bila putus di tengah jalan.
  Future<void> sendBytes(PrinterSettings s, Uint8List data) async {
    if (!s.hasPrinter) throw PrinterException(PrinterFailure.notConfigured);
    await ensureReady();
    try {
      await _connect(s.address);
      await _write(s.address, data);
    } on PrinterException catch (e) {
      if (e.failure != PrinterFailure.write) rethrow;
      _connected = null;
      await PrintBluetoothThermal.disconnect;
      await Future<void>.delayed(const Duration(seconds: 1));
      await _connect(s.address);
      await _write(s.address, data);
    }
  }

  Future<PrinterSettings> _settings() =>
      _ref.read(printerSettingsProvider.future);

  Future<void> printLines(List<ReceiptLine> lines, PrinterSettings s) =>
      sendBytes(s, escposReceipt(lines, cols: s.paper.cols, cut: s.cut));

  Future<void> printTest(AppLocalizations l, {PrinterSettings? settings}) async {
    final s = settings ?? await _settings();
    await printLines(testLines(s.paper, l), s);
  }

  /// Cetak struk nota. [reprint] = cetak ulang: dicatat dulu di server (audit) sehingga struk menampilkan nomor
  /// salinan; bila pencatatan gagal, struk tidak dicetak.
  Future<void> printSale(
    String saleId,
    AppLocalizations l, {
    bool reprint = false,
  }) async {
    final s = await _settings();
    if (!s.hasPrinter) throw PrinterException(PrinterFailure.notConfigured);
    final dio = _ref.read(apiClientProvider).dio;
    try {
      var copy = 0;
      if (reprint) {
        final r = await dio.post<Map<String, dynamic>>(
          '/sales/$saleId/receipt/reprint',
        );
        copy = (r.data!['copy'] as num?)?.toInt() ?? 0;
      }
      final r = await dio.get<Map<String, dynamic>>('/sales/$saleId/receipt');
      final rc = ReceiptData.fromJson(r.data!);
      await printLines(receiptLines(rc, s.paper, l, copy: copy), s);
    } on DioException catch (e) {
      throw ApiError.fromDio(e);
    }
  }
}
