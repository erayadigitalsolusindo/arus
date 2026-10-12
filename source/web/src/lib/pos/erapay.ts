// Klien API pembayaran QRIS EraPay di kasir. Nota hanya bisa disimpan setelah charge lunas (server memeriksa ulang saat simpan).
import { api } from '#lib/api/client.ts';

export type EraPayConfig = {
  /** Kasir bisa memakai QRIS (server siap + key terpasang + metode aktif). */
  enabled: boolean;
  method_id?: string;
  /** Alamat EraPay sudah diatur di server. */
  server_ready: boolean;
  /** Toko ini sudah memasang API key. */
  configured: boolean;
  key_hint?: string;
  environment?: 'sandbox' | 'production';
};
export type EraPayStatus = 'pending' | 'succeeded' | 'failed' | 'expired' | 'cancelled' | 'unknown';
export type EraPayCharge = { id: string; status: EraPayStatus; amount: string; qr_value?: string; expires_at?: string };

export const erapay = {
  config: () => api<EraPayConfig>('/payments/erapay/config'),
  /** Pasang/ganti API key EraPay toko ini (diverifikasi ke EraPay) dan siapkan metode "EraPay QRIS". */
  saveKey: (apiKey: string) => api<EraPayConfig>('/payments/erapay/key', { method: 'PUT', body: JSON.stringify({ api_key: apiKey }) }),
  removeKey: () => api<void>('/payments/erapay/key', { method: 'DELETE' }),
  /** Buat tagihan QRIS; kunci yang sama mengembalikan tagihan yang sama. */
  create: (amount: string, idempotencyKey: string) =>
    api<EraPayCharge>('/payments/erapay/charges', { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify({ amount }) }),
  get: (id: string) => api<EraPayCharge>(`/payments/erapay/charges/${id}`)
};

/** Status akhir yang tidak akan berubah lagi (polling berhenti). */
export const eraFinal = (s: EraPayStatus) => s === 'succeeded' || s === 'failed' || s === 'expired' || s === 'cancelled';
