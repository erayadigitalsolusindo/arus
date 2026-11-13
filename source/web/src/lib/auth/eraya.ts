/** Login/daftar SSO Eraya (OIDC, Authorization Code + PKCE). Browser hanya mengarahkan dan menyimpan state; server yang menukar code dan memverifikasi token. */
import { isCustomDomain } from '#lib/api/client.ts';

export const ERAYA_ISSUER: string = String(import.meta.env.VITE_ERAYA_ISSUER ?? '').trim().replace(/\/+$/, '');
export const ERAYA_CLIENT_ID: string = String(import.meta.env.VITE_ERAYA_CLIENT_ID ?? '').trim();

/** Domain kustom tidak punya redirect URI terdaftar di Eraya, jadi tombolnya tidak ditampilkan di sana. */
export const erayaEnabled = !isCustomDomain && ERAYA_ISSUER !== '' && ERAYA_CLIENT_ID !== '';

export const ERAYA_FLOW_KEY = 'arus.eraya.flow';
/** id_token Eraya yang sudah ditukar, menunggu dipakai halaman login/daftar. */
export const ERAYA_PENDING_KEY = 'arus.eraya.pending';

export type ErayaMode = 'login' | 'register';
export interface ErayaFlow {
  verifier: string;
  state: string;
  mode: ErayaMode;
  next: string;
}
export interface ErayaPending {
  token: string;
  mode: ErayaMode;
  /** Datang dari login yang belum terdaftar: form daftar menampilkan pesan "belum terdaftar". */
  fromLogin?: boolean;
}

export const erayaRedirectUri = () => `${location.origin}/auth/eraya/callback`;

const b64url = (bytes: Uint8Array) => btoa(String.fromCharCode(...bytes)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
const random = (n: number) => b64url(crypto.getRandomValues(new Uint8Array(n)));

/** Mengarahkan browser ke halaman masuk Eraya. `next` = tujuan setelah login (sudah lolos safeNext). */
export async function startEraya(mode: ErayaMode, next = ''): Promise<void> {
  const verifier = random(48); // 64 karakter (syarat PKCE: 43–128)
  const state = random(16);
  const challenge = b64url(new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier))));
  sessionStorage.setItem(ERAYA_FLOW_KEY, JSON.stringify({ verifier, state, mode, next } satisfies ErayaFlow));
  const q = new URLSearchParams({
    response_type: 'code',
    client_id: ERAYA_CLIENT_ID,
    redirect_uri: erayaRedirectUri(),
    scope: 'openid profile email',
    state,
    code_challenge: challenge,
    code_challenge_method: 'S256'
  });
  location.assign(`${ERAYA_ISSUER}/oauth/authorize?${q}`);
}

/** Mengambil lalu menghapus id_token yang menunggu untuk mode ini (sekali pakai). */
export function takeErayaPending(mode: ErayaMode): ErayaPending | null {
  try {
    const raw = sessionStorage.getItem(ERAYA_PENDING_KEY);
    if (!raw) return null;
    const p = JSON.parse(raw) as ErayaPending;
    if (p.mode !== mode) return null;
    sessionStorage.removeItem(ERAYA_PENDING_KEY);
    return p;
  } catch {
    return null;
  }
}

export function putErayaPending(p: ErayaPending): void {
  try {
    sessionStorage.setItem(ERAYA_PENDING_KEY, JSON.stringify(p));
  } catch {
    /* sessionStorage diblokir: pengguna mengulang dari awal */
  }
}
