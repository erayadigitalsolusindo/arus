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
  const ItemThumb({super.key, required this.itemId, required this.imageId});

  final String itemId;
  final String? imageId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final pal = context.pal;
    Widget placeholder(IconData icon) => ColoredBox(
      color: pal.primarySoft,
      child: Center(child: Icon(icon, color: pal.textTertiary, size: 28)),
    );
    final id = imageId;
    if (id == null || id.isEmpty) {
      return placeholder(Icons.inventory_2_outlined);
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
