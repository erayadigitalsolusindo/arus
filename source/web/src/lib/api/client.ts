// Klien HTTP tipis untuk API Go. Error API berbentuk { error: { code, message } } (AGENTS.md §6).

export const API_URL: string = import.meta.env.VITE_API_URL ?? 'http://localhost:8080';

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_URL}${path}`, {
      credentials: 'include',
      ...init,
      headers: { 'Content-Type': 'application/json', ...init.headers }
    });
  } catch {
    throw new ApiError(0, 'NETWORK', 'Tidak dapat terhubung ke server.');
  }

  const body = await res.json().catch(() => null);
  if (!res.ok) {
    throw new ApiError(
      res.status,
      body?.error?.code ?? 'UNKNOWN',
      body?.error?.message ?? `Permintaan gagal (${res.status}).`
    );
  }
  return body as T;
}
