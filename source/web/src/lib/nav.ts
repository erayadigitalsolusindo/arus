// Menu sidebar. Hanya route yang sudah ada yang diberi href; sisanya mengikuti roadmap AGENTS.md §2b.
// Label diambil dari kamus (`nav.*`), bukan teks langsung, agar ikut bahasa aktif.
import type { MessageKey } from '#lib/i18n/index.ts';

export type NavItem = {
  id: string; // kunci stabil (state buka/tutup submenu), independen dari bahasa
  labelKey: MessageKey;
  icon: string; // nama ikon lucide (class `icon-*` dari template)
  href?: string;
  children?: { labelKey: MessageKey; href?: string }[];
};

export type NavGroup = { titleKey: MessageKey; items: NavItem[] };

export const nav: NavGroup[] = [
  {
    titleKey: 'nav.group.main',
    items: [{ id: 'dashboard', labelKey: 'nav.dashboard', icon: 'layout-dashboard', href: '/dashboard' }]
  },
  {
    titleKey: 'nav.group.operations',
    items: [
      { id: 'pos', labelKey: 'nav.pos', icon: 'shopping-cart' },
      {
        id: 'master',
        labelKey: 'nav.masterData',
        icon: 'package',
        children: [
          { labelKey: 'nav.items' },
          { labelKey: 'nav.categories' },
          { labelKey: 'nav.customers' },
          { labelKey: 'nav.suppliers' }
        ]
      },
      { id: 'stock', labelKey: 'nav.stock', icon: 'warehouse' },
      { id: 'purchasing', labelKey: 'nav.purchasing', icon: 'truck' }
    ]
  },
  {
    titleKey: 'nav.group.analysis',
    items: [{ id: 'reports', labelKey: 'nav.reports', icon: 'chart-column' }]
  },
  {
    titleKey: 'nav.group.system',
    items: [
      { id: 'users', labelKey: 'nav.users', icon: 'users' },
      { id: 'settings', labelKey: 'nav.settings', icon: 'settings' }
    ]
  }
];
