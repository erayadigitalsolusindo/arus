// Klien HTTP tipis untuk API Go. Error API berbentuk { error: { code, message, fields? } } (AGENTS.md §6).
import { i18n } from '#lib/i18n/i18n.svelte.ts';

export const API_URL: string = import.meta.env.VITE_API_URL ?? 'http://localhost:8080';

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    /** Kode galat per field untuk error VALIDATION, mis. { email: 'INVALID' }. */
    public fields: Record<string, string> = {}
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

// Token akses hanya di memori (bukan localStorage) agar tidak bisa dicuri skrip XSS; refresh lewat cookie httpOnly.
let accessToken: string | null = null;
export const setAccessToken = (token: string | null) => (accessToken = token);

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_URL}${path}`, {
      credentials: 'include',
      ...init,
      headers: {
        'Content-Type': 'application/json',
        'Accept-Language': i18n.locale,
        ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
        ...init.headers
      }
    });
  } catch {
    throw new ApiError(0, 'NETWORK', 'Network error');
  }

  const body = await res.json().catch(() => null);
  if (!res.ok) {
    // `message` hanya cadangan; teks untuk pengguna diterjemahkan lewat `errorMessage()` (#lib/i18n/errors.ts).
    throw new ApiError(res.status, body?.error?.code ?? 'UNKNOWN', body?.error?.message ?? `HTTP ${res.status}`, body?.error?.fields ?? {});
  }
  return body as T;
}
