import 'package:arus_mobile/core/printing/recap_layout.dart';
import 'package:arus_mobile/core/printing/receipt_layout.dart';
import 'package:arus_mobile/features/pos/sales_models.dart';
import 'package:arus_mobile/l10n/gen/app_localizations_id.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'rekap cetak memuat rentang jam-menit, metode, uang lain, dan isi laci',
    () {
      final d = SalesToday.fromJson({
        'from': '2026-11-03T08:30',
        'to': '2026-11-03T14:45',
        'total': '17000.00',
        'surcharge': '0',
        'data': [
          {
            'id': 'a',
            'doc_no': 'N-1',
            'status': 'completed',
            'created_at': '2026-11-03T02:00:00Z',
            'total': '12000.00',
            'pays': [
              {'name': 'Tunai'},
            ],
          },
          {
            'id': 'b',
            'doc_no': 'N-2',
            'status': 'completed',
            'created_at': '2026-11-03T03:00:00Z',
            'total': '5000.00',
            'pays': [
              {'name': 'QRIS'},
            ],
          },
          {'id': 'c', 'doc_no': 'N-3', 'status': 'void', 'total': '999.00'},
        ],
        'by_method': [
          {'name': 'Tunai', 'kind': 'cash', 'amount': '12000.00'},
          {'name': 'QRIS', 'kind': 'ewallet', 'amount': '5000.00'},
        ],
        'drawer': [
          {'name': 'Tunai', 'kind': 'cash', 'amount': '17000.00'},
          {'name': 'QRIS', 'kind': 'ewallet', 'amount': '5000.00'},
        ],
        'flows': [
          {
            'source': 'petty_cash_in',
            'name': 'Tunai',
            'amount': '5000.00',
            'count': 1,
          },
        ],
      });
      final lines = recapLines(
        d,
        Paper.mm58,
        AppLocalizationsId(),
        tenant: 'Toko Maju',
        outlet: 'Pusat',
        cashier: 'Budi',
      ).map((e) => e.text).toList();
      final all = lines.join('\n');
      expect(all, contains('REKAP PENJUALAN'));
      expect(all, contains('03/11/2026 08:30'));
      expect(all, contains('03/11/2026 14:45'));
      expect(all, contains('Tunai (1)'));
      expect(all, contains('QRIS (1)'));
      expect(all, contains('Nota batal'));
      expect(all, contains('Kas kecil masuk - Tunai'));
      expect(all, contains('Seharusnya ada (cek fisik)'));
      expect(lines.every((t) => t.length <= Paper.mm58.cols), isTrue);
    },
  );
}
