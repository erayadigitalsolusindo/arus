// Tab halaman gaya browser. Hanya path yang disimpan (judul dihitung saat render agar ikut bahasa aktif);
// isi halaman dimuat ulang saat tab dibuka. Disimpan di sessionStorage per pengguna.
import { nav } from '#lib/nav.ts';
import { drafts, isPersistentPath } from '#lib/tabs/drafts.ts';
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
  if (path === '/members/new') return { titleKey: 'members.newTitle', module: 'members' };
  if (path === '/accounting/journals/new') return { titleKey: 'accounting.journals.titleNew', module: 'journals' };
  if (/^\/members\/[^/]+$/.test(path)) return { titleKey: 'members.editTitle', module: 'members' };
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
      const read = (s: Storage): string[] => {
        const raw = JSON.parse(s.getItem(owner) ?? '[]');
        return Array.isArray(raw) ? raw.filter((p): p is string => typeof p === 'string' && !!tabMeta(p)) : [];
      };
      // Tab form tertentu juga disimpan di localStorage agar muncul lagi setelah browser ditutup.
      const saved = [...read(sessionStorage), ...read(localStorage).filter(isPersistentPath)];
      this.paths = [HOME, ...new Set(saved.filter((p) => p !== HOME))];
    } catch {
      this.paths = [HOME];
    }
  }

  reset() {
    drafts.clearAll();
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

  /** Menutup banyak tab sekaligus (dashboard tak pernah ditutup); mengembalikan path tujuan bila tab aktif ikut tertutup. */
  closeMany(list: string[], active: string): string | null {
    const gone = new Set(list.filter((p) => p !== HOME));
    if (!gone.size) return null;
    this.paths = this.paths.filter((p) => !gone.has(p));
    this.#save();
    return gone.has(active) ? (this.paths[this.paths.length - 1] ?? HOME) : null;
  }

  #save() {
    if (!this.#key) return;
    try {
      const list = this.paths.slice(1);
      sessionStorage.setItem(this.#key, JSON.stringify(list));
      localStorage.setItem(this.#key, JSON.stringify(list.filter(isPersistentPath)));
    } catch {
      /* penyimpanan tak tersedia: tab tetap jalan di memori */
    }
  }
}

export const tabs = new Tabs();
