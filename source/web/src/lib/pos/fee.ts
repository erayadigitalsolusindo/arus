import { toCents } from '#lib/pos/money.ts';

/** Tarif biaya (MDR) satu metode bayar; field string dari API ("0.00" = kosong). */
export type FeeRule = { fee_pct?: string; fee_flat?: string; small_tx_limit?: string; small_tx_flat?: string };

/** Aturan transaksi kecil (BI): jumlah <= batas -> MDR flat menggantikan persen + nominal. Batas 0 = nonaktif. */
const smallApplies = (m: FeeRule, amount: bigint) => {
  const limit = toCents(m.small_tx_limit ?? '');
  return limit > 0n && amount <= limit;
};

/** Pratinjau biaya (rumus sama dengan server: jumlah × persen + tetap, dibulatkan sen). Server yang menentukan. */
export const feeCents = (m: FeeRule, amount: bigint): bigint => {
  if (smallApplies(m, amount)) return toCents(m.small_tx_flat ?? '');
  const pctH = BigInt(Math.round(Number(m.fee_pct ?? 0) * 100));
  const flat = toCents(m.fee_flat ?? '');
  return pctH === 0n && flat === 0n ? 0n : (amount * pctH + 5000n) / 10000n + flat;
};

/** "0,7%" / "Rp1.000" / "0,7% + Rp1.000" untuk tarif yang berlaku pada jumlah ini (transaksi kecil: MDR flat saja). Kosong bila tanpa biaya. */
export const feeRate = (m: FeeRule, amount: bigint, money: (c: bigint) => string): string => {
  if (smallApplies(m, amount)) return money(toCents(m.small_tx_flat ?? ''));
  const pct = Number(m.fee_pct ?? 0);
  const flat = toCents(m.fee_flat ?? '');
  return [pct > 0 ? `${pct}%` : '', flat > 0n ? money(flat) : ''].filter(Boolean).join(' + ');
};

/** Label ringkas tak bergantung jumlah (chip metode): tarif biasa, ditambah "≤ batas: MDR flat" bila aturan transaksi kecil aktif. */
export const feeLabel = (m: FeeRule, money: (c: bigint) => string): string => {
  const base = feeRate({ ...m, small_tx_limit: '' }, 0n, money);
  const limit = toCents(m.small_tx_limit ?? '');
  if (limit <= 0n) return base;
  const small = `≤${money(limit)}: ${money(toCents(m.small_tx_flat ?? ''))}`;
  return base ? `${base} · ${small}` : small;
};
