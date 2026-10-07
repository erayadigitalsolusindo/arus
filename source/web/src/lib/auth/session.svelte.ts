// Status sesi di sisi klien: identitas (hanya untuk tampilan), pemulihan sesi saat load, refresh proaktif, logout.
// Otorisasi sebenarnya selalu di server (token akses/RLS); store ini bukan sumber kebenaran.
import { redirect } from '@sveltejs/kit';
import { goto } from '$app/navigation';
import { api, refreshSession, setAccessToken, setAuthListener, CSRF_HEADERS, type AuthResponse, type Identity, type Permissions } from '#lib/api/client.ts';

type Status = 'unknown' | 'authed' | 'anon';

class SessionState {
  status = $state<Status>('unknown');
  user = $state<Identity | null>(null);
  tenant = $state<Identity | null>(null);
  outlet = $state<Identity | null>(null);
  permissions = $state<Permissions>({});
  emailVerified = $state(true); // true sebagai bawaan agar banner tidak berkedip sebelum sesi dimuat
}

export const session = new SessionState();

/** Refresh dijadwalkan sekian detik sebelum token akses habis. */
const REFRESH_LEAD_S = 60;

let timer: ReturnType<typeof setTimeout> | undefined;
let expiresAt = 0;

function apply(res: AuthResponse | null) {
  clearTimeout(timer);
  if (!res) {
    session.status = 'anon';
    session.user = session.tenant = session.outlet = null;
    session.permissions = {};
    session.emailVerified = true;
    return;
  }
  session.status = 'authed';
  session.user = res.user;
  session.tenant = res.tenant;
  session.outlet = res.outlet;
  session.permissions = res.permissions ?? {};
  session.emailVerified = res.email_verified ?? true;
  expiresAt = Date.now() + res.expires_in * 1000;
  timer = setTimeout(() => void refreshSession().catch(() => {}), Math.max(res.expires_in - REFRESH_LEAD_S, 5) * 1000);
}

setAuthListener(apply);

/** Apakah pengguna punya izin `module.action` (hanya untuk menyaring UI; server tetap menegakkan). */
export function can(module: string, action = 'view'): boolean {
  const p = session.permissions;
  if (p["*"] === true) return true;
  const acts = p[module];
  return Array.isArray(acts) && acts.includes(action);
}

/** Menandai email terverifikasi di UI (dipanggil halaman verifikasi bila pengguna sedang masuk). */
export function markEmailVerified() {
  session.emailVerified = true;
}

/** Pindah outlet: token akses baru dengan outlet baru; cookie refresh tidak berubah. */
export async function switchOutlet(outletId: string) {
  const res = await api<AuthResponse>('/auth/switch-outlet', { method: 'POST', headers: CSRF_HEADERS, body: JSON.stringify({ outlet_id: outletId }) });
  setAccessToken(res.access_token);
  apply(res);
}

/** Dipanggil setelah login/register berhasil. */
export function startSession(res: AuthResponse) {
  setAccessToken(res.access_token);
  apply(res);
}

let booting: Promise<void> | null = null;

/** Memulihkan sesi dari cookie refresh (sekali per pemuatan halaman). Error jaringan membiarkan status `unknown`. */
export function bootstrap(): Promise<void> {
  if (session.status !== 'unknown') return Promise.resolve();
  booting ??= refreshSession()
    .then(() => {})
    .catch(() => {})
    .finally(() => (booting = null));
  return booting;
}

/** Penjaga route publik (login/register): pengguna yang sudah masuk diarahkan ke dasbor. */
export async function guestOnly() {
  await bootstrap();
  if (session.status === 'authed') redirect(307, '/dashboard');
}

/** Penjaga halaman yang butuh izin lihat suatu modul: tanpa izin diarahkan ke dasbor. */
export async function requirePermission(module: string, action = 'view') {
  await requireSession();
  if (!can(module, action)) redirect(307, '/dashboard');
}

/** Penjaga route aplikasi: pengguna yang belum masuk diarahkan ke login. */
export async function requireSession() {
  await bootstrap();
  if (session.status !== 'authed') redirect(307, '/login');
}

const channel = typeof BroadcastChannel === 'undefined' ? null : new BroadcastChannel('aciraba-auth');

function dropSession() {
  setAccessToken(null);
  apply(null);
}

export async function logout() {
  try {
    await api('/auth/logout', { method: 'POST', headers: CSRF_HEADERS });
  } catch {
    // Server tak terjangkau: sesi lokal tetap dibuang; cookie refresh habis sendiri.
  }
  dropSession();
  channel?.postMessage('logout');
  await goto('/login');
}

// Tab lain keluar → tab ini ikut keluar (layout (app) mengarahkan ke /login saat status jadi anon).
if (channel) channel.onmessage = (e) => e.data === 'logout' && dropSession();

// Timer ditahan di tab latar belakang; segarkan segera saat tab kembali terlihat dan token hampir habis.
if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible' && session.status === 'authed' && expiresAt - Date.now() < REFRESH_LEAD_S * 1000) {
      void refreshSession().catch(() => {});
    }
  });
}
