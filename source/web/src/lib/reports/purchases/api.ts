// Klien API laporan pembelian: GET /reports/purchases/{invoices,invoice-detail,items,by-date}.
import { api } from '#lib/api/client.ts';

export type PurchaseLine = {
  position: number;
  sku: string;
  name: string;
  unit: string;
  qty: string;
  unit_price: string;
  disc1: string;
  disc2: string;
  disc3: string;
  disc4: string;
  line_total: string;
  unit_cost: string;
};

export type PurchaseInvoice = {
  id: string;
  doc_no: string;
  supplier_invoice_no: string;
  date: string;
  status: string;
  outlet_code: string;
  outlet_name: string;
  supplier_name: string;
  payment_type: 'cash' | 'credit';
  due_date: string;
  note: string;
  subtotal: string;
  tax_pct: string;
  tax_amount: string;
  other_cost: string;
  total: string;
  lines?: PurchaseLine[];
};

export type PurchaseInvoiceSummary = {
  purchase_count: number;
  void_count: number;
  cash_count: number;
  credit_count: number;
  subtotal: string;
  tax_amount: string;
  other_cost: string;
  total: string;
  cash_total: string;
  credit_total: string;
};

export type PurchaseInvoicesResult = {
  from: string;
  to: string;
  data: PurchaseInvoice[];
  summary: PurchaseInvoiceSummary;
  has_summary: boolean;
  next_cursor?: string;
  truncated?: boolean;
};

export type PurchaseItemRow = {
  item_id: string;
  sku: string;
  name: string;
  unit: string;
  purchase_count: number;
  qty: string;
  total: string;
  avg_price: string;
  last_price: string;
};

export type PurchaseItemsResult = {
  from: string;
  to: string;
  data: PurchaseItemRow[];
  summary: { item_count: number; total: string };
};

export type PurchaseDayRow = {
  date: string;
  purchase_count: number;
  void_count: number;
  subtotal: string;
  tax_amount: string;
  other_cost: string;
  total: string;
  cash_total: string;
  credit_total: string;
  void_total: string;
  return_count: number;
  return_total: string;
  net_total: string;
};

export type PurchaseByDateResult = {
  from: string;
  to: string;
  data: PurchaseDayRow[];
  totals: PurchaseDayRow;
};

export type PurchaseReportParams = {
  from: string;
  to: string;
  outlet_id?: string;
  supplier_id?: string;
  payment_type?: string;
  status?: string;
  q?: string;
  sort?: string;
  hide_empty?: boolean;
  all_outlets?: boolean;
  cursor?: string;
  limit?: number;
};

export type PurchaseReportKind = 'invoices' | 'invoice-detail' | 'items' | 'by-date';

function query(params: PurchaseReportParams): URLSearchParams {
  const qs = new URLSearchParams({ from: params.from, to: params.to });
  const opt: [string, string | undefined][] = [
    ['outlet_id', params.outlet_id],
    ['supplier_id', params.supplier_id],
    ['payment_type', params.payment_type],
    ['status', params.status],
    ['q', params.q],
    ['sort', params.sort],
    ['cursor', params.cursor],
    ['limit', params.limit ? String(params.limit) : undefined]
  ];
  for (const [k, v] of opt) if (v) qs.set(k, v);
  if (params.all_outlets) qs.set('all_outlets', 'true');
  if (params.hide_empty) qs.set('hide_empty', 'true');
  return qs;
}

export const purchaseInvoices = (kind: 'invoices' | 'invoice-detail', params: PurchaseReportParams) =>
  api<PurchaseInvoicesResult>(`/reports/purchases/${kind}?${query(params)}`);

export const purchaseItems = (params: PurchaseReportParams) =>
  api<PurchaseItemsResult>(`/reports/purchases/items?${query(params)}`);

export const purchaseByDate = (params: PurchaseReportParams) =>
  api<PurchaseByDateResult>(`/reports/purchases/by-date?${query(params)}`);

export function purchaseReportFileUrl(kind: PurchaseReportKind, params: PurchaseReportParams, format: 'xlsx' | 'pdf'): string {
  const qs = query({ ...params, cursor: undefined, limit: undefined });
  qs.set('format', format);
  return `/reports/purchases/${kind}?${qs}`;
}
