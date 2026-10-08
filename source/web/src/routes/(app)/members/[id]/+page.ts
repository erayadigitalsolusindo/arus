import { error } from '@sveltejs/kit';
import { requirePermission } from '#lib/auth/session.svelte.ts';
import { ApiError } from '#lib/api/client.ts';
import { members } from '#lib/members/api.ts';

export async function load({ params }) {
  await requirePermission('members');
  try {
    return { member: await members.get(params.id) };
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) error(404, 'Not found');
    throw err;
  }
}
