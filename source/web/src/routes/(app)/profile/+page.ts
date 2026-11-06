import { requireSession } from '#lib/auth/session.svelte.ts';

// Profil pribadi: semua pengguna yang masuk boleh membukanya (tanpa izin modul).
export const load = async () => {
  await requireSession();
};
