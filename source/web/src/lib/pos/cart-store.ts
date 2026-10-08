// Penyimpanan keranjang kasir di browser agar tidak hilang saat tab ditutup / mati lampu.
// Hanya preferensi per-perangkat: server tetap menghitung ulang harga & stok lewat quote.
// Sengaja TIDAK menyimpan persetujuan/PIN; baris yang harganya diubah kembali ke harga normal.

export type StoredLine = {
  key: string;
  id: string;
  unitId: string | null;
  sku: string;
  name: string;
  unit: string;
  price: string;
  qty: string;
  goods: boolean;
  imageId: string | null;
};

/** Satu rincian biaya lain-lain (nama + jumlah desimal bertitik). */
export type CostEntry = { label: string; amount: string };

/** Member terpilih (hanya penanda; saldo poin selalu diambil ulang dari server). */
export type StoredMember = { id: string; code: string; name: string; level: string; spend_per_point: string; point_value: string; cover_image_id?: string | null };

/** Salesman terpilih (hanya penanda; server memvalidasi aktif/tidaknya saat nota disimpan). */
export type StoredSalesperson = { id: string; name: string };

export type StoredCart = { lines: StoredLine[]; otherCost: string; costs?: CostEntry[]; taxOn: boolean; note: string; member?: StoredMember | null; redeem?: string; salesperson?: StoredSalesperson | null };

const validSalesperson = (s: unknown): s is StoredSalesperson => !!s && typeof s === 'object' && str((s as StoredSalesperson).id) && str((s as StoredSalesperson).name);

const validMember = (m: unknown): m is StoredMember => !!m && typeof m === 'object' && ['id', 'code', 'name', 'level', 'spend_per_point', 'point_value'].every((k) => str((m as Record<string, unknown>)[k]));

const validCost = (c: unknown): c is CostEntry => !!c && typeof c === 'object' && str((c as CostEntry).label) && str((c as CostEntry).amount);

const MAX_AGE_MS = 24 * 60 * 60 * 1000;
const MAX_LINES = 500;

const str = (v: unknown): v is string => typeof v === 'string';
const strOrNull = (v: unknown): v is string | null => v === null || typeof v === 'string';

export const cartStorageKey = (tenantId: string, outletId: string, userId: string) => `pos.cart.v1:${tenantId}:${outletId}:${userId}`;

function validLine(l: unknown): l is StoredLine {
  if (!l || typeof l !== 'object') return false;
  const x = l as Record<string, unknown>;
  return (
    str(x.key) && str(x.id) && strOrNull(x.unitId) && str(x.sku) && str(x.name) && str(x.unit) && str(x.price) && str(x.qty) && typeof x.goods === 'boolean' && strOrNull(x.imageId)
  );
}

/** Memeriksa bentuk keranjang tersimpan (dipakai juga oleh nota pending); null bila kosong/rusak. */
export function parseCart(d: Record<string, unknown>): StoredCart | null {
  if (!Array.isArray(d.lines) || d.lines.length === 0 || d.lines.length > MAX_LINES || !d.lines.every(validLine)) return null;
  return {
    lines: d.lines,
    otherCost: str(d.otherCost) ? d.otherCost : '',
    costs: Array.isArray(d.costs) ? d.costs.filter(validCost).slice(0, 20) : [],
    taxOn: d.taxOn === true,
    note: str(d.note) ? d.note : '',
    member: validMember(d.member) ? d.member : null,
    redeem: str(d.redeem) ? d.redeem : '',
    salesperson: validSalesperson(d.salesperson) ? d.salesperson : null
  };
}

/** Mengembalikan keranjang tersimpan, atau null bila kosong/rusak/kedaluwarsa. */
export function loadCart(key: string, now = Date.now()): StoredCart | null {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return null;
    const d = JSON.parse(raw) as Record<string, unknown>;
    if (typeof d.at !== 'number' || now - d.at > MAX_AGE_MS || d.at > now + 60_000) {
      localStorage.removeItem(key);
      return null;
    }
    const cart = parseCart(d);
    if (!cart) localStorage.removeItem(key);
    return cart;
  } catch {
    return null;
  }
}

/** Menyimpan keranjang; keranjang kosong menghapus entri. Galat penyimpanan (penuh/diblokir) diabaikan. */
export function saveCart(key: string, cart: StoredCart, now = Date.now()) {
  try {
    if (cart.lines.length === 0) {
      localStorage.removeItem(key);
      return;
    }
    localStorage.setItem(key, JSON.stringify({ at: now, ...cart }));
  } catch {
    /* penyimpanan tak tersedia: kasir tetap bisa bekerja tanpa pemulihan */
  }
}
