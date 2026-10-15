// Debounce bersama untuk pencarian/filter. Jeda standar satu tempat agar seragam.
export const SEARCH_DEBOUNCE_MS = 250;

/** Jalankan `fn` setelah jeda; kembalikan pembatal. Pakai di `$effect`: `return later(() => void load())`. */
export function later(fn: () => void, ms: number = SEARCH_DEBOUNCE_MS): () => void {
  const h = setTimeout(fn, ms);
  return () => clearTimeout(h);
}

/**
 * Nilai turunan yang mengikuti `get()` setelah jeda — untuk filter lokal pada daftar besar
 * (ketikan tidak menyaring ribuan baris di tiap tombol). Panggil saat inisialisasi komponen.
 */
export function debounced<T>(get: () => T, ms: number = SEARCH_DEBOUNCE_MS): { readonly value: T } {
  let value = $state(get());
  $effect(() => {
    const next = get();
    return later(() => (value = next), ms);
  });
  return {
    get value() {
      return value;
    }
  };
}
