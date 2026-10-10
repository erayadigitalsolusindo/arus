// Klien API shift kasir (FR-POS-16). Rekap per metode dihitung SERVER; klien hanya mengirim uang fisik per metode.
import { api } from '#lib/api/client.ts';
import type { PayMethod } from '#lib/sales/api.ts';

export type ShiftCount = {
  method_id: string;
  name: string;
  kind: PayMethod;
  sales: string;
  flows: string;
  opening: string;
  expected: string;
  counted: string | null;
  diff: string | null;
};

export type ShiftFlow = { source: string; method_id: string; name: string; kind: PayMethod; amount: string; count: number };

export type Shift = {
  id: string;
  doc_no: string;
  status: 'open' | 'closed';
  outlet_id: string;
  user_id: string;
  user_name: string;
  opening_cash: string;
  opened_at: string;
  closed_at?: string;
  closed_by_name?: string;
  expected_total: string;
  counted_total: string | null;
  diff_total: string | null;
  diff_abs: string | null;
  sale_count: number;
  void_count: number;
  sales_total: string;
  receivable_total: string;
  note: string;
  approved_by_name?: string;
  counts: ShiftCount[];
  flows: ShiftFlow[];
  opened_local: string;
  closed_local?: string;
  store: { tenant_name: string; outlet_code: string; outlet_name: string; address: string; phone: string };
};

export type ShiftListRow = {
  id: string;
  doc_no: string;
  status: 'open' | 'closed';
  user_id: string;
  user_name: string;
  opening_cash: string;
  opened_at: string;
  closed_at?: string;
  expected_total: string | null;
  counted_total: string | null;
  diff_total: string | null;
  diff_abs: string | null;
  sale_count: number | null;
  sales_total: string | null;
  note: string;
  approved_by_name?: string;
};

export type ShiftList = { data: ShiftListRow[]; next_cursor?: string; has_more: boolean; from: string; to: string; cashiers: { id: string; name: string }[] };

export type CloseInput = { counts: { method_id: string; counted: string }[]; note: string; approval?: { user_id: string; pin: string } };

export const shifts = {
  current: () => api<{ shift: Shift | null }>('/shifts/current').then((r) => r.shift),
  open: (openingCash: string) => api<Shift>('/shifts/open', { method: 'POST', body: JSON.stringify({ opening_cash: openingCash }) }),
  get: (id: string) => api<Shift>(`/shifts/${id}`),
  close: (id: string, input: CloseInput, idempotencyKey: string) =>
    api<Shift>(`/shifts/${id}/close`, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) }),
  list: (p: { from?: string; to?: string; user_id?: string; status?: string; diff?: boolean; cursor?: string; limit?: number }) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(p)) if (v !== undefined && v !== '' && v !== false) qs.set(k, v === true ? '1' : String(v));
    return api<ShiftList>(`/shifts/?${qs}`);
  }
};
