import 'dart:async';

import 'package:flutter/services.dart' show appFlavor;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:in_app_update/in_app_update.dart' as iau;

/// Flavor Play memperbarui lewat Google Play (In-App Update), bukan unduh APK sendiri.
bool get isPlayFlavor => appFlavor == 'play';

/// Hasil tanya Play: cara memasang dan prioritas yang diatur di Play Console.
class PlayUpdateOffer {
  const PlayUpdateOffer({
    required this.versionCode,
    required this.priority,
    required this.flexibleAllowed,
    required this.immediateAllowed,
  });

  final int versionCode;
  final int priority;
  final bool flexibleAllowed;
  final bool immediateAllowed;
}

/// Pembungkus plugin supaya logika bisa dites tanpa Play.
class PlayUpdateClient {
  const PlayUpdateClient();

  /// null = tak ada pembaruan. Melempar bila Play tak tersedia (APK biasa, emulator tanpa Play).
  Future<PlayUpdateOffer?> check() async {
    final i = await iau.InAppUpdate.checkForUpdate();
    if (i.updateAvailability != iau.UpdateAvailability.updateAvailable) {
      return null;
    }
    return PlayUpdateOffer(
      versionCode: i.availableVersionCode ?? 0,
      priority: i.updatePriority,
      flexibleAllowed: i.flexibleUpdateAllowed,
      immediateAllowed: i.immediateUpdateAllowed,
    );
  }

  /// Unduh di latar belakang; true bila pengguna menyetujui. Selesai unduh lewat [downloaded].
  Future<bool> startFlexible() async =>
      await iau.InAppUpdate.startFlexibleUpdate() ==
      iau.AppUpdateResult.success;

  /// Layar penuh milik Play (unduh + mulai ulang otomatis); true bila disetujui.
  Future<bool> performImmediate() async =>
      await iau.InAppUpdate.performImmediateUpdate() ==
      iau.AppUpdateResult.success;

  Future<void> completeFlexible() => iau.InAppUpdate.completeFlexibleUpdate();

  /// true saat unduhan fleksibel selesai; false bila gagal atau dibatalkan.
  Stream<bool> get downloaded => iau.InAppUpdate.installUpdateListener
      .where(
        (s) =>
            s == iau.InstallStatus.downloaded ||
            s == iau.InstallStatus.failed ||
            s == iau.InstallStatus.canceled,
      )
      .map((s) => s == iau.InstallStatus.downloaded);
}

final playUpdateClientProvider = Provider<PlayUpdateClient>(
  (_) => const PlayUpdateClient(),
);

enum PlayUpdatePhase {
  none,
  checking,
  available,
  downloading,
  downloaded,
  upToDate,
  failed,
}

class PlayUpdateState {
  const PlayUpdateState({
    this.phase = PlayUpdatePhase.none,
    this.offer,
    this.showCard = false,
  });

  final PlayUpdatePhase phase;
  final PlayUpdateOffer? offer;

  /// Kartu pembaruan tampil di atas aplikasi (otomatis saat dibuka, atau dari tombol Cek pembaruan).
  final bool showCard;

  /// Ada versi baru yang belum terpasang (lencana pada tombol Cek pembaruan).
  bool get hasUpdate =>
      offer != null &&
      phase != PlayUpdatePhase.none &&
      phase != PlayUpdatePhase.upToDate;

  /// Ada versi baru = wajib diperbarui, tanpa tombol "Nanti".
  bool get forced => offer != null;

  PlayUpdateState copyWith({
    PlayUpdatePhase? phase,
    PlayUpdateOffer? offer,
    bool clearOffer = false,
    bool? showCard,
  }) => PlayUpdateState(
    phase: phase ?? this.phase,
    offer: clearOffer ? null : (offer ?? this.offer),
    showCard: showCard ?? this.showCard,
  );
}

final playUpdateProvider =
    NotifierProvider<PlayUpdateController, PlayUpdateState>(
      PlayUpdateController.new,
    );

class PlayUpdateController extends Notifier<PlayUpdateState> {
  StreamSubscription<bool>? _sub;
  bool _autoPrompted = false;
  DateTime? _lastCheck;

  @override
  PlayUpdateState build() {
    ref.onDispose(() => _sub?.cancel());
    return const PlayUpdateState();
  }

  /// Cek ke Play. [manual] (tombol Cek pembaruan) selalu menampilkan hasilnya, termasuk "sudah terbaru" dan galat.
  /// Cek otomatis (dibuka/dilanjutkan) diam bila tak ada pembaruan, dibatasi sekali per 30 menit, dan kartunya
  /// hanya muncul sekali per pembukaan aplikasi (kecuali wajib).
  Future<void> check({bool manual = false}) async {
    final busy =
        state.phase == PlayUpdatePhase.downloading ||
        state.phase == PlayUpdatePhase.downloaded ||
        state.phase == PlayUpdatePhase.checking;
    if (busy) {
      if (manual) state = state.copyWith(showCard: true);
      return;
    }
    final now = DateTime.now();
    if (!manual &&
        _lastCheck != null &&
        now.difference(_lastCheck!) < const Duration(minutes: 30)) {
      return;
    }
    _lastCheck = now;
    if (manual) {
      state = state.copyWith(phase: PlayUpdatePhase.checking, showCard: true);
    }
    try {
      final offer = await ref.read(playUpdateClientProvider).check();
      if (offer == null) {
        state = manual
            ? state.copyWith(phase: PlayUpdatePhase.upToDate, clearOffer: true)
            : const PlayUpdateState();
        return;
      }
      final next = state.copyWith(
        phase: PlayUpdatePhase.available,
        offer: offer,
      );
      final show = manual || next.forced || !_autoPrompted;
      if (!manual) _autoPrompted = true;
      state = next.copyWith(showCard: show);
      // Cek manual yang menemukan versi baru langsung memperbarui (layar penuh Play), tanpa tombol "Nanti".
      if (manual) await start(immediate: true);
    } catch (_) {
      // Play tak tersedia (APK biasa, tanpa Play Store) atau jaringan: diam saat otomatis, kabari saat manual.
      state = manual
          ? state.copyWith(phase: PlayUpdatePhase.failed, showCard: true)
          : const PlayUpdateState();
    }
  }

  /// Tutup kartu tanpa menunda (hasil "sudah terbaru"/galat, atau kartu yang dibuka manual).
  void dismissCard() {
    if (state.phase == PlayUpdatePhase.upToDate ||
        state.phase == PlayUpdatePhase.failed) {
      state = state.copyWith(
        phase: PlayUpdatePhase.none,
        clearOffer: true,
        showCard: false,
      );
    } else {
      state = state.copyWith(showCard: false);
    }
  }

  /// Mulai memperbarui: wajib atau tak bisa fleksibel → layar penuh Play; selain itu unduh di latar belakang.
  Future<void> start({bool immediate = false}) async {
    final o = state.offer;
    if (o == null) return;
    final client = ref.read(playUpdateClientProvider);
    try {
      if (state.forced ||
          !o.flexibleAllowed ||
          (immediate && o.immediateAllowed)) {
        await client.performImmediate();
        return;
      }
      if (!await client.startFlexible()) return;
      // Unduhan berjalan di latar belakang: kartu ditutup agar kasir tetap bisa bekerja.
      state = state.copyWith(
        phase: PlayUpdatePhase.downloading,
        showCard: false,
      );
      await _sub?.cancel();
      _sub = client.downloaded.listen((ok) {
        state = state.copyWith(
          phase: ok ? PlayUpdatePhase.downloaded : PlayUpdatePhase.failed,
          showCard: true,
        );
      });
    } catch (_) {
      state = state.copyWith(phase: PlayUpdatePhase.failed, showCard: true);
    }
  }

  /// Pasang unduhan fleksibel yang sudah selesai (aplikasi dimulai ulang oleh Play).
  Future<void> install() =>
      ref.read(playUpdateClientProvider).completeFlexible();
}
