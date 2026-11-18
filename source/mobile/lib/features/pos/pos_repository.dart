import 'dart:async';
import 'dart:typed_data';

import 'package:decimal/decimal.dart';
import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_client.dart';
import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/offline/offline_box.dart';
import '../../core/session/session_controller.dart';
import '../reports/erapay_settlement_models.dart';
import '../reports/sales_report_models.dart';
import 'outbox.dart';
import 'pos_models.dart';
import 'sales_models.dart';
import 'shift_models.dart';

final posRepositoryProvider = Provider<PosRepository>((ref) {
  final s = ref.watch(sessionProvider);
  final scope = s is SessionSignedIn
      ? '${s.profile.user.id}.${s.profile.outlet.id}'
      : null;
  return PosRepository(
    ref.watch(apiClientProvider),
    offlineBox: OfflineBox(scope),
    enqueue: (q) => ref.read(outboxProvider.notifier).add(q),
  );
});

/// Akses API kasir. Tenant/outlet/kasir selalu dari token; di sini hanya barang, qty, dan pembayaran.
class PosRepository {
  PosRepository(
    this._api, {
    this.offlineBox = const OfflineBox(null),
    Future<void> Function(QueuedSale)? enqueue,
  }) : _enqueue = enqueue;

  final ApiClient _api;
  final OfflineBox offlineBox;
  final Future<void> Function(QueuedSale)? _enqueue;

  // Cache katalog untuk mode offline: terisi dari pencarian yang sukses + warmCatalog().
  static const _maxCached = 3000;
  final _items = <String, Map<String, dynamic>>{};
  bool _itemsLoaded = false;
  String? _taxSeen;

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
  }) async => (await searchPage(q, limit: limit, categoryId: categoryId)).items;

  /// Satu halaman katalog (keyset): `nextCursor` kosong = sudah habis. Tanpa total/OFFSET, aman untuk katalog besar.
  Future<({List<PosItem> items, String nextCursor})> searchPage(
    String q, {
    int limit = 30,
    String? categoryId,
    String? cursor,
  }) async {
    try {
      final r = await _searchOnline(
        q,
        limit: limit,
        categoryId: categoryId,
        cursor: cursor,
      );
      _remember(r.items);
      return r;
    } on ApiError catch (e) {
      // Offline: cari di cache perangkat (filter kategori & halaman lanjutan hanya online).
      if (!e.isNetwork || categoryId != null || cursor != null) rethrow;
      final c = await _cachedSearch(q, limit);
      if (c == null) rethrow;
      return (items: c, nextCursor: '');
    }
  }

  Future<({List<PosItem> items, String nextCursor})> _searchOnline(
    String q, {
    int limit = 30,
    String? categoryId,
    String? cursor,
  }) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/items/search',
      queryParameters: {
        if (q.isNotEmpty) 'q': q,
        'limit': limit,
        'category_id': ?categoryId,
        if (cursor != null && cursor.isNotEmpty) 'cursor': cursor,
      },
    );
    final items = (r.data!['data'] as List? ?? const [])
        .map((e) => PosItem.fromJson((e as Map).cast<String, dynamic>()))
        .toList();
    return (items: items, nextCursor: '${r.data!['next_cursor'] ?? ''}');
  });

  /// Kode/barcode persis (halaman pertama pencarian membawa `exact`).
  Future<List<PosItem>> exact(String q) async {
    try {
      final list = await _guard(() async {
        final r = await _dio.get<Map<String, dynamic>>(
          '/items/search',
          queryParameters: {'q': q, 'limit': 5},
        );
        return (r.data!['exact'] as List? ?? const [])
            .map((e) => PosItem.fromJson((e as Map).cast<String, dynamic>()))
            .toList();
      });
      _remember(list);
      return list;
    } on ApiError catch (e) {
      if (!e.isNetwork) rethrow;
      await _loadItems();
      final k = q.trim().toLowerCase();
      final hit = [
        for (final j in _items.values)
          if ('${j['barcode']}'.toLowerCase() == k ||
              '${j['sku']}'.toLowerCase() == k)
            PosItem.fromJson(j),
      ];
      if (hit.isEmpty && _items.isEmpty) rethrow;
      return hit;
    }
  }

  Future<void> _loadItems() async {
    if (_itemsLoaded) return;
    _itemsLoaded = true;
    final raw = await offlineBox.read('items');
    if (raw is List) {
      for (final e in raw) {
        final m = (e as Map).cast<String, dynamic>();
        _items['${m['id']}'] = m;
      }
    }
  }

  /// Simpan ke cache; ditulis ke disk hanya bila ada barang baru (pencarian berulang tidak menulis ulang).
  void _remember(List<PosItem> items, {bool persist = true}) {
    if (offlineBox.scope == null || items.isEmpty) return;
    var added = false;
    for (final it in items) {
      added |= !_items.containsKey(it.id);
      _items.remove(
        it.id,
      ); // pindah ke ujung: yang paling lama tak terlihat dibuang lebih dulu
      _items[it.id] = it.toJson();
    }
    while (_items.length > _maxCached) {
      _items.remove(_items.keys.first);
    }
    if (added && persist) _persistItems();
  }

  void _persistItems() => unawaited(() async {
    await _loadItems(); // gabungkan dulu dengan isi disk supaya tidak menimpanya
    await offlineBox.write('items', _items.values.toList());
  }());

  Future<List<PosItem>?> _cachedSearch(String q, int limit) async {
    await _loadItems();
    if (_items.isEmpty) return null;
    final words = q
        .toLowerCase()
        .split(RegExp(r'\s+'))
        .where((w) => w.isNotEmpty);
    final hit = [
      for (final j in _items.values)
        if (words.every(
          (w) => '${j['name']} ${j['sku']} ${j['barcode']}'
              .toLowerCase()
              .contains(w),
        ))
          PosItem.fromJson(j),
    ]..sort((a, b) => a.name.toLowerCase().compareTo(b.name.toLowerCase()));
    return hit.take(limit).toList();
  }

  /// Isi cache katalog penuh saat online (dipanggil berkala dari sinkron beranda). Galat apa pun diabaikan.
  Future<void> warmCatalog() async {
    try {
      String? cursor;
      for (var i = 0; i < _maxCached ~/ 100; i++) {
        final r = await _searchOnline('', limit: 100, cursor: cursor);
        _remember(r.items, persist: false);
        if (r.nextCursor.isEmpty) break;
        cursor = r.nextCursor;
      }
    } on ApiError {
      // offline / server sibuk: coba lagi pada putaran berikutnya
    }
    _persistItems();
  }

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
    if (d.forceEraPay) 'force_erapay': true,
    'salesperson_id': ?d.salesperson?.id,
    if (d.note.trim().isNotEmpty) 'note': d.note.trim(),
    'member_id': ?d.member?.id,
    if (d.member != null && d.redeemPoints > 0) 'redeem_points': d.redeemPoints,
    if (d.approval != null)
      'approval': {'user_id': d.approval!.userId, 'pin': d.approval!.pin},
  };

  /// Pratinjau hitung server. Tidak pernah membawa PIN penyetuju.
  Future<Quote> quote(SaleDraft d) async {
    try {
      final q = await _guard(() async {
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
      final pct = '${q.taxStorePct}|${q.taxGovPct}';
      if (pct != _taxSeen) {
        _taxSeen = pct;
        unawaited(
          offlineBox.write('tax', [
            q.taxStorePct.toString(),
            q.taxGovPct.toString(),
          ]),
        );
      }
      return q;
    } on ApiError catch (e) {
      if (!e.isNetwork) rethrow;
      return await _localQuote(d) ?? (throw e);
    }
  }

  /// Perkiraan total saat offline: hanya nota sederhana (tanpa member/ubah harga/potongan); pajak dari tarif quote terakhir.
  /// Server tetap menghitung ulang saat nota dikirim.
  Future<Quote?> _localQuote(SaleDraft d) async {
    if (d.lines.isEmpty ||
        d.member != null ||
        d.redeemPoints > 0 ||
        d.lines.any((l) => l.adjusted)) {
      return null;
    }
    var sPct = Decimal.zero, gPct = Decimal.zero;
    if (d.applyTax) {
      final t = await offlineBox.read('tax');
      if (t is! List || t.length != 2) return null;
      sPct = dec(t[0]);
      gPct = dec(t[1]);
    }
    final lines = [
      for (final l in d.lines)
        QuoteLine(
          unitPrice: l.item.price,
          lineTotal: l.item.price * l.qty,
          listPrice: l.item.price,
        ),
    ];
    final sub = lines.fold(Decimal.zero, (a, l) => a + l.lineTotal);
    final other = d.costs.fold(Decimal.zero, (a, c) => a + c.amount);
    Decimal tax(Decimal pct) => (sub * pct).shift(-2).round(scale: 2);
    final ts = tax(sPct), tg = tax(gPct);
    return Quote(
      lines: lines,
      subtotal: sub,
      discount: Decimal.zero,
      taxStore: ts,
      taxGov: tg,
      taxStorePct: sPct,
      taxGovPct: gPct,
      otherCost: other,
      total: sub + ts + tg + other,
      redeemAmount: Decimal.zero,
    );
  }

  /// Status QRIS EraPay toko ini; galat apa pun dianggap "tidak aktif" (metode tampil biasa, server tetap menolak nota tanpa charge lunas).
  Future<EraPayConfig> eraPayConfig() async {
    try {
      final r = await _dio.get<Map<String, dynamic>>('/payments/erapay/config');
      return EraPayConfig.fromJson(r.data!);
    } on DioException {
      return const EraPayConfig();
    }
  }

  /// Buat tagihan QRIS; kunci yang sama mengembalikan tagihan yang sama (aman diulang).
  Future<EraCharge> eraCreateCharge(
    Decimal amount,
    String idempotencyKey,
  ) async {
    final c = await _guard(() async {
      final r = await _dio.post<Map<String, dynamic>>(
        '/payments/erapay/charges',
        data: {'amount': decToApi(amount, scale: 2)},
        options: Options(headers: {'Idempotency-Key': idempotencyKey}),
      );
      return EraCharge.fromJson(r.data!);
    });
    await _rememberCharge(c);
    return c;
  }

  // QRIS yang sudah dibuat disimpan di perangkat supaya tidak perlu dibuat ulang bila layar bayar/aplikasi tertutup.
  static const _chargeKeep = Duration(hours: 12);

  Future<List<Map<String, dynamic>>> _storedCharges() async {
    final raw = await offlineBox.read('era_charges');
    final now = DateTime.now().millisecondsSinceEpoch;
    return [
      if (raw is List)
        for (final e in raw)
          if (e is Map &&
              now - ((e['at'] as num?)?.toInt() ?? 0) <
                  _chargeKeep.inMilliseconds)
            e.cast<String, dynamic>(),
    ];
  }

  Future<void> _rememberCharge(EraCharge c) async {
    final list = await _storedCharges();
    list.removeWhere((e) => e['id'] == c.id);
    list.add({
      'id': c.id,
      'amount': decToApi(c.amount, scale: 2),
      'at': DateTime.now().millisecondsSinceEpoch,
    });
    await offlineBox.write(
      'era_charges',
      list.length > 10 ? list.sublist(list.length - 10) : list,
    );
  }

  /// Lupakan tagihan yang sudah dipakai nota (ref_no = id tagihan) supaya tidak dipulihkan untuk nota lain.
  Future<void> _forgetCharges(Iterable<String> ids) async {
    final drop = ids.toSet();
    if (drop.isEmpty) return;
    final list = await _storedCharges();
    list.removeWhere((e) => drop.contains(e['id']));
    await offlineBox.write('era_charges', list.isEmpty ? null : list);
  }

  /// QRIS terakhir dengan nominal sama yang masih menunggu bayar (atau sudah lunas) dan belum dipakai nota.
  /// Status diperiksa ke server; null bila tidak ada / offline.
  Future<EraCharge?> eraRecover(Decimal amount) async {
    final want = decToApi(amount, scale: 2);
    for (final e in (await _storedCharges()).reversed) {
      if (e['amount'] != want) continue;
      try {
        final c = await eraGetCharge('${e['id']}');
        if ((c.status == 'pending' && c.qrValue.isNotEmpty) || c.isPaid) {
          return c;
        }
      } on ApiError {
        return null;
      }
    }
    return null;
  }

  /// Status tagihan (server memutakhirkan dari EraPay bila masih menunggu).
  Future<EraCharge> eraGetCharge(String id) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/payments/erapay/charges/$id',
    );
    return EraCharge.fromJson(r.data!);
  });

  Future<SaleResult> createSale(
    SaleDraft d, {
    required List<PaymentInput> payments,
    required String idempotencyKey,
  }) async {
    final body = _saleBody(d)
      ..['payments'] = [for (final p in payments) p.toJson()];
    try {
      final sale = await _guard(() async {
        final r = await _dio.post<Map<String, dynamic>>(
          '/sales/',
          data: body,
          options: Options(headers: {'Idempotency-Key': idempotencyKey}),
        );
        return SaleResult.fromJson(r.data!);
      });
      unawaited(_forgetCharges(payments.map((p) => p.refNo)));
      return sale;
    } on ApiError catch (e) {
      if (!e.isNetwork) rethrow;
      // Tanpa jaringan: simpan di antrean perangkat (kunci sama, jadi aman bila permintaan tadi sebenarnya sudah sampai).
      return await _queueOffline(d, payments, body, idempotencyKey) ??
          (throw e);
    }
  }

  /// Offline hanya untuk nota tunai sederhana: tanpa member, kredit, ubah harga/potongan, PIN, atau QRIS.
  Future<SaleResult?> _queueOffline(
    SaleDraft d,
    List<PaymentInput> payments,
    Map<String, dynamic> body,
    String key,
  ) async {
    final enqueue = _enqueue;
    if (enqueue == null ||
        d.credit ||
        d.forceEraPay ||
        d.approval != null ||
        payments.isEmpty) {
      return null;
    }
    final methods = await _cachedMethods();
    final cash = {
      for (final m in methods ?? const <PayMethodInfo>[])
        if (m.isCash) m.id,
    };
    if (!payments.every((p) => cash.contains(p.methodId))) return null;
    final q = await _localQuote(d);
    if (q == null) return null;
    final paid = payments.fold(Decimal.zero, (a, p) => a + p.amount);
    if (paid < q.total) return null;
    final now = DateTime.now();
    body['offline'] = true;
    body['occurred_at'] = now.toUtc().toIso8601String();
    final sale = QueuedSale(
      key: key,
      body: body,
      at: now,
      total: decToApi(q.total, scale: 2),
    );
    await enqueue(sale);
    return SaleResult(
      id: 'offline:$key',
      docNo: sale.docNo,
      total: q.total,
      change: paid - q.total,
      surcharge: Decimal.zero,
      receivable: Decimal.zero,
      offline: true,
    );
  }

  /// Kirim satu nota antrean ke server (kunci idempotensi menjamin tidak ganda).
  Future<SaleResult> sendQueued(QueuedSale q) => _guard(() async {
    final r = await _dio.post<Map<String, dynamic>>(
      '/sales/',
      data: q.body,
      options: Options(headers: {'Idempotency-Key': q.key}),
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

  Future<List<PayMethodInfo>> paymentMethods() async {
    try {
      final raw = await _guard(() async {
        final r = await _dio.get<Map<String, dynamic>>(
          '/payment-methods/lookup',
          queryParameters: {'for': 'sale'},
        );
        return r.data!['data'] as List? ?? const [];
      });
      unawaited(offlineBox.write('methods', raw));
      return _methods(raw);
    } on ApiError catch (e) {
      if (!e.isNetwork) rethrow;
      return await _cachedMethods() ?? (throw e);
    }
  }

  List<PayMethodInfo> _methods(List raw) => raw
      .map((e) => PayMethodInfo.fromJson((e as Map).cast<String, dynamic>()))
      .toList();

  Future<List<PayMethodInfo>?> _cachedMethods() async {
    final raw = await offlineBox.read('methods');
    return raw is List ? _methods(raw) : null;
  }

  Future<ShiftInfo?> currentShift() async {
    try {
      final s = await _guard(() async {
        final r = await _dio.get<Map<String, dynamic>>('/shifts/current');
        return r.data!['shift'];
      });
      unawaited(offlineBox.write('shift', s is Map ? s : null));
      return s is Map ? ShiftInfo.fromJson(s.cast<String, dynamic>()) : null;
    } on ApiError catch (e) {
      // Offline: shift terakhir yang diketahui dianggap masih terbuka (nota offline dicek server saat dikirim).
      final c = e.isNetwork ? await offlineBox.read('shift') : null;
      if (c is Map) return ShiftInfo.fromJson(c.cast<String, dynamic>());
      rethrow;
    }
  }

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
    unawaited(offlineBox.write('shift', null));
    return ShiftRecap.fromJson(r.data!);
  });

  /// Catat kas kecil pada shift terbuka. [idempotencyKey] sama = tidak tercatat dua kali.
  Future<void> addPettyCash({
    required bool out,
    required String amount,
    required String category,
    required String note,
    required String idempotencyKey,
    String? approverId,
    String? pin,
  }) => _guard(() async {
    await _dio.post<Map<String, dynamic>>(
      '/shifts/petty-cash',
      data: {
        'direction': out ? 'out' : 'in',
        'amount': amount,
        'category': category,
        'note': note,
        if (approverId != null && pin != null)
          'approval': {'user_id': approverId, 'pin': pin},
      },
      options: Options(headers: {'Idempotency-Key': idempotencyKey}),
    );
  });

  Future<void> voidPettyCash(String id) => _guard(() async {
    await _dio.post<Map<String, dynamic>>('/shifts/petty-cash/$id/void');
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

  /// Penjualan kasir ini pada satu shift ([shiftId]; jendela waktunya dihitung server).
  Future<SalesToday> salesToday({required String shiftId, String q = ''}) =>
      _guard(() async {
        final r = await _dio.get<Map<String, dynamic>>(
          '/sales/',
          queryParameters: {'shift_id': shiftId, if (q.isNotEmpty) 'q': q},
        );
        return SalesToday.fromJson(r.data!);
      });

  /// Semua nota satu hari ([date] = `YYYY-MM-DD`), terbaru dulu; halaman berikutnya diikuti sampai habis.
  Future<List<ReportInvoice>> salesInvoices({
    required String date,
    bool allOutlets = false,
  }) => _guard(() async {
    final out = <ReportInvoice>[];
    String cursor = '';
    do {
      final r = await _dio.get<Map<String, dynamic>>(
        '/reports/sales/invoices',
        queryParameters: {
          'from': date,
          'to': date,
          'all_outlets': allOutlets,
          if (cursor.isNotEmpty) 'cursor': cursor,
        },
      );
      out.addAll(
        (r.data!['data'] as List? ?? const []).map(
          (e) => ReportInvoice.fromJson((e as Map).cast<String, dynamic>()),
        ),
      );
      cursor = '${r.data!['next_cursor'] ?? ''}';
    } while (cursor.isNotEmpty);
    return out;
  });

  /// Laporan penjualan per tanggal (izin `sales_reports.view`). [from]/[to] = `YYYY-MM-DD`; [allOutlets] menjumlah
  /// semua cabang yang boleh diakses akun ini, bila tidak hanya cabang aktif.
  Future<SalesReport> salesReport({
    required String from,
    required String to,
    bool allOutlets = false,
  }) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/reports/sales/by-date',
      queryParameters: {
        'from': from,
        'to': to,
        'hide_empty': true,
        if (allOutlets) 'all_outlets': true,
      },
    );
    return SalesReport.fromJson(r.data!);
  });

  /// Settlement EraPay (izin `sales_reports.view`): tagihan QRIS pada rentang tanggal, uang ARUS vs uang EraPay.
  /// [refresh] = tagihan yang masih menunggu ditanyakan ulang ke EraPay dulu.
  Future<EraSettlementReport> eraSettlement({
    required String from,
    required String to,
    bool allOutlets = false,
    bool refresh = false,
  }) => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/payments/erapay/settlement',
      queryParameters: {
        'from': from,
        'to': to,
        if (allOutlets) 'all_outlets': true,
        if (refresh) 'refresh': true,
      },
      options: Options(receiveTimeout: const Duration(seconds: 90)),
    );
    return EraSettlementReport.fromJson(r.data!);
  });

  /// Satuan aktif untuk form barang (id + nama).
  Future<List<Salesperson>> units() => _guard(() async {
    final r = await _dio.get<Map<String, dynamic>>(
      '/catalog/lookup/units/',
      queryParameters: {'active': true, 'limit': 100},
    );
    return (r.data!['data'] as List? ?? const [])
        .map((e) => Salesperson.fromJson((e as Map).cast<String, dynamic>()))
        .toList();
  });

  /// Membuat satuan/kategori baru ([kind] = `units` | `categories`); butuh izin tambah master itu.
  Future<Salesperson> createMaster(String kind, String name) =>
      _guard(() async {
        final r = await _dio.post<Map<String, dynamic>>(
          '/catalog/$kind/',
          data: {'name': name},
        );
        return Salesperson.fromJson(r.data!);
      });

  /// Saldo awal stok outlet aktif masih boleh diisi (belum dikunci)?
  Future<bool> openingOpen() async {
    try {
      final r = await _dio.get<Map<String, dynamic>>('/stock/opening/status');
      return r.data!['locked'] != true;
    } on DioException {
      return false;
    }
  }

  /// Barang sederhana: harga jual + modal diisi, SKU otomatis dari server. Bila [openingQty] > 0 dan saldo awal masih
  /// terbuka, stok awal ikut dicatat (movement OPENING) lewat endpoint saldo awal yang sama dengan web.
  Future<PosItem> createItem({
    required String name,
    required Decimal sellPrice,
    required Decimal cost,
    required String unitId,
    String? categoryId,
    String barcode = '',
    bool isGoods = true,
    bool allowNegativeStock = false,
    Decimal? openingQty,
  }) => _guard(() async {
    final r = await _dio.post<Map<String, dynamic>>(
      '/items/',
      data: {
        'name': name,
        'sell_price': decToApi(sellPrice, scale: 2),
        'cost': decToApi(cost, scale: 2),
        'unit_id': unitId,
        'category_id': ?categoryId,
        if (barcode.isNotEmpty) 'barcode': barcode,
        'kind': isGoods ? 'goods' : 'service',
        'allow_negative_stock': isGoods && allowNegativeStock,
      },
    );
    final j = r.data!;
    final id = '${j['id']}';
    var stock = Decimal.zero;
    if (isGoods && openingQty != null && openingQty > Decimal.zero) {
      await _dio.put<Map<String, dynamic>>(
        '/stock/opening/items/$id',
        data: {'bucket': 'display', 'qty': decToApi(openingQty)},
      );
      stock = openingQty;
    }
    return PosItem(
      id: id,
      sku: '${j['sku'] ?? ''}',
      barcode: '${j['barcode'] ?? ''}',
      name: '${j['name'] ?? name}',
      unit: '${(j['unit'] as Map?)?['name'] ?? ''}',
      price: dec(j['sell_price']),
      stockDisplay: stock,
      isGoods: isGoods,
      allowNegativeStock: isGoods && allowNegativeStock,
    );
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
    unawaited(offlineBox.write('shift', r.data));
    return ShiftInfo.fromJson(r.data!);
  });
}
