import 'dart:convert';
import 'dart:math';

import 'package:crypto/crypto.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_web_auth_2/flutter_web_auth_2.dart';

import '../config/app_config.dart';

/// Hasil otorisasi Eraya: `code` + `verifier` PKCE, ditukar server lewat `POST /auth/eraya/exchange`.
class ErayaGrant {
  const ErayaGrant(this.code, this.verifier);

  final String code;
  final String verifier;
}

class ErayaAuthFailure implements Exception {
  ErayaAuthFailure(this.cause);

  final Object cause;

  @override
  String toString() => 'ErayaAuthFailure($cause)';
}

/// Pembungkus SSO Eraya (Custom Tab + PKCE) agar layar login bisa diuji tanpa browser (override [erayaAuthProvider]).
abstract class ErayaAuth {
  bool get enabled;

  /// Membuka halaman masuk Eraya. Null bila pengguna menutupnya; [ErayaAuthFailure] untuk kegagalan lain.
  Future<ErayaGrant?> authorize();
}

class PlatformErayaAuth implements ErayaAuth {
  @override
  bool get enabled =>
      AppConfig.erayaIssuer.isNotEmpty && AppConfig.erayaClientId.isNotEmpty;

  static String _b64url(List<int> b) => base64Url.encode(b).replaceAll('=', '');

  static String _random(int n) {
    final r = Random.secure();
    return _b64url(List<int>.generate(n, (_) => r.nextInt(256)));
  }

  @override
  Future<ErayaGrant?> authorize() async {
    final verifier = _random(48); // 64 karakter (syarat PKCE: 43–128)
    final state = _random(16);
    final challenge = _b64url(sha256.convert(utf8.encode(verifier)).bytes);
    final issuer = AppConfig.erayaIssuer.replaceAll(RegExp(r'/+$'), '');
    final url = Uri.parse('$issuer/oauth/authorize').replace(
      queryParameters: {
        'response_type': 'code',
        'client_id': AppConfig.erayaClientId,
        'redirect_uri': AppConfig.erayaRedirectUri,
        'scope': 'openid profile email',
        'state': state,
        'code_challenge': challenge,
        'code_challenge_method': 'S256',
      },
    );
    try {
      final result = await FlutterWebAuth2.authenticate(
        url: url.toString(),
        callbackUrlScheme: AppConfig.erayaCallbackScheme,
      );
      final q = Uri.parse(result).queryParameters;
      final code = q['code'];
      if (q['state'] != state || code == null || code.isEmpty) {
        if (q['error'] == 'access_denied') return null;
        throw ErayaAuthFailure('callback tidak valid');
      }
      return ErayaGrant(code, verifier);
    } on PlatformException catch (e) {
      if (e.code == 'CANCELED') return null;
      throw ErayaAuthFailure(e);
    }
  }
}

final erayaAuthProvider = Provider<ErayaAuth>((_) => PlatformErayaAuth());
