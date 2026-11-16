import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/theme/app_theme.dart';
import 'pos_repository.dart';

/// Thumbnail gambar barang; di-cache selama aplikasi hidup (gambar server tak berubah per id).
final itemThumbProvider = FutureProvider.family<Uint8List, (String, String)>(
  (ref, k) => ref.read(posRepositoryProvider).itemThumb(k.$1, k.$2),
);

class ItemThumb extends ConsumerWidget {
  const ItemThumb({
    super.key,
    required this.itemId,
    required this.imageId,
    this.name = '',
  });

  final String itemId;
  final String? imageId;

  /// Nama barang: tanpa gambar, tampil sebagai inisial (maks 2 huruf).
  final String name;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final pal = context.pal;
    Widget placeholder(IconData icon) => ColoredBox(
      color: pal.primarySoft,
      child: Center(child: Icon(icon, color: pal.textTertiary, size: 28)),
    );
    final id = imageId;
    if (id == null || id.isEmpty) {
      final ini = _initials(name);
      if (ini.isEmpty) return placeholder(Icons.inventory_2_outlined);
      return ColoredBox(
        color: pal.primarySoft,
        child: Center(
          child: FittedBox(
            child: Padding(
              padding: const EdgeInsets.all(8),
              child: Text(
                ini,
                style: TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.w800,
                  color: pal.primary,
                ),
              ),
            ),
          ),
        ),
      );
    }
    return ref
        .watch(itemThumbProvider((itemId, id)))
        .when(
          data: (b) => Image.memory(
            b,
            fit: BoxFit.cover,
            gaplessPlayback: true,
            errorBuilder: (_, _, _) => placeholder(Icons.broken_image_outlined),
          ),
          loading: () => placeholder(Icons.image_outlined),
          error: (_, _) => placeholder(Icons.broken_image_outlined),
        );
  }
}

/// Inisial dari dua kata pertama nama ("Kopi Susu" → "KS"); satu kata → dua huruf awal.
String _initials(String name) {
  final w = name.trim().split(RegExp(r'\s+')).where((e) => e.isNotEmpty).toList();
  if (w.isEmpty) return '';
  if (w.length == 1) {
    final r = w.first.runes.take(2);
    return String.fromCharCodes(r).toUpperCase();
  }
  return (String.fromCharCode(w[0].runes.first) +
          String.fromCharCode(w[1].runes.first))
      .toUpperCase();
}
