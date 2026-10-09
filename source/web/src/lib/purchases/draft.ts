// Draf faktur pembelian di browser agar tidak hilang saat tab ditutup / PC mati.
// Hanya isian mentah: total, PPN, dan HPP dihitung ulang server lewat quote saat dipulihkan.
// Per tenant+outlet+pengguna, kedaluwarsa 24 jam (sama seperti keranjang kasir).

import type { ItemChoice, PaymentType } from './api.ts';

export type DraftRow = { key: string; item: ItemChoice; qd: string; qw: string; price: string; sub: string; disc: [string, string, string, string] };
export type DraftCost = { key: string; name: string; amount: string };

export type PurchaseDraft = {
  supplierId: string;
  supplierLabel: string;
  invoice: string;
  date: string;
  payment: PaymentType;
  due: string;
  taxPct: string;
  costs: DraftCost[];
  note: string;
  rows: DraftRow[];
};

const MAX_AGE_MS = 24 * 60 * 60 * 1000;
const MAX_ROWS = 300;

export const draftKey = (tenantId: string, outletId: string, userId: string) => `purchase.draft.v1:${tenantId}:${outletId}:${userId}`;

const str = (v: unknown): v is string => typeof v === 'string';

function validRow(r: unknown): r is DraftRow {
  if (!r || typeof r !== 'object') return false;
  const x = r as Record<string, unknown>;
  const it = x.item as Record<string, unknown> | null;
  return (
    str(x.key) &&
    !!it && typeof it === 'object' && str(it.id) && str(it.name) &&
    str(x.qd) && str(x.qw) && str(x.price) && str(x.sub) &&
    Array.isArray(x.disc) && x.disc.length === 4 && x.disc.every(str)
  );
}

/** Draf yang masih berlaku, atau null bila kosong, rusak, atau kedaluwarsa (entri rusak dibuang). */
export function loadDraft(key: string, now = Date.now()): PurchaseDraft | null {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return null;
    const d = JSON.parse(raw) as Record<string, unknown>;
    if (typeof d.at !== 'number' || now - d.at > MAX_AGE_MS || d.at > now + 60_000) {
      localStorage.removeItem(key);
      return null;
    }
    if (!Array.isArray(d.rows) || d.rows.length > MAX_ROWS || !d.rows.every(validRow)) {
      localStorage.removeItem(key);
      return null;
    }
    return {
      supplierId: str(d.supplierId) ? d.supplierId : '',
      supplierLabel: str(d.supplierLabel) ? d.supplierLabel : '',
      invoice: str(d.invoice) ? d.invoice : '',
      date: str(d.date) ? d.date : '',
      payment: d.payment === 'credit' ? 'credit' : 'cash',
      due: str(d.due) ? d.due : '',
      taxPct: str(d.taxPct) ? d.taxPct : '',
      costs: Array.isArray(d.costs) ? d.costs.filter((c): c is DraftCost => !!c && typeof c === 'object' && str((c as DraftCost).key) && str((c as DraftCost).name) && str((c as DraftCost).amount)).slice(0, 20) : [],
      note: str(d.note) ? d.note : '',
      rows: d.rows
    };
  } catch {
    return null;
  }
}

/** Menulis draf; isian kosong menghapus entri. Galat penyimpanan (penuh/diblokir) diabaikan. */
export function saveDraft(key: string, draft: PurchaseDraft, now = Date.now()) {
  try {
    const empty = draft.rows.length === 0 && draft.supplierId === '' && draft.invoice === '' && draft.note === '' && draft.costs.length === 0;
    if (empty) localStorage.removeItem(key);
    else localStorage.setItem(key, JSON.stringify({ at: now, ...draft }));
  } catch {
    /* penyimpanan tak tersedia: form tetap bekerja tanpa pemulihan */
  }
}

export function clearDraft(key: string) {
  try {
    localStorage.removeItem(key);
  } catch {
    /* abaikan */
  }
}
