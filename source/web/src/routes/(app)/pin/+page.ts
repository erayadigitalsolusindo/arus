import { redirect } from '@sveltejs/kit';
import { can, requireSession } from '#lib/auth/session.svelte.ts';

// Halaman PIN untuk pemegang salah satu izin penyetuju (ubah harga atau pindah outlet).
export const load = async () => {
  await requireSession();
  if (!can('price_override', 'approve') && !can('outlet_switch', 'approve')) redirect(307, '/dashboard');
};
