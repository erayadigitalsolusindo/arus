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
