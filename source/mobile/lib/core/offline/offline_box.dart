import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

/// Penyimpanan kecil di perangkat untuk mode offline (cache katalog, metode bayar, shift, antrean nota),
/// per kasir + cabang. `scope` null = belum login: tidak membaca/menulis apa pun.
class OfflineBox {
  const OfflineBox(this.scope);

  final String? scope;

  String? _k(String name) => scope == null ? null : 'offline.v1.$scope.$name';

  Future<Object?> read(String name) async {
    final k = _k(name);
    if (k == null) return null;
    try {
      final raw = (await SharedPreferences.getInstance()).getString(k);
      return raw == null ? null : jsonDecode(raw);
    } catch (_) {
      return null; // penyimpanan tidak tersedia / rusak: anggap kosong
    }
  }

  /// null menghapus.
  Future<void> write(String name, Object? value) async {
    final k = _k(name);
    if (k == null) return;
    try {
      final p = await SharedPreferences.getInstance();
      if (value == null) {
        await p.remove(k);
      } else {
        await p.setString(k, jsonEncode(value));
      }
    } catch (_) {}
  }
}
