import 'package:decimal/decimal.dart';

import '../../core/money/money.dart';

/// Barang di hasil pencarian kasir (`GET /items/search`). Harga = harga efektif outlet aktif.
class PosItem {
  const PosItem({
    required this.id,
    required this.sku,
    required this.barcode,
    required this.name,
    required this.unit,
    required this.price,
    required this.stockDisplay,
    required this.isGoods,
    this.origin = '',
  });

  final String id;
  final String sku;
  final String barcode;
  final String name;
  final String unit;
  final Decimal price;
  final Decimal stockDisplay;
  final bool isGoods;
  final String origin;

  factory PosItem.fromJson(Map<String, dynamic> j) => PosItem(
        id: '${j['id']}',
        sku: '${j['sku'] ?? ''}',
        barcode: '${j['barcode'] ?? ''}',
        name: '${j['name'] ?? ''}',
        unit: '${j['unit'] ?? ''}',
        price: dec(j['price']),
        stockDisplay: dec((j['stock'] as Map?)?['display']),
        isGoods: j['kind'] != 'service',
        origin: '${j['origin'] ?? ''}',
      );
}

class CartLine {
  const CartLine({required this.item, required this.qty});

  final PosItem item;
  final Decimal qty;

  CartLine withQty(Decimal q) => CartLine(item: item, qty: q);
}

/// Satu baris hasil hitung server. Hanya field yang ditampilkan kasir.
class QuoteLine {
  const QuoteLine({required this.unitPrice, required this.lineTotal, this.issue, this.available});

  final Decimal unitPrice;
  final Decimal lineTotal;

  /// `STOCK_INSUFFICIENT` | `BELOW_COST` | null
  final String? issue;
  final Decimal? available;

  factory QuoteLine.fromJson(Map<String, dynamic> j) => QuoteLine(
        unitPrice: dec(j['unit_price']),
        lineTotal: dec(j['line_total']),
        issue: j['issue'] as String?,
        available: j['available'] == null ? null : dec(j['available']),
      );
}

/// Hasil `POST /sales/quote` — SERVER yang menghitung; klien hanya menampilkan.
class Quote {
  const Quote({
    required this.lines,
    required this.subtotal,
    required this.discount,
    required this.tax,
    required this.otherCost,
    required this.total,
  });

  final List<QuoteLine> lines;
  final Decimal subtotal;
  final Decimal discount;
  final Decimal tax;
  final Decimal otherCost;
  final Decimal total;

  bool get hasBlockingIssue => lines.any((l) => l.issue != null);

  factory Quote.fromJson(Map<String, dynamic> j) => Quote(
        lines: (j['lines'] as List? ?? const []).map((e) => QuoteLine.fromJson((e as Map).cast<String, dynamic>())).toList(),
        subtotal: dec(j['subtotal']),
        discount: dec(j['discount']),
        tax: dec(j['tax_store']) + dec(j['tax_gov']),
        otherCost: dec(j['other_cost']),
        total: dec(j['total']),
      );
}

/// Metode pembayaran dari master (`GET /payment-methods/lookup?for=sale`).
class PayMethodInfo {
  const PayMethodInfo({required this.id, required this.name, required this.kind});

  final String id;
  final String name;
  final String kind;

  bool get isCash => kind == 'cash';

  factory PayMethodInfo.fromJson(Map<String, dynamic> j) => PayMethodInfo(id: '${j['id']}', name: '${j['name']}', kind: '${j['kind']}');
}

class ShiftInfo {
  const ShiftInfo({required this.id, required this.docNo, required this.openedAt, required this.openingCash});

  final String id;
  final String docNo;
  final DateTime openedAt;
  final Decimal openingCash;

  factory ShiftInfo.fromJson(Map<String, dynamic> j) => ShiftInfo(
        id: '${j['id']}',
        docNo: '${j['doc_no'] ?? ''}',
        openedAt: DateTime.tryParse('${j['opened_at']}')?.toLocal() ?? DateTime.now(),
        openingCash: dec(j['opening_cash']),
      );
}

/// Pembayaran yang dikirim ke server.
class PaymentInput {
  const PaymentInput({required this.methodId, required this.amount, this.refNo = ''});

  final String methodId;
  final Decimal amount;
  final String refNo;

  Map<String, dynamic> toJson() => {
        'method_id': methodId,
        'amount': decToApi(amount, scale: 2),
        if (refNo.isNotEmpty) 'ref_no': refNo,
      };
}

/// Nota yang tersimpan (ringkas).
class SaleResult {
  const SaleResult({required this.id, required this.docNo, required this.total, required this.change});

  final String id;
  final String docNo;
  final Decimal total;
  final Decimal change;

  factory SaleResult.fromJson(Map<String, dynamic> j) =>
      SaleResult(id: '${j['id']}', docNo: '${j['doc_no']}', total: dec(j['total']), change: dec(j['change']));
}
