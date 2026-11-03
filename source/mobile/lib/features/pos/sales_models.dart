import 'package:decimal/decimal.dart';

import '../../core/money/money.dart';

/// Satu baris daftar penjualan kasir (`GET /sales/`).
class SaleRow {
  const SaleRow({
    required this.id,
    required this.docNo,
    required this.status,
    required this.createdAt,
    required this.total,
    required this.surcharge,
    required this.receivable,
    required this.returned,
    required this.payNames,
  });

  final String id;
  final String docNo;
  final String status;
  final DateTime createdAt;
  final Decimal total;
  final Decimal surcharge;
  final Decimal receivable;
  final Decimal returned;
  final List<String> payNames;

  bool get isVoid => status == 'void';

  factory SaleRow.fromJson(Map<String, dynamic> j) => SaleRow(
    id: '${j['id']}',
    docNo: '${j['doc_no'] ?? ''}',
    status: '${j['status'] ?? ''}',
    createdAt:
        DateTime.tryParse('${j['created_at']}')?.toLocal() ?? DateTime.now(),
    total: dec(j['total']),
    surcharge: dec(j['surcharge']),
    receivable: dec(j['receivable']),
    returned: dec(j['returned']),
    payNames: (j['pays'] as List? ?? const [])
        .map((e) => '${(e as Map)['name'] ?? ''}')
        .where((s) => s.isNotEmpty)
        .toList(),
  );
}

class MethodTotal {
  const MethodTotal({required this.name, required this.amount, this.kind = ''});

  final String name;
  final Decimal amount;
  final String kind;

  factory MethodTotal.fromJson(Map<String, dynamic> j) => MethodTotal(
    name: '${j['name'] ?? ''}',
    amount: dec(j['amount']),
    kind: '${j['kind'] ?? ''}',
  );
}

/// Uang lain yang lewat kasir ini pada rentang (bayar piutang, retur, kas kecil, ...); [amount] bertanda (+ masuk).
class FlowLine {
  const FlowLine({
    required this.source,
    required this.name,
    required this.amount,
    required this.count,
  });

  final String source;
  final String name;
  final Decimal amount;
  final int count;

  factory FlowLine.fromJson(Map<String, dynamic> j) => FlowLine(
    source: '${j['source'] ?? ''}',
    name: '${j['name'] ?? ''}',
    amount: dec(j['amount']),
    count: (j['count'] as num?)?.toInt() ?? 0,
  );
}

/// Daftar penjualan satu kasir pada satu rentang waktu (zona waktu outlet ditentukan server).
class SalesToday {
  const SalesToday({
    required this.rows,
    required this.total,
    required this.byMethod,
    required this.truncated,
    this.from = '',
    this.to = '',
    this.surcharge,
    this.credit,
    this.drawer = const [],
    this.flows = const [],
    this.returnsCount = 0,
    this.returnsTotal,
    this.netTotal,
  });

  final List<SaleRow> rows;
  final Decimal total;
  final List<MethodTotal> byMethod;
  final bool truncated;

  /// Batas rentang seperti dipakai server: `YYYY-MM-DD` atau `YYYY-MM-DDTHH:MM`.
  final String from;
  final String to;
  final Decimal? surcharge;
  final Decimal? credit;

  /// Uang yang seharusnya ada per metode (penjualan + uang lain): bahan cek fisik.
  final List<MethodTotal> drawer;
  final List<FlowLine> flows;
  final int returnsCount;
  final Decimal? returnsTotal;
  final Decimal? netTotal;

  int get activeCount => rows.where((r) => !r.isVoid).length;
  int get voidCount => rows.length - activeCount;

  /// Jumlah nota aktif yang memakai metode bernama [name].
  int notesFor(String name) =>
      rows.where((r) => !r.isVoid && r.payNames.contains(name)).length;

  factory SalesToday.fromJson(Map<String, dynamic> j) {
    List<MethodTotal> methods(Object? v) => (v as List? ?? const [])
        .map((e) => MethodTotal.fromJson((e as Map).cast<String, dynamic>()))
        .toList();
    final returns = j['returns'] is Map
        ? (j['returns'] as Map).cast<String, dynamic>()
        : null;
    final totals = j['totals'] is Map
        ? (j['totals'] as Map).cast<String, dynamic>()
        : const <String, dynamic>{};
    return SalesToday(
      rows: (j['data'] as List? ?? const [])
          .map((e) => SaleRow.fromJson((e as Map).cast<String, dynamic>()))
          .toList(),
      total: dec(j['total']),
      byMethod: methods(j['by_method']),
      truncated: j['truncated'] == true,
      from: '${j['from'] ?? ''}',
      to: '${j['to'] ?? ''}',
      surcharge: j['surcharge'] == null ? null : dec(j['surcharge']),
      credit: totals['credit'] == null ? null : dec(totals['credit']),
      drawer: methods(j['drawer']),
      flows: (j['flows'] as List? ?? const [])
          .map((e) => FlowLine.fromJson((e as Map).cast<String, dynamic>()))
          .toList(),
      returnsCount: (returns?['count'] as num?)?.toInt() ?? 0,
      returnsTotal: returns == null ? null : dec(returns['total']),
      netTotal: j['net_total'] == null ? null : dec(j['net_total']),
    );
  }
}

class SaleDetailLine {
  const SaleDetailLine({
    required this.name,
    required this.unit,
    required this.qty,
    required this.unitPrice,
    required this.discount,
    required this.lineTotal,
  });

  final String name;
  final String unit;
  final Decimal qty;
  final Decimal unitPrice;
  final Decimal discount;
  final Decimal lineTotal;

  factory SaleDetailLine.fromJson(Map<String, dynamic> j) => SaleDetailLine(
    name: '${j['name'] ?? ''}',
    unit: '${j['unit'] ?? ''}',
    qty: dec(j['qty']),
    unitPrice: dec(j['unit_price']),
    discount: dec(j['discount']),
    lineTotal: dec(j['line_total']),
  );
}

class SalePayLine {
  const SalePayLine({required this.name, required this.amount});

  final String name;
  final Decimal amount;
}

/// Nota lengkap (`GET /sales/{id}`).
class SaleDetailInfo {
  const SaleDetailInfo({
    required this.docNo,
    required this.status,
    required this.cashier,
    required this.member,
    required this.lines,
    required this.payments,
    required this.subtotal,
    required this.discount,
    required this.tax,
    required this.otherCost,
    required this.total,
    required this.paid,
    required this.change,
    required this.receivable,
    required this.voidReason,
  });

  final String docNo;
  final String status;
  final String cashier;
  final String member;
  final List<SaleDetailLine> lines;
  final List<SalePayLine> payments;
  final Decimal subtotal;
  final Decimal discount;
  final Decimal tax;
  final Decimal otherCost;
  final Decimal total;
  final Decimal paid;
  final Decimal change;
  final Decimal receivable;
  final String voidReason;

  factory SaleDetailInfo.fromJson(Map<String, dynamic> j) => SaleDetailInfo(
    docNo: '${j['doc_no'] ?? ''}',
    status: '${j['status'] ?? ''}',
    cashier: '${j['cashier'] ?? ''}',
    member: j['member'] is Map ? '${(j['member'] as Map)['name'] ?? ''}' : '',
    lines: (j['lines'] as List? ?? const [])
        .map((e) => SaleDetailLine.fromJson((e as Map).cast<String, dynamic>()))
        .toList(),
    payments: (j['payments'] as List? ?? const [])
        .map(
          (e) => SalePayLine(
            name: '${(e as Map)['method_name'] ?? ''}',
            amount: dec(e['amount']),
          ),
        )
        .toList(),
    subtotal: dec(j['subtotal']),
    discount: dec(j['discount']),
    tax: dec(j['tax_store']) + dec(j['tax_gov']),
    otherCost: dec(j['other_cost']),
    total: dec(j['total']),
    paid: dec(j['paid']),
    change: dec(j['change']),
    receivable: dec(j['receivable']),
    voidReason: '${j['void_reason'] ?? ''}',
  );
}
