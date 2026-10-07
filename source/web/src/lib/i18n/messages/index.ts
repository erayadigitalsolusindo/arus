import type { Locale } from '../locales.ts';
import type { Messages } from '../types.ts';
import id from './id/index.ts';
import en from './en/index.ts';

export const messages: Record<Locale, Messages> = { id, en };
