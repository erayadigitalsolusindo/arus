import 'dart:typed_data';

import 'receipt_layout.dart';

/// Penyusun perintah ESC/POS dari baris struk. Port dari `web/src/lib/pos/escpos.ts` — satu template (receipt_layout.dart),
/// di sini hanya diterjemahkan ke bahasa printer thermal: huruf bawaan printer (Font A 12×24 → 32 kolom di kertas 58 mm,
/// 48 di 80 mm), tebal, tinggi ganda, rata tengah, lalu dorong kertas + potong.
const _esc = 0x1b;
const _gs = 0x1d;
const _lf = 0x0a;

const _punct = <String, String>{
  '‘': "'", '’': "'", '‚': "'", '“': '"', '”': '"', '„': '"',
  '–': '-', '—': '-', '−': '-', '•': '*', '·': '*', '…': '...',
  ' ': ' ', ' ': ' ', ' ': ' ', '×': 'x', '°': 'o', '€': 'EUR',
};

// Huruf beraksen → huruf dasar (Dart tidak punya normalisasi Unicode bawaan).
const _accents = <String, String>{
  'a': 'àáâãäåāăą', 'c': 'çćĉċč', 'd': 'ďđ', 'e': 'èéêëēĕėęě', 'g': 'ĝğġģ',
  'i': 'ìíîïĩīĭįı', 'l': 'ĺļľŀł', 'n': 'ñńņňŉ', 'o': 'òóôõöøōŏő',
  's': 'śŝşš', 't': 'ţťŧ', 'u': 'ùúûüũūŭůűų', 'y': 'ýÿŷ', 'z': 'źżž',
  'A': 'ÀÁÂÃÄÅĀĂĄ', 'C': 'ÇĆĈĊČ', 'D': 'ĎĐ', 'E': 'ÈÉÊËĒĔĖĘĚ', 'G': 'ĜĞĠĢ',
  'I': 'ÌÍÎÏĨĪĬĮİ', 'L': 'ĹĻĽĿŁ', 'N': 'ÑŃŅŇ', 'O': 'ÒÓÔÕÖØŌŎŐ',
  'S': 'ŚŜŞŠ', 'T': 'ŢŤŦ', 'U': 'ÙÚÛÜŨŪŬŮŰŲ', 'Y': 'ÝŸŶ', 'Z': 'ŹŻŽ',
};

final Map<String, String> _fold = {
  for (final e in _accents.entries)
    for (final ch in e.value.split('')) ch: e.key,
};

/// Teks → ASCII cetak. Halaman kode printer murah tidak seragam, jadi hanya ASCII yang dipakai: huruf beraksen dilepas
/// tandanya (é → e), tanda baca tipografis diganti padanannya, sisanya menjadi '?'.
String toPrinterAscii(String s) {
  final out = StringBuffer();
  for (final rune in s.runes) {
    final ch = String.fromCharCode(rune);
    if (rune >= 0x20 && rune <= 0x7e) {
      out.write(ch);
    } else {
      out.write(_fold[ch] ?? _punct[ch] ?? '?');
    }
  }
  return out.toString();
}

/// Satu struk lengkap: inisialisasi, baris demi baris, dorong kertas, potong (bila diminta).
Uint8List escposReceipt(
  List<ReceiptLine> lines, {
  required int cols,
  required bool cut,
}) {
  final b = BytesBuilder();
  void bytes(List<int> v) => b.add(v);
  void text(String s) => b.add(toPrinterAscii(s).codeUnits);

  bytes([_esc, 0x40]); // ESC @ : reset printer
  bytes([_esc, 0x74, 0]); // ESC t 0 : halaman kode PC437 (ASCII aman)
  bytes([_esc, 0x32]); // ESC 2 : jarak baris bawaan
  for (final l in lines) {
    if (l.title) {
      // Judul (nama toko): rata tengah oleh printer; ukuran ganda bila muat (lebar 2× → setengah kolom).
      final s = l.text.trim();
      final fits = s.length <= cols ~/ 2;
      bytes([_esc, 0x61, 1, _esc, 0x45, 1]);
      if (fits) bytes([_gs, 0x21, 0x11]);
      text(s);
      bytes([_lf, _gs, 0x21, 0, _esc, 0x45, 0, _esc, 0x61, 0]);
      continue;
    }
    if (l.bold) bytes([_esc, 0x45, 1]);
    if (l.big) bytes([_gs, 0x21, 0x01]); // tinggi ganda, lebar tetap → kolom tidak bergeser
    text(l.text.replaceFirst(RegExp(r'\s+$'), ''));
    bytes([_lf]);
    if (l.big) bytes([_gs, 0x21, 0]);
    if (l.bold) bytes([_esc, 0x45, 0]);
  }
  bytes([_esc, 0x64, 4]); // ESC d 4 : dorong 4 baris agar tulisan terakhir melewati pisau/tepi sobek
  if (cut) bytes([_gs, 0x56, 0x42, 0]); // GS V 66 0 : potong sebagian (tanpa pisau diabaikan)
  return b.toBytes();
}
