// Klien API member, level member, dan poin. Uang dikirim/diterima sebagai string desimal; poin = bilangan bulat.
import { api, apiBlob } from '#lib/api/client.ts';
import type { Page } from '#lib/catalog/api.ts';

export type Gender = '' | 'M' | 'F';

export type LevelRef = { id: string | null; name: string; min_points: number; spend_per_point: string; point_value: string };

export type Member = {
  id: string;
  code: string;
  name: string;
  gender: Gender;
  phone: string;
  email: string;
  address: string;
  district: string;
  city: string;
  province: string;
  postal_code: string;
  credit_limit: string;
  due_days: number;
  /** YYYY-MM-DD; null = selalu aktif. */
  valid_until: string | null;
  active: boolean;
  notes: string;
  cover_image_id: string | null;
  points: number;
  lifetime_points: number;
  level: LevelRef;
  next_level: { name: string; min_points: number; points_needed: number } | null;
  stats: { total_sales: string; total_trx: number; points: number; deposit: string };
  created_at: string;
  updated_at: string;
};

export type Row = {
  id: string;
  code: string;
  name: string;
  phone: string;
  city: string;
  active: boolean;
  valid_until: string | null;
  points: number;
  lifetime_points: number;
  level: string;
  cover_image_id: string | null;
};

export type MemberInput = {
  code: string;
  name: string;
  gender: Gender;
  phone: string;
  email: string;
  address: string;
  district: string;
  city: string;
  province: string;
  postal_code: string;
  credit_limit: string;
  due_days: string;
  valid_until: string;
  notes: string;
  active: boolean;
};

export type PointMove = {
  id: number;
  kind: 'EARN' | 'REDEEM' | 'ADJUST' | 'REVERSAL';
  points: number;
  balance_after: number;
  ref_type: 'SALE' | 'MANUAL';
  ref_id: string | null;
  doc_no: string;
  note: string;
  actor: string;
  created_at: string;
};

/** Satu transaksi pada riwayat member. */
export type MemberSale = {
  id: string;
  doc_no: string;
  status: string;
  created_at: string;
  outlet: string;
  cashier: string;
  total: string;
  line_count: number;
  items: string;
  points_earned: number;
  points_redeemed: number;
};

/** Hasil pencarian untuk kasir. */
export type Lookup = {
  id: string;
  code: string;
  name: string;
  phone: string;
  points: number;
  lifetime_points: number;
  level: string;
  cover_image_id: string | null;
  spend_per_point: string;
  point_value: string;
};

export type Level = {
  id: string;
  name: string;
  min_points: number;
  spend_per_point: string;
  point_value: string;
  active: boolean;
  member_count: number;
  created_at: string;
};

export type LevelInput = { name: string; min_points: string; spend_per_point: string; point_value: string };

export type ListQuery = { q?: string; active?: boolean; limit?: number; offset?: number };

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

export const members = {
  list: (p: ListQuery = {}, signal?: AbortSignal) => api<Page<Row>>(`/members/${qs(p)}`, { signal }),
  get: (id: string) => api<Member>(`/members/${id}`),
  create: (input: MemberInput) => api<Member>('/members/', { method: 'POST', body: json(input) }),
  update: (id: string, input: MemberInput) => api<Member>(`/members/${id}`, { method: 'PUT', body: json(input) }),
  setActive: (id: string, active: boolean) => api<Member>(`/members/${id}/active`, { method: 'PUT', body: json({ active }) }),
  points: (id: string, p: { limit?: number; offset?: number } = {}) => api<Page<PointMove>>(`/members/${id}/points${qs(p)}`),
  sales: (id: string, p: { limit?: number; offset?: number } = {}) => api<Page<MemberSale>>(`/members/${id}/sales${qs(p)}`),
  adjust: (id: string, points: number, note: string) =>
    api<Member>(`/members/${id}/points/adjust`, { method: 'POST', body: json({ points, note }) }),
  /** Pencarian cepat untuk kasir (aktif dan belum kedaluwarsa). */
  lookup: (q: string, signal?: AbortSignal) => api<{ data: Lookup[] }>(`/members/lookup?q=${encodeURIComponent(q)}`, { signal }).then((r) => r.data),
  /** Unggah foto cover (multipart). Server memvalidasi isi, mengecilkan, dan mengubahnya menjadi JPG. */
  uploadCover: (id: string, file: File) => {
    const form = new FormData();
    form.append('file', file);
    return api<Member>(`/members/${id}/cover`, { method: 'POST', body: form });
  },
  removeCover: (id: string) => api<Member>(`/members/${id}/cover`, { method: 'DELETE' })
};

export const levels = {
  list: (active?: boolean) => api<{ data: Level[] }>(`/member-levels/${active === undefined ? '' : `?active=${active}`}`).then((r) => r.data),
  create: (input: LevelInput) => api<Level>('/member-levels/', { method: 'POST', body: json(input) }),
  update: (id: string, input: LevelInput) => api<Level>(`/member-levels/${id}`, { method: 'PUT', body: json(input) }),
  setActive: (id: string, active: boolean) => api<Level>(`/member-levels/${id}/active`, { method: 'PUT', body: json({ active }) })
};

// Cache URL objek per foto: id foto cover berubah setiap kali diganti, jadi satu unduhan per id cukup.
const urlCache = new Map<string, Promise<string>>();

export function coverUrl(memberId: string, coverId: string, size: 'thumb' | 'full'): Promise<string> {
  const key = `${coverId}:${size}`;
  let p = urlCache.get(key);
  if (!p) {
    p = apiBlob(`/members/${memberId}/cover?size=${size}`).then((b) => URL.createObjectURL(b));
    p.catch(() => urlCache.delete(key));
    urlCache.set(key, p);
  }
  return p;
}
