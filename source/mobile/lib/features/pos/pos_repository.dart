import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_client.dart';
import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/session/session_controller.dart';
import 'pos_models.dart';
import 'sales_models.dart';
import 'shift_models.dart';

final posRepositoryProvider = Provider<PosRepository>(
  (ref) => PosRepository(ref.watch(apiClientProvider)),
);

/// Akses API kasir. Tenant/outlet/kasir selalu dari token; di sini hanya barang, qty, dan pembayaran.
class PosRepository {
  PosRepository(this._api);

  final ApiClient _api;

  Dio get _dio => _api.dio;

  Future<T> _guard<T>(Future<T> Function() fn) async {
    try {
      return await fn();
    } on DioException catch (e) {
      throw ApiError.fromDio(e);
    }
  }

  /// Pencarian untuk katalog besar: semua kata wajib cocok (nama/kode/barcode). Kosong = awal katalog.
  Future<List<PosItem>> search(
    String q, {
    int limit = 30,
    String? categoryId,
  }) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/items/search',
      queryParameters: {
        if (q.isNotEmpty) 'q': q,
        'limit': limit,
        'category_id': ?categoryId,
      },
    );
    return (r.data!['data'] as List? ?? const [])
        .map((e) => PosItem.fromJson((e as Map).cast<String, dynamic>()))
        .toList();
  });

  /// Kode/barcode persis (halaman pertama pencarian membawa `exact`).
  Future<List<PosItem>> exact(String q) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/items/search',
      queryParameters: {'q': q, 'limit': 5},
    );
    return (r.data!['exact'] as List? ?? const [])
        .map((e) => PosItem.fromJson((e as Map).cast<String, dynamic>()))
        .toList();
  });

  /// Cari member aktif (nama/kode/telepon); kosong = daftar awal.
  Future<List<MemberInfo>> memberLookup(String q) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/members/lookup',
      queryParameters: {'q': q},
    );
    return (r.data!['data'] as List? ?? const [])
        .map((e) => MemberInfo.fromJson((e as Map).cast<String, dynamic>()))
        .toList();
  });

  Map<String, dynamic> _saleBody(SaleDraft d) => {
    'lines': [
      for (final l in d.lines)
        {
          'item_id': l.item.id,
          'qty': decToApi(l.qty),
          if (l.overridePrice != null)
            'unit_price': decToApi(l.overridePrice!, scale: 2),
          if (l.discountForServer != null)
            'discount': decToApi(l.discountForServer!, scale: 2),
        },
    ],
    'apply_tax': d.applyTax,
    if (d.costs.isNotEmpty)
      'other_costs': [
        for (final c in d.costs)
          {'name': c.name, 'amount': decToApi(c.amount, scale: 2)},
      ],
    if (d.credit) 'credit': true,
    'salesperson_id': ?d.salesperson?.id,
    if (d.note.trim().isNotEmpty) 'note': d.note.trim(),
    'member_id': ?d.member?.id,
    if (d.member != null && d.redeemPoints > 0) 'redeem_points': d.redeemPoints,
    if (d.approval != null)
      'approval': {'user_id': d.approval!.userId, 'pin': d.approval!.pin},
  };

  /// Pratinjau hitung server. Tidak pernah membawa PIN penyetuju.
  Future<Quote> quote(SaleDraft d) => _guard(() async {
    final r = await _dio.post<Map<String, dynamic>>(
      '/sales/quote',
      data: _saleBody(
        SaleDraft(
          lines: d.lines,
          applyTax: d.applyTax,
          member: d.member,
          redeemPoints: d.redeemPoints,
          costs: d.costs,
          salesperson: d.salesperson,
        ),
      ),
    );
    return Quote.fromJson(r.data!);
  });

  Future<SaleResult> createSale(
    SaleDraft d, {
    required List<PaymentInput> payments,
    required String idempotencyKey,
  }) => _guard(() async {
    final body = _saleBody(d)
      ..['payments'] = [for (final p in payments) p.toJson()];
    final r = await _dio.post<Map<String, dynamic>>(
      '/sales/',
      data: body,
      options: Options(headers: {'Idempotency-Key': idempotencyKey}),
    );
    return SaleResult.fromJson(r.data!);
  });

  /// Periksa PIN penyetuju lebih dulu (dihitung sebagai percobaan; 5 salah = terkunci).
  Future<Approver> checkApproval(String userId, String pin) => _guard(() async {
    final r = await _dio.post<Map<String, dynamic>>(
      '/approvals/check',
      data: {'user_id': userId, 'pin': pin},
    );
    return Approver.fromJson(r.data!);
  });

  /// Pintasan barang milik kasir ini (urut slot).
  Future<List<Shortcut>> shortcuts() => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>('/pos/shortcuts/');
    return _shortcutList(r.data!);
  });

  Future<List<Shortcut>> setShortcut(int slot, String itemId) =>
      _guard(() async {
        final r = await _dio.put<Map<String, dynamic>>(
          '/pos/shortcuts/$slot',
          data: {'item_id': itemId},
        );
        return _shortcutList(r.data!);
      });

  Future<List<Shortcut>> clearShortcut(int slot) => _guard(() async {
    final r = await _dio.delete<Map<String, dynamic>>('/pos/shortcuts/$slot');
    return _shortcutList(r.data!);
  });

  List<Shortcut> _shortcutList(Map<String, dynamic> j) =>
      (j['data'] as List? ?? const [])
          .map((e) => Shortcut.fromJson((e as Map).cast<String, dynamic>()))
          .toList();

  /// Kategori aktif untuk filter katalog kasir (id + nama); cukup izin kasir.
  Future<List<Salesperson>> categories() => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/catalog/lookup/categories/',
      queryParameters: {'active': true, 'limit': 100},
    );
    return (r.data!['data'] as List? ?? const [])
        .map((e) => Salesperson.fromJson((e as Map).cast<String, dynamic>()))
        .toList();
  });

  /// Salesman aktif untuk nota (id + nama); cukup izin kasir.
  Future<List<Salesperson>> salespeople(String q) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/catalog/lookup/salespeople/',
      queryParameters: {if (q.isNotEmpty) 'q': q, 'active': true, 'limit': 20},
    );
    return (r.data!['data'] as List? ?? const [])
        .map((e) => Salesperson.fromJson((e as Map).cast<String, dynamic>()))
        .toList();
  });

  Future<List<PayMethodInfo>> paymentMethods() => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/payment-methods/lookup',
      queryParameters: {'for': 'sale'},
    );
    return (r.data!['data'] as List? ?? const [])
        .map((e) => PayMethodInfo.fromJson((e as Map).cast<String, dynamic>()))
        .toList();
  });

  Future<ShiftInfo?> currentShift() => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>('/shifts/current');
    final s = r.data!['shift'];
    return s is Map ? ShiftInfo.fromJson(s.cast<String, dynamic>()) : null;
  });

  /// Gambar utama barang (JPEG kecil). Butuh token, jadi diunduh lewat dio.
  Future<Uint8List> itemThumb(String itemId, String imageId) =>
      _guard(() async {
        final r = await _dio.get<List<int>>(
          '/items/$itemId/images/$imageId/file',
          queryParameters: {'size': 'thumb'},
          options: Options(responseType: ResponseType.bytes),
        );
        return Uint8List.fromList(r.data!);
      });

  Future<ShiftRecap> shiftRecap(String id) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>('/shifts/$id');
    return ShiftRecap.fromJson(r.data!);
  });

  /// Tutup shift: klien hanya mengirim uang fisik per metode; selisih dihitung dan divalidasi server.
  Future<ShiftRecap> closeShift(
    String id, {
    required Map<String, String> counted,
    required String note,
    required String idempotencyKey,
    String? approverId,
    String? pin,
  }) => _guard(() async {
    final r = await _dio.post<Map<String, dynamic>>(
      '/shifts/$id/close',
      data: {
        'counts': [
          for (final e in counted.entries)
            {'method_id': e.key, 'counted': e.value},
        ],
        'note': note,
        if (approverId != null && pin != null)
          'approval': {'user_id': approverId, 'pin': pin},
      },
      options: Options(headers: {'Idempotency-Key': idempotencyKey}),
    );
    return ShiftRecap.fromJson(r.data!);
  });

  /// Tanpa [purpose] = penyetuju ubah harga/potongan di outlet aktif.
  Future<List<Approver>> approvers([String? purpose]) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/approvals/approvers',
      queryParameters: {'for': ?purpose},
    );
    return (r.data!['approvers'] as List? ?? const [])
        .map((e) => Approver.fromJson((e as Map).cast<String, dynamic>()))
        .toList();
  });

  /// Penjualan kasir ini hari ini (tanpa rentang = hari ini menurut zona waktu outlet).
  Future<SalesToday> salesToday({String q = ''}) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/sales/',
      queryParameters: {if (q.isNotEmpty) 'q': q},
    );
    return SalesToday.fromJson(r.data!);
  });

  Future<SaleDetailInfo> saleDetail(String id) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>('/sales/$id');
    return SaleDetailInfo.fromJson(r.data!);
  });

  Future<ShiftInfo> openShift(String openingCash) => _guard(() async {
    final r = await _dio.post<Map<String, dynamic>>(
      '/shifts/open',
      data: {'opening_cash': openingCash},
    );
    return ShiftInfo.fromJson(r.data!);
  });
}
