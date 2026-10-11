import 'package:arus_mobile/core/printing/escpos.dart';
import 'package:arus_mobile/core/printing/receipt_layout.dart';
import 'package:arus_mobile/core/printing/receipt_model.dart';
import 'package:arus_mobile/l10n/gen/app_localizations_id.dart';
import 'package:flutter_test/flutter_test.dart';

Map<String, dynamic> _receipt({String status = 'completed'}) => {
  'local_time': '11/10/2026 09:30',
  'store': {
    'tenant_name': 'Toko Maju',
    'outlet_name': 'Cabang Satu',
    'address': 'Jl. Merdeka No. 1',
    'phone': '0812',
    'header': '',
    'footer': 'Terima kasih',
  },
  'sale': {
    'doc_no': 'INV-001',
    'status': status,
    'cashier': 'Budi',
    'lines': [
      {
        'name': 'Kopi Susu',
        'unit': 'pcs',
        'qty': '2.000',
        'unit_price': '15000.00',
        'discount': '1000.00',
        'line_total': '29000.00',
      },
    ],
    'payments': [
      {
        'method_name': 'Tunai',
        'amount': '50000.00',
        'ref_no': '',
        'fee': '0',
        'fee_bearer': 'store',
      },
    ],
    'subtotal': '29000.00',
    'discount': '0',
    'redeem_amount': '0',
    'tax_store': '0',
    'tax_gov': '0',
    'other_cost': '0',
    'total': '29000.00',
    'surcharge': '0',
    'receivable': '0',
    'change': '21000.00',
    'vouchers': [],
    'other_costs': [],
    'points_earned': 0,
    'points_redeemed': 0,
  },
};

void main() {
  final l = AppLocalizationsId();

  test('struk 58 mm: lebar 32 kolom, total & kembalian benar', () {
    final lines = receiptLines(
      ReceiptData.fromJson(_receipt()),
      Paper.mm58,
      l,
    );
    for (final ln in lines) {
      expect(ln.text.length, lessThanOrEqualTo(32), reason: ln.text);
    }
    final texts = lines.map((e) => e.text.trim()).toList();
    expect(texts.first, 'Toko Maju');
    expect(lines.first.title, isTrue);
    expect(texts, contains('Cabang Satu'));
    expect(texts.any((t) => t.startsWith('TOTAL') && t.endsWith('29.000')), isTrue);
    expect(texts.any((t) => t.contains('2 pcs x 15.000') && t.endsWith('29.000')), isTrue);
    expect(texts.any((t) => t.startsWith('Potongan') && t.endsWith('-1.000')), isTrue);
    expect(texts.any((t) => t.startsWith('Kembali') && t.endsWith('21.000')), isTrue);
    expect(texts.any((t) => t.startsWith('Anda hemat') && t.endsWith('1.000')), isTrue);
    expect(texts.last, 'Terima kasih');
  });

  test('cetak ulang dan nota batal ditandai', () {
    final lines = receiptLines(
      ReceiptData.fromJson(_receipt(status: 'void')),
      Paper.mm80,
      l,
      copy: 2,
    );
    final texts = lines.map((e) => e.text.trim()).toList();
    expect(texts, contains('*** SALINAN KE-2 ***'));
    expect(texts, contains('*** NOTA DIBATALKAN ***'));
    for (final ln in lines) {
      expect(ln.text.length, lessThanOrEqualTo(48));
    }
  });

  test('ESC/POS: reset, judul ganda, potong; non-ASCII dilipat', () {
    final bytes = escposReceipt(
      [
        const ReceiptLine('  Café  ', title: true),
        const ReceiptLine('TOTAL 1', bold: true, big: true),
      ],
      cols: 32,
      cut: true,
    );
    expect(bytes.sublist(0, 2), [0x1b, 0x40]);
    // Judul: ESC a 1, ESC E 1, GS ! 0x11 lalu "Cafe".
    expect(
      String.fromCharCodes(bytes),
      contains('Cafe'),
    );
    expect(bytes.sublist(bytes.length - 4), [0x1d, 0x56, 0x42, 0]);
    expect(toPrinterAscii('Rp 1.000 – “ok” ñ'), 'Rp 1.000 - "ok" n');
  });
}
