// Klien API landing page toko (/storefront). Halaman publiknya dilayani API sebagai HTML statis di /s/{slug}.
import { api, API_URL } from '#lib/api/client.ts';

export type StorefrontConfig = {
  slug: string;
  enabled: boolean;
  title: string;
  html: string;
  css: string;
  version: number;
  updated_at: string;
};
export type StorefrontState = { config: StorefrontConfig | null; starter: { html: string; css: string } };
export type StorefrontInput = { slug: string; enabled: boolean; title: string; html: string; css: string };
export type VersionInfo = { version: number; title: string; created_at: string; bytes: number };

export type TemplateInfo = { id: string; code: string; name: string; description: string; category: string; active: boolean; sort_order: number; bytes: number; updated_at: string };
export type TemplateFull = TemplateInfo & { html: string; css: string };

export const storefront = {
  templates: () => api<{ data: TemplateInfo[] }>('/storefront/templates').then((r) => r.data),
  template: (id: string) => api<TemplateFull>(`/storefront/templates/${id}`),
  get: () => api<StorefrontState>('/storefront/'),
  save: (body: StorefrontInput) => api<{ config: StorefrontConfig; warnings: string[] }>('/storefront/', { method: 'PUT', body: JSON.stringify(body) }),
  /** Halaman utuh hasil render (sudah disaring server) untuk iframe srcdoc ber-sandbox. */
  preview: (body: { html: string; css: string; title: string; slug: string }) => api<{ html: string }>('/storefront/preview', { method: 'POST', body: JSON.stringify(body) }),
  versions: () => api<{ data: VersionInfo[] }>('/storefront/versions').then((r) => r.data),
  version: (v: number) => api<StorefrontConfig>(`/storefront/versions/${v}`)
};

/** Alamat publik halaman. Di domain kustom, `API_URL` kosong sehingga memakai origin yang sama. */
export const publicUrl = (slug: string) => `${API_URL || location.origin}/s/${slug}`;
