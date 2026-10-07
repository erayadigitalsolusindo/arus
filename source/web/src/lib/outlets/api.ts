// Klien API outlet. Pemilih outlet memakai /outlets/accessible (semua pengguna); halaman kelola memakai izin `outlets`.
import { api } from '#lib/api/client.ts';

export type Outlet = {
  id: string;
  code: string;
  name: string;
  /** Persen sebagai string desimal dari server (mis. "11"). */
  tax_store_pct: string;
  tax_gov_pct: string;
  timezone: string;
  active: boolean;
  created_at: string;
};

export type OutletInput = {
  code?: string; // hanya saat membuat
  name: string;
  timezone: string;
  tax_store_pct: number;
  tax_gov_pct: number;
  active?: boolean; // hanya saat mengubah
};

export const outlets = {
  list: () => api<{ outlets: Outlet[] }>('/outlets/').then((r) => r.outlets),
  accessible: () => api<{ outlets: Outlet[]; current_id: string }>('/outlets/accessible'),
  create: (input: OutletInput) => api<Outlet>('/outlets/', { method: 'POST', body: JSON.stringify(input) }),
  update: (id: string, input: OutletInput) => api<Outlet>(`/outlets/${id}`, { method: 'PUT', body: JSON.stringify(input) })
};
