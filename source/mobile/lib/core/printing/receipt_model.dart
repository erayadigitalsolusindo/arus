import 'package:decimal/decimal.dart';

import '../money/money.dart';

/// Model struk dari server (`GET /sales/{id}/receipt`) — hanya field yang dicetak.
class ReceiptStore {
  const ReceiptStore({
    required this.tenantName,
    required this.outletName,
    required this.address,
    required this.phone,
    required this.header,
    required this.footer,
  });

  final String tenantName;
  final String outletName;
  final String address;
  final String phone;
  final String header;
  final String footer;

  factory ReceiptStore.fromJson(Map<String, dynamic> j) => ReceiptStore(
    tenantName: '${j['tenant_name'] ?? ''}',
    outletName: '${j['outlet_name'] ?? ''}',
    address: '${j['address'] ?? ''}',
    phone: '${j['phone'] ?? ''}',
    header: '${j['header'] ?? ''}',
    footer: '${j['footer'] ?? ''}',
  );
}

class ReceiptLineItem {
  const ReceiptLineItem({
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

  factory ReceiptLineItem.fromJson(Map<String, dynamic> j) => ReceiptLineItem(
    name: '${j['name'] ?? ''}',
    unit: '${j['unit'] ?? ''}',
    qty: dec(j['qty']),
    unitPrice: dec(j['unit_price']),
    discount: dec(j['discount']),
    lineTotal: dec(j['line_total']),
  );
}

class ReceiptPayment {
  const ReceiptPayment({
    required this.methodName,
    required this.amount,
    required this.refNo,
    required this.fee,
    required this.feeByCustomer,
  });

  final String methodName;
  final Decimal amount;
  final String refNo;
  final Decimal fee;
  final bool feeByCustomer;

  factory ReceiptPayment.fromJson(Map<String, dynamic> j) => ReceiptPayment(
    methodName: '${j['method_name'] ?? ''}',
    amount: dec(j['amount']),
    refNo: '${j['ref_no'] ?? ''}',
    fee: dec(j['fee']),
    feeByCustomer: j['fee_bearer'] == 'customer',
  );
}

class ReceiptVoucher {
  const ReceiptVoucher({required this.code, required this.amount});

  final String code;
  final Decimal amount;
}

class ReceiptCost {
  const ReceiptCost({required this.name, required this.amount});

  final String name;
  final Decimal amount;
}

class ReceiptData {
  const ReceiptData({
    required this.store,
    required this.localTime,
    required this.docNo,
    required this.status,
    required this.cashier,
    required this.member,
    required this.salesperson,
    required this.lines,
    required this.payments,
    required this.vouchers,
    required this.otherCosts,
    required this.subtotal,
    required this.discount,
    required this.redeemAmount,
    required this.pointsRedeemed,
    required this.pointsEarned,
    required this.taxStore,
    required this.taxStorePct,
    required this.taxGov,
    required this.taxGovPct,
    required this.otherCost,
    required this.total,
    required this.surcharge,
    required this.receivable,
    required this.dueDate,
    required this.change,
    required this.note,
    this.erapayPending = false,
  });

  final ReceiptStore store;
  final String localTime;
  final String docNo;
  final String status;
  final String cashier;
  final String member;
  final String salesperson;
  final List<ReceiptLineItem> lines;
  final List<ReceiptPayment> payments;
  final List<ReceiptVoucher> vouchers;
  final List<ReceiptCost> otherCosts;
  final Decimal subtotal;
  final Decimal discount;
  final Decimal redeemAmount;
  final int pointsRedeemed;
  final int pointsEarned;
  final Decimal taxStore;
  final Decimal taxStorePct;
  final Decimal taxGov;
  final Decimal taxGovPct;
  final Decimal otherCost;
  final Decimal total;
  final Decimal surcharge;
  final Decimal receivable;

  /// Tanggal ISO (yyyy-MM-dd) atau kosong.
  final String dueDate;
  final Decimal change;
  final String note;

  /// Disimpan paksa saat EraPay tidak dapat dihubungi: pembayaran QRIS belum terverifikasi lunas.
  final bool erapayPending;

  factory ReceiptData.fromJson(Map<String, dynamic> j) {
    final s = (j['sale'] as Map).cast<String, dynamic>();
    List<Map<String, dynamic>> list(String k) => (s[k] as List? ?? const [])
        .map((e) => (e as Map).cast<String, dynamic>())
        .toList();
    String nameOf(Object? m) => m is Map ? '${m['name'] ?? ''}' : '';
    return ReceiptData(
      store: ReceiptStore.fromJson(
        ((j['store'] as Map?) ?? const {}).cast<String, dynamic>(),
      ),
      localTime: '${j['local_time'] ?? ''}',
      docNo: '${s['doc_no'] ?? ''}',
      status: '${s['status'] ?? ''}',
      cashier: '${s['cashier'] ?? ''}',
      member: nameOf(s['member']),
      salesperson: nameOf(s['salesperson']),
      lines: list('lines').map(ReceiptLineItem.fromJson).toList(),
      payments: list('payments').map(ReceiptPayment.fromJson).toList(),
      vouchers: [
        for (final v in list('vouchers'))
          ReceiptVoucher(code: '${v['code'] ?? ''}', amount: dec(v['amount'])),
      ],
      otherCosts: [
        for (final c in list('other_costs'))
          ReceiptCost(name: '${c['name'] ?? ''}', amount: dec(c['amount'])),
      ],
      subtotal: dec(s['subtotal']),
      discount: dec(s['discount']),
      redeemAmount: dec(s['redeem_amount']),
      pointsRedeemed: (s['points_redeemed'] as num?)?.toInt() ?? 0,
      pointsEarned: (s['points_earned'] as num?)?.toInt() ?? 0,
      taxStore: dec(s['tax_store']),
      taxStorePct: dec(s['tax_store_pct']),
      taxGov: dec(s['tax_gov']),
      taxGovPct: dec(s['tax_gov_pct']),
      otherCost: dec(s['other_cost']),
      total: dec(s['total']),
      surcharge: dec(s['surcharge']),
      receivable: dec(s['receivable']),
      dueDate: s['credit'] is Map
          ? '${(s['credit'] as Map)['due_date'] ?? ''}'
          : '',
      change: dec(s['change']),
      note: '${s['note'] ?? ''}',
      erapayPending: s['erapay_pending'] == true,
    );
  }
}
