import { redirect } from '@sveltejs/kit';
import { requirePermission } from '#lib/auth/session.svelte.ts';
import { getPosLayout, posPath } from '#lib/pos/layout.ts';

export const load = async ({ url }: { url: URL }) => {
  await requirePermission('sales_orders', 'create');
  // Pengguna memilih tampilan klasik: login berikutnya langsung ke sana tanpa switch.
  if (getPosLayout() === 'classic') redirect(307, posPath('classic') + url.search);
};
