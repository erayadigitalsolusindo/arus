import 'package:dio/dio.dart';

import '../../l10n/gen/app_localizations.dart';

/// Galat dari API ({"error":{"code","message","fields","retry_after","attempts_left"}}) atau jaringan.
/// Teks untuk pengguna selalu dari kamus l10n lewat [ApiError.message]; `serverMessage` hanya untuk log.
class ApiError implements Exception {
  ApiError({
    required this.code,
    this.status = 0,
    this.serverMessage = '',
    this.fields = const {},
    this.retryAfter = 0,
    this.attemptsLeft = 0,
  });

  /// Kode stabil dari server (INVALID_CREDENTIALS, SHIFT_REQUIRED, ...) atau NETWORK / TIMEOUT / UNKNOWN (klien).
  final String code;
  final int status;
  final String serverMessage;
  final Map<String, String> fields;
  final int retryAfter;
  final int attemptsLeft;

  bool get isNetwork => code == 'NETWORK' || code == 'TIMEOUT';

  factory ApiError.fromDio(DioException e) {
    switch (e.type) {
      case DioExceptionType.connectionTimeout:
      case DioExceptionType.sendTimeout:
      case DioExceptionType.receiveTimeout:
        return ApiError(code: 'TIMEOUT');
      case DioExceptionType.connectionError:
        return ApiError(code: 'NETWORK');
      default:
        break;
    }
    final res = e.response;
    final data = res?.data;
    if (data is Map && data['error'] is Map) {
      final err = data['error'] as Map;
      final fields = <String, String>{};
      if (err['fields'] is Map) {
        (err['fields'] as Map).forEach((k, v) => fields['$k'] = '$v');
      }
      return ApiError(
        code: '${err['code'] ?? 'UNKNOWN'}',
        status: res?.statusCode ?? 0,
        serverMessage: '${err['message'] ?? ''}',
        fields: fields,
        retryAfter: (err['retry_after'] as num?)?.toInt() ?? 0,
        attemptsLeft: (err['attempts_left'] as num?)?.toInt() ?? 0,
      );
    }
    return ApiError(
      code: res == null ? 'NETWORK' : 'UNKNOWN',
      status: res?.statusCode ?? 0,
    );
  }

  /// Pesan untuk pengguna dalam bahasa aktif. Kode yang belum punya terjemahan jatuh ke pesan umum.
  String message(AppLocalizations l) {
    switch (code) {
      case 'NETWORK':
        return l.errorNetwork;
      case 'TIMEOUT':
        return l.errorTimeout;
      case 'INVALID_CREDENTIALS':
        return attemptsLeft > 0
            ? l.errorInvalidCredentialsLeft(attemptsLeft)
            : l.errorInvalidCredentials;
      case 'ACCOUNT_DISABLED':
        return l.errorAccountDisabled;
      case 'NO_OUTLET':
        return l.errorNoOutlet;
      case 'ACCOUNT_LOCKED':
        return l.errorAccountLocked((retryAfter / 60).ceil().clamp(1, 1 << 30));
      case 'RATE_LIMITED':
        return l.errorRateLimited;
      case 'UNAVAILABLE':
        return l.errorUnavailable;
      case 'SESSION_INVALID':
        return l.errorSessionInvalid;
      case 'SHIFT_REQUIRED':
        return l.errorShiftRequired;
      case 'STOCK_INSUFFICIENT':
        return l.errorStockInsufficient;
      case 'CREDIT_LIMIT_EXCEEDED':
        return l.errorCreditLimit;
      case 'VALIDATION':
        return l.errorValidation;
      case 'FORBIDDEN':
      case 'OUTLET_FORBIDDEN':
        return l.errorForbidden;
      case 'IDEMPOTENCY_MISMATCH':
        return l.errorIdempotencyMismatch;
      case 'METHOD_INACTIVE':
        return l.errorMethodInactive;
      case 'PIN_REQUIRED':
        return l.errorPinRequired;
      case 'INVALID_PIN':
        return l.errorInvalidPin;
      case 'PIN_LOCKED':
        return l.errorPinLocked;
      case 'SHIFT_RECAP_CHANGED':
        return l.errorShiftRecapChanged;
      case 'SHIFT_DIFF_NOTE_REQUIRED':
        return l.errorShiftDiffNote;
      case 'SHIFT_CLOSED':
        return l.errorShiftClosed;
      case 'SHIFT_ALREADY_OPEN':
        return l.errorShiftAlreadyOpen;
      case 'INTERNAL':
        return l.errorInternal;
      default:
        return l.errorUnknown;
    }
  }

  @override
  String toString() => 'ApiError($code, status=$status)';
}
