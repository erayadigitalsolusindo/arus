import { requirePermission } from '#lib/auth/session.svelte.ts';
import { outlets } from '#lib/outlets/api.ts';

// Tambah item butuh izin `items.create`; daftar cabang yang boleh diatur dimuat untuk kolom harga per cabang.
export async function load() {
  await requirePermission('items', 'create');
  const res = await outlets.accessible();
  return { outlets: res.outlets.map((o) => ({ id: o.id, name: o.name })) };
}
