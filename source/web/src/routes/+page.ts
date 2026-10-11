import { redirect } from '@sveltejs/kit';
import { bootstrap, homePath, session } from '#lib/auth/session.svelte.ts';

// Sudah login: ke beranda aplikasi. Belum: tampilkan landing (berkas statis, bukan route SPA).
export async function load() {
  await bootstrap();
  if (session.status === 'authed') redirect(307, homePath());
  window.location.replace('/landing/');
  return new Promise<never>(() => {});
}
