// Struk laporan kasir: tutup shift dan rekap penjualan (Penjualan Hari Ini). Memakai tata letak berlebar tetap yang
// sama dengan struk nota (receipt.ts) sehingga bisa dicetak lewat print-agent (ESC/POS) maupun browser.
import { formatDateTime, t, type MessageKey } from '#lib/i18n/index.ts';
import type { SaleList } from '#lib/sales/api.ts';
import type { Shift } from '#lib/shift/api.ts';
import { amt, center, lr, receiptCols, wrap, type Paper, type ReceiptLine } from '#lib/pos/receipt.ts';

/** String desimal bertanda dari API ("-1500.00") → sen. */
export function signedCents(s: string | null | undefined): bigint {
  if (!s) return 0n;
  const neg = s.trim().startsWith('-');
  const clean = s.trim().replace(/^[-+]/, '');
  if (!/^\d+(\.\d{0,2})?$/.test(clean)) return 0n;
  const [i, f = ''] = clean.split('.');
  const v = BigInt(i) * 100n + BigInt((f + '00').slice(0, 2));
  return neg ? -v : v;
}

const money = (s: string | null | undefined) => amt(signedCents(s));
const signed = (s: string | null | undefined) => {
  const c = signedCents(s);
  return c > 0n ? `+${amt(c)}` : amt(c);
};

function builder(paper: Paper) {
  const w = receiptCols(paper);
  const out: ReceiptLine[] = [];
  const push = (text: string, o: Omit<ReceiptLine, 'text'> = {}) => out.push({ text, ...o });
  return {
    w,
    out,
    push,
    rule: () => push('-'.repeat(w)),
    centered: (text: string, o: Omit<ReceiptLine, 'text'> = {}) => wrap(text, w).forEach((l) => push(center(l, w), o)),
    row: (l: string, r: string, o: Omit<ReceiptLine, 'text'> = {}) => push(lr(l, r, w), o)
  };
}

const flowLabel = (src: string) => t(`pos.today.flows.${src}` as MessageKey);

/** Struk tutup shift (atau rekap shift berjalan bila status masih open). */
export function shiftLines(sh: Shift, paper: Paper): ReceiptLine[] {
  const b = builder(paper);
  b.centered(sh.store.tenant_name, { bold: true, title: true });
  if (sh.store.outlet_name && sh.store.outlet_name !== sh.store.tenant_name) b.centered(sh.store.outlet_name);
  if (sh.store.address) b.centered(sh.store.address);
  b.rule();
  b.centered(sh.status === 'closed' ? t('shift.receipt.title') : t('shift.receipt.titleOpen'), { bold: true });
  b.row(t('shift.receipt.no'), sh.doc_no);
  b.row(t('shift.receipt.cashier'), sh.user_name);
  b.row(t('shift.receipt.opened'), sh.opened_local);
  if (sh.closed_local) b.row(t('shift.receipt.closed'), sh.closed_local);
  b.rule();
  b.row(t('shift.receipt.salesCount'), String(sh.sale_count));
  if (sh.void_count > 0) b.row(t('shift.receipt.voidCount'), String(sh.void_count));
  b.row(t('shift.receipt.salesTotal'), money(sh.sales_total));
  if (signedCents(sh.receivable_total) > 0n) b.row(t('shift.receipt.receivable'), money(sh.receivable_total));
  b.rule();

  for (const c of sh.counts) {
    b.push(c.name, { bold: true });
    if (signedCents(c.opening) !== 0n) b.row(`  ${t('shift.receipt.opening')}`, money(c.opening));
    if (signedCents(c.sales) !== 0n) b.row(`  ${t('shift.receipt.sales')}`, money(c.sales));
    if (signedCents(c.flows) !== 0n) b.row(`  ${t('shift.receipt.flows')}`, signed(c.flows));
    b.row(`  ${t('shift.receipt.expected')}`, money(c.expected));
    if (c.counted !== null) {
      b.row(`  ${t('shift.receipt.counted')}`, money(c.counted));
      if (signedCents(c.diff) !== 0n) b.row(`  ${t('shift.receipt.diff')}`, signed(c.diff), { bold: true });
    }
  }
  if (sh.flows.length) {
    b.rule();
    for (const f of sh.flows) b.row(`${flowLabel(f.source)} (${f.count})`.slice(0, b.w - 12), signed(f.amount));
  }
  b.rule();
  b.row(t('shift.receipt.totalExpected'), money(sh.expected_total), { bold: true });
  if (sh.counted_total !== null) {
    b.row(t('shift.receipt.totalCounted'), money(sh.counted_total), { bold: true });
    b.row(t('shift.receipt.totalDiff'), signed(sh.diff_total), { bold: true, big: signedCents(sh.diff_abs) !== 0n });
  }
  if (sh.approved_by_name) b.row(t('shift.receipt.approvedBy'), sh.approved_by_name);
  if (sh.note) {
    b.push(`${t('shift.receipt.note')}:`);
    wrap(sh.note, b.w).forEach((l) => b.push(l));
  }
  if (sh.status === 'closed') {
    b.push('');
    b.push('');
    const half = Math.floor(b.w / 2);
    b.push(center(t('shift.receipt.signCashier'), half).padEnd(half) + center(t('shift.receipt.signSupervisor'), b.w - half));
    b.push('');
    b.push('');
    b.push(center('(' + '.'.repeat(half - 6) + ')', half).padEnd(half) + center('(' + '.'.repeat(half - 6) + ')', b.w - half));
  }
  b.rule();
  b.centered(formatDateTime(new Date()));
  return b.out;
}

/** Struk rekap Penjualan Hari Ini milik kasir yang login (rentang tanggal yang dipilih). */
export function dailyRecapLines(list: SaleList, meta: { tenant: string; outlet: string; cashier: string }, paper: Paper): ReceiptLine[] {
  const b = builder(paper);
  b.centered(meta.tenant, { bold: true, title: true });
  if (meta.outlet && meta.outlet !== meta.tenant) b.centered(meta.outlet);
  b.rule();
  b.centered(t('pos.today.recap.title'), { bold: true });
  b.row(t('pos.today.recap.cashier'), meta.cashier);
  const fmt = (d: string) => d.split('-').reverse().join('/');
  b.row(t('pos.today.recap.period'), list.from === list.to ? fmt(list.from) : `${fmt(list.from)}-${fmt(list.to)}`);
  b.rule();
  const done = list.data.filter((r) => r.status === 'completed').length;
  const voided = list.data.filter((r) => r.status === 'void').length;
  b.row(t('pos.today.recap.count'), String(done));
  if (voided) b.row(t('pos.today.recap.voided'), String(voided));
  b.row(t('pos.today.recap.revenue'), money(list.total), { bold: true });
  const credit = (list.totals as Record<string, string | undefined>).credit;
  if (credit && signedCents(credit) > 0n) b.row(t('pos.today.recap.receivable'), money(credit));
  if (list.returns && list.returns.count > 0) {
    b.row(t('pos.today.returnsRow', { count: list.returns.count }), '-' + money(list.returns.total));
    b.row(t('pos.today.netTotal'), money(list.net_total ?? list.total), { bold: true });
  }
  if (signedCents(list.surcharge) > 0n) b.row(t('pos.today.recap.surcharge'), money(list.surcharge));
  b.rule();
  b.push(t('pos.today.recap.byMethod'), { bold: true });
  for (const m of list.by_method) b.row(`  ${m.name}`, money(m.amount));
  if (list.flows.length) {
    b.rule();
    b.push(t('pos.today.flows.title'), { bold: true });
    for (const f of list.flows) b.row(`  ${flowLabel(f.source)} - ${f.name}`, signed(f.amount));
  }
  b.rule();
  b.push(t('pos.today.drawer'), { bold: true });
  for (const m of list.drawer) b.row(`  ${m.name}`, money(m.amount), { bold: m.kind === 'cash' });
  if (list.truncated) {
    b.rule();
    wrap(t('pos.today.truncated'), b.w).forEach((l) => b.push(l));
  }
  b.rule();
  b.centered(formatDateTime(new Date()));
  return b.out;
}
