// Klien API Langganan tenant: paket aktif, pemakaian hari ini, dan pembelian paket via QRIS EraPay milik platform.
import { api } from '#lib/api/client.ts';

export type EffectivePlan = {
  plan_id: string;
  code: string;
  name: string;
  /** null = tanpa batas. */
  max_tx_per_day: number | null;
  max_users: number | null;
  /** Menu yang termasuk paket; null = semua menu. */
  modules: string[] | null;
  /** null = tanpa kedaluwarsa. */
  expires_at: string | null;
  is_default: boolean;
};
export type PlanInfo = {
  id: string;
  code: string;
  name: string;
  description: string;
  price: string;
  duration_days: number | null;
  max_tx_per_day: number | null;
  max_users: number | null;
  /** Menu yang termasuk paket; null = semua menu. */
  modules: string[] | null;
  /** Poin keunggulan bebas yang ditulis admin. */
  highlights: string[];
};
export type SubscriptionStatus = {
  current: EffectivePlan;
  usage: { tx_today: number; users: number };
  plans: PlanInfo[];
  payments_enabled: boolean;
};
export type PayStatus = 'pending' | 'succeeded' | 'failed' | 'expired' | 'cancelled' | 'unknown';
export type PlanPayment = {
  id: string;
  plan_name: string;
  status: PayStatus;
  amount: string;
  qr_value?: string;
  expires_at?: string;
  /** Langganan sudah aktif dari pembayaran ini. */
  applied: boolean;
  subscription_ends?: string;
};

export const subscription = {
  status: () => api<SubscriptionStatus>('/subscription'),
  /** Buat tagihan QRIS untuk paket; kunci yang sama mengembalikan tagihan yang sama. */
  checkout: (planId: string, idempotencyKey: string) =>
    api<PlanPayment>('/subscription/checkout', { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify({ plan_id: planId }) }),
  payment: (id: string) => api<PlanPayment>(`/subscription/payments/${id}`)
};

/** Status akhir yang tidak berubah lagi (polling berhenti). */
export const payFinal = (s: PayStatus) => s === 'succeeded' || s === 'failed' || s === 'expired' || s === 'cancelled';
