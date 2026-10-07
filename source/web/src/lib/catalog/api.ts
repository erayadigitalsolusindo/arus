// Klien API master pendukung katalog (satuan, kategori, brand, principal, supplier).
import { api } from '#lib/api/client.ts';

/** Id master sederhana = segmen URL = id modul izin. */
export type SimpleKind = 'units' | 'categories' | 'brands' | 'principals';

export type Entry = { id: string; name: string; active: boolean; created_at: string };

export type Supplier = {
  id: string;
  code: string;
  name: string;
  contact_name: string;
  phone: string;
  email: string;
  address: string;
  note: string;
  active: boolean;
  created_at: string;
};

export type SupplierInput = Omit<Supplier, 'id' | 'active' | 'created_at'>;

export type ListQuery = {
  q?: string;
  /** undefined = semua. */
  active?: boolean;
  limit?: number;
  offset?: number;
};

export type Page<T> = { data: T[]; total: number };

function qs(p: ListQuery): string {
  const s = new URLSearchParams();
  if (p.q) s.set('q', p.q);
  if (p.active !== undefined) s.set('active', String(p.active));
  if (p.limit) s.set('limit', String(p.limit));
  if (p.offset) s.set('offset', String(p.offset));
  const out = s.toString();
  return out ? `?${out}` : '';
}

const json = (body: unknown) => JSON.stringify(body);

/** Master sederhana: hanya nama + status. */
export const simple = (kind: SimpleKind) => ({
  list: (p: ListQuery = {}, signal?: AbortSignal) => api<Page<Entry>>(`/catalog/${kind}/${qs(p)}`, { signal }),
  create: (name: string) => api<Entry>(`/catalog/${kind}/`, { method: 'POST', body: json({ name }) }),
  rename: (id: string, name: string) => api<Entry>(`/catalog/${kind}/${id}`, { method: 'PUT', body: json({ name }) }),
  setActive: (id: string, active: boolean) => api<Entry>(`/catalog/${kind}/${id}/active`, { method: 'PUT', body: json({ active }) })
});

/** Pilihan master untuk combobox di form lain (mis. item): hanya id + nama, aktif saja. */
export type LookupKind = SimpleKind | 'suppliers';

export const lookup = (kind: LookupKind) => ({
  search: (q: string, limit = 20) =>
    api<Page<Pick<Entry, 'id' | 'name'>>>(`/catalog/lookup/${kind}/${qs({ q, active: true, limit })}`).then((r) => r.data)
});

export const suppliers = {
  list: (p: ListQuery = {}, signal?: AbortSignal) => api<Page<Supplier>>(`/catalog/suppliers/${qs(p)}`, { signal }),
  create: (input: SupplierInput) => api<Supplier>('/catalog/suppliers/', { method: 'POST', body: json(input) }),
  update: (id: string, input: SupplierInput) => api<Supplier>(`/catalog/suppliers/${id}`, { method: 'PUT', body: json(input) }),
  setActive: (id: string, active: boolean) => api<Supplier>(`/catalog/suppliers/${id}/active`, { method: 'PUT', body: json({ active }) })
};
