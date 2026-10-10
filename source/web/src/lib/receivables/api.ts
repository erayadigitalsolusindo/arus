// Klien API piutang member (penjualan kredit). Uang sebagai string desimal; pembayaran memakai Idempotency-Key per percobaan simpan.
import { api } from '#lib/api/client.ts';

export type ReceivableStatus = 'open' | 'overdue' | 'paid' | 'void';
export type ReceivableFilter = Exclude<ReceivableStatus, 'void'> | 'all';

export type Receivable = {
  id: string;
  /** Kosong untuk saldo awal (tanpa nota). */
  sale_id: string | null;
  kind: 'sale' | 'opening';
  /** No. nota/bon lama (saldo awal). */
  ref_no?: string;
  doc_no: string;
  member_id: string;
  member_code: string;
  member_name: string;
  amount: string;
  paid: string;
  returned: string;
  balance: string;
  /** YYYY-MM-DD; kosong = tanpa jatuh tempo. */
  due_date?: string;
  created_at: string;
  status: ReceivableStatus;
};

export type ReceivablePayment = {
  id: string;
  doc_no: string;
  method: string;
  method_id: string;
  method_name: string;
  amount: string;
  ref_no: string;
  fee_pct: string;
  fee: string;
  fee_bearer: 'store' | 'customer';
  note: string;
  received_by: string;
  created_at: string;
};

export type ReceivableDetail = Receivable & {
  sale_total: string;
  sale_at: string;
  payments: ReceivablePayment[];
  doc_date?: string;
  note?: string;
  created_by?: string;
  voided_at?: string;
  void_reason?: string;
  voided_by?: string;
};

export type OpeningInput = { member_id: string; ref_no?: string; doc_date: string; amount: string; due_date?: string; note?: string };

export type ReceivableSummary = { outstanding: string; overdue: string; open_count: number; total_count: number };
export type ReceivableList = { data: Receivable[]; summary: ReceivableSummary; has_more: boolean };

export type ReceivablePayInput = { method_id: string; amount: string; ref_no?: string; note?: string };

export const receivables = {
  list: (p: { status?: ReceivableFilter; q?: string; member_id?: string; limit?: number; offset?: number }) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(p)) if (v !== undefined && v !== '' && v !== 0) qs.set(k, String(v));
    return api<ReceivableList>(`/receivables/?${qs}`);
  },
  get: (id: string) => api<ReceivableDetail>(`/receivables/${id}`),
  /** Saldo awal piutang member (onboarding). Idempotency-Key per isian simpan. */
  createOpening: (input: OpeningInput, idempotencyKey: string) =>
    api<ReceivableDetail>('/receivables/opening', { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) }),
  /** Batalkan saldo awal yang belum dibayar. */
  voidOpening: (id: string, reason: string) => api<ReceivableDetail>(`/receivables/${id}/void`, { method: 'POST', body: JSON.stringify({ reason }) }),
  pay: (id: string, input: ReceivablePayInput, idempotencyKey: string) =>
    api<ReceivableDetail>(`/receivables/${id}/payments`, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) })
};
