import { ApiError } from '#lib/api/client.ts';
import { t, tryT } from './i18n.svelte.ts';

/** Pesan error untuk pengguna: terjemahan berdasarkan `code` API, lalu `message` server, lalu pesan umum. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    // ACCOUNT_LOCKED memuat {count} menit: isi dari retryAfter (detik) agar tidak tampil mentah.
    const params = err.code === 'ACCOUNT_LOCKED' ? { count: Math.max(1, Math.ceil(err.retryAfter / 60)) } : undefined;
    return tryT(`errors.${err.code}`, params) ?? (err.message || t('errors.UNKNOWN'));
  }
  return t('errors.UNKNOWN');
}

/** Pesan untuk kode galat per field dari API (REQUIRED, INVALID, TOO_LONG, TOO_SHORT, WEAK). */
export function fieldMessage(code: string | undefined): string | undefined {
  if (!code) return undefined;
  return tryT(`errors.FIELD_${code}`) ?? t('errors.VALIDATION');
}
