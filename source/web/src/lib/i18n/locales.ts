// Daftar bahasa yang didukung. Menambah bahasa: tambahkan entri di sini + folder `messages/<kode>/`
// (bertipe `Messages`, jadi kunci yang kurang langsung gagal di `npm run check`).
export const LOCALES = {
  id: { name: 'Bahasa Indonesia', short: 'ID', intl: 'id-ID' },
  en: { name: 'English', short: 'EN', intl: 'en-US' }
} as const;

export type Locale = keyof typeof LOCALES;

export const localeCodes = Object.keys(LOCALES) as Locale[];

// Bahasa sumber kebenaran sekaligus fallback bila kunci belum diterjemahkan.
export const DEFAULT_LOCALE: Locale = 'id';

export const STORAGE_KEY = 'locale';

export function isLocale(value: unknown): value is Locale {
  return typeof value === 'string' && value in LOCALES;
}
