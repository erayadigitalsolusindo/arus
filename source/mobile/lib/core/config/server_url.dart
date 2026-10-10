import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'app_config.dart';

const _prefsKey = 'arus.server_url';

/// Alamat API yang dipakai aplikasi. Urutan: pilihan pengguna (tersimpan di perangkat) → `--dart-define=API_URL` →
/// bawaan emulator. Nilai awal dimuat di `main()` sebelum `runApp` agar permintaan pertama sudah memakai alamat benar.
final serverUrlProvider = NotifierProvider<ServerUrlController, String>(
  ServerUrlController.new,
);

/// Nilai awal dari penyimpanan; di-override di `main()`.
final initialServerUrlProvider = Provider<String?>((_) => null);

Future<String?> loadSavedServerUrl() async {
  try {
    return (await SharedPreferences.getInstance()).getString(_prefsKey);
  } catch (_) {
    return null;
  }
}

/// Mengembalikan alamat yang sudah dirapikan (tanpa spasi dan garis miring akhir) atau null bila bukan http(s) yang sah.
String? normalizeServerUrl(String raw) {
  var s = raw.trim();
  while (s.endsWith('/')) {
    s = s.substring(0, s.length - 1);
  }
  final u = Uri.tryParse(s);
  if (u == null ||
      !(u.scheme == 'http' || u.scheme == 'https') ||
      u.host.isEmpty) {
    return null;
  }
  return s;
}

class ServerUrlController extends Notifier<String> {
  @override
  String build() => ref.read(initialServerUrlProvider) ?? AppConfig.apiUrl;

  /// Simpan alamat baru. Kosong = kembali ke bawaan. Mengembalikan false bila alamat tidak sah.
  Future<bool> set(String raw) async {
    if (raw.trim().isEmpty) {
      state = AppConfig.apiUrl;
      try {
        await (await SharedPreferences.getInstance()).remove(_prefsKey);
      } catch (_) {}
      return true;
    }
    final url = normalizeServerUrl(raw);
    if (url == null) return false;
    state = url;
    try {
      await (await SharedPreferences.getInstance()).setString(_prefsKey, url);
    } catch (_) {}
    return true;
  }
}
