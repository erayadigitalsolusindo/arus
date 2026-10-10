import 'dart:async';

import 'package:dio/dio.dart';

import '../config/app_config.dart';
import '../session/session_models.dart';
import '../session/token_store.dart';
import 'api_error.dart';

/// Klien HTTP ke API Go. Aturan (sama dengan web, AGENTS.md §3/§6):
/// - tenant/outlet/user hanya dari token; klien tidak pernah mengirimnya sebagai sumber kebenaran;
/// - access token hanya di memori, refresh token di [TokenStore] (Keystore) dan dirotasi server tiap refresh;
/// - 401 `TOKEN_EXPIRED` → refresh satu kali (single-flight), lalu ulang permintaan;
/// - semua galat dilempar sebagai [ApiError].
class ApiClient {
  ApiClient({
    required this.tokens,
    required this.onSessionExpired,
    String? baseUrl,
    Dio? raw,
    Dio? authed,
  }) : _raw = raw ?? _newDio(baseUrl ?? AppConfig.apiUrl),
       dio = authed ?? _newDio(baseUrl ?? AppConfig.apiUrl) {
    dio.interceptors.add(
      InterceptorsWrapper(onRequest: _attach, onError: _retryOnExpired),
    );
  }

  final TokenStore tokens;

  /// Dipanggil saat refresh token ditolak server (sesi berakhir / dicabut) — UI kembali ke login.
  final void Function() onSessionExpired;

  /// Dio untuk endpoint ber-auth (Bearer + refresh otomatis).
  final Dio dio;

  /// Dio polos (tanpa interceptor auth) untuk login / refresh agar tidak rekursif.
  final Dio _raw;

  String? _accessToken;
  Future<AuthSession>? _refreshing;

  /// Ganti alamat server (pengaturan pengguna). Sesi lama tidak berlaku di server lain, jadi pemanggil melakukan logout lokal.
  void setBaseUrl(String url) {
    _raw.options.baseUrl = url;
    dio.options.baseUrl = url;
  }

  static Dio _newDio(String baseUrl) => Dio(
    BaseOptions(
      baseUrl: baseUrl,
      connectTimeout: const Duration(seconds: 10),
      receiveTimeout: const Duration(seconds: 20),
      sendTimeout: const Duration(seconds: 20),
      headers: {
        AppConfig.clientHeader: AppConfig.clientValue,
        'Accept': 'application/json',
      },
    ),
  );

  /// Bahasa untuk email/pesan server (Accept-Language).
  void setLanguage(String code) {
    _raw.options.headers['Accept-Language'] = code;
    dio.options.headers['Accept-Language'] = code;
  }

  // ---- auth ----

  Future<AuthSession> login({
    required String email,
    required String password,
    required bool remember,
  }) async {
    final s = await _call(
      () => _raw.post<Map<String, dynamic>>(
        '/auth/login',
        data: {'email': email, 'password': password, 'remember': remember},
      ),
    );
    await _adopt(s);
    return s;
  }

  /// Memulihkan sesi dari refresh token tersimpan. Null bila tidak ada / ditolak server; galat jaringan dilempar.
  Future<AuthSession?> restore() async {
    final token = await tokens.readRefreshToken();
    if (token == null || token.isEmpty) return null;
    try {
      return await refresh();
    } on ApiError catch (e) {
      if (e.status == 401 || e.status == 403) {
        await tokens.clear();
        return null;
      }
      rethrow;
    }
  }

  /// Satu refresh pada satu waktu: panggilan bersamaan menunggu hasil yang sama (token dirotasi, tidak boleh dipakai dua kali).
  Future<AuthSession> refresh() =>
      _refreshing ??= _doRefresh().whenComplete(() => _refreshing = null);

  Future<AuthSession> _doRefresh() async {
    final token = await tokens.readRefreshToken();
    if (token == null || token.isEmpty) {
      throw ApiError(code: 'SESSION_INVALID', status: 401);
    }
    final s = await _call(
      () => _raw.post<Map<String, dynamic>>(
        '/auth/refresh',
        data: {'refresh_token': token},
      ),
    );
    await _adopt(s);
    return s;
  }

  /// Pindah outlet: token akses baru (refresh token tidak dirotasi server). `pos` = dari layar kasir → server meminta
  /// persetujuan PIN ([approvalUserId] + [pin]) kecuali pelaku sendiri penyetuju.
  Future<AuthSession> switchOutlet(
    String outletId, {
    bool pos = false,
    String? approvalUserId,
    String? pin,
  }) async {
    final token = await tokens.readRefreshToken();
    if (token == null || token.isEmpty) {
      throw ApiError(code: 'SESSION_INVALID', status: 401);
    }
    try {
      final r = await dio.post<Map<String, dynamic>>(
        '/auth/switch-outlet',
        data: {
          'outlet_id': outletId,
          'refresh_token': token,
          if (pos) 'pos': true,
          if (pos && approvalUserId != null)
            'approval': {'user_id': approvalUserId, 'pin': pin},
        },
      );
      final s = AuthSession.fromJson(r.data!);
      await _adopt(s);
      return s;
    } on DioException catch (e) {
      throw ApiError.fromDio(e);
    }
  }

  Future<void> logout() async {
    final token = await tokens.readRefreshToken();
    _accessToken = null;
    await tokens.clear();
    if (token == null || token.isEmpty) return;
    try {
      await _raw.post<void>('/auth/logout', data: {'refresh_token': token});
    } on DioException {
      // Keluar tetap berhasil di perangkat; sesi di server akan kedaluwarsa sendiri.
    }
  }

  /// Hapus sesi lokal tanpa memanggil server (dipakai saat server sudah menolak).
  Future<void> clearLocal() async {
    _accessToken = null;
    await tokens.clear();
  }

  Future<void> _adopt(AuthSession s) async {
    _accessToken = s.accessToken;
    if (s.refreshToken.isNotEmpty) {
      await tokens.writeRefreshToken(s.refreshToken);
    }
  }

  // ---- interceptor ----

  void _attach(RequestOptions o, RequestInterceptorHandler h) {
    final t = _accessToken;
    if (t != null) o.headers['Authorization'] = 'Bearer $t';
    h.next(o);
  }

  Future<void> _retryOnExpired(
    DioException e,
    ErrorInterceptorHandler h,
  ) async {
    final err = ApiError.fromDio(e);
    final retried = e.requestOptions.extra['retried'] == true;
    if (e.response?.statusCode != 401 ||
        err.code != 'TOKEN_EXPIRED' ||
        retried) {
      return h.next(e);
    }
    try {
      await refresh();
    } on ApiError catch (re) {
      if (re.status == 401 || re.status == 403) {
        await clearLocal();
        onSessionExpired();
      }
      return h.next(e);
    }
    try {
      final o = e.requestOptions..extra['retried'] = true;
      o.headers['Authorization'] = 'Bearer $_accessToken';
      h.resolve(await dio.fetch<dynamic>(o));
    } on DioException catch (e2) {
      h.next(e2);
    }
  }

  // ---- util ----

  Future<AuthSession> _call(
    Future<Response<Map<String, dynamic>>> Function() fn,
  ) async {
    try {
      final r = await fn();
      return AuthSession.fromJson(r.data!);
    } on DioException catch (e) {
      throw ApiError.fromDio(e);
    }
  }
}
