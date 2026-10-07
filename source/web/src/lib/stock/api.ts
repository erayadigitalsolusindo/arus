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
