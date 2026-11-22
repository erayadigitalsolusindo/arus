import 'dart:async';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:dio/dio.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../config/app_config.dart';
import '../config/server_url.dart';

/// Metadata APK terbaru di server (`GET /public/mobile/apk/info`, diunggah Platform Admin).
class AppUpdateInfo {
  const AppUpdateInfo({
    required this.version,
    required this.size,
    required this.sha256,
  });

  final String version;
  final int size;
  final String sha256;
}

/// Membandingkan versi `a.b.c[+build]`. Mengembalikan >0 bila [a] lebih baru dari [b].
/// Bagian build hanya dibandingkan bila kedua sisi memilikinya (Platform Admin boleh mengisi `1.0.1` saja).
int compareVersions(String a, String b) {
  List<int> nums(String v) => v
      .split('+')
      .first
      .split(RegExp(r'[.\-]'))
      .map((p) => int.tryParse(p.replaceAll(RegExp(r'\D'), '')) ?? 0)
      .toList();
  int? build(String v) =>
      v.contains('+') ? int.tryParse(v.split('+').last) : null;

  final x = nums(a), y = nums(b);
  final n = x.length > y.length ? x.length : y.length;
  for (var i = 0; i < n; i++) {
    final d = (i < x.length ? x[i] : 0) - (i < y.length ? y[i] : 0);
    if (d != 0) return d;
  }
  final bx = build(a), by = build(b);
  return bx != null && by != null ? bx - by : 0;
}

/// Jembatan ke Android (MainActivity.kt): info versi terpasang dan pembuka pemasang sistem.
class UpdaterPlatform {
  const UpdaterPlatform();

  static const _ch = MethodChannel('id.co.erayadigital.arus/updater');

  /// `versionName+versionCode` terpasang dan folder cache untuk unduhan; null bila bukan Android (tes, desktop).
  Future<({String version, String cacheDir})?> info() async {
    try {
      final m = await _ch.invokeMapMethod<String, Object?>('info');
      if (m == null) return null;
      return (
        version: '${m['versionName']}+${m['versionCode']}',
        cacheDir: m['cacheDir'] as String,
      );
    } on MissingPluginException {
      return null;
    } on PlatformException {
      return null;
    }
  }

  /// Mengantre unduhan ke DownloadManager sistem (berjalan di latar belakang). null bila bukan Android.
  Future<({int id, String path})?> startDownload(
    String url,
    Map<String, String> headers, {
    bool wifiOnly = false,
  }) async {
    try {
      final m = await _ch.invokeMapMethod<String, Object?>('download', {
        'url': url,
        'headers': headers,
        'wifiOnly': wifiOnly,
      });
      if (m == null) return null;
      return (id: (m['id'] as num).toInt(), path: m['path'] as String);
    } on MissingPluginException {
      return null;
    } on PlatformException {
      return null;
    }
  }

  /// state: pending | running | paused | done | failed | missing.
  Future<({String state, int got, int total})> downloadStatus(int id) async {
    try {
      final m = await _ch.invokeMapMethod<String, Object?>('downloadStatus', {
        'id': id,
      });
      return (
        state: '${m?['state'] ?? 'missing'}',
        got: (m?['got'] as num?)?.toInt() ?? 0,
        total: (m?['total'] as num?)?.toInt() ?? 0,
      );
    } on PlatformException {
      return (state: 'missing', got: 0, total: 0);
    }
  }

  Future<void> removeDownload(int id) async {
    try {
      await _ch.invokeMethod<void>('downloadRemove', {'id': id});
    } on PlatformException {
      // sudah hilang
    }
  }

  /// 'started' (pemasang terbuka) atau 'needs_permission' (layar izin "pasang dari sumber ini" dibuka).
  Future<String> install(String path) async =>
      (await _ch.invokeMethod<String>('install', {'path': path})) ?? 'started';
}

final updaterPlatformProvider = Provider<UpdaterPlatform>(
  (_) => const UpdaterPlatform(),
);

enum UpdatePhase {
  none,
  available,
  downloading,
  installing,
  needsPermission,
  failed,
}

class UpdateState {
  const UpdateState({
    this.phase = UpdatePhase.none,
    this.info,
    this.progress = 0,
    this.current = '',
    this.waiting = false,
  });

  final UpdatePhase phase;
  final AppUpdateInfo? info;
  final double progress;
  final String current;

  /// Unduhan antre menunggu Wi-Fi (pengaturan "hanya Wi-Fi"); pengguna boleh memilih pakai data.
  final bool waiting;

  /// Ada versi lebih baru: layar pembaruan wajib tampil sampai pembaruan terpasang.
  bool get required => phase != UpdatePhase.none;
}

/// Unduh pembaruan otomatis hanya lewat Wi-Fi (bawaan). Bila dimatikan, boleh memakai data seluler.
final updateWifiOnlyProvider = NotifierProvider<UpdateWifiOnly, bool>(
  UpdateWifiOnly.new,
);

class UpdateWifiOnly extends Notifier<bool> {
  static const _key = 'arus.update_wifi_only';

  @override
  bool build() {
    Future.microtask(() async {
      try {
        state = (await SharedPreferences.getInstance()).getBool(_key) ?? true;
      } catch (_) {}
    });
    return true;
  }

  Future<void> set(bool v) async {
    state = v;
    try {
      await (await SharedPreferences.getInstance()).setBool(_key, v);
    } catch (_) {}
  }
}

final appUpdateProvider = NotifierProvider<AppUpdateController, UpdateState>(
  AppUpdateController.new,
);

class AppUpdateController extends Notifier<UpdateState> {
  static const _prefsKey = 'arus.update_dl'; // "id|path|versi"
  bool _watching = false;
  int _gen = 0; // naik saat unduhan diganti, supaya pemantau lama berhenti

  @override
  UpdateState build() => const UpdateState();

  Dio _dio() => Dio(
    BaseOptions(
      baseUrl: ref.read(serverUrlProvider),
      connectTimeout: const Duration(seconds: 8),
      receiveTimeout: const Duration(seconds: 30),
      headers: {AppConfig.clientHeader: AppConfig.clientValue},
    ),
  );

  Future<List<String>> _saved() async {
    try {
      final v = (await SharedPreferences.getInstance()).getString(_prefsKey);
      final p = v?.split('|') ?? const <String>[];
      return p.length == 3 ? p : const [];
    } catch (_) {
      return const [];
    }
  }

  Future<void> _save(String? v) async {
    try {
      final sp = await SharedPreferences.getInstance();
      v == null ? await sp.remove(_prefsKey) : await sp.setString(_prefsKey, v);
    } catch (_) {
      // tanpa penyimpanan: unduhan tetap jalan, hanya tak bisa disambung setelah aplikasi ditutup
    }
  }

  /// Cek versi terbaru. Galat jaringan diabaikan (aplikasi tetap bisa dipakai tanpa pembaruan).
  Future<void> check() async {
    if (state.phase == UpdatePhase.downloading ||
        state.phase == UpdatePhase.installing) {
      return;
    }
    final updater = ref.read(updaterPlatformProvider);
    final local = await updater.info();
    if (local == null) return;
    try {
      final r = await _dio().get<Map<String, dynamic>>(
        '/public/mobile/apk/info',
      );
      final d = r.data;
      final remote = '${d?['version'] ?? ''}';
      if (d == null ||
          d['available'] != true ||
          remote.isEmpty ||
          compareVersions(remote, local.version) <= 0) {
        state = const UpdateState();
        final old = await _saved();
        if (old.isNotEmpty) {
          await _save(null);
          await updater.removeDownload(int.tryParse(old[0]) ?? -1);
        }
        return;
      }
      state = UpdateState(
        phase: UpdatePhase.available,
        info: AppUpdateInfo(
          version: remote,
          size: (d['size'] as num?)?.toInt() ?? 0,
          sha256: '${d['sha256'] ?? ''}',
        ),
        current: local.version,
      );
      // Unduhan yang masih berjalan sejak sebelum aplikasi ditutup/tidur: tampilkan lagi progresnya.
      final old = await _saved();
      if (old.isNotEmpty && old[2] == remote) {
        final st = await updater.downloadStatus(int.parse(old[0]));
        if (const {'pending', 'running', 'paused'}.contains(st.state)) {
          unawaited(_watch(int.parse(old[0]), old[1]));
          return;
        }
      }
      // Belum ada unduhan untuk versi ini: mulai otomatis (Wi-Fi saja kecuali diizinkan data).
      unawaited(start(auto: true));
    } on DioException {
      // Diam: dicek lagi saat aplikasi dibuka/dilanjutkan berikutnya.
    }
  }

  UpdateState _at(UpdatePhase p, [double progress = 0, bool waiting = false]) =>
      UpdateState(
        phase: p,
        info: state.info,
        current: state.current,
        progress: progress,
        waiting: waiting,
      );

  /// Unduh APK lewat DownloadManager (jalan di latar belakang, bisa lanjut), cocokkan SHA-256, lalu buka pemasang.
  /// [auto]: dipicu otomatis saat ada versi baru; patuhi pengaturan Wi-Fi. Tombol manual selalu boleh pakai data.
  Future<void> start({bool auto = false}) async {
    final info = state.info;
    if (info == null ||
        (state.phase == UpdatePhase.downloading && !state.waiting)) {
      return;
    }
    state = _at(UpdatePhase.downloading);
    try {
      final updater = ref.read(updaterPlatformProvider);
      final old = await _saved();
      if (old.isNotEmpty && old[2] == info.version) {
        final st = await updater.downloadStatus(int.parse(old[0]));
        final queued = st.state == 'pending' || st.state == 'paused';
        if (st.state != 'failed' &&
            st.state != 'missing' &&
            (auto || !queued)) {
          return await _watch(
            int.parse(old[0]),
            old[1],
          ); // sambung / pasang yang sudah selesai
        }
      }
      _gen++; // hentikan pemantau unduhan lama (antrean Wi-Fi) sebelum diganti
      _watching = false;
      if (old.isNotEmpty) await updater.removeDownload(int.parse(old[0]));
      final d = await updater.startDownload(
        '${ref.read(serverUrlProvider)}/public/mobile/apk',
        {AppConfig.clientHeader: AppConfig.clientValue},
        wifiOnly: auto && ref.read(updateWifiOnlyProvider),
      );
      if (d == null) throw StateError('download');
      await _save('${d.id}|${d.path}|${info.version}');
      await _watch(d.id, d.path);
    } catch (_) {
      state = _at(UpdatePhase.failed);
    }
  }

  Future<void> _watch(int id, String path) async {
    if (_watching) return;
    _watching = true;
    final g = _gen;
    final info = state.info;
    final updater = ref.read(updaterPlatformProvider);
    try {
      state = _at(UpdatePhase.downloading, state.progress);
      while (true) {
        final st = await updater.downloadStatus(id);
        if (g != _gen) return;
        if (st.state == 'done') break;
        if (st.state == 'failed' || st.state == 'missing') {
          await _save(null);
          await updater.removeDownload(id);
          state = _at(UpdatePhase.failed);
          return;
        }
        final t = st.total > 0 ? st.total : (info?.size ?? 0);
        state = _at(
          UpdatePhase.downloading,
          t > 0 ? st.got / t : 0,
          st.state == 'pending' || st.state == 'paused',
        );
        await Future<void>.delayed(const Duration(seconds: 1));
      }
      final file = File(path);
      if (info != null &&
          info.sha256.isNotEmpty &&
          (await sha256.bind(file.openRead()).first).toString() !=
              info.sha256) {
        await _save(null);
        await updater.removeDownload(id);
        state = _at(UpdatePhase.failed);
        return;
      }
      state = _at(UpdatePhase.installing, 1);
      final res = await updater.install(path);
      state = _at(
        res == 'needs_permission'
            ? UpdatePhase.needsPermission
            : UpdatePhase.available,
      );
    } catch (_) {
      state = _at(UpdatePhase.failed);
    } finally {
      if (g == _gen) _watching = false;
    }
  }
}

/// Versi terpasang `versionName+versionCode` (mis. 1.0.0+1); null di luar Android (tes).
final appVersionProvider = FutureProvider<String?>(
  (ref) async => (await ref.read(updaterPlatformProvider).info())?.version,
);
