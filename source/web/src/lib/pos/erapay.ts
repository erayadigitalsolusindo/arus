// Klien API pembayaran QRIS EraPay di kasir. Nota hanya bisa disimpan setelah charge lunas (server memeriksa ulang saat simpan).
import { api } from '#lib/api/client.ts';

export type EraPayConfig = { enabled: boolean; method_id?: string; server_ready: boolean };
export type EraPayStatus = 'pending' | 'succeeded' | 'failed' | 'expired' | 'cancelled' | 'unknown';
export type EraPayCharge = { id: string; status: EraPayStatus; amount: string; qr_value?: string; expires_at?: string };

export const erapay = {
  config: () => api<EraPayConfig>('/payments/erapay/config'),
  /** Menyiapkan metode "EraPay QRIS" untuk tenant (izin Metode Pembayaran). */
  setup: () => api<EraPayConfig>('/payments/erapay/setup', { method: 'POST', body: '{}' }),
  /** Buat tagihan QRIS; kunci yang sama mengembalikan tagihan yang sama. */
  create: (amount: string, idempotencyKey: string) =>
    api<EraPayCharge>('/payments/erapay/charges', { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify({ amount }) }),
  get: (id: string) => api<EraPayCharge>(`/payments/erapay/charges/${id}`)
};

/** Status akhir yang tidak akan berubah lagi (polling berhenti). */
export const eraFinal = (s: EraPayStatus) => s === 'succeeded' || s === 'failed' || s === 'expired' || s === 'cancelled';
