// Klien API laporan hutang / piutang: GET /reports/{payables,receivables}/{outstanding,payments,balances,ledger}
// dan GET /reports/supplier-credits (saldo di pemasok).
import { api } from '#lib/api/client.ts';

export type DebtKind = 'payable' | 'receivable';
export type DebtView = 'outstanding' | 'payments' | 'balances' | 'ledger' | 'credit';

export type Buckets = { current: string; d1_30: string; d31_60: string; d61_90: string; over_90: string };

export type DebtSummary = {
  doc_count: number;
  party_count?: number;
  amount: string;
  paid: string;
  returned: string;
  balance: string;
  overdue: string;
  buckets: Buckets;
};

export type OutstandingRow = {
  id: string;
  doc_no: string;
  ref_no: string;
  outlet_code: string;
  party_code: string;
  party_name: string;
  doc_date: string;
  due_date: string;
  amount: string;
  paid: string;
  returned: string;
  balance: string;
  days_overdue: number;
  /** 0 belum jatuh tempo, 1: 1-30, 2: 31-60, 3: 61-90, 4: >90 hari. */
  bucket: number;
};

export type BalanceRow = {
  party_id: string;
  code: string;
  name: string;
  doc_count: number;
  amount: string;
  paid: string;
  returned: string;
  balance: string;
  overdue: string;
  buckets: Buckets;
  oldest_due: string;
};

export type PaymentRow = {
  id: string;
  doc_no: string;
  date: string;
  time: string;
  outlet_code: string;
  target_doc_no: string;
  party_name: string;
  method: string;
  ref_no: string;
  amount: string;
  fee: string;
  user_name: string;
  note: string;
  settlement_no: string;
};

export type PaymentSummary = {
  count: number;
  amount: string;
  fee: string;
  by_method: { name: string; count: number; amount: string }[];
};

export type LedgerRow = {
  date: string;
  type: 'invoice' | 'payment' | 'return';
  doc_no: string;
  ref_no: string;
  memo: string;
  increase: string;
  decrease: string;
  balance: string;
};

export type LedgerSummary = { opening: string; increase: string; decrease: string; closing: string };

export type CreditRow = {
  supplier_id: string;
  code: string;
  name: string;
  opening: string;
  added: string;
  used: string;
  cashed_out: string;
  voided: string;
  closing: string;
};

export type CreditSummary = {
  supplier_count: number;
  opening: string;
  added: string;
  used: string;
  cashed_out: string;
  voided: string;
  closing: string;
};

type Page<R, S> = {
  as_of?: string;
  from?: string;
  to?: string;
  party_code?: string;
  party_name?: string;
  data: R[];
  summary: S;
  has_summary: boolean;
  next_cursor?: string;
  truncated?: boolean;
};

export type DebtResultMap = {
  outstanding: Page<OutstandingRow, DebtSummary>;
  balances: Page<BalanceRow, DebtSummary>;
  payments: Page<PaymentRow, PaymentSummary>;
  ledger: Page<LedgerRow, LedgerSummary>;
  credit: Page<CreditRow, CreditSummary>;
};

export type DebtParams = {
  as_of?: string;
  from?: string;
  to?: string;
  outlet_id?: string;
  party_id?: string;
  q?: string;
  all_outlets?: boolean;
  cursor?: string;
  limit?: number;
};

const base = (kind: DebtKind, view: DebtView) =>
  view === 'credit' ? '/reports/supplier-credits' : `${kind === 'payable' ? '/reports/payables' : '/reports/receivables'}/${view}`;

function query(params: DebtParams): URLSearchParams {
  const qs = new URLSearchParams();
  const opt: [string, string | undefined][] = [
    ['as_of', params.as_of],
    ['from', params.from],
    ['to', params.to],
    ['outlet_id', params.outlet_id],
    ['party_id', params.party_id],
    ['q', params.q],
    ['cursor', params.cursor],
    ['limit', params.limit ? String(params.limit) : undefined]
  ];
  for (const [k, v] of opt) if (v) qs.set(k, v);
  if (params.all_outlets) qs.set('all_outlets', 'true');
  return qs;
}

export const debtReport = <V extends DebtView>(kind: DebtKind, view: V, params: DebtParams) =>
  api<DebtResultMap[V]>(`${base(kind, view)}?${query(params)}`);

export function debtReportFileUrl(kind: DebtKind, view: DebtView, params: DebtParams, format: 'xlsx' | 'pdf'): string {
  const qs = query({ ...params, cursor: undefined, limit: undefined });
  qs.set('format', format);
  return `${base(kind, view)}?${qs}`;
}
