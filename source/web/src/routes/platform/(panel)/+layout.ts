import { requirePlatform } from '#lib/platform/session.svelte.ts';

// Seluruh panel butuh sesi Platform Admin (dipulihkan dari cookie refresh platform atau dialihkan ke login platform)
// dan 2FA aktif (sebelum itu hanya halaman keamanan).
export async function load({ url }) {
  await requirePlatform(url.pathname);
}
