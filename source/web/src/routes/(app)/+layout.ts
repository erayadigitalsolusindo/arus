import { requireSession } from '#lib/auth/session.svelte.ts';

// Semua halaman di (app) butuh sesi: dipulihkan dari cookie refresh atau dialihkan ke /login.
export async function load() {
  await requireSession();
}
