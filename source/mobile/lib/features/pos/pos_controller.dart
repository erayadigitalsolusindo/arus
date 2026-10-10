import 'dart:async';
import 'dart:convert';

import 'package:decimal/decimal.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../core/api/api_error.dart';
import '../../core/money/money.dart';
import '../../core/session/session_controller.dart';
import 'pos_models.dart';
import 'pos_repository.dart';

class CartState {
  const CartState({
    this.lines = const [],
    this.applyTax = false,
    this.member,
    this.redeemPoints = 0,
    this.costs = const [],
    this.approval,
    this.salesperson,
    this.note = '',
    this.quote,
    this.quoting = false,
    this.quoteError,
  });

  final List<CartLine> lines;
  final bool applyTax;

  /// Member nota ini (null = pelanggan umum) dan poin yang ditukar jadi potongan.
  final MemberInfo? member;
  final int redeemPoints;

  final List<CostEntry> costs;

  /// Penyetuju ubah harga/potongan (PIN hanya di memori; tidak ikut disimpan lokal).
  final ApprovalGrant? approval;

  /// Salesman nota (null = Umum).
  final Salesperson? salesperson;

  /// Keterangan nota (opsional, maks [maxNote] huruf).
  final String note;

  static const maxNote = 200;

  static const maxCosts = 20;

  bool get needsApproval => lines.any((l) => l.adjusted);

  Decimal get costsTotal => costs.fold(Decimal.zero, (a, c) => a + c.amount);

  /// Isi nota untuk server; PIN penyetuju hanya ikut saat [forPay].
  SaleDraft draft({bool forPay = false}) => SaleDraft(
    lines: lines,
    applyTax: applyTax,
    member: member,
    redeemPoints: redeemPoints,
    costs: costs,
    approval: forPay && needsApproval ? approval : null,
    salesperson: salesperson,
    note: forPay ? note : '',
  );

  /// Hasil hitung server untuk isi keranjang saat ini; null = belum/gagal dihitung.
  final Quote? quote;
  final bool quoting;
  final ApiError? quoteError;

  bool get isEmpty => lines.isEmpty;

  /// Batas tukar poin: saldo poin dari quote, dan nilainya tidak melebihi belanja (selain potongan lain).
  int get redeemCap {
    final q = quote;
    final m = member;
    final pv = m?.pointValue;
    if (q == null || q.member == null || pv == null || pv <= Decimal.zero) {
      return 0;
    }
    final base = q.subtotal - (q.discount - q.redeemAmount);
    if (base <= Decimal.zero) return 0;
    final byValue = (base / pv).floor().toInt();
    return byValue < q.member!.points ? byValue : q.member!.points;
  }

  Decimal get itemCount => lines.fold(Decimal.zero, (a, l) => a + l.qty);

  /// Siap dibayar: ada isi, total sudah dari server untuk isi ini, dan tidak ada masalah stok/HPP.
  bool get canPay =>
      lines.isNotEmpty &&
      quote != null &&
      !quoting &&
      quoteError == null &&
      !quote!.hasBlockingIssue;

  /// Bentuk simpan lokal (keranjang aktif dan nota pending): hanya isi nota, tanpa harga/total (selalu dihitung ulang
  /// server lewat quote) dan tanpa penyetuju/PIN.
  Map<String, dynamic> toSnapshot() => {
    'apply_tax': applyTax,
    if (member != null) 'member': member!.toJson(),
    if (redeemPoints > 0) 'redeem': redeemPoints,
    if (salesperson != null) 'salesperson': salesperson!.toJson(),
    if (note.isNotEmpty) 'note': note,
    if (costs.isNotEmpty)
      'costs': [
        for (final c in costs)
          {'name': c.name, 'amount': decToApi(c.amount, scale: 2)},
      ],
    'lines': [
      for (final l in lines) {'item': l.item.toJson(), 'qty': decToApi(l.qty)},
    ],
  };

  /// Kebalikan [toSnapshot]; null bila kosong/rusak.
  static CartState? fromSnapshot(Map<String, dynamic> j) {
    try {
      final lines = [
        for (final e in (j['lines'] as List? ?? const []))
          CartLine(
            item: PosItem.fromJson(
              ((e as Map)['item'] as Map).cast<String, dynamic>(),
            ),
            qty: dec(e['qty']),
          ),
      ];
      if (lines.isEmpty) return null;
      final m = j['member'];
      return CartState(
        lines: lines,
        applyTax: j['apply_tax'] == true,
        member: m is Map
            ? MemberInfo.fromJson(m.cast<String, dynamic>())
            : null,
        redeemPoints: (j['redeem'] as num?)?.toInt() ?? 0,
        note: '${j['note'] ?? ''}',
        salesperson: j['salesperson'] is Map
            ? Salesperson.fromJson(
                (j['salesperson'] as Map).cast<String, dynamic>(),
              )
            : null,
        costs: [
          for (final c in (j['costs'] as List? ?? const []))
            CostEntry(
              name: '${(c as Map)['name'] ?? ''}',
              amount: dec(c['amount']),
            ),
        ],
      );
    } catch (_) {
      return null;
    }
  }

  CartState copyWith({
    List<CartLine>? lines,
    bool? applyTax,
    MemberInfo? member,
    bool clearMember = false,
    int? redeemPoints,
    List<CostEntry>? costs,
    ApprovalGrant? approval,
    bool clearApproval = false,
    Salesperson? salesperson,
    bool clearSalesperson = false,
    String? note,
    Quote? quote,
    bool clearQuote = false,
    bool? quoting,
    ApiError? quoteError,
    bool clearError = false,
  }) => CartState(
    lines: lines ?? this.lines,
    applyTax: applyTax ?? this.applyTax,
    member: clearMember ? null : (member ?? this.member),
    redeemPoints: clearMember ? 0 : (redeemPoints ?? this.redeemPoints),
    costs: costs ?? this.costs,
    approval: clearApproval ? null : (approval ?? this.approval),
    salesperson: clearSalesperson ? null : (salesperson ?? this.salesperson),
    note: note ?? this.note,
    quote: clearQuote ? null : (quote ?? this.quote),
    quoting: quoting ?? this.quoting,
    quoteError: clearError ? null : (quoteError ?? this.quoteError),
  );
}

final cartProvider = NotifierProvider<CartController, CartState>(
  CartController.new,
);

/// Keranjang kasir. Total TIDAK dihitung di sini: setiap perubahan memanggil `POST /sales/quote` (debounce) dan hanya
/// hasil terbaru yang dipakai (nomor urut mencegah respons lama menimpa yang baru).
class CartController extends Notifier<CartState> {
  Timer? _debounce;
  int _seq = 0;

  @override
  CartState build() {
    ref.onDispose(() => _debounce?.cancel());
    return const CartState();
  }

  /// Kunci simpan per pengguna + cabang: keranjang cabang lain tidak tercampur.
  String? get _storeKey {
    final s = ref.read(sessionProvider);
    if (s is! SessionSignedIn) return null;
    return 'pos.cart.${s.profile.user.id}.${s.profile.outlet.id}';
  }

  /// Keranjang yang belum jadi transaksi disimpan di perangkat, jadi tetap ada saat aplikasi dibuka lagi.
  /// Hanya isi dan qty yang dipulihkan; harga/total selalu dihitung ulang server (quote).
  Future<void> restore() async {
    final key = _storeKey;
    _seq++;
    _debounce?.cancel();
    state = CartState(applyTax: state.applyTax);
    if (key == null) return;
    try {
      final raw = (await SharedPreferences.getInstance()).getString(key);
      if (raw == null || _storeKey != key || state.lines.isNotEmpty) return;
      final next = CartState.fromSnapshot(
        jsonDecode(raw) as Map<String, dynamic>,
      );
      if (next == null) return;
      _changed(next);
    } catch (_) {
      // Penyimpanan tidak tersedia atau isinya rusak: mulai dari keranjang kosong.
    }
  }

  Future<void> _persist() async {
    final key = _storeKey;
    if (key == null) return;
    try {
      final p = await SharedPreferences.getInstance();
      if (state.lines.isEmpty) {
        await p.remove(key);
      } else {
        await p.setString(key, jsonEncode(state.toSnapshot()));
      }
    } catch (_) {}
  }

  /// Ganti seluruh isi keranjang dengan nota pending; harga & stok dihitung ulang server.
  void load(CartState next) {
    _seq++;
    _debounce?.cancel();
    state = CartState();
    _changed(next);
  }

  void add(PosItem item, {Decimal? qty}) {
    final q = qty ?? Decimal.one;
    final idx = state.lines.indexWhere((l) => l.item.id == item.id);
    final lines = [...state.lines];
    if (idx >= 0) {
      // Barang yang baru ditambah naik ke atas (sama dengan web).
      final old = lines.removeAt(idx);
      lines.insert(0, old.withQty(old.qty + q));
    } else {
      lines.insert(0, CartLine(item: item, qty: q));
    }
    _changed(state.copyWith(lines: lines));
  }

  void setQty(String itemId, Decimal qty) {
    if (qty <= Decimal.zero) return remove(itemId);
    _changed(
      state.copyWith(
        lines: [
          for (final l in state.lines) l.item.id == itemId ? l.withQty(qty) : l,
        ],
      ),
    );
  }

  void remove(String itemId) => _changed(
    state.copyWith(
      lines: [
        for (final l in state.lines)
          if (l.item.id != itemId) l,
      ],
    ),
  );

  void clear() {
    _seq++;
    _debounce?.cancel();
    state = CartState(applyTax: state.applyTax);
    _persist();
  }

  void setTax(bool on) => _changed(state.copyWith(applyTax: on));

  /// Pilih member (poin tukar direset); null = pelanggan umum.
  void setMember(MemberInfo? m) => _changed(
    m == null
        ? state.copyWith(clearMember: true)
        : state.copyWith(clearMember: true).copyWith(member: m),
  );

  /// Ubah harga / beri potongan satu baris setelah penyetuju lolos cek PIN. [price]/[disc] null = hapus.
  void adjustLine(
    String itemId, {
    Decimal? price,
    Decimal? disc,
    bool discTotal = false,
    required ApprovalGrant approval,
  }) => _changed(
    state.copyWith(
      approval: approval,
      lines: [
        for (final l in state.lines)
          l.item.id == itemId
              ? l.withAdjust(price: price, disc: disc, total: discTotal)
              : l,
      ],
    ),
  );

  /// Kembalikan baris ke harga normal tanpa potongan; penyetuju dibuang bila tak ada lagi baris yang diubah.
  void resetLine(String itemId) {
    final lines = [
      for (final l in state.lines) l.item.id == itemId ? l.withAdjust() : l,
    ];
    _changed(
      state.copyWith(
        lines: lines,
        clearApproval: !lines.any((l) => l.adjusted),
      ),
    );
  }

  /// Ganti seluruh rincian biaya lain-lain (baris tanpa jumlah dibuang; maks [CartState.maxCosts]).
  void setCosts(List<CostEntry> costs) => _changed(
    state.copyWith(
      costs: [
        for (final c in costs)
          if (c.amount > Decimal.zero) c,
      ].take(CartState.maxCosts).toList(),
    ),
  );

  /// Keterangan nota; tidak memicu quote ulang (tidak memengaruhi total).
  void setNote(String v) {
    final t = v.length > CartState.maxNote
        ? v.substring(0, CartState.maxNote)
        : v;
    state = state.copyWith(note: t);
    _persist();
  }

  /// Salesman nota (label laporan/komisi); null = Umum.
  void setSalesperson(Salesperson? s) => _changed(
    s == null
        ? state.copyWith(clearSalesperson: true)
        : state.copyWith(salesperson: s),
  );

  /// Poin yang ditukar jadi potongan nota; batas wajar dijaga server (galat bila melebihi saldo/nilai belanja).
  void setRedeem(int points) =>
      _changed(state.copyWith(redeemPoints: points < 0 ? 0 : points));

  void _changed(CartState next) {
    _debounce?.cancel();
    if (next.lines.isEmpty) {
      _seq++;
      // Keranjang kosong = transaksi baru: potongan nota, biaya lain, dan penyetuju sisa nota sebelumnya tidak terbawa.
      state = next.copyWith(
        clearQuote: true,
        quoting: false,
        clearError: true,
        costs: const [],
        clearApproval: true,
        note: '',
      );
      _persist();
      return;
    }
    // Total lama tidak ditampilkan sebagai benar: tandai sedang menghitung ulang.
    state = next.copyWith(quoting: true, clearError: true);
    _persist();
    _debounce = Timer(const Duration(milliseconds: 250), _runQuote);
  }

  Future<void> _runQuote() async {
    final my = ++_seq;
    final snapshot = state;
    try {
      final q = await ref.read(posRepositoryProvider).quote(snapshot.draft());
      if (my != _seq) return;
      state = state.copyWith(quote: q, quoting: false, clearError: true);
    } on ApiError catch (e) {
      if (my != _seq) return;
      state = state.copyWith(clearQuote: true, quoting: false, quoteError: e);
    }
  }
}

/// Shift kasir di outlet aktif. `null` = belum ada shift terbuka.
final shiftProvider = AsyncNotifierProvider<ShiftController, ShiftInfo?>(
  ShiftController.new,
);

class ShiftController extends AsyncNotifier<ShiftInfo?> {
  @override
  Future<ShiftInfo?> build() => ref.read(posRepositoryProvider).currentShift();

  Future<void> open(String openingCash) async {
    final s = await ref.read(posRepositoryProvider).openShift(openingCash);
    state = AsyncData(s);
  }

  /// Dipanggil setelah shift berhasil ditutup di server.
  void closed() => state = const AsyncData(null);
}

final paymentMethodsProvider = FutureProvider.autoDispose<List<PayMethodInfo>>(
  (ref) => ref.read(posRepositoryProvider).paymentMethods(),
);
