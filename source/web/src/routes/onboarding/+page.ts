import { redirect } from '@sveltejs/kit';
import { homePath, posOnly, requireSession } from '#lib/auth/session.svelte.ts';

// Wizard hanya untuk pengguna masuk; akun "Hanya Kasir" tidak ikut.
export async function load() {
  await requireSession();
  if (posOnly()) redirect(307, homePath());
}
