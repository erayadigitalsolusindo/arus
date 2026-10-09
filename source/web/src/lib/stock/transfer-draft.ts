// Draf form Mutasi Stok di browser agar tidak hilang saat tab ditutup / PC mati (pola sama dengan lib/purchases/draft.ts).
// Hanya isian mentah: stok pada baris disegarkan dari server saat draf dipulihkan; server tetap otoritatif saat kirim.
// Per tenant+outlet+pengguna, kedaluwarsa 24 jam.

import type { Bucket, OpeningRow } from './api.ts';

export type DraftLine = { row: OpeningRow; qty: string };

export type TransferDraft = {
  toOutlet: string;
  fromBucket: Bucket;
  toBucket: Bucket;
  note: string;
  lines: DraftLine[];
};

const MAX_AGE_MS = 24 * 60 * 60 * 1000;
const MAX_LINES = 200;
const BUCKETS = ['display', 'warehouse', 'returns'];

export const draftKey = (tenantId: string, outletId: string, userId: string) => `transfer.draft.v1:${tenantId}:${outletId}:${userId}`;

const str = (v: unknown): v is string => typeof v === 'string';

function validLine(l: unknown): l is DraftLine {
  if (!l || typeof l !== 'object') return false;
  const x = l as Record<string, unknown>;
  const r = x.row as Record<string, unknown> | null;
  return (
    str(x.qty) &&
    !!r &&
    typeof r === 'object' &&
    str(r.id) &&
    str(r.sku) &&
    str(r.name) &&
    str(r.unit) &&
    str(r.display) &&
    str(r.warehouse) &&
    str(r.returns)
  );
}

/** Draf yang masih berlaku, atau null bila kosong, rusak, atau kedaluwarsa (entri rusak dibuang). */
export function loadDraft(key: string, now = Date.now()): TransferDraft | null {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return null;
    const d = JSON.parse(raw) as Record<string, unknown>;
    if (
      typeof d.at !== 'number' ||
      now - d.at > MAX_AGE_MS ||
      d.at > now + 60_000 ||
      !Array.isArray(d.lines) ||
      d.lines.length > MAX_LINES ||
      !d.lines.every(validLine)
    ) {
      localStorage.removeItem(key);
      return null;
    }
    return {
      toOutlet: str(d.toOutlet) ? d.toOutlet : '',
      fromBucket: BUCKETS.includes(d.fromBucket as string) ? (d.fromBucket as Bucket) : 'display',
      toBucket: BUCKETS.includes(d.toBucket as string) ? (d.toBucket as Bucket) : 'display',
      note: str(d.note) ? d.note : '',
      lines: d.lines
    };
  } catch {
    return null;
  }
}

/** Menulis draf; isian kosong (tanpa barang, tujuan, catatan) menghapus entri. Galat penyimpanan diabaikan. */
export function saveDraft(key: string, draft: TransferDraft, now = Date.now()) {
  try {
    if (draft.lines.length === 0 && draft.toOutlet === '' && draft.note === '') localStorage.removeItem(key);
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
