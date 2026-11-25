import { redirect } from '@sveltejs/kit';
import { requirePlatform } from '#lib/platform/session.svelte.ts';
import { firstAllowedHref, pathAllowed } from '#lib/platform/nav.ts';

// Seluruh panel butuh sesi Platform Admin (dipulihkan dari cookie refresh platform atau dialihkan ke login platform)
// dan 2FA aktif (sebelum itu hanya halaman keamanan). Halaman di luar izin role dialihkan ke halaman pertama yang boleh
// (penyaringan tampilan saja; server menolak 403 untuk panggilan API di luar izin).
export async function load({ url }) {
  await requirePlatform(url.pathname);
  if (!pathAllowed(url.pathname)) redirect(307, firstAllowedHref());
}
