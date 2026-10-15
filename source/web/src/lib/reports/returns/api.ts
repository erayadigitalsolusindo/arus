// Klien API laporan retur: GET /reports/{sales-returns,purchase-returns}/{invoices,invoice-detail}.
import { api } from '#lib/api/client.ts';

export type ReturnKind = 'sale' | 'purchase';
export type ReturnView = 'invoice' | 'detail';

export type ReturnLine = {
  position: number;
  sku: string;
  name: string;
  unit: string;
  qty: string;
  unit_price: string;
  value: string;
  discount: string;
  tax_amount: string;
  net: string;
};

export type ReturnRow = {
  id: string;
  doc_no: string;
  /** Nota (penjualan) / faktur (pembelian) asal. */
  ref_doc_no: string;
  date: string;
  status: 'completed' | 'void';
  outlet_code: string;
  outlet_name: string;
  /** Member (retur penjualan) / pemasok (retur pembelian). */
  party_name: string;
  user_name: string;
  note: string;
  void_reason: string;
  subtotal: string;
  discount: string;
  tax_amount: string;
  surcharge: string;
  total: string;
  /** Memotong piutang (penjualan) / hutang (pembelian). */
  offset: string;
  /** Uang dikembalikan. */
  refund: string;
  refund_method: string;
  lines?: ReturnLine[];
};

export type ReturnSummary = {
  return_count: number;
  void_count: number;
  subtotal: string;
  discount: string;
  tax_amount: string;
  surcharge: string;
  total: string;
  offset: string;
  refund: string;
  /** Total penjualan/pembelian completed pada periode & filter yang sama. */
  base_total: string;
  /** Total retur ÷ base_total × 100. */
  ratio_pct: string;
};

export type ReturnResult = {
  from: string;
  to: string;
  data: ReturnRow[];
  summary: ReturnSummary;
  has_summary: boolean;
  next_cursor?: string;
  truncated?: boolean;
};

export type ReturnParams = {
  from: string;
  to: string;
  outlet_id?: string;
  /** member_id (retur penjualan) / supplier_id (retur pembelian). */
  party_id?: string;
  status?: string;
  settle?: string;
  q?: string;
  all_outlets?: boolean;
  cursor?: string;
  limit?: number;
};

const base = (kind: ReturnKind) => (kind === 'sale' ? '/reports/sales-returns' : '/reports/purchase-returns');
const path = (view: ReturnView) => (view === 'detail' ? 'invoice-detail' : 'invoices');

function query(kind: ReturnKind, params: ReturnParams): URLSearchParams {
  const qs = new URLSearchParams({ from: params.from, to: params.to });
  const opt: [string, string | undefined][] = [
    ['outlet_id', params.outlet_id],
    [kind === 'sale' ? 'member_id' : 'supplier_id', params.party_id],
    ['status', params.status],
    ['settle', params.settle],
    ['q', params.q],
    ['cursor', params.cursor],
    ['limit', params.limit ? String(params.limit) : undefined]
  ];
  for (const [k, v] of opt) if (v) qs.set(k, v);
  if (params.all_outlets) qs.set('all_outlets', 'true');
  return qs;
}

export const returnReport = (kind: ReturnKind, view: ReturnView, params: ReturnParams) =>
  api<ReturnResult>(`${base(kind)}/${path(view)}?${query(kind, params)}`);

export function returnReportFileUrl(kind: ReturnKind, view: ReturnView, params: ReturnParams, format: 'xlsx' | 'pdf'): string {
  const qs = query(kind, { ...params, cursor: undefined, limit: undefined });
  qs.set('format', format);
  return `${base(kind)}/${path(view)}?${qs}`;
}
