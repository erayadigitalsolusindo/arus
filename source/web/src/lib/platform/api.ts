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

export type LanguageRow = {
  code: string;
  name: string;
  short: string;
  intl: string;
  protected: boolean;
  active: boolean;
  sort_order: number;
  created_at: string;
};

const json = (v: unknown) => JSON.stringify(v);

export type ExtCredential = {
  id: string;
  tenant_id: string;
  tenant_name: string;
  user_id: string;
  user_name: string;
  outlet_id: string;
  outlet_name: string;
  label: string;
  key_id: string;
  ip_allowlist: string[];
  rate_limit_min: number;
  active: boolean;
  expires_at: string | null;
  last_used_at: string | null;
  last_used_ip: string | null;
  rotating: boolean;
  created_at: string;
};
export type ExtCreated = { id: string; key_id: string; secret: string };
export type ExtActors = { users: { id: string; name: string; email: string; role: string }[]; outlets: { id: string; name: string }[] };
export type ExtBrand = {
  id: string;
  slug: string;
  domain: string | null;
  is_default: boolean;
  active: boolean;
  name: string;
  tagline: string;
  logo_url: string;
  accent: string;
  support_email: string;
  support_url: string;
  footer: string;
  base_url: string;
  hidden_modules: string[];
  sandbox_note: string;
};
export type ExtBrandInput = Omit<ExtBrand, 'id'>;
export type ExtGroup = { id: string; title: string; count: number };

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
  },

  extCredentials: (tenantId = '') => papi<{ items: ExtCredential[] }>(`/platform/ext/credentials${tenantId ? `?tenant_id=${tenantId}` : ''}`),
  extActors: (tenantId: string) => papi<ExtActors>(`/platform/ext/tenants/${tenantId}/actors`),
  extCreateCredential: (body: { tenant_id: string; user_id: string; outlet_id: string; label: string; ip_allowlist: string[]; rate_limit_min: number; expires_at: string | null }) =>
    papi<ExtCreated>('/platform/ext/credentials', { method: 'POST', body: json(body) }),
  extUpdateCredential: (id: string, body: { label: string; ip_allowlist: string[]; rate_limit_min: number; active: boolean; expires_at: string | null }) =>
    papi<void>(`/platform/ext/credentials/${id}`, { method: 'PATCH', body: json(body) }),
  extRotateCredential: (id: string, graceMinutes: number) => papi<ExtCreated>(`/platform/ext/credentials/${id}/rotate`, { method: 'POST', body: json({ grace_minutes: graceMinutes }) }),
  extDeleteCredential: (id: string) => papi<void>(`/platform/ext/credentials/${id}`, { method: 'DELETE' }),
  extBrands: () => papi<{ items: ExtBrand[]; groups: ExtGroup[] }>('/platform/ext/brands'),
  extCreateBrand: (body: ExtBrandInput) => papi<{ id: string }>('/platform/ext/brands', { method: 'POST', body: json(body) }),
  extUpdateBrand: (id: string, body: ExtBrandInput) => papi<{ id: string }>(`/platform/ext/brands/${id}`, { method: 'PUT', body: json(body) }),
  extDeleteBrand: (id: string) => papi<void>(`/platform/ext/brands/${id}`, { method: 'DELETE' }),

  languages: () => papi<{ items: LanguageRow[] }>('/platform/languages'),
  createLanguage: (body: { code: string; name: string; short: string; intl: string; sort_order: number }) =>
    papi<{ code: string }>('/platform/languages', { method: 'POST', body: json(body) }),
  updateLanguage: (code: string, body: { name: string; active: boolean; sort_order: number }) =>
    papi<void>(`/platform/languages/${code}`, { method: 'PUT', body: json(body) }),
  deleteLanguage: (code: string) => papi<void>(`/platform/languages/${code}`, { method: 'DELETE' })
};
