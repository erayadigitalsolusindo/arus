import 'dart:async';

import 'package:decimal/decimal.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_error.dart';
import 'pos_models.dart';
import 'pos_repository.dart';

class CartState {
  const CartState({this.lines = const [], this.applyTax = false, this.quote, this.quoting = false, this.quoteError});

  final List<CartLine> lines;
  final bool applyTax;

  /// Hasil hitung server untuk isi keranjang saat ini; null = belum/gagal dihitung.
  final Quote? quote;
  final bool quoting;
  final ApiError? quoteError;

  bool get isEmpty => lines.isEmpty;

  Decimal get itemCount => lines.fold(Decimal.zero, (a, l) => a + l.qty);

  /// Siap dibayar: ada isi, total sudah dari server untuk isi ini, dan tidak ada masalah stok/HPP.
  bool get canPay => lines.isNotEmpty && quote != null && !quoting && quoteError == null && !quote!.hasBlockingIssue;

  CartState copyWith({List<CartLine>? lines, bool? applyTax, Quote? quote, bool clearQuote = false, bool? quoting, ApiError? quoteError, bool clearError = false}) =>
      CartState(
        lines: lines ?? this.lines,
        applyTax: applyTax ?? this.applyTax,
        quote: clearQuote ? null : (quote ?? this.quote),
        quoting: quoting ?? this.quoting,
        quoteError: clearError ? null : (quoteError ?? this.quoteError),
      );
}

final cartProvider = NotifierProvider<CartController, CartState>(CartController.new);

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
    _changed(state.copyWith(lines: [for (final l in state.lines) l.item.id == itemId ? l.withQty(qty) : l]));
  }

  void remove(String itemId) => _changed(state.copyWith(lines: [for (final l in state.lines) if (l.item.id != itemId) l]));

  void clear() {
    _seq++;
    _debounce?.cancel();
    state = CartState(applyTax: state.applyTax);
  }

  void setTax(bool on) => _changed(state.copyWith(applyTax: on));

  void _changed(CartState next) {
    _debounce?.cancel();
    if (next.lines.isEmpty) {
      _seq++;
      state = next.copyWith(clearQuote: true, quoting: false, clearError: true);
      return;
    }
    // Total lama tidak ditampilkan sebagai benar: tandai sedang menghitung ulang.
    state = next.copyWith(quoting: true, clearError: true);
    _debounce = Timer(const Duration(milliseconds: 250), _runQuote);
  }

  Future<void> _runQuote() async {
    final my = ++_seq;
    final snapshot = state;
    try {
      final q = await ref.read(posRepositoryProvider).quote(snapshot.lines, applyTax: snapshot.applyTax);
      if (my != _seq) return;
      state = state.copyWith(quote: q, quoting: false, clearError: true);
    } on ApiError catch (e) {
      if (my != _seq) return;
      state = state.copyWith(clearQuote: true, quoting: false, quoteError: e);
    }
  }
}

/// Shift kasir di outlet aktif. `null` = belum ada shift terbuka.
final shiftProvider = AsyncNotifierProvider<ShiftController, ShiftInfo?>(ShiftController.new);

class ShiftController extends AsyncNotifier<ShiftInfo?> {
  @override
  Future<ShiftInfo?> build() => ref.read(posRepositoryProvider).currentShift();

  Future<void> open(String openingCash) async {
    final s = await ref.read(posRepositoryProvider).openShift(openingCash);
    state = AsyncData(s);
  }
}

final paymentMethodsProvider = FutureProvider.autoDispose<List<PayMethodInfo>>((ref) => ref.read(posRepositoryProvider).paymentMethods());
