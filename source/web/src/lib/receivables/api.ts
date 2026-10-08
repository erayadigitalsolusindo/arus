// Klien API piutang member (penjualan kredit). Uang sebagai string desimal; pembayaran memakai Idempotency-Key per percobaan simpan.
import { api } from '#lib/api/client.ts';

export type ReceivableStatus = 'open' | 'overdue' | 'paid';
export type ReceivableFilter = ReceivableStatus | 'all';

export type Receivable = {
  id: string;
  sale_id: string;
  doc_no: string;
  member_id: string;
  member_code: string;
  member_name: string;
  amount: string;
  paid: string;
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

export type ReceivableDetail = Receivable & { sale_total: string; sale_at: string; payments: ReceivablePayment[] };

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
  pay: (id: string, input: ReceivablePayInput, idempotencyKey: string) =>
    api<ReceivableDetail>(`/receivables/${id}/payments`, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) })
};
