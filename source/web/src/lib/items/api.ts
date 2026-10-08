// Klien API Daftar Item. Uang/berat dikirim dan diterima sebagai string desimal (tidak lewat float).
import { api, apiBlob } from '#lib/api/client.ts';
import type { Page } from '#lib/catalog/api.ts';

export type Ref = { id: string; name: string };
export type ItemKind = 'goods' | 'service';

/** Satu anak tangga harga grosir: mulai min_qty (satuan DASAR), harga per satuan = price. */
export type Tier = { min_qty: string; price: string };
export type OutletTiers = { outlet_id: string; outlet_name: string; tiers: Tier[] };
export type Wholesale = { default: Tier[]; outlets: OutletTiers[] };

/** Satuan tambahan: 1 satuan ini = factor satuan dasar. sell_price null = factor × harga satuan dasar. */
export type AltUnit = { unit_id: string; unit_name: string; factor: string; barcode: string; sell_price: string | null };

export type ItemImage = {
  id: string;
  is_main: boolean;
  position: number;
  width: number;
  height: number;
  bytes: number;
  thumb_bytes: number;
  created_at: string;
};

export type OutletPrice = { outlet_id: string; outlet_name: string; sell_price: string | number | null };

export type Item = {
  id: string;
  sku: string;
  barcode: string;
  origin: string;
  name: string;
  weight_grams: string;
  last_cost: string;
  avg_cost: string;
  sell_price: string;
  kind: ItemKind;
  allow_negative_stock: boolean;
  sell_below_cost: boolean;
  description: string;
  active: boolean;
  unit: Ref;
  category: Ref | null;
  brand: Ref | null;
  principal: Ref | null;
  supplier: Ref | null;
  outlet_prices: OutletPrice[];
  images: ItemImage[];
  wholesale: Wholesale;
  units: AltUnit[];
  created_at: string;
  updated_at: string;
};

export type Row = {
  id: string;
  sku: string;
  barcode: string;
  origin: string;
  name: string;
  kind: ItemKind;
  active: boolean;
  unit: string;
  category: string;
  brand: string;
  price: string;
  /** Hanya mode semua cabang: price = termurah, price_max = termahal. */
  price_max?: string;
  price_override: boolean;
  /** Rincian per cabang (hanya mode semua cabang). */
  outlets?: { outlet_id: string; code: string; name: string; price: string; stock: StockQty }[];
  /** Harga rata (HPP rata-rata) dan harga beli akhir. */
  avg_cost: string;
  last_cost: string;
  main_image_id: string | null;
  stock: StockQty;
};

export type StockQty = { display: string; warehouse: string; returns: string; total: string };

export type ItemInput = {
  sku: string;
  barcode: string;
  /** Pembeda opsional (mis. negara asal) untuk barcode kembar. */
  origin: string;
  name: string;
  weight_grams: string;
  /** HPP awal; hanya dipakai saat membuat. */
  cost?: string;
  sell_price: string;
  unit_id: string;
  category_id: string;
  brand_id: string;
  principal_id: string;
  supplier_id: string;
  kind: ItemKind;
  allow_negative_stock: boolean;
  sell_below_cost: boolean;
  description: string;
  /** Hilang = harga cabang tidak diubah; daftar = menggantikan harga khusus cabang yang boleh diakses. */
  outlet_prices?: { outlet_id: string; sell_price: string }[];
  /** Hilang = grosir tidak diubah; ada = menggantikan set default + set cabang yang boleh diakses. */
  wholesale?: { default: Tier[]; outlets: { outlet_id: string; tiers: Tier[] }[] };
  /** Hilang = satuan tambahan tidak diubah; ada = menggantikan seluruhnya. sell_price kosong = dihitung otomatis. */
  units?: { unit_id: string; factor: string; barcode: string; sell_price: string }[];
};

/** Satu barang yang memakai barcode yang dicari: matched "item" = barcode barang, "unit" = barcode satuan tambahan. */
export type BarcodeMatch = {
  id: string;
  sku: string;
  name: string;
  origin: string;
  active: boolean;
  matched: 'item' | 'unit';
  unit_id: string;
  unit: string;
  factor: string;
  price: string;
};

export type ListQuery = { q?: string; active?: boolean; category_id?: string; allOutlets?: boolean; limit?: number; offset?: number };

function qs(p: ListQuery): string {
  const s = new URLSearchParams();
  if (p.q) s.set('q', p.q);
  if (p.active !== undefined) s.set('active', String(p.active));
  if (p.category_id) s.set('category_id', p.category_id);
  if (p.allOutlets) s.set('outlet', 'all');
  if (p.limit) s.set('limit', String(p.limit));
  if (p.offset) s.set('offset', String(p.offset));
  const out = s.toString();
  return out ? `?${out}` : '';
}

// Angka desimal dikirim sebagai string desimal (server membacanya sebagai json.Number, tanpa float); kosong = 0.
const num = (s: string | undefined) => (s === undefined || s.trim() === '' ? '0' : s.trim());

function body(input: ItemInput): string {
  const { weight_grams, cost, sell_price, outlet_prices, units, ...rest } = input;
  return JSON.stringify({
    ...rest,
    weight_grams: num(weight_grams),
    sell_price: num(sell_price),
    ...(cost !== undefined ? { cost: num(cost) } : {}),
    ...(outlet_prices ? { outlet_prices: outlet_prices.map((p) => ({ outlet_id: p.outlet_id, sell_price: num(p.sell_price) })) } : {}),
    // Harga satuan tambahan kosong = null di server (dihitung otomatis), jadi kuncinya dihilangkan, bukan "0".
    ...(units
      ? { units: units.map((u) => ({ unit_id: u.unit_id, factor: u.factor.trim(), barcode: u.barcode, ...(u.sell_price.trim() ? { sell_price: u.sell_price.trim() } : {}) })) }
      : {})
  });
}

export const items = {
  list: (p: ListQuery = {}) => api<Page<Row>>(`/items/${qs(p)}`),
  get: (id: string) => api<Item>(`/items/${id}`),
  create: (input: ItemInput) => api<Item>('/items/', { method: 'POST', body: body(input) }),
  update: (id: string, input: ItemInput) => api<Item>(`/items/${id}`, { method: 'PUT', body: body(input) }),
  setActive: (id: string, active: boolean) => api<Item>(`/items/${id}/active`, { method: 'PUT', body: JSON.stringify({ active }) }),

  /** Barang yang memakai barcode ini (barcode boleh kembar). exclude = id item yang sedang diubah. */
  byBarcode: (code: string, exclude?: string) =>
    api<{ data: BarcodeMatch[] }>(`/items/by-barcode?code=${encodeURIComponent(code)}${exclude ? `&exclude_id=${exclude}` : ''}`).then((r) => r.data),

  /** Unggah satu gambar (multipart). Server memvalidasi isi, mengecilkan, dan mengubahnya menjadi JPG. */
  uploadImage: (id: string, file: File) => {
    const form = new FormData();
    form.append('file', file);
    return api<ItemImage>(`/items/${id}/images`, { method: 'POST', body: form });
  },
  setMainImage: (id: string, imageId: string) => api<null>(`/items/${id}/images/${imageId}/main`, { method: 'PUT' }),
  deleteImage: (id: string, imageId: string) => api<null>(`/items/${id}/images/${imageId}`, { method: 'DELETE' }),
  /** Berkas gambar sebagai Blob (butuh token, jadi tidak bisa lewat <img src> langsung). */
  imageBlob: (id: string, imageId: string, size: 'thumb' | 'full') => apiBlob(`/items/${id}/images/${imageId}/file?size=${size}`)
};

// Cache URL objek per gambar: id gambar tidak pernah berubah isinya, jadi satu unduhan cukup selama sesi halaman.
const urlCache = new Map<string, Promise<string>>();

export function imageUrl(itemId: string, imageId: string, size: 'thumb' | 'full'): Promise<string> {
  const key = `${imageId}:${size}`;
  let p = urlCache.get(key);
  if (!p) {
    p = items.imageBlob(itemId, imageId, size).then((b) => URL.createObjectURL(b));
    p.catch(() => urlCache.delete(key)); // gagal → boleh dicoba lagi
    urlCache.set(key, p);
  }
  return p;
}
