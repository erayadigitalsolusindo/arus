import { redirect } from '@sveltejs/kit';
import { bootstrap, homePath, session } from '#lib/auth/session.svelte.ts';
import { isCustomDomain } from '#lib/api/client.ts';

// Sudah login: ke beranda aplikasi. Belum: tampilkan landing ARUS (berkas statis, bukan route SPA). Domain kustom (white
// label) tidak menampilkan landing ARUS: langsung ke login; landing milik tenant dilayani edge di root domain bila dinyalakan.
export async function load() {
  await bootstrap();
  if (session.status === 'authed') redirect(307, homePath());
  if (isCustomDomain) redirect(307, '/login');
  window.location.replace('/landing/');
  return new Promise<never>(() => {});
}
