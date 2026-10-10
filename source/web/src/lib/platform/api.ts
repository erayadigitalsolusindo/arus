// Klien API Platform Admin. Semua lewat token platform (papi); tidak pernah memakai token tenant.
import type { AuthResponse } from '#lib/api/client.ts';
import { papi } from './session.svelte.ts';

export type TenantRow = {
  id: string;
  code: string;
  name: string;
  active: boolean;
  created_at: string;
  outlet_count: number;
  user_count: number;
  last_login_at: string | null;
  owner_email: string;
};
export type TenantPage = { items: TenantRow[]; total: number; limit: number; offset: number };

export type OutletRow = { id: string; code: string; name: string; active: boolean };
export type UserRow = {
  id: string;
  name: string;
  email: string;
  active: boolean;
  role_name: string;
  last_login_at: string | null;
  email_verified: boolean;
  created_at: string;
};
export type TenantDetail = { id: string; code: string; name: string; active: boolean; created_at: string; sale_edit_window_days: number; outlets: OutletRow[]; users: UserRow[] };

export type AdminRow = { id: string; email: string; name: string; active: boolean; mfa_enabled: boolean; last_login_at: string | null; created_at: string };

export type TenantAuditItem = {
  id: number;
  actor_name: string;
  action: string;
  entity: string;
  entity_id: string;
  details: Record<string, unknown>;
  ip: string;
  created_at: string;
};

export type PlatformAuditItem = {
  id: number;
  admin_id: string | null;
  admin_name: string;
  action: string;
  tenant_id: string | null;
  tenant_name: string;
  details: Record<string, unknown>;
  ip: string;
  created_at: string;
};

const json = (v: unknown) => JSON.stringify(v);

export type ApkInfo = { available: boolean; size?: number; version?: string; sha256?: string; uploaded_at?: string };

export const platformApi = {
  apkInfo: () => papi<ApkInfo>('/public/mobile/apk/info'),
  /** Unggah APK Kasir (multipart; versi harus dikirim sebelum berkas). */
  uploadApk: (file: File, version: string) => {
    const form = new FormData();
    if (version) form.append('version', version);
    form.append('file', file);
    return papi<ApkInfo>('/platform/mobile/apk', { method: 'PUT', body: form });
  },
  deleteApk: () => papi<void>('/platform/mobile/apk', { method: 'DELETE' }),
  tenants: (q: string, limit: number, offset: number) => papi<TenantPage>(`/platform/tenants?${new URLSearchParams({ q, limit: String(limit), offset: String(offset) })}`),
  tenant: (id: string) => papi<TenantDetail>(`/platform/tenants/${id}`),
  /** Batas hari edit/batal nota tenant (0 = hanya hari nota dibuat, maks 3650). Hanya operator platform yang boleh mengubah. */
  setSaleEditWindow: (id: string, days: number) => papi<void>(`/platform/tenants/${id}`, { method: 'PATCH', body: json({ sale_edit_window_days: days }) }),
  setTenantActive: (id: string, active: boolean) => papi<void>(`/platform/tenants/${id}`, { method: 'PATCH', body: json({ active }) }),
  tenantAudit: (id: string, cursor = '') =>
    papi<{ items: TenantAuditItem[]; next_cursor: string }>(`/platform/tenants/${id}/audit?${new URLSearchParams({ limit: '30', ...(cursor ? { cursor } : {}) })}`),
  impersonate: (id: string, outletId = '') =>
    papi<AuthResponse>(`/platform/tenants/${id}/impersonate`, { method: 'POST', body: json(outletId ? { outlet_id: outletId } : {}) }),

  admins: () => papi<{ items: AdminRow[] }>('/platform/admins'),
  createAdmin: (body: { name: string; email: string; password: string }) => papi<{ id: string }>('/platform/admins', { method: 'POST', body: json(body) }),
  setAdminActive: (id: string, active: boolean) => papi<void>(`/platform/admins/${id}`, { method: 'PATCH', body: json({ active }) }),
  setAdminPassword: (id: string, password: string) => papi<void>(`/platform/admins/${id}/password`, { method: 'PUT', body: json({ password }) }),

  audit: (f: { tenant_id?: string; admin_id?: string; action?: string; before?: number }) => {
    const q = new URLSearchParams({ limit: '50' });
    for (const [k, v] of Object.entries(f)) if (v !== undefined && v !== '') q.set(k, String(v));
    return papi<{ items: PlatformAuditItem[]; next_before?: number }>(`/platform/audit?${q}`);
  }
};
