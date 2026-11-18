import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_error.dart';
import '../../core/session/session_controller.dart';
import 'pos_controller.dart';
import 'pos_repository.dart';

/// Nota yang dibuat saat offline dan menunggu dikirim. `key` = Idempotency-Key (kirim ulang aman, tidak ganda).
/// `error` != null = ditolak server (bukan masalah jaringan): perlu ditinjau kasir/pemilik.
class QueuedSale {
  const QueuedSale({
    required this.key,
    required this.body,
    required this.at,
    required this.total,
    this.error,
  });

  final String key;
  final Map<String, dynamic> body;
  final DateTime at;

  /// Perkiraan total saat dibuat (hanya tampilan; server menghitung ulang).
  final String total;
  final String? error;

  bool get failed => error != null;

  String get docNo => 'OFF-${key.substring(0, 6).toUpperCase()}';

  QueuedSale withError(String? e) =>
      QueuedSale(key: key, body: body, at: at, total: total, error: e);

  Map<String, dynamic> toJson() => {
    'key': key,
    'body': body,
    'at': at.millisecondsSinceEpoch,
    'total': total,
    'error': error,
  };

  static QueuedSale? fromJson(Object? j) {
    try {
      final m = (j as Map).cast<String, dynamic>();
      return QueuedSale(
        key: '${m['key']}',
        body: (m['body'] as Map).cast<String, dynamic>(),
        at: DateTime.fromMillisecondsSinceEpoch((m['at'] as num).toInt()),
        total: '${m['total']}',
        error: m['error'] as String?,
      );
    } catch (_) {
      return null;
    }
  }
}

final outboxProvider = NotifierProvider<OutboxController, List<QueuedSale>>(
  OutboxController.new,
);

/// Antrean nota offline di perangkat (per kasir + cabang). Dikirim berurutan sesuai waktu transaksi.
class OutboxController extends Notifier<List<QueuedSale>> {
  bool _busy = false;

  @override
  List<QueuedSale> build() => const [];

  /// Nota yang masih akan dikirim (belum ditolak server).
  int get pendingCount => state.where((s) => !s.failed).length;

  Future<void> load() async {
    final raw = await ref.read(posRepositoryProvider).offlineBox.read('outbox');
    state = [
      if (raw is List)
        for (final e in raw) ?QueuedSale.fromJson(e),
    ];
  }

  Future<void> _save() => ref
      .read(posRepositoryProvider)
      .offlineBox
      .write(
        'outbox',
        state.isEmpty ? null : [for (final s in state) s.toJson()],
      );

  Future<void> add(QueuedSale s) async {
    state = [...state, s]..sort((a, b) => a.at.compareTo(b.at));
    await _save(); // simpan ke disk sebelum apa pun, supaya tidak hilang bila aplikasi mati
  }

  Future<void> remove(String key) async {
    state = [
      for (final s in state)
        if (s.key != key) s,
    ];
    await _save();
  }

  /// Kirim semua nota yang menunggu. Berhenti di galat jaringan/server (dicoba lagi nanti); penolakan permanen
  /// (mis. validasi) menandai nota itu dan lanjut ke berikutnya.
  Future<void> flush({bool retryFailed = false}) async {
    if (_busy || state.isEmpty) return;
    _busy = true;
    try {
      if (retryFailed) {
        state = [for (final s in state) s.withError(null)];
      }
      var sent = false;
      for (final s in [...state]) {
        if (s.failed) continue;
        try {
          await ref.read(posRepositoryProvider).sendQueued(s);
          state = [
            for (final x in state)
              if (x.key != s.key) x,
          ];
          sent = true;
        } on ApiError catch (e) {
          if (e.isNetwork || e.status >= 500 || e.status == 401) break;
          state = [
            for (final x in state)
              if (x.key == s.key) x.withError(e.code) else x,
          ];
        }
        await _save();
      }
      await _save();
      if (sent) ref.read(catalogVersionProvider.notifier).bump();
    } finally {
      _busy = false;
    }
  }
}

/// Dipasang sekali di beranda: memuat antrean, mengirimnya tiap 30 detik, dan menyegarkan cache katalog.
final offlineSyncProvider = Provider<void>((ref) {
  final s = ref.watch(sessionProvider);
  if (s is! SessionSignedIn) return;
  final box = ref.read(outboxProvider.notifier);
  var tick = 0;
  Future<void> run() async {
    await box.flush();
    if (tick++ % 40 == 0) await ref.read(posRepositoryProvider).warmCatalog();
  }

  unawaited(box.load().then((_) => run()));
  final t = Timer.periodic(const Duration(seconds: 30), (_) => run());
  ref.onDispose(t.cancel);
});
