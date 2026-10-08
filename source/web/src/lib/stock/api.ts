// Klien API Saldo Awal Stok. Qty dikirim dan diterima sebagai string desimal (satuan dasar item).
import { api } from '#lib/api/client.ts';
import type { Page } from '#lib/catalog/api.ts';

export type Bucket = 'display' | 'warehouse' | 'returns';
export const BUCKETS: Bucket[] = ['display', 'warehouse', 'returns'];

export type OpeningRow = { id: string; sku: string; name: string; unit: string; display: string; warehouse: string; returns: string };
export type OpeningStatus = { locked: boolean; start_date: string | null; locked_at: string | null };

export const opening = {
  status: () => api<OpeningStatus>('/stock/opening/status'),
  list: (p: { q?: string; limit?: number; offset?: number } = {}) => {
    const s = new URLSearchParams();
    if (p.q) s.set('q', p.q);
    if (p.limit) s.set('limit', String(p.limit));
    if (p.offset) s.set('offset', String(p.offset));
    const qs = s.toString();
    return api<Page<OpeningRow>>(`/stock/opening/items${qs ? `?${qs}` : ''}`);
  },
  /** Menetapkan saldo awal (saldo target, bukan selisih) satu barang pada satu bucket outlet aktif. */
  set: (itemId: string, bucket: Bucket, qty: string) =>
    api<{ bucket: Bucket; qty: string }>(`/stock/opening/items/${itemId}`, { method: 'PUT', body: JSON.stringify({ bucket, qty }) }),
  lock: (startDate: string) => api<OpeningStatus>('/stock/opening/lock', { method: 'POST', body: JSON.stringify({ start_date: startDate }) })
};

// ---- Pecah satuan ----
export type ConvItem = { id: string; sku: string; name: string; unit: string; qty: string };
export type Conversion = {
  id: string;
  doc_no: string;
  from: ConvItem;
  to: ConvItem;
  from_unit_cost: string;
  to_unit_cost: string;
  cost_applied: boolean;
  note: string;
  actor: string;
  created_at: string;
};
export type ConvertInput = { from_item_id: string; from_qty: string; to_item_id: string; to_qty: string; note?: string };

export const conversions = {
  /** Barang bertipe goods + stok display outlet aktif (pemilih barang; izin pecah satuan, bukan izin Daftar Item). */
  items: (q: string, limit = 20) => api<Page<OpeningRow>>(`/stock/conversions/items?${new URLSearchParams({ q, limit: String(limit) })}`),
  list: (limit = 20, offset = 0) => api<Page<Conversion>>(`/stock/conversions/?limit=${limit}&offset=${offset}`),
  create: (input: ConvertInput, idempotencyKey: string) =>
    api<Conversion>('/stock/conversions/', { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) })
};

// ---- Kartu stok ----
export type CardItem = { id: string; sku: string; name: string; unit: string };
export type CardRow = {
  id: number;
  at: string;
  bucket: Bucket;
  ref_type: string;
  ref_id?: string;
  note: string;
  actor: string;
  qty_delta: string;
  balance: string;
};
export type CardResult = {
  item: CardItem;
  from: string;
  to: string;
  bucket: Bucket | '';
  opening: string;
  in: string;
  out: string;
  closing: string;
  rows: CardRow[];
  next_cursor: number | null;
};

export const card = {
  /** Pemilih barang berstok (izin Kartu Stok; termasuk barang yang diarsipkan). */
  items: (q: string) => api<{ data: CardItem[] }>(`/stock/card/items?${new URLSearchParams({ q })}`),
  get: (p: { itemId: string; from?: string; to?: string; bucket?: Bucket | ''; cursor?: number | null }) => {
    const s = new URLSearchParams({ item_id: p.itemId });
    if (p.from) s.set('from', p.from);
    if (p.to) s.set('to', p.to);
    if (p.bucket) s.set('bucket', p.bucket);
    if (p.cursor) s.set('cursor', String(p.cursor));
    return api<CardResult>(`/stock/card/?${s}`);
  }
};
