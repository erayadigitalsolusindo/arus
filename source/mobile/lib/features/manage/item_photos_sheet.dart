import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:image_picker/image_picker.dart';

import '../../core/api/api_error.dart';
import '../../core/theme/app_theme.dart';
import '../../l10n/gen/app_localizations.dart';
import '../pos/item_thumb.dart';
import 'manage_repository.dart';

const _maxPhotos = 5; // sama dengan batas server (item.MaxImages)

/// Kelola foto satu barang: tambah, ganti, hapus, jadikan utama. Mengembalikan true bila ada yang berubah.
Future<bool> showItemPhotosSheet(
  BuildContext context, {
  required String itemId,
  required String name,
}) async =>
    await showModalBottomSheet<bool>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (_) => _PhotosSheet(itemId: itemId, name: name),
    ) ??
    false;

class _PhotosSheet extends ConsumerStatefulWidget {
  const _PhotosSheet({required this.itemId, required this.name});

  final String itemId;
  final String name;

  @override
  ConsumerState<_PhotosSheet> createState() => _PhotosSheetState();
}

class _PhotosSheetState extends ConsumerState<_PhotosSheet> {
  List<({String id, bool isMain})> _imgs = const [];
  bool _loading = true;
  bool _busy = false;
  bool _changed = false;

  ManageRepository get _repo => ref.read(manageRepositoryProvider);

  @override
  void initState() {
    super.initState();
    _reload();
  }

  void _snack(String t) => ScaffoldMessenger.of(context)
    ..clearSnackBars()
    ..showSnackBar(SnackBar(content: Text(t)));

  Future<void> _reload() async {
    try {
      final r = await _repo.itemImages(widget.itemId);
      if (mounted) setState(() => _imgs = r);
    } on ApiError catch (e) {
      if (mounted) _snack(e.message(AppLocalizations.of(context)));
    }
    if (mounted) setState(() => _loading = false);
  }

  /// Jalankan aksi; galat server tampil sebagai snackbar, sukses memuat ulang daftar.
  Future<void> _run(Future<void> Function() fn, [String? ok]) async {
    final l = AppLocalizations.of(context);
    setState(() => _busy = true);
    try {
      await fn();
      _changed = true;
      if (ok != null && mounted) _snack(ok);
      await _reload();
    } on ApiError catch (e) {
      if (mounted) _snack(e.message(l));
    } catch (_) {
      if (mounted) _snack(l.itemsPhotoFailed);
    }
    if (mounted) setState(() => _busy = false);
  }

  /// Foto dari kamera/galeri, dikecilkan di perangkat (server tetap memvalidasi dan mengompres ulang).
  Future<String?> _pick() async {
    final l = AppLocalizations.of(context);
    final src = await showModalBottomSheet<ImageSource>(
      context: context,
      builder: (ctx) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              leading: const Icon(Icons.photo_camera_outlined),
              title: Text(l.itemsPhotoCamera),
              onTap: () => Navigator.pop(ctx, ImageSource.camera),
            ),
            ListTile(
              leading: const Icon(Icons.photo_library_outlined),
              title: Text(l.itemsPhotoGallery),
              onTap: () => Navigator.pop(ctx, ImageSource.gallery),
            ),
          ],
        ),
      ),
    );
    if (src == null) return null;
    final f = await ImagePicker().pickImage(
      source: src,
      maxWidth: 1600,
      maxHeight: 1600,
      imageQuality: 85,
    );
    return f?.path;
  }

  Future<void> _add() async {
    final l = AppLocalizations.of(context);
    final path = await _pick();
    if (path == null) return;
    await _run(
      () => _repo.uploadItemImage(widget.itemId, path),
      l.itemsPhotoSaved(widget.name),
    );
  }

  /// Ganti = unggah yang baru, pindahkan status utama, lalu hapus yang lama.
  Future<void> _replace(({String id, bool isMain}) old) async {
    final l = AppLocalizations.of(context);
    final path = await _pick();
    if (path == null) return;
    await _run(() async {
      final before = {for (final i in _imgs) i.id};
      if (_imgs.length >= _maxPhotos) {
        await _repo.deleteItemImage(widget.itemId, old.id);
      }
      await _repo.uploadItemImage(widget.itemId, path);
      final now = await _repo.itemImages(widget.itemId);
      final fresh = now.where((i) => !before.contains(i.id)).firstOrNull;
      if (old.isMain && fresh != null) {
        await _repo.setMainItemImage(widget.itemId, fresh.id);
      }
      if (_imgs.length < _maxPhotos) {
        await _repo.deleteItemImage(widget.itemId, old.id);
      }
    }, l.itemsPhotoSaved(widget.name));
  }

  Future<void> _delete(({String id, bool isMain}) img) async {
    final l = AppLocalizations.of(context);
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(l.itemsPhotoDelete),
        content: Text(l.itemsPhotoDeleteAsk(widget.name)),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: Text(l.commonCancel),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: Text(l.itemsPhotoDelete),
          ),
        ],
      ),
    );
    if (ok != true) return;
    await _run(() async {
      await _repo.deleteItemImage(widget.itemId, img.id);
      // Foto utama dihapus: jadikan foto pertama yang tersisa sebagai utama.
      if (img.isMain) {
        final rest = _imgs.where((i) => i.id != img.id).firstOrNull;
        if (rest != null) await _repo.setMainItemImage(widget.itemId, rest.id);
      }
    }, l.itemsPhotoDeleted);
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    return PopScope(
      canPop: false,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop) Navigator.pop(context, _changed);
      },
      child: Padding(
        padding: EdgeInsets.fromLTRB(
          16,
          0,
          16,
          16 + MediaQuery.of(context).viewInsets.bottom,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              '${l.itemsPhoto} · ${widget.name}',
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 12),
            if (_loading)
              const Padding(
                padding: EdgeInsets.all(24),
                child: Center(child: CircularProgressIndicator()),
              )
            else ...[
              if (_imgs.isEmpty)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 24),
                  child: Center(
                    child: Text(
                      l.itemsPhotoEmpty,
                      style: TextStyle(color: pal.textTertiary),
                    ),
                  ),
                )
              else
                GridView.count(
                  crossAxisCount: 3,
                  mainAxisSpacing: 10,
                  crossAxisSpacing: 10,
                  shrinkWrap: true,
                  physics: const NeverScrollableScrollPhysics(),
                  children: [for (final i in _imgs) _tile(i, l, pal)],
                ),
              const SizedBox(height: 14),
              SizedBox(
                width: double.infinity,
                child: FilledButton.icon(
                  onPressed: _busy
                      ? null
                      : _imgs.length >= _maxPhotos
                      ? () => _snack(l.itemsPhotoLimit)
                      : _add,
                  icon: const Icon(Icons.add_a_photo_outlined),
                  label: Text(l.itemsPhotoAdd),
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget _tile(({String id, bool isMain}) i, AppLocalizations l, AppPalette pal) {
    return Stack(
      fit: StackFit.expand,
      children: [
        ClipRRect(
          borderRadius: BorderRadius.circular(12),
          child: ItemThumb(itemId: widget.itemId, imageId: i.id),
        ),
        if (i.isMain)
          Positioned(
            left: 6,
            top: 6,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
              decoration: BoxDecoration(
                color: pal.accent,
                borderRadius: BorderRadius.circular(8),
              ),
              child: Text(
                l.itemsPhotoMainBadge,
                style: const TextStyle(
                  color: Colors.white,
                  fontSize: 10,
                  fontWeight: FontWeight.w800,
                ),
              ),
            ),
          ),
        Positioned(
          right: 2,
          top: 2,
          child: PopupMenuButton<int>(
            enabled: !_busy,
            icon: const CircleAvatar(
              radius: 13,
              backgroundColor: Colors.black54,
              child: Icon(Icons.more_vert, size: 16, color: Colors.white),
            ),
            padding: EdgeInsets.zero,
            onSelected: (v) => switch (v) {
              0 => _run(
                () => _repo.setMainItemImage(widget.itemId, i.id),
              ),
              1 => _replace(i),
              _ => _delete(i),
            },
            itemBuilder: (_) => [
              if (!i.isMain)
                PopupMenuItem(
                  value: 0,
                  child: _menuRow(Icons.star_outline, l.itemsPhotoMain),
                ),
              PopupMenuItem(
                value: 1,
                child: _menuRow(Icons.edit_outlined, l.itemsPhotoReplace),
              ),
              PopupMenuItem(
                value: 2,
                child: _menuRow(Icons.delete_outline, l.itemsPhotoDelete),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _menuRow(IconData icon, String text) => Row(
    children: [Icon(icon, size: 18), const SizedBox(width: 10), Text(text)],
  );
}
