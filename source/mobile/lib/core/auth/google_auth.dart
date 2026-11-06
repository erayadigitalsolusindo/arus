import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:google_sign_in/google_sign_in.dart';

import '../config/app_config.dart';

/// Pembungkus Google Sign-In agar layar login bisa diuji tanpa layanan Google (override [googleAuthProvider]).
abstract class GoogleAuth {
  /// false = `GOOGLE_SERVER_CLIENT_ID` belum diisi → tombol disembunyikan.
  bool get enabled;

  /// Membuka pemilih akun Google dan mengembalikan ID token (diverifikasi server), atau null bila dibatalkan.
  /// Melempar [GoogleAuthFailure] untuk kegagalan lain.
  Future<String?> idToken();
}

class GoogleAuthFailure implements Exception {
  GoogleAuthFailure(this.cause);

  final Object cause;

  @override
  String toString() => 'GoogleAuthFailure($cause)';
}

class PlatformGoogleAuth implements GoogleAuth {
  bool _ready = false;

  @override
  bool get enabled => AppConfig.googleServerClientId.isNotEmpty;

  @override
  Future<String?> idToken() async {
    try {
      final g = GoogleSignIn.instance;
      if (!_ready) {
        await g.initialize(serverClientId: AppConfig.googleServerClientId);
        _ready = true;
      }
      final account = await g.authenticate();
      final token = account.authentication.idToken;
      // Lepas akun terpilih agar login berikutnya menampilkan pemilih akun lagi.
      await g.signOut();
      if (token == null || token.isEmpty) {
        throw GoogleAuthFailure('idToken kosong');
      }
      return token;
    } on GoogleSignInException catch (e) {
      if (e.code == GoogleSignInExceptionCode.canceled ||
          e.code == GoogleSignInExceptionCode.interrupted) {
        return null;
      }
      throw GoogleAuthFailure(e);
    }
  }
}

final googleAuthProvider = Provider<GoogleAuth>((_) => PlatformGoogleAuth());
