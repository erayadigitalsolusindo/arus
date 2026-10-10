// Klien API Saldo Awal Stok. Qty dikirim dan diterima sebagai string desimal (satuan dasar item).
import { api } from '#lib/api/client.ts';
import type { Page } from '#lib/catalog/api.ts';

export type Bucket = 'display' | 'warehouse' | 'returns';
export const BUCKETS: Bucket[] = ['display', 'warehouse', 'returns'];

export type OpeningRow = { id: string; sku: string; name: string; unit: string; display: string; warehouse: string; returns: string };
export type OpeningStats = { items: number; filled: number; display: string; warehouse: string; returns: string };
export type OpeningStatus = { locked: boolean; start_date: string | null; locked_at: string | null; stats?: OpeningStats };

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

// ---- Stok opname ----
export type CountKind = 'session' | 'quick';
/** replace = stok diganti sebesar yang dimasukkan; adjust = jumlah yang dimasukkan ditambah/dikurangkan (+/−). */
export type QuickMode = 'replace' | 'adjust';
export type CountsOverview = { open: number; done: number; diff_lines: number; plus: string; minus: string };
export type QuickInput = { bucket: Bucket; mode: QuickMode; note: string; items: { item_id: string; qty: string }[] };
export type CountStatus = 'draft' | 'completed' | 'cancelled';
export type CountSummary = {
  id: string;
  doc_no: string;
  bucket: Bucket;
  status: CountStatus;
  note: string;
  created_by: string;
  created_at: string;
  completed_at: string | null;
  cancelled_at: string | null;
  completed_by?: string;
  cancelled_by?: string;
  lines: number;
  counted: number;
  differences: number;
  kind: CountKind;
  mode?: QuickMode;
  /** Nilai bersih selisih (Σ selisih × HPP) baris yang sudah dihitung. */
  diff_amount: string;
};
export type CountLine = {
  item_id: string;
  sku: string;
  name: string;
  unit: string;
  snapshot: string;
  current: string;
  counted: string | null;
  diff: string | null;
  unit_cost: string;
  value: string | null;
};
export type CountDetail = CountSummary & { items: CountLine[]; diff_value: string; diff_plus: string; diff_minus: string };
export type CountScope = { all?: boolean; item_ids?: string[]; category_id?: string; brand_id?: string };

export const counts = {
  list: (p: { status?: CountStatus | ''; kind?: CountKind | ''; q?: string; limit?: number; offset?: number } = {}) => {
    const s = new URLSearchParams();
    if (p.status) s.set('status', p.status);
    if (p.kind) s.set('kind', p.kind);
    if (p.q) s.set('q', p.q);
    if (p.limit) s.set('limit', String(p.limit));
    if (p.offset) s.set('offset', String(p.offset));
    const qs = s.toString();
    return api<Page<CountSummary> & { summary: CountsOverview }>(`/stock/counts/${qs ? `?${qs}` : ''}`);
  },
  /** Opname langsung: tanpa draf, stok langsung berubah. Butuh izin Setujui pada modul Stok Opname. */
  quick: (input: QuickInput) => api<CountDetail>('/stock/counts/quick', { method: 'POST', body: JSON.stringify(input) }),
  get: (id: string) => api<CountDetail>(`/stock/counts/${id}`),
  create: (bucket: Bucket, note: string) => api<CountDetail>('/stock/counts/', { method: 'POST', body: JSON.stringify({ bucket, note }) }),
  /** Pemilih barang (izin Stok Opname). */
  items: (q: string, limit = 20) => api<Page<OpeningRow>>(`/stock/counts/items?${new URLSearchParams({ q, limit: String(limit) })}`),
  addItems: (id: string, scope: CountScope) => api<{ added: number }>(`/stock/counts/${id}/items`, { method: 'POST', body: JSON.stringify(scope) }),
  /** Mengisi hasil hitung; null mengosongkan (kembali belum dihitung). */
  setCounted: (id: string, itemId: string, qty: string | null) =>
    api<void>(`/stock/counts/${id}/items/${itemId}`, { method: 'PUT', body: JSON.stringify({ counted_qty: qty }) }),
  removeItem: (id: string, itemId: string) => api<void>(`/stock/counts/${id}/items/${itemId}`, { method: 'DELETE' }),
  complete: (id: string) => api<CountDetail>(`/stock/counts/${id}/complete`, { method: 'POST' }),
  cancel: (id: string) => api<void>(`/stock/counts/${id}/cancel`, { method: 'POST' })
};

// ---- Mutasi stok (antar cabang / antar bucket) ----
export type TransferOutlet = { id: string; code: string; name: string };
export type TransferStatus = 'sent' | 'received' | 'cancelled';
export type TransferLine = {
  item_id: string;
  sku: string;
  name: string;
  unit: string;
  qty_sent: string;
  qty_received: string | null;
  unit_cost: string;
};
export type Transfer = {
  id: string;
  doc_no: string;
  from: TransferOutlet;
  from_bucket: Bucket;
  to: TransferOutlet;
  to_bucket: Bucket;
  status: TransferStatus;
  note: string;
  sent_by: string;
  sent_at: string;
  received_by?: string;
  received_at: string | null;
  cancelled_by?: string;
  cancelled_at: string | null;
  cancel_reason?: string;
  line_count: number;
  qty_sent: string;
  qty_short: string;
  lines?: TransferLine[];
};
export type TransferPage = {
  data: Transfer[];
  next_cursor: string;
  has_more: boolean;
  summary: { to_receive: number; in_transit: number };
};
export type SendTransferInput = {
  to_outlet_id: string;
  from_bucket: Bucket;
  to_bucket: Bucket;
  note?: string;
  lines: { item_id: string; qty: string }[];
};

export const transfers = {
  /** Barang bertipe goods + stok per bucket cabang aktif (pemilih barang; izin kirim mutasi). */
  items: (q: string, limit = 20) => api<Page<OpeningRow>>(`/stock/transfers/items?${new URLSearchParams({ q, limit: String(limit) })}`),
  destinations: () => api<{ data: TransferOutlet[] }>('/stock/transfers/destinations'),
  list: (p: { direction: 'out' | 'in'; status?: string; q?: string; cursor?: string; limit?: number }) => {
    const s = new URLSearchParams({ direction: p.direction });
    if (p.status) s.set('status', p.status);
    if (p.q) s.set('q', p.q);
    if (p.cursor) s.set('cursor', p.cursor);
    if (p.limit) s.set('limit', String(p.limit));
    return api<TransferPage>(`/stock/transfers/?${s}`);
  },
  get: (id: string) => api<Transfer>(`/stock/transfers/${id}`),
  send: (input: SendTransferInput, idempotencyKey: string) =>
    api<Transfer>('/stock/transfers/', { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) }),
  /** lines kosong = semua diterima penuh. */
  receive: (id: string, lines: { item_id: string; qty: string }[] = []) =>
    api<Transfer>(`/stock/transfers/${id}/receive`, { method: 'POST', body: JSON.stringify({ lines }) }),
  cancel: (id: string, reason: string) => api<Transfer>(`/stock/transfers/${id}/cancel`, { method: 'POST', body: JSON.stringify({ reason }) })
};
