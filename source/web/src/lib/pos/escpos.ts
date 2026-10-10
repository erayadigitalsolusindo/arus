// Penyusun perintah ESC/POS dari baris struk (receiptLines). Template struk tetap satu (receipt.ts); di sini hanya
// diterjemahkan ke bahasa printer thermal: huruf bawaan printer (Font A 12×24 → 32 kolom di kertas 58 mm, 48 di
// 80 mm), tebal, tinggi ganda, rata tengah, lalu dorong kertas + potong. Dikirim mentah lewat print-agent; kelak
// dipakai juga untuk printer Bluetooth di kasir Android.
import type { ReceiptLine } from '#lib/pos/receipt.ts';

const ESC = 0x1b;
const GS = 0x1d;
const LF = 0x0a;

export type EscposOptions = { cols: number; cut: boolean };

const PUNCT: Record<string, string> = {
  '‘': "'", '’': "'", '‚': "'", '“': '"', '”': '"', '„': '"',
  '–': '-', '—': '-', '−': '-', '•': '*', '·': '*', '…': '...',
  ' ': ' ', ' ': ' ', ' ': ' ', '×': 'x', '°': 'o', '€': 'EUR'
};

/**
 * Teks → byte ASCII cetak. Halaman kode printer murah tidak seragam, jadi hanya ASCII yang dipakai: huruf beraksen
 * dilepas tandanya (é → e), tanda baca tipografis diganti padanannya, sisanya menjadi '?'.
 */
export function toPrinterAscii(s: string): string {
  let out = '';
  for (const ch of s.normalize('NFKD')) {
    const code = ch.codePointAt(0)!;
    if (code >= 0x20 && code <= 0x7e) out += ch;
    else if (code >= 0x300 && code <= 0x36f) continue; // tanda aksen hasil NFKD
    else out += PUNCT[ch] ?? '?';
  }
  return out;
}

class Buf {
  private parts: number[] = [];
  bytes(...b: number[]) {
    this.parts.push(...b);
    return this;
  }
  text(s: string) {
    for (const ch of toPrinterAscii(s)) this.parts.push(ch.charCodeAt(0));
    return this;
  }
  done() {
    return Uint8Array.from(this.parts);
  }
}

/** Susun satu struk lengkap: inisialisasi, baris demi baris, dorong kertas, potong (bila diminta). */
export function escposReceipt(lines: ReceiptLine[], opts: EscposOptions): Uint8Array {
  const b = new Buf();
  b.bytes(ESC, 0x40); // ESC @ : reset printer ke keadaan bawaan
  b.bytes(ESC, 0x74, 0); // ESC t 0 : halaman kode PC437 (ASCII aman)
  b.bytes(ESC, 0x32); // ESC 2 : jarak baris bawaan
  for (const l of lines) {
    if (l.title) {
      // Judul (nama toko): rata tengah oleh printer; ukuran ganda bila muat (lebar 2× → setengah kolom).
      const s = l.text.trim();
      const fits = s.length <= Math.floor(opts.cols / 2);
      b.bytes(ESC, 0x61, 1).bytes(ESC, 0x45, 1);
      if (fits) b.bytes(GS, 0x21, 0x11);
      b.text(s).bytes(LF);
      b.bytes(GS, 0x21, 0).bytes(ESC, 0x45, 0).bytes(ESC, 0x61, 0);
      continue;
    }
    if (l.bold) b.bytes(ESC, 0x45, 1);
    if (l.big) b.bytes(GS, 0x21, 0x01); // tinggi ganda, lebar tetap → kolom tidak bergeser
    b.text(l.text.replace(/\s+$/, '')).bytes(LF);
    if (l.big) b.bytes(GS, 0x21, 0);
    if (l.bold) b.bytes(ESC, 0x45, 0);
  }
  b.bytes(ESC, 0x64, 4); // ESC d 4 : dorong 4 baris agar tulisan terakhir melewati pisau/tepi sobek
  if (opts.cut) b.bytes(GS, 0x56, 0x42, 0); // GS V 66 0 : potong sebagian (printer tanpa pisau mengabaikannya)
  return b.done();
}

export function toBase64(bytes: Uint8Array): string {
  let s = '';
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s);
}
