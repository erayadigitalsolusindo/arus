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
    this.mainImageId,
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
  final String? mainImageId;

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
    mainImageId: j['main_image_id'] as String?,
  );
}

/// Bentuk simpan lokal (keranjang yang belum jadi transaksi). Harga di sini hanya tampilan; total tetap dari server.
extension PosItemJson on PosItem {
  Map<String, dynamic> toJson() => {
    'id': id,
    'sku': sku,
    'barcode': barcode,
    'name': name,
    'unit': unit,
    'price': price.toString(),
    'stock': {'display': stockDisplay.toString()},
    'kind': isGoods ? 'goods' : 'service',
    'origin': origin,
    'main_image_id': mainImageId,
  };
}

/// Member hasil pencarian (`GET /members/lookup`). Saldo poin/deposit yang dipakai hitung selalu dari quote server;
/// angka di sini hanya untuk daftar pilihan dan aturan nilai poin.
class MemberInfo {
  const MemberInfo({
    required this.id,
    required this.code,
    required this.name,
    this.phone = '',
    this.points = 0,
    this.level = '',
    this.pointValue,
  });

  final String id;
  final String code;
  final String name;
  final String phone;
  final int points;
  final String level;

  /// Nilai rupiah per 1 poin yang ditukar (0/kosong = poin tidak bisa ditukar).
  final Decimal? pointValue;

  factory MemberInfo.fromJson(Map<String, dynamic> j) => MemberInfo(
    id: '${j['id']}',
    code: '${j['code'] ?? ''}',
    name: '${j['name'] ?? ''}',
    phone: '${j['phone'] ?? ''}',
    points: (j['points'] as num?)?.toInt() ?? 0,
    level: '${j['level'] ?? ''}',
    pointValue: j['point_value'] == null ? null : dec(j['point_value']),
  );

  Map<String, dynamic> toJson() => {
    'id': id,
    'code': code,
    'name': name,
    'phone': phone,
    'points': points,
    'level': level,
    'point_value': pointValue?.toString(),
  };
}

/// Salesman nota (opsional; hanya label untuk laporan/komisi).
class Salesperson {
  const Salesperson({required this.id, required this.name});

  final String id;
  final String name;

  factory Salesperson.fromJson(Map<String, dynamic> j) =>
      Salesperson(id: '${j['id']}', name: '${j['name'] ?? ''}');

  Map<String, dynamic> toJson() => {'id': id, 'name': name};
}

/// Satu slot pintasan barang kasir (`GET /pos/shortcuts/`); 16 slot per kasir, tersimpan di server.
class Shortcut {
  const Shortcut({
    required this.slot,
    required this.item,
    required this.active,
  });

  final int slot;
  final PosItem item;

  /// Barang sudah dinonaktifkan: slot tetap tampil tetapi tidak bisa dijual.
  final bool active;

  factory Shortcut.fromJson(Map<String, dynamic> j) => Shortcut(
    slot: (j['slot'] as num).toInt(),
    active: j['active'] != false,
    item: PosItem(
      id: '${j['item_id']}',
      sku: '${j['sku'] ?? ''}',
      barcode: '',
      name: '${j['name'] ?? ''}',
      unit: '${j['unit'] ?? ''}',
      price: dec(j['price']),
      stockDisplay: Decimal
          .zero, // stok tidak dibawa pintasan; server memeriksa saat quote
      isGoods: j['kind'] != 'service',
      mainImageId: j['main_image_id'] as String?,
    ),
  );
}

const shortcutSlots = 16;

/// Syarat kredit member di hasil quote: limit (0 = tanpa batas), piutangnya sekarang, dan hari jatuh tempo (0 = tanpa).
class QuoteCredit {
  const QuoteCredit({
    required this.limit,
    required this.outstanding,
    required this.dueDays,
  });

  final Decimal limit;
  final Decimal outstanding;
  final int dueDays;

  factory QuoteCredit.fromJson(Map<String, dynamic> j) => QuoteCredit(
    limit: dec(j['limit']),
    outstanding: dec(j['outstanding']),
    dueDays: (j['due_days'] as num?)?.toInt() ?? 0,
  );
}

/// Member di hasil quote: saldo poin dan deposit saat ini.
class QuoteMember {
  const QuoteMember({required this.points, required this.deposit});

  final int points;
  final Decimal deposit;

  factory QuoteMember.fromJson(Map<String, dynamic> j) => QuoteMember(
    points: (j['points'] as num?)?.toInt() ?? 0,
    deposit: dec(j['deposit']),
  );
}

class CartLine {
  const CartLine({
    required this.item,
    required this.qty,
    this.overridePrice,
    this.discount,
    this.discountTotal = false,
  });

  final PosItem item;
  final Decimal qty;

  /// Harga satuan pengganti (butuh penyetuju + PIN); null = harga normal dari server.
  final Decimal? overridePrice;

  /// Potongan manual: per satuan (Rp) atau, bila [discountTotal], total baris. Butuh penyetuju + PIN.
  final Decimal? discount;
  final bool discountTotal;

  bool get adjusted => overridePrice != null || discount != null;

  CartLine withQty(Decimal q) => CartLine(
    item: item,
    qty: q,
    overridePrice: overridePrice,
    discount: discount,
    discountTotal: discountTotal,
  );

  /// Ganti harga/potongan. Argumen null = hapus.
  CartLine withAdjust({Decimal? price, Decimal? disc, bool total = false}) =>
      CartLine(
        item: item,
        qty: qty,
        overridePrice: price,
        discount: disc,
        discountTotal: total,
      );

  /// Potongan total baris (nilai rupiah) seperti yang dikirim ke server.
  Decimal? get discountForServer {
    final d = discount;
    if (d == null) return null;
    return discountTotal ? d : (d * qty).round(scale: 2);
  }
}

/// Satu baris hasil hitung server. Hanya field yang ditampilkan kasir.
class QuoteLine {
  const QuoteLine({
    required this.unitPrice,
    required this.lineTotal,
    required this.listPrice,
    this.issue,
    this.available,
  });

  final Decimal unitPrice;
  final Decimal lineTotal;

  /// Harga normal per satuan (sebelum ubah harga).
  final Decimal listPrice;

  /// `STOCK_INSUFFICIENT` | `BELOW_COST` | null
  final String? issue;
  final Decimal? available;

  factory QuoteLine.fromJson(Map<String, dynamic> j) => QuoteLine(
    unitPrice: dec(j['unit_price']),
    lineTotal: dec(j['line_total']),
    listPrice: j['list_price'] == null
        ? dec(j['unit_price'])
        : dec(j['list_price']),
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
    required this.taxStore,
    required this.taxGov,
    required this.taxStorePct,
    required this.taxGovPct,
    required this.otherCost,
    required this.total,
    this.member,
    required this.redeemAmount,
    this.pointsEarn = 0,
    this.credit,
  });

  final List<QuoteLine> lines;
  final Decimal subtotal;
  final Decimal discount;
  final Decimal taxStore;
  final Decimal taxGov;
  final Decimal taxStorePct;
  final Decimal taxGovPct;
  final Decimal otherCost;

  Decimal get tax => taxStore + taxGov;
  final Decimal total;

  /// Ada bila nota punya member.
  final QuoteMember? member;

  /// Potongan dari tukar poin (sudah termasuk di [discount]) dan poin yang akan diperoleh.
  final Decimal redeemAmount;
  final int pointsEarn;

  /// Ada bila nota punya member.
  final QuoteCredit? credit;

  bool get hasBlockingIssue => lines.any((l) => l.issue != null);

  factory Quote.fromJson(Map<String, dynamic> j) => Quote(
    lines: (j['lines'] as List? ?? const [])
        .map((e) => QuoteLine.fromJson((e as Map).cast<String, dynamic>()))
        .toList(),
    subtotal: dec(j['subtotal']),
    discount: dec(j['discount']),
    taxStore: dec(j['tax_store']),
    taxGov: dec(j['tax_gov']),
    taxStorePct: dec(j['tax_store_pct']),
    taxGovPct: dec(j['tax_gov_pct']),
    otherCost: dec(j['other_cost']),
    total: dec(j['total']),
    member: j['member'] is Map
        ? QuoteMember.fromJson((j['member'] as Map).cast<String, dynamic>())
        : null,
    redeemAmount: dec(j['redeem_amount']),
    pointsEarn: (j['points_earn'] as num?)?.toInt() ?? 0,
    credit: j['credit'] is Map
        ? QuoteCredit.fromJson((j['credit'] as Map).cast<String, dynamic>())
        : null,
  );
}

/// Metode pembayaran dari master (`GET /payment-methods/lookup?for=sale`).
class PayMethodInfo {
  const PayMethodInfo({
    required this.id,
    required this.name,
    required this.kind,
    this.feePct,
    this.feeFlat,
    this.feeByCustomer = false,
  });

  final String id;
  final String name;
  final String kind;

  /// Biaya metode (persen + tetap per transaksi). Pratinjau saja; server yang menentukan.
  final Decimal? feePct;
  final Decimal? feeFlat;

  /// true = ditagihkan ke pelanggan di luar total nota (FR-POS-20); false = biaya toko.
  final bool feeByCustomer;

  /// "0,7%" / "Rp 1.000" / "0,7% + Rp 1.000" untuk label; kosong bila tanpa biaya.
  String get feeLabel => [
    if ((feePct ?? Decimal.zero) > Decimal.zero) '${formatQty(feePct!)}%',
    if ((feeFlat ?? Decimal.zero) > Decimal.zero) formatMoney(feeFlat!),
  ].join(' + ');

  bool get hasFee =>
      (feePct ?? Decimal.zero) > Decimal.zero ||
      (feeFlat ?? Decimal.zero) > Decimal.zero;

  /// Rumus server: jumlah × persen / 100 (dibulatkan sen) + biaya tetap.
  Decimal feeOf(Decimal amount) {
    if (!hasFee || amount <= Decimal.zero) return Decimal.zero;
    final pct = ((amount * (feePct ?? Decimal.zero)) / Decimal.fromInt(100))
        .toDecimal(scaleOnInfinitePrecision: 6)
        .round(scale: 2);
    return pct + (feeFlat ?? Decimal.zero);
  }

  bool get isCash => kind == 'cash';
  bool get isDeposit => kind == 'deposit';

  factory PayMethodInfo.fromJson(Map<String, dynamic> j) => PayMethodInfo(
    id: '${j['id']}',
    name: '${j['name']}',
    kind: '${j['kind']}',
    feePct: dec(j['fee_pct']),
    feeFlat: dec(j['fee_flat']),
    feeByCustomer: j['fee_bearer'] == 'customer',
  );
}

class ShiftInfo {
  const ShiftInfo({
    required this.id,
    required this.docNo,
    required this.openedAt,
    required this.openingCash,
  });

  final String id;
  final String docNo;
  final DateTime openedAt;
  final Decimal openingCash;

  factory ShiftInfo.fromJson(Map<String, dynamic> j) => ShiftInfo(
    id: '${j['id']}',
    docNo: '${j['doc_no'] ?? ''}',
    openedAt:
        DateTime.tryParse('${j['opened_at']}')?.toLocal() ?? DateTime.now(),
    openingCash: dec(j['opening_cash']),
  );
}

/// Pembayaran yang dikirim ke server.
class PaymentInput {
  const PaymentInput({
    required this.methodId,
    required this.amount,
    this.refNo = '',
  });

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
  const SaleResult({
    required this.id,
    required this.docNo,
    required this.total,
    required this.change,
    required this.surcharge,
    required this.receivable,
    this.dueDate,
  });

  final String id;
  final String docNo;
  final Decimal total;
  final Decimal change;

  /// Biaya metode bayar yang ditagihkan ke pelanggan (di luar total).
  final Decimal surcharge;

  /// Sisa yang belum dibayar (nota kredit) dan jatuh temponya (tanggal ISO; null = tanpa jatuh tempo).
  final Decimal receivable;
  final String? dueDate;

  factory SaleResult.fromJson(Map<String, dynamic> j) => SaleResult(
    id: '${j['id']}',
    docNo: '${j['doc_no']}',
    total: dec(j['total']),
    change: dec(j['change']),
    surcharge: dec(j['surcharge']),
    receivable: dec(j['receivable']),
    dueDate: (j['credit'] as Map?)?['due_date'] as String?,
  );
}

/// Biaya lain-lain nota (nama + jumlah).
class CostEntry {
  const CostEntry({required this.name, required this.amount});

  final String name;
  final Decimal amount;
}

/// Penyetuju yang sudah lolos cek PIN. PIN hanya di memori sampai nota selesai/keranjang dikosongkan (tidak disimpan).
class ApprovalGrant {
  const ApprovalGrant({
    required this.userId,
    required this.name,
    required this.pin,
  });

  final String userId;
  final String name;
  final String pin;
}

/// Isi nota yang dikirim ke server (quote/bayar). Klien tidak mengirim total.
class SaleDraft {
  const SaleDraft({
    required this.lines,
    required this.applyTax,
    this.member,
    this.redeemPoints = 0,
    this.costs = const [],
    this.approval,
    this.credit = false,
    this.salesperson,
    this.note = '',
  });

  final List<CartLine> lines;
  final bool applyTax;
  final MemberInfo? member;
  final int redeemPoints;
  final List<CostEntry> costs;

  /// Hanya diisi saat bayar (quote tidak pernah membawa PIN).
  final ApprovalGrant? approval;

  /// Nota kredit (hanya member): sisa yang belum dibayar menjadi piutang.
  final bool credit;

  /// Salesman nota; null = Umum.
  final Salesperson? salesperson;

  /// Keterangan nota (maks 200 huruf); hanya dikirim saat bayar, bukan saat quote.
  final String note;

  SaleDraft asCredit(ApprovalGrant? approval) => SaleDraft(
    lines: lines,
    applyTax: applyTax,
    member: member,
    redeemPoints: redeemPoints,
    costs: costs,
    approval: approval,
    credit: true,
    salesperson: salesperson,
    note: note,
  );
}
