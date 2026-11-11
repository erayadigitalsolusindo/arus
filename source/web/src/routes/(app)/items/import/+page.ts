import { requirePermission } from '#lib/auth/session.svelte.ts';

// Impor barang menyimpan data baru: butuh izin `items.create`.
export const load = () => requirePermission('items', 'create');
