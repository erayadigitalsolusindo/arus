// Klien Danger Zone (Owner): hapus permanen data transaksi tenant. Server meminta kata sandi Owner + kata konfirmasi.
import { api } from '#lib/api/client.ts';

export type DangerScope = 'sales' | 'purchasing' | 'payment-methods';

export const danger = {
  clear: (scope: DangerScope, body: { password: string; confirm: string }) =>
    api<{ deleted: Record<string, unknown> }>(`/danger-zone/${scope}`, { method: 'POST', body: JSON.stringify(body) })
};
