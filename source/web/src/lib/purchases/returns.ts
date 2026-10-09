// Klien API retur pembelian (Fase 6.6). Uang/qty sebagai string desimal; simpan memakai Idempotency-Key per isi permintaan.
import { api } from '#lib/api/client.ts';
import type { PaymentType } from '#lib/purchases/api.ts';

export type ReturnStatus = 'completed' | 'void';

export type ReturnInput = {
  purchase_id: string;
  /** position = posisi baris nota asal; qty dalam satuan dasar. */
  lines: { position: number; qty: string }[];
  note?: string;
  /** Wajib bila ada dana dikembalikan pemasok (nilai retur melebihi sisa hutang). */
  refund_method_id?: string;
  refund_ref?: string;
};

export type ReturnablePurchase = {
  id: string;
  doc_no: string;
  supplier_name: string;
  supplier_invoice_no: string;
  purchase_date: string;
  payment_type: PaymentType;
  total: string;
};

export type ReturnSourceLine = {
  position: number;
  item_id: string;
  sku: string;
  name: string;
  unit: string;
  qty: string;
  returned: string;
  returnable: string;
  net_price: string;
  unit_cost: string;
  return_stock: string;
};

export type ReturnSource = {
  purchase_id: string;
  doc_no: string;
  supplier_id: string;
  supplier_name: string;
  supplier_invoice_no: string;
  purchase_date: string;
  payment_type: PaymentType;
  tax_pct: string;
  /** null = nota tanpa hutang (tunai). */
  payable_balance: string | null;
  lines: ReturnSourceLine[];
};

export type ReturnQuote = {
  lines: { position: number; item_id: string; qty: string; value: string; return_stock: string; issue?: 'STOCK_INSUFFICIENT' }[];
  subtotal: string;
  tax_amount: string;
  total: string;
  payable_balance: string | null;
  payable_cut: string;
  refund: string;
};

export type PurchaseReturn = {
  id: string;
  doc_no: string;
  outlet_id: string;
  outlet_code: string;
  outlet_name: string;
  purchase_id: string;
  purchase_doc_no: string;
  supplier_id: string;
  supplier_name: string;
  supplier_invoice_no: string;
  return_date: string;
  status: ReturnStatus;
  note: string;
  subtotal: string;
  tax_amount: string;
  total: string;
  payable_cut: string;
  refund: string;
  refund_method_name: string;
  refund_ref: string;
  created_at: string;
  created_by: string;
  void_reason: string;
  voided_at: string | null;
  voided_by: string;
  lines: { position: number; purchase_position: number; item_id: string; sku: string; name: string; unit: string; qty: string; value: string; unit_cost: string }[];
};

export type ReturnRow = {
  id: string;
  doc_no: string;
  purchase_id: string;
  purchase_doc_no: string;
  supplier_name: string;
  return_date: string;
  status: ReturnStatus;
  total: string;
  payable_cut: string;
  refund: string;
  lines: number;
  created_at: string;
  created_by: string;
};

export type ReturnList = {
  data: ReturnRow[];
  has_more: boolean;
  next_cursor: string;
  summary: { count: number; total: string; payable_cut: string; refund: string };
};

export const purchaseReturns = {
  list: (p: { from?: string; to?: string; status?: ReturnStatus | ''; q?: string; limit?: number; cursor?: string }) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(p)) if (v !== undefined && v !== '' && v !== 0) qs.set(k, String(v));
    return api<ReturnList>(`/purchase-returns/?${qs}`);
  },
  get: (id: string) => api<PurchaseReturn>(`/purchase-returns/${id}`),
  purchases: (q: string) => api<{ data: ReturnablePurchase[] }>(`/purchase-returns/purchases?q=${encodeURIComponent(q)}`).then((r) => r.data),
  source: (purchaseId: string) => api<ReturnSource>(`/purchase-returns/source/${purchaseId}`),
  quote: (input: ReturnInput, signal?: AbortSignal) => api<ReturnQuote>('/purchase-returns/quote', { method: 'POST', body: JSON.stringify(input), signal }),
  create: (input: ReturnInput, idempotencyKey: string) =>
    api<PurchaseReturn>('/purchase-returns/', { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) }),
  void: (id: string, reason: string) => api<PurchaseReturn>(`/purchase-returns/${id}/void`, { method: 'POST', body: JSON.stringify({ reason }) })
};
