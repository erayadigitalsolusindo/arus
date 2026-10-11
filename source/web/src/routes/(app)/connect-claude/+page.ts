import { redirect } from '@sveltejs/kit';
import { bootstrap, can, homePath, session } from '#lib/auth/session.svelte.ts';

// Halaman persetujuan connector Claude: pengguna belum masuk → login dulu lalu kembali ke sini (parameter OAuth ikut terbawa).
export const load = async ({ url }: { url: URL }) => {
  await bootstrap();
  if (session.status !== 'authed') redirect(307, `/login?next=${encodeURIComponent(url.pathname + url.search)}`);
  if (!can('ai_assistant')) redirect(307, homePath());
};
