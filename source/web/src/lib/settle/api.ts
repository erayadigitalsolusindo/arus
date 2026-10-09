// Klien pelunasan kolektif (bayar per pemasok / per member). Server membagi uang ke nota; uang sebagai string desimal.
import { api } from '#lib/api/client.ts';

export type SettleKind = 'payable' | 'receivable';
export type SettleMode = 'auto' | 'manual';

export type SettleInput = {
  party_id: string;
  mode: SettleMode;
  method_id?: string;
  /** Total uang (mode auto). */
  amount?: string;
  /** Mode manual: nota + jumlah per nota. */
  allocations?: { id: string; amount: string }[];
  ref_no?: string;
  note?: string;
};

/** Satu baris alokasi, sudah dinormalkan dari bentuk hutang/piutang. */
export type SettleAllocation = { id: string; doc: string; date: string; due?: string; balance?: string; amount: string; balance_after?: string };
export type SettlePlan = { total: string; outstanding: string; open_count: number; allocations: SettleAllocation[] };
export type SettleResult = { id: string; doc_no: string; total: string; fee?: string; allocations: SettleAllocation[] };

type RawAlloc = Record<string, string | undefined>;
const norm = (kind: SettleKind, a: RawAlloc): SettleAllocation => ({
  id: (kind === 'payable' ? a.payable_id : a.receivable_id) ?? '',
  doc: (kind === 'payable' ? a.purchase_doc_no : a.sale_doc_no) ?? '',
  date: (kind === 'payable' ? a.purchase_date : a.sale_date) ?? '',
  due: a.due_date,
  balance: a.balance,
  amount: a.amount ?? '0',
  balance_after: a.balance_after
});

function body(kind: SettleKind, i: SettleInput) {
  const idKey = kind === 'payable' ? 'payable_id' : 'receivable_id';
  return JSON.stringify({
    [kind === 'payable' ? 'supplier_id' : 'member_id']: i.party_id,
    mode: i.mode,
    method_id: i.method_id,
    amount: i.mode === 'auto' ? i.amount : undefined,
    allocations: i.mode === 'manual' ? (i.allocations ?? []).map((a) => ({ [idKey]: a.id, amount: a.amount })) : undefined,
    ref_no: i.ref_no || undefined,
    note: i.note || undefined
  });
}

const base = (kind: SettleKind) => (kind === 'payable' ? '/payables' : '/receivables');

export const settle = {
  quote: async (kind: SettleKind, i: SettleInput): Promise<SettlePlan> => {
    const r = await api<{ total: string; outstanding: string; open_count: number; allocations: RawAlloc[] }>(`${base(kind)}/settlements/quote`, { method: 'POST', body: body(kind, i) });
    return { ...r, allocations: r.allocations.map((a) => norm(kind, a)) };
  },
  create: async (kind: SettleKind, i: SettleInput, idempotencyKey: string): Promise<SettleResult> => {
    const r = await api<{ id: string; doc_no: string; total: string; fee?: string; allocations: RawAlloc[] }>(`${base(kind)}/settlements`, {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey },
      body: body(kind, i)
    });
    return { ...r, allocations: r.allocations.map((a) => norm(kind, a)) };
  }
};
