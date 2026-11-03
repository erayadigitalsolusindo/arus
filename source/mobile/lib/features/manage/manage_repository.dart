import 'package:decimal/decimal.dart';
import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_client.dart';
import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/session/session_controller.dart';
import '../pos/pos_models.dart';

final manageRepositoryProvider = Provider<ManageRepository>(
  (ref) => ManageRepository(ref.watch(apiClientProvider)),
);

/// Barang yang diarsipkan (tidak muncul di katalog kasir) pada daftar kelola barang.
class ArchivedItem {
  const ArchivedItem({
    required this.id,
    required this.name,
    required this.sku,
    required this.unit,
    required this.price,
  });

  final String id;
  final String name;
  final String sku;
  final String unit;
  final Decimal price;

  factory ArchivedItem.fromJson(Map<String, dynamic> j) => ArchivedItem(
    id: '${j['id']}',
    name: '${j['name'] ?? ''}',
    sku: '${j['sku'] ?? ''}',
    unit: '${j['unit'] ?? ''}',
    price: dec(j['price']),
  );
}

/// Pilihan barang di form pembelian (`GET /purchases/items`).
class PurchaseItemChoice {
  const PurchaseItemChoice({
    required this.id,
    required this.name,
    required this.sku,
    required this.unit,
    required this.lastCost,
    required this.stock,
  });

  final String id;
  final String name;
  final String sku;
  final String unit;
  final Decimal lastCost;
  final String stock;

  factory PurchaseItemChoice.fromJson(Map<String, dynamic> j) =>
      PurchaseItemChoice(
        id: '${j['id']}',
        name: '${j['name'] ?? ''}',
        sku: '${j['sku'] ?? ''}',
        unit: '${j['unit'] ?? ''}',
        lastCost: dec(j['last_cost']),
        stock: '${j['stock_total'] ?? ''}',
      );
}

/// Satu baris daftar pembelian.
class PurchaseRow {
  const PurchaseRow({
    required this.id,
    required this.docNo,
    required this.supplier,
    required this.date,
    required this.status,
    required this.total,
    required this.lines,
    required this.paymentType,
  });

  final String id;
  final String docNo;
  final String supplier;
  final String date;
  final String status;
  final Decimal total;
  final int lines;
  final String paymentType;

  bool get isActive => status == 'completed';

  factory PurchaseRow.fromJson(Map<String, dynamic> j) => PurchaseRow(
    id: '${j['id']}',
    docNo: '${j['doc_no'] ?? ''}',
    supplier: '${j['supplier_name'] ?? ''}',
    date: '${j['purchase_date'] ?? ''}',
    status: '${j['status'] ?? ''}',
    total: dec(j['total']),
    lines: (j['lines'] as num?)?.toInt() ?? 0,
    paymentType: '${j['payment_type'] ?? ''}',
  );
}

class PurchasePage {
  const PurchasePage({
    required this.rows,
    required this.nextCursor,
    required this.count,
    required this.total,
  });

  final List<PurchaseRow> rows;
  final String nextCursor;
  final int count;
  final Decimal total;
}

/// Baris pembelian di form: [discounts] dan [qtyWarehouse] hanya dibawa dari nota lama saat revisi (form sederhana
/// tidak mengubahnya, tetapi tidak boleh hilang).
class PurchaseLineDraft {
  PurchaseLineDraft({
    required this.itemId,
    required this.name,
    required this.unit,
    required this.qty,
    required this.price,
    this.discounts = const [],
    this.qtyWarehouse = '',
  });

  final String itemId;
  final String name;
  final String unit;
  Decimal qty;
  Decimal price;
  final List<String> discounts;
  final String qtyWarehouse;

  Decimal get total => qty * price;

  Map<String, dynamic> toJson() => {
    'item_id': itemId,
    'qty_display': decToApi(qty),
    if (qtyWarehouse.isNotEmpty) 'qty_warehouse': qtyWarehouse,
    'unit_price': decToApi(price, scale: 2),
    if (discounts.isNotEmpty) 'discounts': discounts,
  };
}

/// Nota pembelian lengkap (`GET /purchases/{id}`).
class PurchaseDetail {
  const PurchaseDetail({
    required this.id,
    required this.docNo,
    required this.supplierId,
    required this.supplier,
    required this.invoiceNo,
    required this.date,
    required this.status,
    required this.note,
    required this.subtotal,
    required this.taxPct,
    required this.taxAmount,
    required this.otherCost,
    required this.total,
    required this.createdBy,
    required this.lines,
    required this.costs,
    required this.voidReason,
    required this.hasReturns,
  });

  final String id;
  final String docNo;
  final String supplierId;
  final String supplier;
  final String invoiceNo;
  final String date;
  final String status;
  final String note;
  final Decimal subtotal;
  final String taxPct;
  final Decimal taxAmount;
  final Decimal otherCost;
  final Decimal total;
  final String createdBy;
  final List<PurchaseLineDraft> lines;
  final List<({String name, String amount})> costs;
  final String voidReason;
  final bool hasReturns;

  bool get isActive => status == 'completed';

  factory PurchaseDetail.fromJson(Map<String, dynamic> j) => PurchaseDetail(
    id: '${j['id']}',
    docNo: '${j['doc_no'] ?? ''}',
    supplierId: '${j['supplier_id'] ?? ''}',
    supplier: '${j['supplier_name'] ?? ''}',
    invoiceNo: '${j['supplier_invoice_no'] ?? ''}',
    date: '${j['purchase_date'] ?? ''}',
    status: '${j['status'] ?? ''}',
    note: '${j['note'] ?? ''}',
    subtotal: dec(j['subtotal']),
    taxPct: '${j['tax_pct'] ?? '0'}',
    taxAmount: dec(j['tax_amount']),
    otherCost: dec(j['other_cost']),
    total: dec(j['total']),
    createdBy: '${j['created_by'] ?? ''}',
    voidReason: '${j['void_reason'] ?? ''}',
    hasReturns: (j['returns'] as List? ?? const []).any(
      (e) => (e as Map)['status'] == 'completed',
    ),
    lines: (j['lines'] as List? ?? const []).map((e) {
      final m = (e as Map).cast<String, dynamic>();
      final wh = dec(m['qty_warehouse']);
      return PurchaseLineDraft(
        itemId: '${m['item_id']}',
        name: '${m['name'] ?? ''}',
        unit: '${m['unit'] ?? ''}',
        qty: dec(m['qty_display']),
        price: dec(m['unit_price']),
        discounts: (m['discounts'] as List? ?? const [])
            .map((d) => '$d')
            .toList(),
        qtyWarehouse: wh > Decimal.zero ? decToApi(wh) : '',
      );
    }).toList(),
    costs: (j['costs'] as List? ?? const [])
        .map(
          (e) => (
            name: '${(e as Map)['name'] ?? ''}',
            amount: '${e['amount'] ?? '0'}',
          ),
        )
        .toList(),
  );
}

/// API kelola barang (CRUD ringan) dan pembelian tunai dari HP. Tenant/outlet selalu dari token.
class ManageRepository {
  ManageRepository(this._api);

  final ApiClient _api;

  Dio get _dio => _api.dio;

  Future<T> _guard<T>(Future<T> Function() fn) async {
    try {
      return await fn();
    } on DioException catch (e) {
      throw ApiError.fromDio(e);
    }
  }

  // ---- barang ----

  /// Barang diarsipkan yang cocok dengan [q] (maks 30; persempit dengan pencarian, tanpa OFFSET).
  Future<List<ArchivedItem>> archivedItems(String q) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/items/',
      queryParameters: {'active': false, 'limit': 30, if (q.isNotEmpty) 'q': q},
    );
    return (r.data!['data'] as List? ?? const [])
        .map((e) => ArchivedItem.fromJson((e as Map).cast<String, dynamic>()))
        .toList();
  });

  Future<Map<String, dynamic>> getItem(String id) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>('/items/$id');
    return r.data!;
  });

  /// Ubah barang: server mengganti semua field, jadi [body] dibangun dari barang yang ada lalu ditimpa field yang diedit.
  Future<void> updateItem(String id, Map<String, dynamic> body) =>
      _guard(() async {
        await _dio.put<Map<String, dynamic>>('/items/$id', data: body);
      });

  Future<void> setItemActive(String id, bool active) => _guard(() async {
    await _dio.put<Map<String, dynamic>>(
      '/items/$id/active',
      data: {'active': active},
    );
  });

  // ---- pembelian ----

  Future<List<Salesperson>> suppliers(String q) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/catalog/lookup/suppliers/',
      queryParameters: {if (q.isNotEmpty) 'q': q, 'active': true, 'limit': 20},
    );
    return (r.data!['data'] as List? ?? const [])
        .map((e) => Salesperson.fromJson((e as Map).cast<String, dynamic>()))
        .toList();
  });

  Future<List<PurchaseItemChoice>> purchaseItems(String q) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/purchases/items',
      queryParameters: {if (q.isNotEmpty) 'q': q},
    );
    return (r.data!['data'] as List? ?? const [])
        .map(
          (e) =>
              PurchaseItemChoice.fromJson((e as Map).cast<String, dynamic>()),
        )
        .toList();
  });

  /// Daftar pembelian (keyset). [from]/[to] `YYYY-MM-DD`; kosong = 30 hari terakhir.
  Future<PurchasePage> purchases({
    String from = '',
    String to = '',
    String q = '',
    String cursor = '',
  }) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/purchases/',
      queryParameters: {
        if (from.isNotEmpty) 'from': from,
        if (to.isNotEmpty) 'to': to,
        if (q.isNotEmpty) 'q': q,
        if (cursor.isNotEmpty) 'cursor': cursor,
        'limit': 30,
      },
    );
    final d = r.data!;
    final sum = (d['summary'] as Map?)?.cast<String, dynamic>() ?? const {};
    return PurchasePage(
      rows: (d['data'] as List? ?? const [])
          .map((e) => PurchaseRow.fromJson((e as Map).cast<String, dynamic>()))
          .toList(),
      nextCursor: d['has_more'] == true ? '${d['next_cursor'] ?? ''}' : '',
      count: (sum['count'] as num?)?.toInt() ?? 0,
      total: dec(sum['total']),
    );
  });

  Future<PurchaseDetail> purchase(String id) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>('/purchases/$id');
    return PurchaseDetail.fromJson(r.data!);
  });

  Map<String, dynamic> _purchaseBody({
    required String supplierId,
    required String date,
    required String invoiceNo,
    required String note,
    required List<PurchaseLineDraft> lines,
    String taxPct = '',
    List<({String name, String amount})> costs = const [],
  }) => {
    'supplier_id': supplierId,
    'supplier_invoice_no': invoiceNo,
    'purchase_date': date,
    'payment_type':
        'cash', // pembelian dari HP selalu tunai: tidak melahirkan hutang
    if (taxPct.isNotEmpty) 'tax_pct': taxPct,
    if (costs.isNotEmpty)
      'other_costs': [
        for (final c in costs) {'name': c.name, 'amount': c.amount},
      ],
    'note': note,
    'lines': [for (final l in lines) l.toJson()],
  };

  Future<PurchaseDetail> createPurchase({
    required String supplierId,
    required String date,
    required String invoiceNo,
    required String note,
    required List<PurchaseLineDraft> lines,
    required String idempotencyKey,
  }) => _guard(() async {
    final r = await _dio.post<Map<String, dynamic>>(
      '/purchases/',
      data: _purchaseBody(
        supplierId: supplierId,
        date: date,
        invoiceNo: invoiceNo,
        note: note,
        lines: lines,
      ),
      options: Options(headers: {'Idempotency-Key': idempotencyKey}),
    );
    return PurchaseDetail.fromJson(r.data!);
  });

  /// Revisi nota (nota lama digantikan; stok dikoreksi server). PPN dan biaya lain dari nota lama dibawa apa adanya.
  Future<PurchaseDetail> editPurchase(
    PurchaseDetail old, {
    required String supplierId,
    required String date,
    required String invoiceNo,
    required String note,
    required List<PurchaseLineDraft> lines,
    required String reason,
    required String idempotencyKey,
  }) => _guard(() async {
    final r = await _dio.put<Map<String, dynamic>>(
      '/purchases/${old.id}',
      data: {
        ..._purchaseBody(
          supplierId: supplierId,
          date: date,
          invoiceNo: invoiceNo,
          note: note,
          lines: lines,
          taxPct: old.taxPct,
          costs: old.costs,
        ),
        'reason': reason,
      },
      options: Options(headers: {'Idempotency-Key': idempotencyKey}),
    );
    return PurchaseDetail.fromJson(r.data!);
  });

  Future<void> voidPurchase(String id, String reason) => _guard(() async {
    await _dio.post<Map<String, dynamic>>(
      '/purchases/$id/void',
      data: {'reason': reason},
    );
  });
}
