import 'dart:convert';

import 'package:decimal/decimal.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../core/money/money.dart';
import '../../core/session/session_controller.dart';
import 'pos_controller.dart';

const maxPending = 20;
const maxPendingLabel = 60;
const _maxAge = Duration(hours: 24);

/// Nota pending: keranjang yang diparkir kasir lalu dibuka lagi. Disimpan di perangkat (per kasir + cabang), bukan
/// dokumen server; harga & stok dihitung ulang server lewat quote saat dibuka dan dibayar (FR-POS-09).
class PendingNote {
  const PendingNote({
    required this.id,
    required this.no,
    required this.at,
    required this.label,
    required this.total,
    required this.cart,
  });

  final String id;

  /// Nomor urut tampilan (Pending 1, 2, ...), unik di dalam daftar.
  final int no;
  final DateTime at;
  final String label;

  /// Perkiraan total saat diparkir; hanya tampilan.
  final Decimal? total;
  final CartState cart;

  int get lineCount => cart.lines.length;

  Map<String, dynamic> toJson() => {
    'id': id,
    'no': no,
    'at': at.millisecondsSinceEpoch,
    'label': label,
    'total': total == null ? null : decToApi(total!, scale: 2),
    'cart': cart.toSnapshot(),
  };

  static PendingNote? fromJson(Map<String, dynamic> j, DateTime now) {
    try {
      final at = DateTime.fromMillisecondsSinceEpoch((j['at'] as num).toInt());
      // Kedaluwarsa 24 jam; jam perangkat yang melompat ke depan juga dibuang.
      if (now.difference(at) > _maxAge ||
          at.isAfter(now.add(const Duration(minutes: 1)))) {
        return null;
      }
      final cart = CartState.fromSnapshot(
        (j['cart'] as Map).cast<String, dynamic>(),
      );
      if (cart == null) return null;
      final label = '${j['label'] ?? ''}';
      return PendingNote(
        id: '${j['id']}',
        no: (j['no'] as num).toInt(),
        at: at,
        label: label.length > maxPendingLabel
            ? label.substring(0, maxPendingLabel)
            : label,
        total: j['total'] == null ? null : dec(j['total']),
        cart: cart,
      );
    } catch (_) {
      return null;
    }
  }
}

final pendingProvider = NotifierProvider<PendingController, List<PendingNote>>(
  PendingController.new,
);

class PendingController extends Notifier<List<PendingNote>> {
  @override
  List<PendingNote> build() => const [];

  String? get _key {
    final s = ref.read(sessionProvider);
    if (s is! SessionSignedIn) return null;
    return 'pos.pending.v1.${s.profile.user.id}.${s.profile.outlet.id}';
  }

  /// Muat daftar kasir + cabang aktif (yang kedaluwarsa dibuang). Dipanggil saat layar kasir dibuka / cabang berganti.
  Future<void> load() async {
    final key = _key;
    state = const [];
    if (key == null) return;
    try {
      final raw = (await SharedPreferences.getInstance()).getString(key);
      if (raw == null || _key != key) return;
      final now = DateTime.now();
      final list = <PendingNote>[
        for (final e in (jsonDecode(raw) as List))
          ?PendingNote.fromJson((e as Map).cast<String, dynamic>(), now),
      ]..sort((a, b) => b.at.compareTo(a.at));
      state = list.take(maxPending).toList();
      if (state.length != (jsonDecode(raw) as List).length) await _save();
    } catch (_) {
      // Penyimpanan tidak tersedia atau isinya rusak: daftar kosong.
    }
  }

  Future<void> _save() async {
    final key = _key;
    if (key == null) return;
    try {
      final p = await SharedPreferences.getInstance();
      if (state.isEmpty) {
        await p.remove(key);
      } else {
        await p.setString(key, jsonEncode([for (final n in state) n.toJson()]));
      }
    } catch (_) {}
  }

  bool get isFull => state.length >= maxPending;

  /// Parkir isi keranjang sekarang ke [list]; null bila keranjang kosong.
  List<PendingNote>? _parkInto(List<PendingNote> list, String label) {
    final cart = ref.read(cartProvider);
    if (cart.isEmpty) return null;
    final no = list.fold(0, (m, n) => n.no > m ? n.no : m) + 1;
    final trimmed = label.trim();
    return [
      PendingNote(
        id: DateTime.now().microsecondsSinceEpoch.toRadixString(36),
        no: no,
        at: DateTime.now(),
        label: trimmed.length > maxPendingLabel
            ? trimmed.substring(0, maxPendingLabel)
            : trimmed,
        total: cart.quote != null && !cart.quoting ? cart.quote!.total : null,
        // Hanya isi nota: ubah harga/potongan, quote, dan PIN penyetuju tidak ikut (dihitung & diminta ulang saat dibuka).
        cart: CartState.fromSnapshot(cart.toSnapshot())!,
      ),
      ...list,
    ];
  }

  /// Tunda keranjang sekarang. Mengembalikan nomor pending, atau null bila keranjang kosong / daftar penuh.
  int? hold(String label) {
    if (isFull) return null;
    final next = _parkInto(state, label);
    if (next == null) return null;
    state = next;
    ref.read(cartProvider.notifier).clear();
    _save();
    return next.first.no;
  }

  /// Buka nota pending. Keranjang yang sedang berisi diparkir dulu (tukar tempat dengan nota yang dibuka).
  /// Mengembalikan nomor pending hasil tukar (null bila keranjang tadinya kosong).
  int? open(PendingNote n) {
    final rest = [
      for (final p in state)
        if (p.id != n.id) p,
    ];
    final parked = _parkInto(rest, '');
    state = parked ?? rest;
    ref.read(cartProvider.notifier).load(n.cart);
    _save();
    return parked?.first.no;
  }

  void delete(PendingNote n) {
    state = [
      for (final p in state)
        if (p.id != n.id) p,
    ];
    _save();
  }
}
