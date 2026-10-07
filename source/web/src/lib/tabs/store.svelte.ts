// Tab halaman gaya browser. Hanya path yang disimpan (judul dihitung saat render agar ikut bahasa aktif);
// isi halaman dimuat ulang saat tab dibuka. Disimpan di sessionStorage per pengguna.
import { nav } from '#lib/nav.ts';
import type { MessageKey } from '#lib/i18n/index.ts';

export const HOME = '/dashboard';

export type TabMeta = { titleKey: MessageKey; module?: string };

const menu = new Map<string, TabMeta>();
for (const g of nav) {
  for (const i of g.items) {
    if (i.href) menu.set(i.href, { titleKey: i.labelKey, module: i.module });
    for (const c of i.children ?? []) if (c.href) menu.set(c.href, { titleKey: c.labelKey, module: c.module });
  }
}

/** Judul & izin untuk sebuah path; null bila bukan halaman yang boleh jadi tab. */
export function tabMeta(path: string): TabMeta | null {
  const hit = menu.get(path);
  if (hit) return hit;
  if (path === '/items/new') return { titleKey: 'items.newTitle', module: 'items' };
  if (/^\/items\/[^/]+$/.test(path)) return { titleKey: 'items.editTitle', module: 'items' };
  return null;
}

class Tabs {
  paths = $state<string[]>([HOME]);
  #key: string | null = null;

  /** Muat tab milik pengguna ini (dipanggil sekali saat sesi diketahui). */
  load(owner: string) {
    if (this.#key === owner) return;
    this.#key = owner;
    try {
      const raw = JSON.parse(sessionStorage.getItem(owner) ?? '[]');
      const saved = Array.isArray(raw) ? raw.filter((p) => typeof p === 'string' && tabMeta(p)) : [];
      this.paths = [HOME, ...saved.filter((p) => p !== HOME)];
    } catch {
      this.paths = [HOME];
    }
  }

  reset() {
    this.#key = null;
    this.paths = [HOME];
  }

  /** Dipanggil tiap navigasi: path baru jadi tab baru, path yang sudah ada dipakai ulang. */
  open(path: string) {
    if (!this.#key || !tabMeta(path) || this.paths.includes(path)) return;
    this.paths.push(path);
    this.#save();
  }

  /** Menutup tab; mengembalikan path tujuan bila tab yang ditutup sedang aktif. */
  close(path: string, active: string): string | null {
    const i = this.paths.indexOf(path);
    if (i <= 0) return null; // dashboard tidak bisa ditutup
    this.paths.splice(i, 1);
    this.#save();
    return path === active ? (this.paths[i] ?? this.paths[i - 1] ?? HOME) : null;
  }

  #save() {
    if (!this.#key) return;
    try {
      sessionStorage.setItem(this.#key, JSON.stringify(this.paths.slice(1)));
    } catch {
      /* penyimpanan tak tersedia: tab tetap jalan di memori */
    }
  }
}

export const tabs = new Tabs();
