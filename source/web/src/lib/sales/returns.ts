import { api } from '#lib/api/client.ts';

export type SaleReturnInput = {
  sale_id: string;
  lines: { position: number; qty: string }[];
  note?: string;
  refund_method_id?: string;
  refund_ref?: string;
};

export type SaleReturnChoice = {
  sale_id: string;
  doc_no: string;
  created_at: string;
  total: string;
  member_name: string;
  receivable: string;
};

export type SaleReturnSourceLine = {
  position: number;
  item_id: string;
  sku: string;
  name: string;
  unit: string;
  qty: string;
  returned: string;
  returnable: string;
  unit_price: string;
  return_stock: string;
};

export type SaleReturnSource = {
  sale_id: string;
  doc_no: string;
  outlet_name: string;
  created_at: string;
  total: string;
  member_name: string;
  has_member: boolean;
  receivable: string;
  lines: SaleReturnSourceLine[];
};

export type SaleReturnQuote = {
  lines: { position: number; item_id: string; sku: string; name: string; unit: string; qty: string; value: string }[];
  subtotal: string;
  discount: string;
  tax_amount: string;
  surcharge: string;
  total: string;
  receivable_balance: string;
  receivable_cut: string;
  refund: string;
  points_earned_reversed: number;
  points_redeemed_restored: number;
};

export type SaleReturnLine = {
  position: number;
  sale_position: number;
  item_id: string;
  sku: string;
  name: string;
  unit: string;
  qty: string;
  value: string;
  discount: string;
  tax_amount: string;
  unit_cost: string;
};

export type SaleReturn = {
  id: string;
  doc_no: string;
  outlet_id: string;
  outlet_name: string;
  sale_id: string;
  sale_doc_no: string;
  member_name: string;
  return_date: string;
  note: string;
  subtotal: string;
  discount: string;
  tax_amount: string;
  surcharge: string;
  total: string;
  receivable_cut: string;
  refund: string;
  refund_method: string;
  refund_method_name: string;
  refund_ref: string;
  points_earned_reversed: number;
  points_redeemed_restored: number;
  created_at: string;
  created_by: string;
  status: 'completed' | 'void';
  void_reason: string;
  voided_at: string | null;
  voided_by: string;
  lines: SaleReturnLine[];
};

export type SaleReturnRow = {
  id: string;
  doc_no: string;
  sale_id: string;
  sale_doc_no: string;
  member_name: string;
  return_date: string;
  status: 'completed' | 'void';
  total: string;
  refund: string;
  receivable_cut: string;
  lines: number;
  created_at: string;
  created_by: string;
};

export type SaleReturnList = { data: SaleReturnRow[]; has_more: boolean; next_cursor: string };

export const salesReturns = {
  list: (p: { from?: string; to?: string; q?: string; status?: '' | 'completed' | 'void'; limit?: number; cursor?: string }) => {
    const qs = new URLSearchParams();
    for (const [key, value] of Object.entries(p)) if (value !== undefined && value !== '' && value !== 0) qs.set(key, String(value));
    return api<SaleReturnList>(`/sales-returns/?${qs}`);
  },
  choices: (q: string) => api<{ data: SaleReturnChoice[] }>(`/sales-returns/sales?q=${encodeURIComponent(q)}`).then((r) => r.data),
  source: (saleId: string) => api<SaleReturnSource>(`/sales-returns/source/${saleId}`),
  quote: (input: SaleReturnInput, signal?: AbortSignal) => api<SaleReturnQuote>('/sales-returns/quote', { method: 'POST', body: JSON.stringify(input), signal }),
  create: (input: SaleReturnInput, idempotencyKey: string) =>
    api<SaleReturn>('/sales-returns/', { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) }),
  get: (id: string) => api<SaleReturn>(`/sales-returns/${id}`),
  void: (id: string, reason: string) => api<SaleReturn>(`/sales-returns/${id}/void`, { method: 'POST', body: JSON.stringify({ reason }) })
};
