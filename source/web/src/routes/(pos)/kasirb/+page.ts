import { redirect } from '@sveltejs/kit';
import { requirePermission } from '#lib/auth/session.svelte.ts';
import { getPosLayout, posPath } from '#lib/pos/layout.ts';

export const load = async ({ url }: { url: URL }) => {
  await requirePermission('sales_orders', 'create');
  if (getPosLayout() === 'modern') redirect(307, posPath('modern') + url.search);
};
