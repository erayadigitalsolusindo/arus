// Persetujuan Owner/Supervisor lewat PIN (ubah harga di kasir). PIN tidak pernah disimpan di klien selain di memori
// halaman kasir sampai nota selesai; server memverifikasi ulang saat nota disimpan.
import { api } from '#lib/api/client.ts';

export type Approver = { id: string; name: string };

export const approvals = {
  /** Tanpa argumen = penyetuju ubah harga di outlet aktif; `outlet_switch` + outlet tujuan = penyetuju pindah outlet. */
  approvers: (purpose?: 'outlet_switch', outletId?: string) => {
    const q = purpose ? `?for=${purpose}${outletId ? `&outlet_id=${encodeURIComponent(outletId)}` : ''}` : '';
    return api<{ approvers: Approver[] }>(`/approvals/approvers${q}`).then((r) => r.approvers);
  },
  /** Memeriksa PIN lebih dulu (hitung sebagai percobaan; 5 salah = terkunci 15 menit). */
  check: (userId: string, pin: string) => api<Approver>('/approvals/check', { method: 'POST', body: JSON.stringify({ user_id: userId, pin }) }),
  pinStatus: () => api<{ has_pin: boolean; can_approve: boolean }>('/approvals/pin'),
  setPin: (password: string, pin: string) => api<{ has_pin: boolean }>('/approvals/pin', { method: 'PUT', body: JSON.stringify({ password, pin }) })
};
