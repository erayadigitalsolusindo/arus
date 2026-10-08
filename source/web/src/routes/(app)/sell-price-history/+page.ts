import { requirePermission } from '#lib/auth/session.svelte.ts';

export const load = () => requirePermission('sell_price_history');
