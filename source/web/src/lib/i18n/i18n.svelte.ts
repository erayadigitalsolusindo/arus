import { DEFAULT_LOCALE, LOCALES, STORAGE_KEY, isLocale, localeCodes, type Locale } from './locales.ts';
import { messages } from './messages/index.ts';
import type { MessageKey, Params, Plural } from './types.ts';

function detect(): Locale {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (isLocale(stored)) return stored;
  } catch {
    /* penyimpanan diblokir: abaikan */
  }
  for (const tag of navigator.languages ?? [navigator.language]) {
    const base = tag.toLowerCase().split('-')[0];
    if (isLocale(base)) return base;
  }
  return DEFAULT_LOCALE;
}

let current = $state<Locale>(detect());

function applyHtmlLang(locale: Locale) {
  document.documentElement.lang = locale;
}
applyHtmlLang(current);

export const i18n = {
  get locale() {
    return current;
  },
  get intl() {
    return LOCALES[current].intl;
  },
  locales: localeCodes
};

export function setLocale(locale: Locale) {
  if (!isLocale(locale) || locale === current) return;
  current = locale;
  applyHtmlLang(locale);
  try {
    localStorage.setItem(STORAGE_KEY, locale);
  } catch {
    /* penyimpanan diblokir: abaikan */
  }
}

function lookup(locale: Locale, key: string): string | Plural | undefined {
  let node: unknown = messages[locale];
  for (const part of key.split('.')) {
    if (node === null || typeof node !== 'object') return undefined;
    node = (node as Record<string, unknown>)[part];
  }
  return typeof node === 'string' || (node !== null && typeof node === 'object') ? (node as string | Plural) : undefined;
}

function resolve(key: string): { text: string | Plural; locale: Locale } | undefined {
  const own = lookup(current, key);
  if (own !== undefined) return { text: own, locale: current };
  const fallback = lookup(DEFAULT_LOCALE, key);
  return fallback !== undefined ? { text: fallback, locale: DEFAULT_LOCALE } : undefined;
}

function render(text: string | Plural, locale: Locale, params?: Params): string {
  let out: string;
  if (typeof text === 'string') {
    out = text;
  } else {
    const count = Number(params?.count ?? 0);
    const category = count === 0 && text.zero !== undefined ? 'zero' : new Intl.PluralRules(LOCALES[locale].intl).select(count);
    out = text[category as keyof Plural] ?? text.other;
  }
  if (!params) return out;
  const nf = new Intl.NumberFormat(LOCALES[locale].intl);
  return out.replace(/\{(\w+)\}/g, (m, name: string) => {
    const v = params[name];
    return v === undefined ? m : typeof v === 'number' ? nf.format(v) : v;
  });
}

/**
 * Terjemahkan kunci bertipe. Reaktif: memanggilnya di template/`$derived` ikut berubah saat bahasa diganti.
 * Kunci yang belum ada di bahasa aktif jatuh ke Indonesia.
 */
export function t(key: MessageKey, params?: Params): string {
  const hit = resolve(key);
  return hit ? render(hit.text, hit.locale, params) : key;
}

/** Untuk kunci dinamis (mis. kode error dari API). Mengembalikan `undefined` bila tidak ada. */
export function tryT(key: string, params?: Params): string | undefined {
  const hit = resolve(key);
  return hit ? render(hit.text, hit.locale, params) : undefined;
}

type DateInput = Date | number | string;
const toDate = (d: DateInput) => (d instanceof Date ? d : new Date(d));

export function formatNumber(n: number | bigint, options?: Intl.NumberFormatOptions): string {
  return new Intl.NumberFormat(i18n.intl, options).format(n);
}

/** Uang rupiah dengan 2 desimal secara default (tampilan; perhitungan tetap di server). */
export function formatCurrency(n: number | bigint, currency = 'IDR', options?: Intl.NumberFormatOptions): string {
  return new Intl.NumberFormat(i18n.intl, { style: 'currency', currency, minimumFractionDigits: 2, maximumFractionDigits: 2, ...options }).format(n);
}

export function formatDate(d: DateInput, options: Intl.DateTimeFormatOptions = { dateStyle: 'medium' }): string {
  return new Intl.DateTimeFormat(i18n.intl, options).format(toDate(d));
}

export function formatDateTime(d: DateInput, options: Intl.DateTimeFormatOptions = { dateStyle: 'medium', timeStyle: 'short' }): string {
  return new Intl.DateTimeFormat(i18n.intl, options).format(toDate(d));
}
