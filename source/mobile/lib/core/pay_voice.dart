import 'package:flutter_tts/flutter_tts.dart';
import 'package:shared_preferences/shared_preferences.dart';

const voiceKinds = ['cash', 'transfer', 'debit', 'credit_card', 'ewallet'];
const _key = 'pos.voiceKinds';
final _tts = FlutterTts();

/// Metode bayar yang nominalnya diucapkan (per perangkat); bawaan semua non-tunai.
Future<Set<String>> loadVoiceKinds() async {
  final p = await SharedPreferences.getInstance();
  return (p.getStringList(_key) ?? voiceKinds.where((k) => k != 'cash').toList())
      .toSet();
}

Future<void> saveVoiceKinds(Set<String> k) async =>
    (await SharedPreferences.getInstance()).setStringList(_key, k.toList());

const _s = ['', 'satu', 'dua', 'tiga', 'empat', 'lima', 'enam', 'tujuh', 'delapan', 'sembilan', 'sepuluh', 'sebelas'];

/// 125000 -> "seratus dua puluh lima ribu" (sama dengan dashboard EraPay).
String terbilang(int n) {
  if (n == 0) return 'nol';
  String w(int x) {
    if (x < 12) return _s[x];
    if (x < 20) return '${_s[x - 10]} belas';
    if (x < 100) return '${_s[x ~/ 10]} puluh ${_s[x % 10]}';
    if (x < 200) return 'seratus ${w(x - 100)}';
    if (x < 1000) return '${_s[x ~/ 100]} ratus ${w(x % 100)}';
    if (x < 2000) return 'seribu ${w(x - 1000)}';
    if (x < 1000000) return '${w(x ~/ 1000)} ribu ${w(x % 1000)}';
    if (x < 1000000000) return '${w(x ~/ 1000000)} juta ${w(x % 1000000)}';
    return '${w(x ~/ 1000000000)} miliar ${w(x % 1000000000)}';
  }
  return w(n).replaceAll(RegExp(r'\s+'), ' ').trim();
}

Future<void> speakPaid(double amount) async {
  try {
    await _tts.setLanguage('id-ID');
    await _tts.speak(
      'Transaksi dengan nominal ${terbilang(amount.round())} rupiah berhasil. Terima kasih.',
    );
  } catch (_) {
    // mesin suara tidak tersedia: abaikan
  }
}
