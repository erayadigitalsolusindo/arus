import { ApiError } from '#lib/api/client.ts';
import { t, tryT } from './i18n.svelte.ts';

/** Pesan error untuk pengguna: terjemahan berdasarkan `code` API, lalu `message` server, lalu pesan umum. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    return tryT(`errors.${err.code}`) ?? (err.message || t('errors.UNKNOWN'));
  }
  return t('errors.UNKNOWN');
}

/** Pesan untuk kode galat per field dari API (REQUIRED, INVALID, TOO_LONG, TOO_SHORT, WEAK). */
export function fieldMessage(code: string | undefined): string | undefined {
  if (!code) return undefined;
  return tryT(`errors.FIELD_${code}`) ?? t('errors.VALIDATION');
}
