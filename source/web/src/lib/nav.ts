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
      { id: 'live-sales', labelKey: 'nav.liveSales', icon: 'activity', module: 'sales_list', href: '/live-sales' },
      // Pintu masuk ke mode SIAK (sidebar berganti ke `siakNav`); href-nya diganti Sidebar ke halaman SIAK pertama yang boleh dibuka.
      { id: 'siak', labelKey: 'nav.siak', icon: 'book-open', anyOf: ['accounts', 'journals', 'general_ledger', 'accounting_periods'], href: '/accounting/accounts' },
      // Asisten AI hanya-baca (Claude, API key milik tenant): menganalisis data usaha dan memberi saran.
      { id: 'halloarus', labelKey: 'nav.halloarus', icon: 'sparkles', module: 'ai_assistant', href: '/halloarus' }
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
      { id: 'pettyCash', labelKey: 'nav.pettyCash', icon: 'wallet', module: 'sales_orders', href: '/petty-cash' },
      {
        id: 'salesOrders',
        labelKey: 'nav.salesOrdersReturns',
        icon: 'shopping-cart',
        module: 'sales_orders',
        children: [{ labelKey: 'nav.salesReturns', module: 'sales_returns', href: '/sales-returns' }]
      },
      {
        id: 'salesData',
        labelKey: 'nav.salesData',
        icon: 'file-text',
        children: [
          { labelKey: 'nav.salesList', module: 'sales_list', href: '/sales' },
          { labelKey: 'nav.cashShifts', module: 'cash_shifts', href: '/shifts' },
          { labelKey: 'nav.sellPriceHistory', module: 'sell_price_history', href: '/sell-price-history' },
          { labelKey: 'nav.memberReceivables', module: 'member_receivables', href: '/receivables' }
        ]
      }
    ]
  },
  {
    titleKey: 'nav.group.purchasing',
    items: [
      {
        id: 'purchaseInvoices',
        labelKey: 'nav.purchaseInvoicesReturns',
        icon: 'receipt',
        children: [
          { labelKey: 'nav.purchaseInvoices', module: 'purchase_invoices', href: '/purchases/new' },
          { labelKey: 'nav.purchaseReturns', module: 'purchase_returns', href: '/purchase-returns' }
        ]
      },
      {
        id: 'purchaseData',
        labelKey: 'nav.purchaseData',
        icon: 'truck',
        children: [
          { labelKey: 'nav.purchaseList', module: 'purchase_list', href: '/purchases' },
          { labelKey: 'nav.buyPriceHistory', module: 'buy_price_history', href: '/buy-price-history' },
          { labelKey: 'nav.supplierPayables', module: 'supplier_payables', href: '/supplier-payables' },
          { labelKey: 'nav.supplierCredits', module: 'supplier_credits', href: '/supplier-credits' }
        ]
      }
    ]
  },
  {
    titleKey: 'nav.group.adjustments',
    items: [
      { id: 'stockOpening', labelKey: 'nav.stockOpening', icon: 'package-plus', module: 'stock_opening', href: '/stock-opening' },
      { id: 'stockConversion', labelKey: 'nav.stockConversion', icon: 'split', module: 'stock_conversion', href: '/stock-conversion' },
      { id: 'stockOpname', labelKey: 'nav.stockOpname', icon: 'clipboard-check', module: 'stock_opname', href: '/stock-opname' },
      { id: 'stockTransfer', labelKey: 'nav.stockTransfer', icon: 'arrow-left-right', module: 'stock_transfer', href: '/stock-transfer' }
    ]
  },
  {
    titleKey: 'nav.group.reports',
    items: [
      {
        id: 'salesReports',
        labelKey: 'nav.salesReports',
        icon: 'file-bar-chart',
        module: 'sales_reports',
        children: [
          { labelKey: 'nav.salesInvoice', module: 'sales_reports', href: '/reports/sales/invoices' },
          { labelKey: 'nav.salesInvoiceDetail', module: 'sales_reports', href: '/reports/sales/invoice-detail' },
          { labelKey: 'nav.salesItem', module: 'sales_reports', href: '/reports/sales/items' },
          { labelKey: 'nav.salesCashier', module: 'sales_reports', href: '/reports/sales/cashiers' },
          { labelKey: 'nav.salesCustomer', module: 'sales_reports', href: '/reports/sales/customers' },
          { labelKey: 'nav.salesCustomerInvoice', module: 'sales_reports', href: '/reports/sales/customer-invoices' },
          { labelKey: 'nav.salesByDate', module: 'sales_reports', href: '/reports/sales/by-date' },
          { labelKey: 'nav.salesDetail', module: 'sales_reports', href: '/reports/sales/detail' }
        ]
      },
      {
        id: 'salesReturnReports',
        labelKey: 'nav.salesReturnReports',
        icon: 'file-bar-chart',
        module: 'sales_reports',
        children: [
          { labelKey: 'nav.salesReturnReportInvoice', module: 'sales_reports', href: '/reports/sales-returns/invoices' },
          { labelKey: 'nav.salesReturnReportInvoiceDetail', module: 'sales_reports', href: '/reports/sales-returns/invoice-detail' }
        ]
      },
      {
        id: 'purchaseReports',
        labelKey: 'nav.purchaseReports',
        icon: 'file-bar-chart',
        module: 'purchase_reports',
        children: [
          { labelKey: 'nav.purchaseReportInvoice', module: 'purchase_reports', href: '/reports/purchases/invoices' },
          { labelKey: 'nav.purchaseReportInvoiceDetail', module: 'purchase_reports', href: '/reports/purchases/invoice-detail' },
          { labelKey: 'nav.purchaseReportItem', module: 'purchase_reports', href: '/reports/purchases/items' },
          { labelKey: 'nav.purchaseReportByDate', module: 'purchase_reports', href: '/reports/purchases/by-date' }
        ]
      },
      {
        id: 'purchaseReturnReports',
        labelKey: 'nav.purchaseReturnReports',
        icon: 'file-bar-chart',
        module: 'purchase_reports',
        children: [
          { labelKey: 'nav.purchaseReturnReportInvoice', module: 'purchase_reports', href: '/reports/purchase-returns/invoices' },
          { labelKey: 'nav.purchaseReturnReportInvoiceDetail', module: 'purchase_reports', href: '/reports/purchase-returns/invoice-detail' }
        ]
      },
      {
        id: 'masterReports',
        labelKey: 'nav.masterReports',
        icon: 'database',
        children: [
          { labelKey: 'nav.stockReport', module: 'stock_reports', href: '/reports/stock/valuation' },
          { labelKey: 'nav.masterMember', module: 'stock_reports', href: '/reports/master/members' },
          { labelKey: 'nav.masterSupplier', module: 'stock_reports', href: '/reports/master/suppliers' },
          { labelKey: 'nav.masterSalesperson', module: 'stock_reports', href: '/reports/master/salespeople' }
        ]
      },
      {
        id: 'payableReports',
        labelKey: 'nav.debtReports',
        icon: 'file-bar-chart',
        children: [
          { labelKey: 'nav.debtReportOutstanding', module: 'supplier_payables', href: '/reports/payables/outstanding' },
          { labelKey: 'nav.debtReportPayments', module: 'supplier_payables', href: '/reports/payables/payments' },
          { labelKey: 'nav.debtReportBalances', module: 'supplier_payables', href: '/reports/payables/balances' },
          { labelKey: 'nav.debtReportLedger', module: 'supplier_payables', href: '/reports/payables/ledger' },
          { labelKey: 'nav.debtReportCredit', module: 'supplier_credits', href: '/reports/supplier-credits' }
        ]
      },
      {
        id: 'receivableReports',
        labelKey: 'nav.recvReports',
        icon: 'file-bar-chart',
        children: [
          { labelKey: 'nav.recvReportOutstanding', module: 'member_receivables', href: '/reports/receivables/outstanding' },
          { labelKey: 'nav.recvReportPayments', module: 'member_receivables', href: '/reports/receivables/payments' },
          { labelKey: 'nav.recvReportBalances', module: 'member_receivables', href: '/reports/receivables/balances' },
          { labelKey: 'nav.recvReportLedger', module: 'member_receivables', href: '/reports/receivables/ledger' }
        ]
      }
    ]
  },
  {
    titleKey: 'nav.group.system',
    items: [
      { id: 'users', labelKey: 'nav.users', icon: 'users', module: 'users', href: '/users' },
      { id: 'roles', labelKey: 'nav.roles', icon: 'shield-check', module: 'roles', href: '/roles' },
      { id: 'pin', labelKey: 'nav.pin', icon: 'key-round', module: 'price_override', anyOf: ['outlet_switch', 'sale_edit', 'credit_limit', 'shift_close'], href: '/pin' },
      { id: 'outlets', labelKey: 'nav.outlets', icon: 'store', module: 'outlets', href: '/outlets' },
      { id: 'auditLog', labelKey: 'nav.auditLog', icon: 'history', module: 'audit_log', href: '/audit-log' }
    ]
  }
];

/**
 * Menyaring menu menurut izin: item tanpa izin `view` dibuang. Induk yang punya izin sendiri tetapi tidak punya anak
 * yang boleh dilihat menjadi item biasa; induk tanpa izin sendiri hilang bila semua anaknya hilang.
 */
export function visibleNav(allowed: (module: string) => boolean, source: NavGroup[] = nav): NavGroup[] {
  const out: NavGroup[] = [];
  for (const group of source) {
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

/** Halaman SIAK berada di bawah awalan ini; di sana sidebar berganti ke `siakNav`. */
export const SIAK_PREFIX = '/accounting';

/** Menu mode SIAK (akuntansi): tampil menggantikan menu ARUS selama pengguna berada di halaman SIAK. */
export const siakNav: NavGroup[] = [
  {
    titleKey: 'nav.group.siak',
    items: [
      {
        id: 'acc-accounting',
        labelKey: 'nav.accAccounting',
        icon: 'calculator',
        children: [
          { labelKey: 'nav.accAccounts', module: 'accounts', href: '/accounting/accounts' },
          { labelKey: 'nav.accMappings', module: 'accounts', href: '/accounting/mappings' },
          { labelKey: 'nav.accJournals', module: 'journals', href: '/accounting/journals' },
          { labelKey: 'nav.accCashBank', module: 'general_ledger', href: '/accounting/cash-bank' },
          { labelKey: 'nav.accPeriods', module: 'accounting_periods', href: '/accounting/periods' }
        ]
      },
      {
        id: 'acc-reports',
        labelKey: 'nav.accReports',
        icon: 'chart-no-axes-combined',
        children: [
          { labelKey: 'nav.accLedger', module: 'general_ledger', href: '/accounting/ledger' },
          { labelKey: 'nav.accGeneralJournal', module: 'general_ledger', href: '/accounting/general-journal' },
          { labelKey: 'nav.accBankReconciliation', module: 'general_ledger', href: '/accounting/bank-reconciliation' },
          { labelKey: 'nav.accPettyCashReport', module: 'general_ledger', href: '/accounting/petty-cash' },
          { labelKey: 'nav.accTrialBalance', module: 'general_ledger', href: '/accounting/trial-balance' },
          { labelKey: 'nav.accBalanceSheet', module: 'general_ledger', href: '/accounting/balance-sheet' },
          { labelKey: 'nav.accIncomeStatement', module: 'general_ledger', href: '/accounting/income-statement' }
        ]
      }
    ]
  }
];
