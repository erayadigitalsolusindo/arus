import 'package:decimal/decimal.dart';

import '../../core/money/money.dart';

/// Satu metode bayar pada satu hari laporan.
class ReportMethod {
  const ReportMethod({
    required this.name,
    required this.kind,
    required this.amount,
  });

  final String name;
  final String kind;
  final Decimal amount;

  factory ReportMethod.fromJson(Map<String, dynamic> j) => ReportMethod(
    name: '${j['name'] ?? ''}',
    kind: '${j['kind'] ?? ''}',
    amount: dec(j['amount']),
  );
}

/// Satu hari pada laporan penjualan (`GET /reports/sales/by-date`).
class ReportDay {
  const ReportDay({
    required this.date,
    required this.saleCount,
    required this.voidCount,
    required this.total,
    required this.receivable,
    required this.returnCount,
    required this.returnTotal,
    required this.netTotal,
    required this.methods,
  });

  final String date;
  final int saleCount;
  final int voidCount;
  final Decimal total;
  final Decimal receivable;
  final int returnCount;
  final Decimal returnTotal;
  final Decimal netTotal;
  final List<ReportMethod> methods;

  factory ReportDay.fromJson(Map<String, dynamic> j) => ReportDay(
    date: '${j['date'] ?? ''}',
    saleCount: (j['sale_count'] as num?)?.toInt() ?? 0,
    voidCount: (j['void_count'] as num?)?.toInt() ?? 0,
    total: dec(j['total']),
    receivable: dec(j['receivable']),
    returnCount: (j['return_count'] as num?)?.toInt() ?? 0,
    returnTotal: dec(j['return_total']),
    netTotal: dec(j['net_total']),
    methods: (j['methods'] as List? ?? const [])
        .map((e) => ReportMethod.fromJson((e as Map).cast<String, dynamic>()))
        .toList(),
  );
}

/// Laporan penjualan ringkas satu rentang tanggal; angka total dijumlahkan dari baris harian.
class SalesReport {
  const SalesReport({
    required this.from,
    required this.to,
    required this.outlet,
    required this.days,
  });

  final String from;
  final String to;
  final String outlet;
  final List<ReportDay> days;

  int get saleCount => days.fold(0, (a, d) => a + d.saleCount);
  int get voidCount => days.fold(0, (a, d) => a + d.voidCount);
  int get returnCount => days.fold(0, (a, d) => a + d.returnCount);
  Decimal get total => days.fold(Decimal.zero, (a, d) => a + d.total);
  Decimal get receivable => days.fold(Decimal.zero, (a, d) => a + d.receivable);
  Decimal get returnTotal =>
      days.fold(Decimal.zero, (a, d) => a + d.returnTotal);
  Decimal get netTotal => days.fold(Decimal.zero, (a, d) => a + d.netTotal);

  /// Jumlah per metode bayar atas seluruh rentang, terbesar dulu (tunai tetap digabung per nama metode).
  List<ReportMethod> get byMethod {
    final sums = <String, Decimal>{};
    final kinds = <String, String>{};
    for (final d in days) {
      for (final m in d.methods) {
        sums[m.name] = (sums[m.name] ?? Decimal.zero) + m.amount;
        kinds[m.name] = m.kind;
      }
    }
    final out = [
      for (final e in sums.entries)
        ReportMethod(name: e.key, kind: kinds[e.key] ?? '', amount: e.value),
    ]..sort((a, b) => b.amount.compareTo(a.amount));
    return out;
  }

  factory SalesReport.fromJson(Map<String, dynamic> j) => SalesReport(
    from: '${j['from'] ?? ''}',
    to: '${j['to'] ?? ''}',
    outlet: '${j['outlet_name'] ?? ''}',
    days: (j['data'] as List? ?? const [])
        .map((e) => ReportDay.fromJson((e as Map).cast<String, dynamic>()))
        .toList(),
  );
}
