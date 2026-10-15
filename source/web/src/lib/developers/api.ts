// Klien portal dokumentasi: profil white label + katalog endpoint (publik, tanpa login).
import { API_URL } from '#lib/api/client.ts';

export type Brand = {
  slug: string;
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

export type SpecInfo = {
  algorithm: string;
  prefix: string;
  headers: { key: string; timestamp: string; nonce: string; signature: string };
  skew_seconds: number;
  max_body_bytes: number;
  nonce_hint: string;
  blocked_prefixes: string[];
  default_rate_per_min: number;
};

export type Param = { name: string; in: 'path' | 'query' | 'header' | 'body'; type?: string; required?: boolean; description?: string };
export type Example = { status?: number; description?: string; body?: unknown };
export type ErrorRef = { status: number; code: string; description: string };
export type PermRef = { module: string; action: string };

export type Endpoint = {
  method: string;
  path: string;
  group: string;
  summary: string;
  description?: string;
  auth: 'hmac' | 'jwt';
  permissions?: PermRef[];
  params?: Param[];
  request?: Example;
  responses?: Example[];
  errors?: ErrorRef[];
  notes?: string[];
  documented: boolean;
};

export type Group = { id: string; title: string; description?: string; count: number; documented: number };
export type Catalog = { groups: Group[]; endpoints: Endpoint[] };

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, { headers: { Accept: 'application/json' } });
  if (!res.ok) throw new Error(String(res.status));
  return (await res.json()) as T;
}

export async function loadPortal(host: string): Promise<{ brand: Brand; spec: SpecInfo; catalog: Catalog }> {
  const q = new URLSearchParams({ host });
  const [cfg, catalog] = await Promise.all([
    get<{ brand: Brand; spec: SpecInfo }>(`/public/api-docs/config?${q}`),
    get<Catalog>(`/public/api-docs/catalog?${q}`)
  ]);
  return { ...cfg, catalog };
}

/** Alamat dasar API yang ditampilkan: profil white label, atau alamat API aplikasi ini. */
export function baseUrlOf(brand: Brand): string {
  return (brand.base_url || API_URL).replace(/\/$/, '');
}
