import { i18n } from '#lib/i18n/i18n.svelte.ts';

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 31536000],
  ['month', 2592000],
  ['week', 604800],
  ['day', 86400],
  ['hour', 3600],
  ['minute', 60]
];

/** "3 hari lalu" / "3 days ago" menurut bahasa aktif. Bukan sumber kebenaran waktu — hanya tampilan. */
export function relativeTime(iso: string): string {
  const secs = Math.round((new Date(iso).getTime() - Date.now()) / 1000);
  const rtf = new Intl.RelativeTimeFormat(i18n.locale, { numeric: 'auto' });
  for (const [unit, size] of UNITS) {
    if (Math.abs(secs) >= size) return rtf.format(Math.round(secs / size), unit);
  }
  return rtf.format(0, 'second');
}
