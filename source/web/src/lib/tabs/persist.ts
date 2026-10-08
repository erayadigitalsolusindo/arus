// Menyimpan keadaan halaman (filter, tanggal, pilihan) per tab: disimpan saat halaman dilepas karena pindah tab,
// dipulihkan saat tab dibuka lagi. Panggil SAAT INISIALISASI komponen (memakai onDestroy). Menutup tab membuangnya.
import { onDestroy } from 'svelte';
import { page } from '$app/state';
import { drafts } from '#lib/tabs/drafts.ts';

/** `apply` dipanggil sekali bila ada keadaan tersimpan; mengembalikan true bila dipulihkan. */
export function persistPage<T>(collect: () => T, apply: (saved: T) => void): boolean {
  const path = page.url.pathname;
  const saved = drafts.load<T>(path);
  if (saved) apply(saved);
  onDestroy(() => {
    if (drafts.takeDiscard(path)) return;
    drafts.save(path, collect());
  });
  return !!saved;
}
