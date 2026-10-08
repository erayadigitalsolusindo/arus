// Draf form per tab (kunci = path halaman). Pindah tab membuang komponen halaman, jadi form menyimpan isiannya di sini
// saat dilepas dan memulihkannya saat tab itu dibuka lagi. Disimpan di memori + sessionStorage (selamat dari reload,
// hilang saat browser ditutup). Berkas (gambar/cover yang belum diunggah) tidak bisa disimpan.
const PREFIX = 'draft:';
const mem = new Map<string, string>();
const discarded = new Set<string>();

function read(path: string): string | null {
  const hit = mem.get(path);
  if (hit !== undefined) return hit;
  try {
    return sessionStorage.getItem(PREFIX + path);
  } catch {
    return null;
  }
}

export const drafts = {
  load<T>(path: string): T | null {
    const raw = read(path);
    if (!raw) return null;
    try {
      return JSON.parse(raw) as T;
    } catch {
      return null;
    }
  },

  has(path: string): boolean {
    return read(path) !== null;
  },

  save(path: string, data: unknown) {
    const raw = JSON.stringify(data);
    mem.set(path, raw);
    try {
      sessionStorage.setItem(PREFIX + path, raw);
    } catch {
      /* penyimpanan penuh/tak tersedia: draf tetap ada di memori */
    }
  },

  clear(path: string) {
    mem.delete(path);
    try {
      sessionStorage.removeItem(PREFIX + path);
    } catch {
      /* abaikan */
    }
  },

  /** Tab ditutup oleh pengguna: buang draf, dan jangan simpan ulang saat form dilepas. */
  discard(path: string) {
    this.clear(path);
    discarded.add(path);
  },

  /** Dipanggil form saat dilepas: true bila draf tidak boleh disimpan (tab sedang ditutup). */
  takeDiscard(path: string): boolean {
    return discarded.delete(path);
  },

  clearAll() {
    mem.clear();
    discarded.clear();
    try {
      for (const k of Object.keys(sessionStorage)) if (k.startsWith(PREFIX)) sessionStorage.removeItem(k);
    } catch {
      /* abaikan */
    }
  }
};
