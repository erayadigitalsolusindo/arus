import 'package:decimal/decimal.dart';

import '../../core/money/money.dart';

/// Satu baris rekap tutup shift: uang yang seharusnya ada per metode bayar (dihitung SERVER).
class ShiftCount {
  const ShiftCount({
    required this.methodId,
    required this.name,
    required this.kind,
    required this.opening,
    required this.sales,
    required this.flows,
    required this.expected,
  });

  final String methodId;
  final String name;
  final String kind;
  final Decimal opening;
  final Decimal sales;
  final Decimal flows;
  final Decimal expected;

  factory ShiftCount.fromJson(Map<String, dynamic> j) => ShiftCount(
    methodId: '${j['method_id']}',
    name: '${j['name'] ?? ''}',
    kind: '${j['kind'] ?? ''}',
    opening: dec(j['opening']),
    sales: dec(j['sales']),
    flows: dec(j['flows']),
    expected: dec(j['expected']),
  );
}

/// Rekap shift (`GET /shifts/{id}`) atau hasil tutup (`POST /shifts/{id}/close`).
class ShiftRecap {
  const ShiftRecap({
    required this.id,
    required this.docNo,
    required this.closed,
    required this.openedAt,
    required this.saleCount,
    required this.voidCount,
    required this.salesTotal,
    required this.receivableTotal,
    required this.expectedTotal,
    required this.countedTotal,
    required this.diffTotal,
    required this.counts,
  });

  final String id;
  final String docNo;
  final bool closed;
  final DateTime openedAt;
  final int saleCount;
  final int voidCount;
  final Decimal salesTotal;
  final Decimal receivableTotal;
  final Decimal expectedTotal;
  final Decimal? countedTotal;
  final Decimal? diffTotal;
  final List<ShiftCount> counts;

  factory ShiftRecap.fromJson(Map<String, dynamic> j) => ShiftRecap(
    id: '${j['id']}',
    docNo: '${j['doc_no'] ?? ''}',
    closed: j['status'] == 'closed',
    openedAt:
        DateTime.tryParse('${j['opened_at']}')?.toLocal() ?? DateTime.now(),
    saleCount: (j['sale_count'] as num?)?.toInt() ?? 0,
    voidCount: (j['void_count'] as num?)?.toInt() ?? 0,
    salesTotal: dec(j['sales_total']),
    receivableTotal: dec(j['receivable_total']),
    expectedTotal: dec(j['expected_total']),
    countedTotal: j['counted_total'] == null ? null : dec(j['counted_total']),
    diffTotal: j['diff_total'] == null ? null : dec(j['diff_total']),
    counts: (j['counts'] as List? ?? const [])
        .map((e) => ShiftCount.fromJson((e as Map).cast<String, dynamic>()))
        .toList(),
  );
}

/// Penyetuju selisih kas (Owner/Supervisor dengan izin `shift_close.approve`).
class Approver {
  const Approver({required this.id, required this.name});

  final String id;
  final String name;

  factory Approver.fromJson(Map<String, dynamic> j) =>
      Approver(id: '${j['id']}', name: '${j['name'] ?? ''}');
}
