// Sesi Platform Admin (operator ACIRABA): terpisah dari sesi tenant. Token akses hanya di memori; refresh lewat cookie
// httpOnly `platform_refresh` (Path /platform/auth). Otorisasi sebenarnya di server.
import { redirect } from '@sveltejs/kit';
import { goto } from '$app/navigation';
import { ApiError, CSRF_HEADERS, request } from '#lib/api/client.ts';

export type PlatformAdmin = { id: string; name: string; email: string; mfa_enabled: boolean };
export type PlatformAuth = { access_token: string; expires_in: number; admin: PlatformAdmin };
type Status = 'unknown' | 'authed' | 'anon';

class PlatformState {
  status = $state<Status>('unknown');
  admin = $state<PlatformAdmin | null>(null);
}
export const platform = new PlatformState();

const REFRESH_LEAD_S = 60;
let token: string | null = null;
let timer: ReturnType<typeof setTimeout> | undefined;

function apply(res: PlatformAuth | null) {
  clearTimeout(timer);
  token = res?.access_token ?? null;
  platform.status = res ? 'authed' : 'anon';
  platform.admin = res?.admin ?? null;
  if (res) timer = setTimeout(() => void refresh().catch(() => {}), Math.max(res.expires_in - REFRESH_LEAD_S, 5) * 1000);
}

let refreshing: Promise<PlatformAuth | null> | null = null;

/** Menukar cookie refresh dengan token baru; null bila sesi memang berakhir, galat jaringan dilempar. */
export function refresh(): Promise<PlatformAuth | null> {
  refreshing ??= (async () => {
    try {
      const res = await request<PlatformAuth>('/platform/auth/refresh', { method: 'POST', headers: CSRF_HEADERS }, null);
      apply(res);
      return res;
    } catch (err) {
      if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
        apply(null);
        return null;
      }
      throw err;
    }
  })().finally(() => (refreshing = null));
  return refreshing;
}

/** Panggilan API platform dengan token platform; satu kali refresh bila 401. */
export async function papi<T>(path: string, init: RequestInit = {}): Promise<T> {
  const sent = token;
  try {
    return await request<T>(path, init, token);
  } catch (err) {
    if (!(err instanceof ApiError) || err.status !== 401 || sent === null) throw err;
    if (token === sent && !(await refresh())) throw err;
    return request<T>(path, init, token);
  }
}

export function startPlatformSession(res: PlatformAuth) {
  apply(res);
}

/** Membuang sesi lokal (server sudah mencabutnya, mis. setelah 2FA dinonaktifkan). */
export function endPlatformSession() {
  apply(null);
}

let booting: Promise<void> | null = null;
export function bootstrap(): Promise<void> {
  if (platform.status !== 'unknown') return Promise.resolve();
  booting ??= refresh()
    .then(() => {})
    .catch(() => {})
    .finally(() => (booting = null));
  return booting;
}

/**
 * Penjaga halaman panel: tanpa sesi platform diarahkan ke login platform; admin yang belum mengaktifkan 2FA hanya boleh
 * ke halaman keamanan (server juga menegakkan: MFA_ENROLL_REQUIRED).
 */
export async function requirePlatform(pathname = '') {
  await bootstrap();
  if (platform.status !== 'authed') redirect(307, '/platform/login');
  if (platform.admin && !platform.admin.mfa_enabled && pathname !== '/platform/security') redirect(307, '/platform/security');
}

/** Penjaga halaman login/setup: yang sudah masuk diarahkan ke panel. */
export async function platformGuestOnly() {
  await bootstrap();
  if (platform.status === 'authed') redirect(307, '/platform/tenants');
}

export async function platformLogout() {
  try {
    await request('/platform/auth/logout', { method: 'POST', headers: CSRF_HEADERS }, null);
  } catch {
    // Server tak terjangkau: sesi lokal tetap dibuang; cookie refresh habis sendiri.
  }
  apply(null);
  await goto('/platform/login');
}
