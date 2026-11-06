/** Login/daftar Google (Google Identity Services). Server yang memverifikasi ID token; di sini hanya memuat skrip dan membaca nama/email untuk prefill. */
import { isCustomDomain } from '#lib/api/client.ts';

export const GOOGLE_CLIENT_ID: string = String(import.meta.env.VITE_GOOGLE_CLIENT_ID ?? '').trim();

/** Domain white label tidak terdaftar sebagai origin OAuth Google, jadi tombol hanya di host utama. */
export const googleEnabled: boolean = GOOGLE_CLIENT_ID !== '' && !isCustomDomain;

interface GoogleId {
  initialize(cfg: { client_id: string; callback: (r: { credential?: string }) => void; ux_mode?: string }): void;
  renderButton(el: HTMLElement, opts: Record<string, unknown>): void;
}
declare global {
  interface Window {
    google?: { accounts: { id: GoogleId } };
  }
}

let loading: Promise<GoogleId> | null = null;

export function loadGoogle(): Promise<GoogleId> {
  if (window.google) return Promise.resolve(window.google.accounts.id);
  loading ??= new Promise<GoogleId>((resolve, reject) => {
    const s = document.createElement('script');
    s.src = 'https://accounts.google.com/gsi/client';
    s.async = true;
    s.onload = () => (window.google ? resolve(window.google.accounts.id) : reject(new Error('gsi')));
    s.onerror = () => {
      loading = null;
      reject(new Error('gsi'));
    };
    document.head.appendChild(s);
  });
  return loading;
}

/** Kunci sessionStorage: kredensial Google dari halaman login dibawa ke /register bila akunnya belum terdaftar. */
export const PENDING_KEY = 'arus.google.signup';

export interface GoogleProfile {
  email: string;
  name: string;
}

/** Membaca klaim untuk prefill form (BUKAN verifikasi; server memverifikasi ulang tanda tangannya). */
export function decodeCredential(credential: string): GoogleProfile | null {
  try {
    const b64 = credential.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
    const json = new TextDecoder().decode(Uint8Array.from(atob(b64), (c) => c.charCodeAt(0)));
    const p = JSON.parse(json) as { email?: string; name?: string };
    return p.email ? { email: p.email, name: p.name ?? '' } : null;
  } catch {
    return null;
  }
}
