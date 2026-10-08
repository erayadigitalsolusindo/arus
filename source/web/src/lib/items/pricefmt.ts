// Format tampilan riwayat harga jual (dipakai tab Riwayat Harga item dan laporan History Harga Jual).
import { t, formatCurrency } from '#lib/i18n/index.ts';
import type { PriceChange, PriceEvent } from './api.ts';

const money = (v: string) => formatCurrency(Number(v), 'IDR', { maximumFractionDigits: 2 });

/** Set tier dari server berbentuk "min:harga,min:harga" → "≥3 · Rp11.500, ≥6 · Rp11.000". */
const tiers = (v: string) =>
  v
    ? v
        .split(',')
        .map((p) => p.split(':'))
        .map(([min, price]) => `≥${min} · ${money(price)}`)
        .join(', ')
    : t('items.priceHistory.none');

/** Nilai satu sisi perubahan; null (belum ada) diganti teks `empty`. */
export const changeValue = (ev: Pick<PriceEvent, 'kind'>, v: string | null, empty: string) => (v === null ? empty : ev.kind === 'wholesale' ? tiers(v) : money(v));

/** Harga naik = merah, turun = hijau (hanya bila kedua sisi ada). */
export const diffClass = (c: PriceChange) =>
  c.before === null || c.after === null ? '' : Number(c.after) > Number(c.before) ? 'text-[var(--color-danger-600,#dc2626)]' : Number(c.after) < Number(c.before) ? 'text-[var(--color-success-600,#16a34a)]' : '';

/** Nama cabang dari id; null = harga default; id tak dikenal = "—". */
export const scopeName = (c: PriceChange, names: Map<string, string>) =>
  c.outlet_id === null ? t('items.priceHistory.default') : (names.get(c.outlet_id) ?? t('items.priceHistory.none'));

/** Harga default di atas, cabang menurut nama. */
export const orderChanges = (cs: PriceChange[], names: Map<string, string>) =>
  [...cs].sort((a, b) => (a.outlet_id === null ? -1 : b.outlet_id === null ? 1 : scopeName(a, names).localeCompare(scopeName(b, names))));
