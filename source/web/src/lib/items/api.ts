// Klien API Daftar Item. Uang/berat dikirim dan diterima sebagai string desimal (tidak lewat float).
import { api } from '#lib/api/client.ts';
import type { Page } from '#lib/catalog/api.ts';

export type Ref = { id: string; name: string };
export type ItemKind = 'goods' | 'service';

export type OutletPrice = { outlet_id: string; outlet_name: string; sell_price: string | number | null };

export type Item = {
  id: string;
  sku: string;
  barcode: string;
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
  created_at: string;
  updated_at: string;
};

export type Row = {
  id: string;
  sku: string;
  barcode: string;
  name: string;
  kind: ItemKind;
  active: boolean;
  unit: string;
  category: string;
  brand: string;
  price: string;
  price_override: boolean;
};

export type ItemInput = {
  sku: string;
  barcode: string;
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
};

export type ListQuery = { q?: string; active?: boolean; category_id?: string; limit?: number; offset?: number };

function qs(p: ListQuery): string {
  const s = new URLSearchParams();
  if (p.q) s.set('q', p.q);
  if (p.active !== undefined) s.set('active', String(p.active));
  if (p.category_id) s.set('category_id', p.category_id);
  if (p.limit) s.set('limit', String(p.limit));
  if (p.offset) s.set('offset', String(p.offset));
  const out = s.toString();
  return out ? `?${out}` : '';
}

// Angka desimal dikirim sebagai string desimal (server membacanya sebagai json.Number, tanpa float); kosong = 0.
const num = (s: string | undefined) => (s === undefined || s.trim() === '' ? '0' : s.trim());

function body(input: ItemInput): string {
  const { weight_grams, cost, sell_price, outlet_prices, ...rest } = input;
  return JSON.stringify({
    ...rest,
    weight_grams: num(weight_grams),
    sell_price: num(sell_price),
    ...(cost !== undefined ? { cost: num(cost) } : {}),
    ...(outlet_prices ? { outlet_prices: outlet_prices.map((p) => ({ outlet_id: p.outlet_id, sell_price: num(p.sell_price) })) } : {})
  });
}

export const items = {
  list: (p: ListQuery = {}) => api<Page<Row>>(`/items/${qs(p)}`),
  get: (id: string) => api<Item>(`/items/${id}`),
  create: (input: ItemInput) => api<Item>('/items/', { method: 'POST', body: body(input) }),
  update: (id: string, input: ItemInput) => api<Item>(`/items/${id}`, { method: 'PUT', body: body(input) }),
  setActive: (id: string, active: boolean) => api<Item>(`/items/${id}/active`, { method: 'PUT', body: JSON.stringify({ active }) })
};
