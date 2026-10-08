// Klien API slot pintasan barang kasir (16 slot per kasir, disimpan di server).
import { api } from '#lib/api/client.ts';

export type Shortcut = {
  slot: number;
  item_id: string;
  sku: string;
  name: string;
  unit: string;
  kind: 'goods' | 'service';
  active: boolean;
  price: string;
  main_image_id: string | null;
};

export const SHORTCUT_SLOTS = 16;

type Res = { data: Shortcut[]; slots: number };

export const shortcuts = {
  list: () => api<Res>('/pos/shortcuts/'),
  set: (slot: number, itemId: string) => api<Res>(`/pos/shortcuts/${slot}`, { method: 'PUT', body: JSON.stringify({ item_id: itemId }) }),
  clear: (slot: number) => api<Res>(`/pos/shortcuts/${slot}`, { method: 'DELETE' })
};
