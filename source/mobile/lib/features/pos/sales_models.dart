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
  const MethodTotal({required this.name, required this.amount});

  final String name;
  final Decimal amount;
}

/// Daftar penjualan satu kasir pada satu hari (zona waktu outlet ditentukan server).
class SalesToday {
  const SalesToday({
    required this.rows,
    required this.total,
    required this.byMethod,
    required this.truncated,
  });

  final List<SaleRow> rows;
  final Decimal total;
  final List<MethodTotal> byMethod;
  final bool truncated;

  factory SalesToday.fromJson(Map<String, dynamic> j) => SalesToday(
    rows: (j['data'] as List? ?? const [])
        .map((e) => SaleRow.fromJson((e as Map).cast<String, dynamic>()))
        .toList(),
    total: dec(j['total']),
    byMethod: (j['by_method'] as List? ?? const [])
        .map(
          (e) => MethodTotal(
            name: '${(e as Map)['name'] ?? ''}',
            amount: dec(e['amount']),
          ),
        )
        .toList(),
    truncated: j['truncated'] == true,
  );
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
