// Validasi & normalisasi input di klien. HANYA untuk kenyamanan (umpan balik cepat); server tetap memvalidasi
// ulang semua aturan yang sama (source/backend/internal/platform/sanitize). Kode galat identik dengan server
// sehingga diterjemahkan lewat `fieldMessage()`. Output ke DOM tetap di-escape Svelte: jangan pakai {@html}.

export type FieldCode = 'REQUIRED' | 'INVALID' | 'TOO_LONG' | 'TOO_SHORT' | 'WEAK';
export type Checked = { value: string; code?: FieldCode };

// Karakter kontrol, format tak terlihat (zero-width, bidi override), private use, dan U+FFFD.
const FORBIDDEN = /[\p{Cc}\p{Cf}\p{Co}�]/u;

/** NFC + padatkan whitespace + trim; null bila memuat karakter terlarang. */
export function cleanText(s: string): string | null {
  const v = s.normalize('NFC').replace(/\s+/gu, ' ').trim();
  return FORBIDDEN.test(v) ? null : v;
}

export function checkName(s: string, max = 100): Checked {
  const v = cleanText(s);
  if (v === null || /[<>]/.test(v)) return { value: '', code: 'INVALID' };
  if (v === '') return { value: '', code: 'REQUIRED' };
  if ([...v].length > max) return { value: v, code: 'TOO_LONG' };
  return { value: v };
}

const EMAIL = /^[a-z0-9.!#$%&'*+/=?^_{|}~-]{1,64}@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$/;

export function checkEmail(s: string): Checked {
  const v = s.trim().toLowerCase();
  if (v === '') return { value: '', code: 'REQUIRED' };
  if (v.length > 254) return { value: v, code: 'TOO_LONG' };
  if (!EMAIL.test(v) || v.includes('..')) return { value: v, code: 'INVALID' };
  return { value: v };
}

export function checkPhone(s: string): Checked {
  const v = s.trim();
  if (v === '') return { value: '', code: 'REQUIRED' };
  if (!/^\+?[0-9 ().-]+$/.test(v) || v.lastIndexOf('+') > 0) return { value: v, code: 'INVALID' };
  let out = v.replace(/[ ().-]/g, '');
  if (out.startsWith('0')) out = '+62' + out.slice(1);
  const digits = out.replace('+', '');
  return digits.length < 8 || digits.length > 15 ? { value: v, code: 'INVALID' } : { value: out };
}

export function checkPassword(s: string, email: string): FieldCode | undefined {
  const n = [...s].length;
  if (s === '') return 'REQUIRED';
  if (n > 128) return 'TOO_LONG';
  if (n < 10) return 'TOO_SHORT';
  if (/\p{Cc}/u.test(s)) return 'INVALID';
  if (s.toLowerCase() === email) return 'WEAK';
  if (!/\p{L}/u.test(s) || !/\p{Nd}/u.test(s)) return 'WEAK';
  return undefined;
}

/** Skor kekuatan 0–2 untuk indikator visual (bukan pengganti aturan server). */
export function passwordStrength(s: string): 0 | 1 | 2 {
  if ([...s].length < 10) return 0;
  const classes = [/\p{Ll}/u, /\p{Lu}/u, /\p{Nd}/u, /[^\p{L}\p{Nd}]/u].filter((r) => r.test(s)).length;
  return [...s].length >= 14 && classes >= 3 ? 2 : classes >= 2 ? 1 : 0;
}
