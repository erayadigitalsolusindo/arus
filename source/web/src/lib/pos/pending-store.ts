// Nota pending: keranjang yang diparkir kasir (mis. pelanggan lupa satu barang) lalu dibuka lagi nanti.
// Disimpan di browser per tenant+outlet+kasir, sama seperti keranjang aktif; bukan dokumen server —
// harga & stok selalu dihitung ulang lewat quote saat nota dibuka dan dibayar.

import { parseCart, type StoredCart } from './cart-store.ts';

export type PendingNote = StoredCart & {
  id: string;
  /** Nomor urut tampilan (Pending 1, 2, …), unik di dalam daftar. */
  no: number;
  at: number;
  /** Keterangan dari kasir (nama pelanggan, ciri-ciri) agar mudah dikenali; boleh kosong. */
  label: string;
  /** Perkiraan total saat diparkir (string desimal) — hanya tampilan, bukan angka resmi. */
  total: string | null;
};

const MAX_AGE_MS = 24 * 60 * 60 * 1000;
export const MAX_PENDING = 20;
export const MAX_LABEL = 60;

export const pendingStorageKey = (tenantId: string, outletId: string, userId: string) => `pos.pending.v1:${tenantId}:${outletId}:${userId}`;

/** Daftar nota pending yang masih berlaku, terbaru dulu. Entri rusak/kedaluwarsa dibuang. */
export function loadPending(key: string, now = Date.now()): PendingNote[] {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return [];
    const arr = JSON.parse(raw) as unknown;
    if (!Array.isArray(arr)) return [];
    const out: PendingNote[] = [];
    for (const e of arr) {
      if (!e || typeof e !== 'object') continue;
      const d = e as Record<string, unknown>;
      if (typeof d.id !== 'string' || typeof d.no !== 'number' || typeof d.at !== 'number') continue;
      if (now - d.at > MAX_AGE_MS || d.at > now + 60_000) continue;
      const cart = parseCart(d);
      if (!cart) continue;
      out.push({ ...cart, id: d.id, no: d.no, at: d.at, label: typeof d.label === 'string' ? d.label.slice(0, MAX_LABEL) : '', total: typeof d.total === 'string' ? d.total : null });
    }
    return out.sort((a, b) => b.at - a.at).slice(0, MAX_PENDING);
  } catch {
    return [];
  }
}

export function savePending(key: string, list: PendingNote[]) {
  try {
    if (list.length === 0) localStorage.removeItem(key);
    else localStorage.setItem(key, JSON.stringify(list));
  } catch {
    /* penyimpanan tak tersedia / penuh: pending tetap ada di memori selama halaman terbuka */
  }
}

/** Nomor urut berikutnya yang belum dipakai. */
export const nextPendingNo = (list: PendingNote[]) => list.reduce((m, p) => Math.max(m, p.no), 0) + 1;
