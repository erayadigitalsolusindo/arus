import { redirect } from '@sveltejs/kit';
import { homePath, posOnly, requireSession } from '#lib/auth/session.svelte.ts';

// Semua halaman di (app) butuh sesi: dipulihkan dari cookie refresh atau dialihkan ke /login.
// Akun "Hanya Kasir" tidak boleh melihat halaman admin mana pun: diarahkan ke layar kasir.
export async function load() {
  await requireSession();
  if (posOnly()) redirect(307, homePath());
}
