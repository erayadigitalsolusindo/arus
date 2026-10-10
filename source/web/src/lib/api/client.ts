// Klien HTTP tipis untuk API Go. Error API berbentuk { error: { code, message, fields? } } (AGENTS.md §6).
import { i18n } from '#lib/i18n/i18n.svelte.ts';

export const API_URL: string = import.meta.env.VITE_API_URL ?? 'http://localhost:8080';

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    /** Kode galat per field untuk error VALIDATION, mis. { email: 'INVALID' }. */
    public fields: Record<string, string> = {},
    /** Detik sampai boleh mencoba lagi (RATE_LIMITED, ACCOUNT_LOCKED). */
    public retryAfter = 0,
    /** Sisa percobaan login gagal sebelum akun dikunci (INVALID_CREDENTIALS); 0 = tidak diketahui. */
    public attemptsLeft = 0
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export type Identity = { id: string; code?: string; name: string; email?: string };

/** Izin efektif: { "*": true } (Owner) atau { modul: [aksi, ...] }. Hanya untuk UI; penegakan di server. */
export type Permissions = { "*"?: true } & Record<string, string[] | true | undefined>;

/** Respons /auth/register, /auth/login, dan /auth/refresh. */
export type AuthResponse = {
  access_token: string;
  expires_in: number;
  user: Identity;
  tenant: Identity;
  outlet: Identity;
  permissions: Permissions;
  email_verified: boolean;
  /** true pada sesi "masuk sebagai" Platform Admin (hanya-baca, tanpa refresh). */
  impersonating?: boolean;
};

// Header kustom wajib di endpoint ber-cookie (refresh/logout): memaksa preflight CORS (perlindungan CSRF).
export const CSRF_HEADERS = { 'X-Requested-With': 'aciraba' } as const;

// Token akses hanya di memori (bukan localStorage) agar tidak bisa dicuri skrip XSS; refresh lewat cookie httpOnly.
let accessToken: string | null = null;
export const setAccessToken = (token: string | null) => (accessToken = token);

// Penyimpan sesi mendaftar di sini agar klien tidak bergantung pada store (hindari impor melingkar).
let onAuthChange: (res: AuthResponse | null) => void = () => {};
export const setAuthListener = (fn: typeof onAuthChange) => (onAuthChange = fn);

/** Satu permintaan HTTP. `token` bawaan = token akses tenant di memori; sesi Platform Admin memakai tokennya sendiri. */
export async function request<T>(path: string, init: RequestInit, token: string | null = accessToken): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_URL}${path}`, {
      credentials: 'include',
      ...init,
      headers: {
        // FormData: biarkan browser menetapkan Content-Type multipart beserta boundary-nya.
        ...(init.body instanceof FormData ? {} : { 'Content-Type': 'application/json' }),
        'Accept-Language': i18n.locale,
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...init.headers
      }
    });
  } catch {
    throw new ApiError(0, 'NETWORK', 'Network error');
  }

  const body = await res.json().catch(() => null);
  if (!res.ok) {
    // `message` hanya cadangan; teks untuk pengguna diterjemahkan lewat `errorMessage()` (#lib/i18n/errors.ts).
    throw new ApiError(res.status, body?.error?.code ?? 'UNKNOWN', body?.error?.message ?? `HTTP ${res.status}`, body?.error?.fields ?? {}, body?.error?.retry_after ?? 0, body?.error?.attempts_left ?? 0);
  }
  return body as T;
}

// Single-flight: semua permintaan yang gagal 401 bersamaan menunggu satu panggilan refresh yang sama,
// sehingga refresh token hanya dirotasi sekali.
let refreshing: Promise<AuthResponse | null> | null = null;

/**
 * Menukar cookie refresh dengan token akses baru. Mengembalikan null bila sesi memang berakhir (401/403);
 * error jaringan/server dilempar agar sesi yang masih sah tidak ikut terhapus.
 */
export function refreshSession(): Promise<AuthResponse | null> {
  refreshing ??= doRefresh().finally(() => (refreshing = null));
  return refreshing;
}

async function doRefresh(): Promise<AuthResponse | null> {
  try {
    const res = await request<AuthResponse>('/auth/refresh', { method: 'POST', headers: CSRF_HEADERS });
    accessToken = res.access_token;
    onAuthChange(res);
    return res;
  } catch (err) {
    if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
      accessToken = null;
      onAuthChange(null);
      return null;
    }
    throw err;
  }
}

// Mode "masuk sebagai" (Platform Admin, hanya-baca): token tenant tidak punya cookie refresh. Bila token itu ditolak
// (kedaluwarsa/admin dinonaktifkan), mode diakhiri — bukan di-refresh, agar tidak diam-diam berganti ke sesi tenant asli.
let impersonating = false;
export const setImpersonating = (on: boolean) => (impersonating = on);
let onImpersonationEnd: () => void = () => {};
export const setImpersonationEndListener = (fn: () => void) => (onImpersonationEnd = fn);

// Endpoint yang menerbitkan/mencabut sesi tidak boleh memicu refresh (mencegah loop). /auth/me tetap ikut.
const NO_RETRY = new Set(['/auth/login', '/auth/register', '/auth/refresh', '/auth/logout']);

/** Permintaan yang mengembalikan berkas (mis. gambar) sebagai Blob; error API tetap dilempar sebagai ApiError. */
async function requestBlob(path: string, token: string | null = accessToken): Promise<Blob> {
  let res: Response;
  try {
    res = await fetch(`${API_URL}${path}`, {
      credentials: 'include',
      headers: { 'Accept-Language': i18n.locale, ...(token ? { Authorization: `Bearer ${token}` } : {}) }
    });
  } catch {
    throw new ApiError(0, 'NETWORK', 'Network error');
  }
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new ApiError(res.status, body?.error?.code ?? 'UNKNOWN', body?.error?.message ?? `HTTP ${res.status}`);
  }
  return res.blob();
}

// Menjalankan `run`; bila token akses kedaluwarsa (401) refresh sekali lalu ulangi. Endpoint penerbit sesi dikecualikan.
async function withRefresh<T>(path: string, run: () => Promise<T>): Promise<T> {
  const sentWith = accessToken;
  try {
    return await run();
  } catch (err) {
    if (!(err instanceof ApiError) || err.status !== 401 || sentWith === null || NO_RETRY.has(path)) throw err;
    if (impersonating) {
      onImpersonationEnd();
      throw err;
    }
    // Bila permintaan lain sudah menyegarkan token, cukup ulangi dengan token yang baru.
    if (accessToken === sentWith && !(await refreshSession())) throw err;
    return run();
  }
}

/** Aliran SSE yang butuh autentikasi (EventSource tidak bisa mengirim header Authorization, jadi memakai fetch). */
async function requestStream(path: string, signal: AbortSignal, token: string | null = accessToken): Promise<Response> {
  let res: Response;
  try {
    res = await fetch(`${API_URL}${path}`, {
      credentials: 'include',
      signal,
      headers: { Accept: 'text/event-stream', 'Accept-Language': i18n.locale, ...(token ? { Authorization: `Bearer ${token}` } : {}) }
    });
  } catch (err) {
    if (signal.aborted) throw err;
    throw new ApiError(0, 'NETWORK', 'Network error');
  }
  if (!res.ok || !res.body) {
    const body = await res.json().catch(() => null);
    throw new ApiError(res.status, body?.error?.code ?? 'UNKNOWN', body?.error?.message ?? `HTTP ${res.status}`);
  }
  return res;
}

export const apiStream = (path: string, signal: AbortSignal): Promise<Response> => withRefresh(path, () => requestStream(path, signal));

export const api = <T>(path: string, init: RequestInit = {}): Promise<T> => withRefresh(path, () => request<T>(path, init));

/** Mengunduh berkas yang butuh autentikasi (token ada di header, bukan cookie, jadi <img src> langsung tidak bisa). */
export const apiBlob = (path: string): Promise<Blob> => withRefresh(path, () => requestBlob(path));
