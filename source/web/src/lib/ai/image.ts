// Foto nota dikecilkan di perangkat sebelum dikirim ke HalloArus: foto ponsel bisa 5–10 MB, padahal server membatasi 3 MB per foto
// dan teks nota tetap terbaca di sisi terpanjang ±1600 px. Hasil selalu JPEG.
import type { ChatImage } from '#lib/ai/api.ts';

const MAX_SIDE = 1600;
const ACCEPT = ['image/jpeg', 'image/png', 'image/webp'];

export const IMAGE_ACCEPT = ACCEPT.join(',');
export const MAX_IMAGES = 3;

export type PreparedImage = { image: ChatImage; preview: string };

/** Mengembalikan null bila berkas bukan gambar yang didukung atau tidak bisa didekode. */
export async function prepareImage(file: File): Promise<PreparedImage | null> {
  if (!ACCEPT.includes(file.type)) return null;
  let bmp: ImageBitmap;
  try {
    bmp = await createImageBitmap(file);
  } catch {
    return null;
  }
  const scale = Math.min(1, MAX_SIDE / Math.max(bmp.width, bmp.height));
  const canvas = document.createElement('canvas');
  canvas.width = Math.max(1, Math.round(bmp.width * scale));
  canvas.height = Math.max(1, Math.round(bmp.height * scale));
  const ctx = canvas.getContext('2d');
  if (!ctx) return null;
  ctx.fillStyle = '#fff'; // PNG transparan → latar putih agar teks nota tetap terbaca
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.drawImage(bmp, 0, 0, canvas.width, canvas.height);
  bmp.close();
  const url = canvas.toDataURL('image/jpeg', 0.85);
  const data = url.slice(url.indexOf(',') + 1);
  return data ? { image: { media_type: 'image/jpeg', data }, preview: url } : null;
}
