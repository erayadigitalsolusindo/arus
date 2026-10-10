// Pilihan tampilan kasir per peramban: 'modern' (/kasir) atau 'classic' (/kasirb). Kosong = belum memilih (URL dipakai apa adanya).
const KEY = 'pos.layout';
export type PosLayout = 'modern' | 'classic';
export const posPath = (l: PosLayout) => (l === 'classic' ? '/kasirb' : '/kasir');

export function getPosLayout(): PosLayout | null {
  try {
    const v = localStorage.getItem(KEY);
    return v === 'classic' || v === 'modern' ? v : null;
  } catch {
    return null;
  }
}

export function setPosLayout(l: PosLayout) {
  try {
    localStorage.setItem(KEY, l);
  } catch {
    /* penyimpanan diblokir: abaikan */
  }
}
