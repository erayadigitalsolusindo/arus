import 'package:decimal/decimal.dart';

/// Uang dan qty memakai [Decimal] (tidak pernah double). API mengirim/menerima string desimal.

Decimal dec(Object? v) {
  if (v == null) return Decimal.zero;
  final s = '$v'.trim();
  if (s.isEmpty) return Decimal.zero;
  return Decimal.tryParse(s) ?? Decimal.zero;
}

/// String untuk dikirim ke API: tanpa notasi ilmiah, maks 3 desimal untuk qty / 2 untuk uang diatur pemanggil.
String decToApi(Decimal d, {int scale = 3}) {
  final s = d.toStringAsFixed(scale);
  if (!s.contains('.')) return s;
  var out = s.replaceFirst(RegExp(r'0+$'), '');
  if (out.endsWith('.')) out = out.substring(0, out.length - 1);
  return out;
}

/// "Rp 1.234.500" atau "Rp 1.234.500,50" (desimal hanya bila ada sen). Pemisah ribuan titik, desimal koma.
String formatMoney(Decimal d, {bool symbol = true}) {
  final neg = d < Decimal.zero;
  final abs = neg ? -d : d;
  final fixed = abs.toStringAsFixed(2);
  final parts = fixed.split('.');
  final grouped = _group(parts[0]);
  final frac = parts.length > 1 && parts[1] != '00' ? ',${parts[1]}' : '';
  return '${neg ? '-' : ''}${symbol ? 'Rp ' : ''}$grouped$frac';
}

/// Qty tanpa nol di belakang: 2.000 → "2", 1.5 → "1,5".
String formatQty(Decimal d) {
  final s = decToApi(d);
  return s.replaceFirst('.', ',');
}

String _group(String digits) {
  final b = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) b.write('.');
    b.write(digits[i]);
  }
  return b.toString();
}

/// Membaca isian pengguna ("15.000", "15000,50", "1 500") menjadi Decimal; kosong/rusak → 0.
Decimal parseInput(String raw) {
  var s = raw.trim().replaceAll(RegExp(r'[Rp\s]'), '');
  if (s.isEmpty) return Decimal.zero;
  // Format Indonesia: titik = ribuan, koma = desimal.
  if (s.contains(',')) {
    s = s.replaceAll('.', '').replaceAll(',', '.');
  } else if (RegExp(r'^\d{1,3}(\.\d{3})+$').hasMatch(s)) {
    s = s.replaceAll('.', '');
  }
  return Decimal.tryParse(s) ?? Decimal.zero;
}
