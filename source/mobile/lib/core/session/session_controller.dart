import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/api_client.dart';
import '../api/api_error.dart';
import 'session_models.dart';
import 'token_store.dart';

final tokenStoreProvider = Provider<TokenStore>((ref) => SecureTokenStore());

final apiClientProvider = Provider<ApiClient>((ref) {
  return ApiClient(
    tokens: ref.watch(tokenStoreProvider),
    onSessionExpired: () => ref.read(sessionProvider.notifier).expire(),
  );
});

/// Status sesi untuk router: [booting] saat memulihkan dari penyimpanan, lalu [signedOut] / [signedIn].
sealed class SessionState {
  const SessionState();
}

class SessionBooting extends SessionState {
  const SessionBooting();
}

class SessionSignedOut extends SessionState {
  const SessionSignedOut({this.expired = false});

  /// true = sesi berakhir di tengah pemakaian (bukan keluar manual) → login menampilkan pemberitahuan.
  final bool expired;
}

class SessionSignedIn extends SessionState {
  const SessionSignedIn(this.profile);

  final Profile profile;
}

final sessionProvider = NotifierProvider<SessionController, SessionState>(SessionController.new);

class SessionController extends Notifier<SessionState> {
  @override
  SessionState build() {
    Future.microtask(_restore);
    return const SessionBooting();
  }

  ApiClient get _api => ref.read(apiClientProvider);

  Future<void> _restore() async {
    try {
      final s = await _api.restore();
      state = s == null ? const SessionSignedOut() : SessionSignedIn(s.profile);
    } on ApiError {
      // Server tidak terjangkau saat dibuka: token tetap tersimpan, minta masuk ulang tanpa menghapusnya.
      state = const SessionSignedOut();
    }
  }

  Future<void> login({required String email, required String password, required bool remember}) async {
    final s = await _api.login(email: email, password: password, remember: remember);
    state = SessionSignedIn(s.profile);
  }

  Future<void> logout() async {
    await _api.logout();
    state = const SessionSignedOut();
  }

  /// Dipanggil klien API saat refresh token ditolak.
  void expire() => state = const SessionSignedOut(expired: true);
}
