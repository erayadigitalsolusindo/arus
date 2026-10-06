// Menu sidebar. Hanya route yang sudah ada yang diberi href; sisanya mengikuti roadmap AGENTS.md §2b.
export type NavItem = {
  label: string;
  icon: string; // nama ikon lucide (class `icon-*` dari template)
  href?: string;
  children?: { label: string; href?: string }[];
};

export type NavGroup = { title: string; items: NavItem[] };

export const nav: NavGroup[] = [
  {
    title: 'Utama',
    items: [{ label: 'Dasbor', icon: 'layout-dashboard', href: '/dashboard' }]
  },
  {
    title: 'Operasional',
    items: [
      { label: 'Kasir', icon: 'shopping-cart' },
      {
        label: 'Master Data',
        icon: 'package',
        children: [{ label: 'Barang' }, { label: 'Kategori' }, { label: 'Pelanggan' }, { label: 'Pemasok' }]
      },
      { label: 'Stok', icon: 'warehouse' },
      { label: 'Pembelian', icon: 'truck' }
    ]
  },
  {
    title: 'Analisis',
    items: [{ label: 'Laporan', icon: 'chart-column' }]
  },
  {
    title: 'Sistem',
    items: [
      { label: 'Pengguna & Hak Akses', icon: 'users' },
      { label: 'Pengaturan', icon: 'settings' }
    ]
  }
];
