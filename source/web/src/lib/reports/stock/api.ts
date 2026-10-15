// Klien API laporan persedian item.
import { api } from '#lib/api/client.ts';

// Tipe data respons: GET /reports/stock/valuation
export type ValuationRow = {
  id: string;
  sku: string;
  name: string;
  unit: string;
  display_qty: string;
  warehouse_qty: string;
  returns_qty: string;
  total_qty: string;
  avg_cost: string;
  stock_value: string;
};

export type ValuationTotals = {
  display_qty: string;
  warehouse_qty: string;
  returns_qty: string;
  total_qty: string;
  stock_value: string;
};

export type ValuationResult = {
  from: string;
  to: string;
  outlet_id?: string;
  category_id?: string;
  data: ValuationRow[];
  totals: ValuationTotals;
  limit: number;
  next_cursor?: string;
};

// GET /reports/stock/valuation
export async function stockValuation(params: {
  outlet_id?: string;
  category_id?: string;
  from?: string;
  to?: string;
  limit?: number;
  cursor?: string;
}): Promise<ValuationResult> {
  const qs = new URLSearchParams();
  if (params.outlet_id) qs.set('outlet_id', params.outlet_id);
  if (params.category_id) qs.set('category_id', params.category_id);
  if (params.from) qs.set('from', params.from);
  if (params.to) qs.set('to', params.to);
  if (params.limit) qs.set('limit', String(params.limit));
  if (params.cursor) qs.set('cursor', params.cursor);
  return api(`/reports/stock/valuation?${qs}`);
}

// Laporan master: GET /reports/master/{members|suppliers|salespeople}
export type MasterKind = 'members' | 'suppliers' | 'salespeople';

export type MasterResult<T> = { data: T[]; limit: number; next_cursor?: string };

export function masterReport<T>(kind: MasterKind, params: { q?: string; status?: string; cursor?: string; limit?: number }): Promise<MasterResult<T>> {
  const qs = new URLSearchParams();
  if (params.q) qs.set('q', params.q);
  if (params.status) qs.set('status', params.status);
  if (params.cursor) qs.set('cursor', params.cursor);
  if (params.limit) qs.set('limit', String(params.limit));
  return api(`/reports/master/${kind}?${qs}`);
}

// URL unduh ekspor (format=xlsx|pdf) — dipakai apiBlob. Seluruh hasil filter (tanpa cursor), dibatasi di server.
export function masterReportFileUrl(kind: MasterKind, params: { q?: string; status?: string }, format: 'xlsx' | 'pdf'): string {
  const qs = new URLSearchParams();
  if (params.q) qs.set('q', params.q);
  if (params.status) qs.set('status', params.status);
  qs.set('format', format);
  return `/reports/master/${kind}?${qs}`;
}
