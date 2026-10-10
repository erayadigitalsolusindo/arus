// Klien API hutang pemasok (pembelian kredit). Uang sebagai string desimal; pembayaran memakai Idempotency-Key per percobaan simpan.
import { api } from '#lib/api/client.ts';

export type PayableStatus = 'open' | 'overdue' | 'paid';
export type PayableFilter = PayableStatus | 'all';

export type Payable = {
  id: string;
  purchase_id: string;
  doc_no: string;
  supplier_invoice_no: string;
  supplier_id: string;
  supplier_code: string;
  supplier_name: string;
  purchase_date: string;
  amount: string;
  paid: string;
  /** Dipotong retur pembelian aktif. */
  returned: string;
  balance: string;
  /** YYYY-MM-DD; kosong = tanpa jatuh tempo. */
  due_date?: string;
  status: PayableStatus;
};

export type PayablePayment = {
  id: string;
  doc_no: string;
  method: string;
  method_id: string;
  method_name: string;
  amount: string;
  ref_no: string;
  note: string;
  paid_by: string;
  created_at: string;
};

export type PayableDetail = Payable & { purchase_total: string; payments: PayablePayment[] };

export type PayableSummary = {
  outstanding: string;
  overdue: string;
  open_count: number;
  total_count: number;
  aging: { current: string; d1_30: string; d31_60: string; d60_plus: string };
};
/** Paginasi keyset: kirim next_cursor sebagai cursor untuk halaman berikutnya. */
export type PayableList = { data: Payable[]; summary: PayableSummary; has_more: boolean; next_cursor: string };

export type PayablePayInput = { method_id: string; amount: string; ref_no?: string; note?: string };

export const payables = {
  list: (p: { status?: PayableFilter; q?: string; supplier_id?: string; limit?: number; cursor?: string }) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(p)) if (v !== undefined && v !== '' && v !== 0) qs.set(k, String(v));
    return api<PayableList>(`/payables/?${qs}`);
  },
  get: (id: string) => api<PayableDetail>(`/payables/${id}`),
  pay: (id: string, input: PayablePayInput, idempotencyKey: string) =>
    api<PayableDetail>(`/payables/${id}/payments`, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) })
};
