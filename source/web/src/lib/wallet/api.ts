import { api } from '#lib/api/client.ts';

/** Jenis baris ledger saldo titipan (deposit member / kredit pemasok). */
export type WalletEntryKind =
  | 'TOPUP'
  | 'WITHDRAW'
  | 'SALE_PAYMENT'
  | 'SALE_REVERSAL'
  | 'RECEIVABLE_PAYMENT'
  | 'SALE_RETURN'
  | 'SALE_RETURN_VOID'
  | 'PURCHASE_RETURN'
  | 'PURCHASE_RETURN_VOID'
  | 'PAYABLE_PAYMENT'
  | 'CASH_OUT';

export type WalletEntry = {
  id: number;
  kind: WalletEntryKind;
  amount: string; // bertanda: + menambah saldo
  balance_after: string;
  ref_id?: string;
  doc_no: string;
  method_name: string;
  ref_no: string;
  note: string;
  outlet_name: string;
  actor_name: string;
  created_at: string;
};

export type WalletAccount = {
  owner_id: string;
  name: string;
  code: string;
  balance: string;
  entries: WalletEntry[];
  has_more: boolean;
  next_before: number;
};

export type CashInput = { amount: string; method_id: string; ref_no?: string; note?: string };

export type CreditRow = { supplier_id: string; code: string; name: string; balance: string };

const post = (path: string, input: CashInput, key: string) =>
  api<WalletAccount>(path, { method: 'POST', headers: { 'Idempotency-Key': key }, body: JSON.stringify(input) });

export const memberDeposits = {
  account: (memberId: string, before = 0) => api<WalletAccount>(`/member-deposits/${memberId}${before ? `?before=${before}` : ''}`),
  topup: (memberId: string, input: CashInput, key: string) => post(`/member-deposits/${memberId}/topup`, input, key),
  withdraw: (memberId: string, input: CashInput, key: string) => post(`/member-deposits/${memberId}/withdraw`, input, key)
};

export const supplierCredits = {
  list: (p: { q?: string; positive?: boolean; cursor?: string; limit?: number } = {}) => {
    const qs = new URLSearchParams();
    if (p.q) qs.set('q', p.q);
    if (p.positive) qs.set('positive', 'true');
    if (p.cursor) qs.set('cursor', p.cursor);
    if (p.limit) qs.set('limit', String(p.limit));
    return api<{ data: CreditRow[]; next_cursor: string; has_more: boolean }>(`/supplier-credits/?${qs}`);
  },
  account: (supplierId: string, before = 0) => api<WalletAccount>(`/supplier-credits/${supplierId}${before ? `?before=${before}` : ''}`),
  cashOut: (supplierId: string, input: CashInput, key: string) => post(`/supplier-credits/${supplierId}/cash-out`, input, key)
};
