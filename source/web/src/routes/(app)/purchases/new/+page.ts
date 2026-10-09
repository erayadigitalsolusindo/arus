import { requirePermission } from '#lib/auth/session.svelte.ts';

export const load = () => requirePermission('purchase_invoices', 'create');
