// Menu sidebar. Hanya route yang sudah ada yang diberi href; sisanya mengikuti roadmap AGENTS.md §2b.
// Label diambil dari kamus (`nav.*`), bukan teks langsung, agar ikut bahasa aktif.
// `module` = id modul izin (backend/internal/iam/permissions.go): item hanya tampil bila pengguna punya izin `view`-nya.
// Induk tanpa `module` tampil bila ada anak yang tampil.
import type { MessageKey } from '#lib/i18n/index.ts';

export type NavChild = { labelKey: MessageKey; module?: string; href?: string };

export type NavItem = {
  id: string; // kunci stabil (state buka/tutup submenu), independen dari bahasa
  labelKey: MessageKey;
  icon: string; // nama ikon lucide (class `icon-*` dari template)
  module?: string;
  /** Alternatif izin: item juga tampil bila pengguna punya salah satu modul ini. */
  anyOf?: string[];
  href?: string;
  children?: NavChild[];
};

export type NavGroup = { titleKey: MessageKey; items: NavItem[] };

export const nav: NavGroup[] = [
  {
    titleKey: 'nav.group.main',
    items: [
      { id: 'dashboard', labelKey: 'nav.dashboard', icon: 'layout-dashboard', href: '/dashboard' }, // selalu tampil
      { id: 'siak', labelKey: 'nav.siak', icon: 'book-open', module: 'siak' }
    ]
  },
  {
    titleKey: 'nav.group.masterData',
    items: [
      {
        id: 'items',
        labelKey: 'nav.itemList',
        icon: 'package',
        children: [
          { labelKey: 'nav.itemList', module: 'items', href: '/items' },
          { labelKey: 'nav.stockCard', module: 'stock_card', href: '/stock-card' },
          { labelKey: 'nav.coupons', module: 'coupons', href: '/vouchers' }
        ]
      },
      {
        id: 'people',
        labelKey: 'nav.people',
        icon: 'contact-round',
        children: [
          { labelKey: 'nav.suppliers', module: 'suppliers', href: '/suppliers' },
          { labelKey: 'nav.members', module: 'members', href: '/members' },
          { labelKey: 'nav.salespeople', module: 'salespeople', href: '/salespeople' }
        ]
      },
      {
        id: 'support',
        labelKey: 'nav.support',
        icon: 'database',
        children: [
          { labelKey: 'nav.units', module: 'units', href: '/units' },
          { labelKey: 'nav.categories', module: 'categories', href: '/categories' },
          { labelKey: 'nav.memberCategories', module: 'member_categories', href: '/member-levels' },
          { labelKey: 'nav.paymentMethods', module: 'payment_methods', href: '/payment-methods' },
          { labelKey: 'nav.brands', module: 'brands', href: '/brands' },
          { labelKey: 'nav.principals', module: 'principals', href: '/principals' }
        ]
      }
    ]
  },
  {
    titleKey: 'nav.group.sales',
    items: [
      { id: 'pos', labelKey: 'nav.pos', icon: 'monitor-smartphone', module: 'sales_orders', href: '/kasir' },
      {
        id: 'salesOrders',
        labelKey: 'nav.salesOrdersReturns',
        icon: 'shopping-cart',
        module: 'sales_orders',
        children: [{ labelKey: 'nav.salesReturns', module: 'sales_returns' }]
      },
      {
        id: 'salesData',
        labelKey: 'nav.salesData',
        icon: 'file-text',
        children: [
          { labelKey: 'nav.salesList', module: 'sales_list', href: '/sales' },
          { labelKey: 'nav.sellPriceHistory', module: 'sell_price_history', href: '/sell-price-history' },
          { labelKey: 'nav.memberReceivables', module: 'member_receivables', href: '/receivables' }
        ]
      },
      { id: 'orderList', labelKey: 'nav.orderList', icon: 'clipboard-list', module: 'order_list' }
    ]
  },
  {
    titleKey: 'nav.group.purchasing',
    items: [
      {
        id: 'purchaseInvoices',
        labelKey: 'nav.purchaseInvoicesReturns',
        icon: 'receipt',
        module: 'purchase_invoices',
        children: [{ labelKey: 'nav.purchaseReturns', module: 'purchase_returns' }]
      },
      {
        id: 'purchaseData',
        labelKey: 'nav.purchaseData',
        icon: 'truck',
        children: [
          { labelKey: 'nav.purchaseList', module: 'purchase_list' },
          { labelKey: 'nav.buyPriceHistory', module: 'buy_price_history' },
          { labelKey: 'nav.supplierPayables', module: 'supplier_payables' }
        ]
      }
    ]
  },
  {
    titleKey: 'nav.group.adjustments',
    items: [
      { id: 'stockOpening', labelKey: 'nav.stockOpening', icon: 'package-plus', module: 'stock_opening', href: '/stock-opening' },
      { id: 'stockConversion', labelKey: 'nav.stockConversion', icon: 'split', module: 'stock_conversion', href: '/stock-conversion' },
      { id: 'stockOpname', labelKey: 'nav.stockOpname', icon: 'clipboard-check', module: 'stock_opname' },
      { id: 'stockTransfer', labelKey: 'nav.stockTransfer', icon: 'arrow-left-right', module: 'stock_transfer' }
    ]
  },
  {
    titleKey: 'nav.group.system',
    items: [
      { id: 'users', labelKey: 'nav.users', icon: 'users', module: 'users', href: '/users' },
      { id: 'roles', labelKey: 'nav.roles', icon: 'shield-check', module: 'roles', href: '/roles' },
      { id: 'pin', labelKey: 'nav.pin', icon: 'key-round', module: 'price_override', anyOf: ['outlet_switch', 'sale_edit'], href: '/pin' },
      { id: 'outlets', labelKey: 'nav.outlets', icon: 'store', module: 'outlets', href: '/outlets' },
      { id: 'auditLog', labelKey: 'nav.auditLog', icon: 'history', module: 'audit_log', href: '/audit-log' }
    ]
  }
];

/**
 * Menyaring menu menurut izin: item tanpa izin `view` dibuang. Induk yang punya izin sendiri tetapi tidak punya anak
 * yang boleh dilihat menjadi item biasa; induk tanpa izin sendiri hilang bila semua anaknya hilang.
 */
export function visibleNav(allowed: (module: string) => boolean): NavGroup[] {
  const out: NavGroup[] = [];
  for (const group of nav) {
    const items: NavItem[] = [];
    for (const item of group.items) {
      if (item.children) {
        const children = item.children.filter((c) => !c.module || allowed(c.module));
        if (children.length) items.push({ ...item, children });
        else if (item.module && allowed(item.module)) items.push({ ...item, children: undefined });
      } else if (item.id === 'dashboard' || (item.module && allowed(item.module)) || item.anyOf?.some(allowed)) {
        items.push(item);
      }
    }
    if (items.length) out.push({ ...group, items });
  }
  return out;
}
