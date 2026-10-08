// Klien API kupon belanja global. Uang sebagai string desimal; tanggal "YYYY-MM-DD" (zona waktu outlet saat dipakai).
import { api } from '#lib/api/client.ts';

export type VoucherKind = 'percent' | 'amount';

export type Voucher = {
  id: string;
  code: string;
  name: string;
  kind: VoucherKind;
  value: string;
  /** Batas potongan (hanya kupon persen); '' = tanpa batas. */
  max_discount: string;
  min_spend: string;
  starts_on: string;
  ends_on: string;
  /** null = tak terbatas. */
  max_uses: number | null;
  used_count: number;
  active: boolean;
  created_at: string;
};

export type VoucherInput = {
  code: string;
  name: string;
  kind: VoucherKind;
  value: string;
  max_discount?: string;
  min_spend?: string;
  starts_on?: string;
  ends_on?: string;
  max_uses?: number;
};

export type VoucherQuery = { q?: string; active?: boolean; limit?: number; offset?: number };
export type VoucherPage = { data: Voucher[]; total: number };

function qs(p: VoucherQuery): string {
  const s = new URLSearchParams();
  if (p.q) s.set('q', p.q);
  if (p.active !== undefined) s.set('active', String(p.active));
  if (p.limit) s.set('limit', String(p.limit));
  if (p.offset) s.set('offset', String(p.offset));
  const out = s.toString();
  return out ? `?${out}` : '';
}

export const vouchers = {
  list: (p: VoucherQuery = {}, signal?: AbortSignal) => api<VoucherPage>(`/vouchers/${qs(p)}`, { signal }),
  create: (input: VoucherInput) => api<Voucher>('/vouchers/', { method: 'POST', body: JSON.stringify(input) }),
  update: (id: string, input: VoucherInput) => api<Voucher>(`/vouchers/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
  setActive: (id: string, active: boolean) => api<Voucher>(`/vouchers/${id}/active`, { method: 'PUT', body: JSON.stringify({ active }) })
};

export type VoucherState = 'inactive' | 'upcoming' | 'expired' | 'exhausted' | 'running';

/** Status tampilan kupon pada tanggal `today` (YYYY-MM-DD). Server tetap yang memutuskan saat kupon dipakai. */
export function voucherState(v: Voucher, today: string): VoucherState {
  if (!v.active) return 'inactive';
  if (v.starts_on && today < v.starts_on) return 'upcoming';
  if (v.ends_on && today > v.ends_on) return 'expired';
  if (v.max_uses !== null && v.used_count >= v.max_uses) return 'exhausted';
  return 'running';
}
