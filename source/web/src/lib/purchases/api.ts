// Klien API pembelian (Fase 6.2). Uang/qty sebagai string desimal; simpan memakai Idempotency-Key per isi permintaan.
import { api } from '#lib/api/client.ts';

export type PaymentType = 'cash' | 'credit';

export type LineInput = {
  item_id: string;
  qty_display: string;
  qty_warehouse: string;
  unit_price: string;
  /** Maks 4 tingkat bertingkat: < 100 = persen, >= 100 = rupiah. */
  discounts: string[];
};

export type PurchaseInput = {
  supplier_id: string;
  supplier_invoice_no?: string;
  purchase_date?: string;
  payment_type: PaymentType;
  due_date?: string;
  tax_pct?: string;
  other_costs: { name: string; amount: string }[];
  note?: string;
  lines: LineInput[];
};

export type QuoteLine = {
  item_id: string;
  qty: string;
  line_total: string;
  cost_alloc: string;
  unit_cost: string;
  stock_before: string;
  avg_before: string;
  avg_after: string;
};
export type Quote = { lines: QuoteLine[]; subtotal: string; tax_pct: string; tax_amount: string; other_cost: string; total: string };

export type PurchaseLine = {
  item_id: string;
  sku: string;
  name: string;
  unit: string;
  qty_display: string;
  qty_warehouse: string;
  qty: string;
  unit_price: string;
  discounts: string[];
  line_total: string;
  cost_alloc: string;
  unit_cost: string;
  stock_before: string;
  avg_before: string;
  avg_after: string;
};

export type Purchase = {
  id: string;
  doc_no: string;
  outlet_id: string;
  outlet_code: string;
  outlet_name: string;
  supplier_id: string;
  supplier_name: string;
  supplier_invoice_no: string;
  purchase_date: string;
  payment_type: PaymentType;
  due_date: string | null;
  status: string;
  note: string;
  subtotal: string;
  tax_pct: string;
  tax_amount: string;
  other_cost: string;
  total: string;
  created_at: string;
  created_by: string;
  lines: PurchaseLine[];
  costs: { name: string; amount: string }[];
  payable: { id: string; amount: string; due_date: string | null } | null;
  events: PurchaseEvent[];
  revision: number;
  revision_reason: string;
  revised_at: string | null;
  superseded_by: string | null;
  supersedes_id: string | null;
  void_reason: string;
  voided_at: string | null;
};

/** Catatan audit satu nota: siapa, kapan, dan rincian aksinya. */
export type PurchaseEvent = { action: string; actor: string; at: string; details: unknown };

export type PurchaseRow = {
  id: string;
  doc_no: string;
  supplier_id: string;
  supplier_name: string;
  supplier_invoice_no: string;
  purchase_date: string;
  payment_type: PaymentType;
  due_date: string | null;
  status: string;
  total: string;
  lines: number;
  created_at: string;
  created_by: string;
};

/** Paginasi keyset: kirim next_cursor sebagai cursor untuk halaman berikutnya (has_more = masih ada). */
export type PurchaseList = {
  data: PurchaseRow[];
  has_more: boolean;
  next_cursor: string;
  summary: { count: number; total: string; credit_total: string };
};

export type ItemChoice = { id: string; sku: string; barcode: string; name: string; unit: string; avg_cost: string; last_cost: string; stock_total: string };

export const purchases = {
  list: (p: { from?: string; to?: string; supplier_id?: string; payment_type?: PaymentType | ''; q?: string; limit?: number; cursor?: string }) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(p)) if (v !== undefined && v !== '' && v !== 0) qs.set(k, String(v));
    return api<PurchaseList>(`/purchases/?${qs}`);
  },
  get: (id: string) => api<Purchase>(`/purchases/${id}`),
  items: (q: string) => api<{ data: ItemChoice[] }>(`/purchases/items?q=${encodeURIComponent(q)}`).then((r) => r.data),
  quote: (input: Partial<PurchaseInput>, signal?: AbortSignal) => api<Quote>('/purchases/quote', { method: 'POST', body: JSON.stringify(input), signal }),
  create: (input: PurchaseInput, idempotencyKey: string) =>
    api<Purchase>('/purchases/', { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) }),
  /** Revisi nota (nota lama dibalik lalu diganti nota baru). Alasan wajib 3–200 karakter. */
  edit: (id: string, input: PurchaseInput, reason: string, idempotencyKey: string) =>
    api<Purchase>(`/purchases/${id}`, { method: 'PUT', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify({ ...input, reason }) }),
  /** Batalkan nota: stok & HPP dibalik, nota tetap tercatat berstatus dibatalkan. */
  void: (id: string, reason: string) => api<Purchase>(`/purchases/${id}/void`, { method: 'POST', body: JSON.stringify({ reason }) })
};
