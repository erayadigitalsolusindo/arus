import { redirect } from '@sveltejs/kit';

// Alias tunggal: /developer[/...] -> /developers[/...] (portal dokumentasi).
export function load({ params }: { params: { path?: string } }) {
  redirect(307, params.path ? `/developers/${params.path}` : '/developers');
}
