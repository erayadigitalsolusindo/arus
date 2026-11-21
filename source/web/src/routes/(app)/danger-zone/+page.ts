import { redirect } from '@sveltejs/kit';
import { bootstrap, can, homePath, session } from '#lib/auth/session.svelte.ts';

// Hanya Owner (akses penuh); selain itu dialihkan. Server menegakkan ulang.
export const load = async ({ url }: { url: URL }) => {
  await bootstrap();
  if (session.status !== 'authed') redirect(307, `/login?next=${encodeURIComponent(url.pathname)}`);
  if (!can('danger_zone')) redirect(307, homePath());
};
