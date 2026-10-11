import 'dart:async';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:dio/dio.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

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
  int? build(String v) => v.contains('+') ? int.tryParse(v.split('+').last) : null;

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
  });

  final UpdatePhase phase;
  final AppUpdateInfo? info;
  final double progress;
  final String current;

  /// Ada versi lebih baru: layar pembaruan wajib tampil sampai pembaruan terpasang.
  bool get required => phase != UpdatePhase.none;
}

final appUpdateProvider = NotifierProvider<AppUpdateController, UpdateState>(
  AppUpdateController.new,
);

class AppUpdateController extends Notifier<UpdateState> {
  String _cacheDir = '';

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

  /// Cek versi terbaru. Galat jaringan diabaikan (aplikasi tetap bisa dipakai tanpa pembaruan).
  Future<void> check() async {
    if (state.phase == UpdatePhase.downloading ||
        state.phase == UpdatePhase.installing) {
      return;
    }
    final local = await ref.read(updaterPlatformProvider).info();
    if (local == null) return;
    _cacheDir = local.cacheDir;
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
    } on DioException {
      // Diam: dicek lagi saat aplikasi dibuka/dilanjutkan berikutnya.
    }
  }

  /// Unduh APK, cocokkan SHA-256 dengan metadata server, lalu buka pemasang sistem.
  Future<void> start() async {
    final info = state.info;
    if (info == null || state.phase == UpdatePhase.downloading) return;
    final current = state.current;
    UpdateState at(UpdatePhase p, [double progress = 0]) =>
        UpdateState(phase: p, info: info, current: current, progress: progress);
    state = at(UpdatePhase.downloading);
    final file = File('$_cacheDir/arus-update.apk');
    try {
      await _dio().download(
        '/public/mobile/apk',
        file.path,
        options: Options(receiveTimeout: const Duration(minutes: 5)),
        onReceiveProgress: (got, total) {
          final t = total > 0 ? total : info.size;
          if (t > 0 && state.phase == UpdatePhase.downloading) {
            state = at(UpdatePhase.downloading, got / t);
          }
        },
      );
      if (info.sha256.isNotEmpty &&
          (await sha256.bind(file.openRead()).first).toString() !=
              info.sha256) {
        await file.delete();
        throw const FormatException('sha256');
      }
      state = at(UpdatePhase.installing, 1);
      final res = await ref.read(updaterPlatformProvider).install(file.path);
      state = at(
        res == 'needs_permission'
            ? UpdatePhase.needsPermission
            : UpdatePhase.available,
      );
    } catch (_) {
      state = at(UpdatePhase.failed);
    }
  }
}
