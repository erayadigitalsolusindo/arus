// Kelompok fitur yang dijelaskan di halaman Langganan. Sebuah paket memuat fitur bila salah satu modul penandanya termasuk
// dalam daftar menu paket (modules null = semua menu). Teks: kamus `subscription.features.<id>`.
export const featureGroups = [
  { id: 'pos', icon: 'monitor-smartphone', any: ['sales_orders'] },
  { id: 'sales', icon: 'receipt', any: ['sales_list', 'sales_reports'] },
  { id: 'stock', icon: 'package', any: ['items', 'stock_opname'] },
  { id: 'qris', icon: 'qr-code', any: ['payment_methods', 'qris_force'] },
  { id: 'members', icon: 'users', any: ['members', 'member_receivables'] },
  { id: 'purchasing', icon: 'truck', any: ['purchase_invoices', 'supplier_payables'] },
  { id: 'accounting', icon: 'book-open', any: ['journals', 'general_ledger'] },
  { id: 'ai', icon: 'sparkles', any: ['ai_assistant'] },
  { id: 'audit', icon: 'history', any: ['audit_log'] }
] as const;

export type FeatureId = (typeof featureGroups)[number]['id'];

/** Apakah paket dengan daftar menu `modules` (null = semua) memuat kelompok fitur ini. */
export function planHas(modules: string[] | null, group: (typeof featureGroups)[number]): boolean {
  return modules === null || group.any.some((m) => modules.includes(m));
}
