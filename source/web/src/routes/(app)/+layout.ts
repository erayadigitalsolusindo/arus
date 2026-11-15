import { redirect } from '@sveltejs/kit';
import { can, homePath, posOnly, requireSession } from '#lib/auth/session.svelte.ts';
import { onboarding } from '#lib/onboarding/api.ts';

// Semua halaman di (app) butuh sesi: dipulihkan dari cookie refresh atau dialihkan ke /login.
// Akun "Hanya Kasir" tidak boleh melihat halaman admin mana pun: diarahkan ke layar kasir.
export async function load() {
  await requireSession();
  if (posOnly()) redirect(307, homePath());
  // Tenant baru yang belum menyelesaikan/melewati wizard penyiapan diarahkan ke sana (hanya pemilik bagan akun).
  // Gagal memuat status tidak boleh memblokir aplikasi.
  if (can('accounts', 'create')) {
    const done = await onboarding.status().then((s) => s.completed, () => true);
    if (!done) redirect(307, '/onboarding');
  }
}
