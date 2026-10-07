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
    items: [
      { id: 'dashboard', labelKey: 'nav.dashboard', icon: 'layout-dashboard', href: '/dashboard' },
      { id: 'siak', labelKey: 'nav.siak', icon: 'book-open' }
    ]
  },
  {
    titleKey: 'nav.group.masterData',
    items: [
      {
        id: 'items',
        labelKey: 'nav.itemList',
        icon: 'package',
        children: [{ labelKey: 'nav.itemList' }, { labelKey: 'nav.stockCard' }, { labelKey: 'nav.coupons' }]
      },
      {
        id: 'people',
        labelKey: 'nav.people',
        icon: 'contact-round',
        children: [{ labelKey: 'nav.suppliers' }, { labelKey: 'nav.members' }, { labelKey: 'nav.salespeople' }]
      },
      {
        id: 'support',
        labelKey: 'nav.support',
        icon: 'database',
        children: [
          { labelKey: 'nav.units' },
          { labelKey: 'nav.categories' },
          { labelKey: 'nav.memberCategories' },
          { labelKey: 'nav.paymentMethods' },
          { labelKey: 'nav.brands' },
          { labelKey: 'nav.principals' }
        ]
      }
    ]
  },
  {
    titleKey: 'nav.group.sales',
    items: [
      {
        id: 'salesOrders',
        labelKey: 'nav.salesOrdersReturns',
        icon: 'shopping-cart',
        children: [{ labelKey: 'nav.salesReturns' }]
      },
      {
        id: 'salesData',
        labelKey: 'nav.salesData',
        icon: 'file-text',
        children: [{ labelKey: 'nav.salesList' }, { labelKey: 'nav.sellPriceHistory' }, { labelKey: 'nav.memberReceivables' }]
      },
      { id: 'orderList', labelKey: 'nav.orderList', icon: 'clipboard-list' }
    ]
  },
  {
    titleKey: 'nav.group.purchasing',
    items: [
      {
        id: 'purchaseInvoices',
        labelKey: 'nav.purchaseInvoicesReturns',
        icon: 'receipt',
        children: [{ labelKey: 'nav.purchaseReturns' }]
      },
      {
        id: 'purchaseData',
        labelKey: 'nav.purchaseData',
        icon: 'truck',
        children: [{ labelKey: 'nav.purchaseList' }, { labelKey: 'nav.buyPriceHistory' }, { labelKey: 'nav.supplierPayables' }]
      }
    ]
  },
  {
    titleKey: 'nav.group.adjustments',
    items: [
      { id: 'stockOpname', labelKey: 'nav.stockOpname', icon: 'clipboard-check' },
      { id: 'stockTransfer', labelKey: 'nav.stockTransfer', icon: 'arrow-left-right' }
    ]
  }
];
