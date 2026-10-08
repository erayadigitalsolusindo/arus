import { requireSession } from '#lib/auth/session.svelte.ts';

// Layar kasir butuh sesi; dipisah dari (app) karena tampil layar penuh tanpa sidebar/header.
export async function load() {
  await requireSession();
}
