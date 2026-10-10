import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_client.dart';
import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/session/session_controller.dart';
import 'pos_models.dart';

final posRepositoryProvider = Provider<PosRepository>((ref) => PosRepository(ref.watch(apiClientProvider)));

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
  Future<List<PosItem>> search(String q, {int limit = 30}) => _guard(() async {
        final r = await _dio.get<Map<String, dynamic>>('/items/search', queryParameters: {if (q.isNotEmpty) 'q': q, 'limit': limit});
        return (r.data!['data'] as List? ?? const []).map((e) => PosItem.fromJson((e as Map).cast<String, dynamic>())).toList();
      });

  /// Kode/barcode persis (halaman pertama pencarian membawa `exact`).
  Future<List<PosItem>> exact(String q) => _guard(() async {
        final r = await _dio.get<Map<String, dynamic>>('/items/search', queryParameters: {'q': q, 'limit': 5});
        return (r.data!['exact'] as List? ?? const []).map((e) => PosItem.fromJson((e as Map).cast<String, dynamic>())).toList();
      });

  Map<String, dynamic> _saleBody(List<CartLine> lines, bool applyTax) => {
        'lines': [
          for (final l in lines) {'item_id': l.item.id, 'qty': decToApi(l.qty)},
        ],
        'apply_tax': applyTax,
      };

  Future<Quote> quote(List<CartLine> lines, {required bool applyTax}) => _guard(() async {
        final r = await _dio.post<Map<String, dynamic>>('/sales/quote', data: _saleBody(lines, applyTax));
        return Quote.fromJson(r.data!);
      });

  Future<SaleResult> createSale(List<CartLine> lines, {required bool applyTax, required List<PaymentInput> payments, required String idempotencyKey}) =>
      _guard(() async {
        final body = _saleBody(lines, applyTax)..['payments'] = [for (final p in payments) p.toJson()];
        final r = await _dio.post<Map<String, dynamic>>(
          '/sales/',
          data: body,
          options: Options(headers: {'Idempotency-Key': idempotencyKey}),
        );
        return SaleResult.fromJson(r.data!);
      });

  Future<List<PayMethodInfo>> paymentMethods() => _guard(() async {
        final r = await _dio.get<Map<String, dynamic>>('/payment-methods/lookup', queryParameters: {'for': 'sale'});
        return (r.data!['data'] as List? ?? const []).map((e) => PayMethodInfo.fromJson((e as Map).cast<String, dynamic>())).toList();
      });

  Future<ShiftInfo?> currentShift() => _guard(() async {
        final r = await _dio.get<Map<String, dynamic>>('/shifts/current');
        final s = r.data!['shift'];
        return s is Map ? ShiftInfo.fromJson(s.cast<String, dynamic>()) : null;
      });

  Future<ShiftInfo> openShift(String openingCash) => _guard(() async {
        final r = await _dio.post<Map<String, dynamic>>('/shifts/open', data: {'opening_cash': openingCash});
        return ShiftInfo.fromJson(r.data!);
      });
}
