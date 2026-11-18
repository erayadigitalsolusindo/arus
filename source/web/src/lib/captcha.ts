// Cloudflare Turnstile: aktif hanya bila VITE_TURNSTILE_SITE_KEY diisi (backend: TURNSTILE_SECRET_KEY).
export const turnstileSiteKey = (import.meta.env.VITE_TURNSTILE_SITE_KEY as string | undefined)?.trim() || '';
export const captchaOn = turnstileSiteKey !== '';
export const captchaHeaders = (token: string): Record<string, string> => (token ? { 'X-Turnstile-Token': token } : {});
