import { requirePlatform } from '#lib/platform/session.svelte.ts';

// Seluruh panel butuh sesi Platform Admin (dipulihkan dari cookie refresh platform atau dialihkan ke login platform).
export async function load() {
  await requirePlatform();
}
