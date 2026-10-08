// Hitungan uang tampilan kasir tanpa float: rupiah disimpan sebagai sen (BigInt), qty sebagai per-mil (3 desimal).
// Hanya PRATINJAU di layar; total resmi dihitung server saat nota disimpan (AGENTS.md §3.3).

/** "12500.50" / "12.500,5" tidak diterima: hanya string desimal titik dari API atau ketikan pengguna dengan koma → titik. */
function parseScaled(s: string, scale: number): bigint {
  const clean = s.trim().replace(',', '.');
  if (!/^\d*\.?\d*$/.test(clean) || clean === '' || clean === '.') return 0n;
  const [i, f = ''] = clean.split('.');
  const frac = (f + '0'.repeat(scale)).slice(0, scale);
  return BigInt(i || '0') * 10n ** BigInt(scale) + BigInt(frac || '0');
}

export const toCents = (s: string) => parseScaled(s, 2);
export const toMilli = (s: string) => parseScaled(s, 3);

/** harga (sen) × qty (per-mil), dibulatkan ke sen terdekat. */
export function lineTotal(priceCents: bigint, qtyMilli: bigint): bigint {
  return (priceCents * qtyMilli + 500n) / 1000n;
}

/** sen → Number untuk ditampilkan (aman: nilai uang toko jauh di bawah 2^53 sen). */
export const centsToNumber = (c: bigint) => Number(c) / 100;

/** persen (string "11") dari sen. */
export function percentOf(cents: bigint, pct: string): bigint {
  const p = parseScaled(pct, 2); // persen ×100
  return (cents * p + 5000n) / 10000n;
}
