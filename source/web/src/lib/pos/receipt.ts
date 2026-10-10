// Struk kasir (FR-POS-15). Model struk datang dari server (GET /sales/{id}/receipt); di sini ditata menjadi baris
// teks berlebar tetap (32 kolom = kertas 58 mm, 48 kolom = 80 mm) — satu template untuk semua cara cetak:
//   - "agent"  : diubah ke perintah ESC/POS (escpos.ts) dan dikirim mentah ke printer lewat print-agent di PC kasir
//                (huruf bawaan printer → paling tajam dan cepat, kertas dipotong otomatis);
//   - "browser": dokumen HTML dicetak lewat iframe tersembunyi (cadangan; butuh Chrome `--kiosk-printing`).
// "auto" mencoba agent dulu dan jatuh ke browser hanya bila agent tidak berjalan.
import { formatDateTime, formatNumber, t } from '#lib/i18n/index.ts';
import { errorMessage } from '#lib/i18n/errors.ts';
import { sales, type Receipt } from '#lib/sales/api.ts';
import { toCents } from '#lib/pos/money.ts';
import { escposReceipt, toBase64 } from '#lib/pos/escpos.ts';

export type Paper = 58 | 80;
export type PrintMethod = 'auto' | 'agent' | 'browser';
export type ReceiptSettings = { auto: boolean; paper: Paper; method: PrintMethod; cut: boolean; agentUrl: string };
/** title = nama toko (ESC/POS: rata tengah + ukuran ganda bila muat). */
export type ReceiptLine = { text: string; bold?: boolean; big?: boolean; title?: boolean };

export const DEFAULT_AGENT_URL = 'http://127.0.0.1:9100';

const SETTINGS_KEY = 'aciraba.receipt.settings';
const COLS: Record<Paper, number> = { 58: 32, 80: 48 };

/** Pengaturan per perangkat (PC kasir), bukan per akun: tiap komputer bisa punya printer berbeda. */
export function loadReceiptSettings(): ReceiptSettings {
  try {
    const raw = JSON.parse(localStorage.getItem(SETTINGS_KEY) ?? '{}') as Partial<ReceiptSettings>;
    const method: PrintMethod = raw.method === 'agent' || raw.method === 'browser' ? raw.method : 'auto';
    const agentUrl = typeof raw.agentUrl === 'string' && /^http:\/\/(127\.0\.0\.1|localhost)(:\d+)?$/.test(raw.agentUrl) ? raw.agentUrl : DEFAULT_AGENT_URL;
    return { auto: raw.auto !== false, paper: raw.paper === 80 ? 80 : 58, method, cut: raw.cut !== false, agentUrl };
  } catch {
    return { auto: true, paper: 58, method: 'auto', cut: true, agentUrl: DEFAULT_AGENT_URL };
  }
}

export function saveReceiptSettings(s: ReceiptSettings) {
  try {
    localStorage.setItem(SETTINGS_KEY, JSON.stringify(s));
  } catch {
    /* penyimpanan browser tidak tersedia: pengaturan berlaku sampai halaman ditutup */
  }
}

/** Rupiah tanpa simbol; desimal hanya bila ada sen. */
function amt(cents: bigint): string {
  const neg = cents < 0n;
  const abs = neg ? -cents : cents;
  const whole = formatNumber(abs / 100n);
  const frac = abs % 100n;
  const s = frac === 0n ? whole : `${whole}${formatNumber(1.5).charAt(1)}${frac.toString().padStart(2, '0')}`;
  return neg ? `-${s}` : s;
}
const money = (s: string) => amt(toCents(s));

/** "2.000" → "2"; "1.250" → "1,25" mengikuti bahasa aktif. */
function qty(s: string): string {
  const n = Number(s);
  return Number.isFinite(n) ? formatNumber(n, { maximumFractionDigits: 3 }) : s;
}

/** Pecah teks panjang menjadi beberapa baris selebar `w` (per kata; kata yang terlalu panjang dipotong). */
function wrap(text: string, w: number): string[] {
  const out: string[] = [];
  for (const para of text.split('\n')) {
    let cur = '';
    for (const word of para.split(/\s+/).filter(Boolean)) {
      let wd = word;
      while (wd.length > w) {
        if (cur) out.push(cur), (cur = '');
        out.push(wd.slice(0, w));
        wd = wd.slice(w);
      }
      if (!cur) cur = wd;
      else if (cur.length + 1 + wd.length <= w) cur += ' ' + wd;
      else out.push(cur), (cur = wd);
    }
    out.push(cur);
  }
  return out;
}

const center = (s: string, w: number) => ' '.repeat(Math.max(0, Math.floor((w - s.length) / 2))) + s;

/** Label kiri + nilai kanan dalam satu baris; label dipotong bila tidak muat. */
function lr(left: string, right: string, w: number): string {
  const room = w - right.length - 1;
  if (room < 1) return right.padStart(w);
  const l = left.length > room ? left.slice(0, room) : left;
  return l + ' '.repeat(w - l.length - right.length) + right;
}

/** Susun struk menjadi baris teks. `copy` > 0 = cetak ulang ke-n (ditandai di struk). */
export function receiptLines(rc: Receipt, paper: Paper, copy = 0): ReceiptLine[] {
  const w = COLS[paper];
  const { store, sale } = rc;
  const out: ReceiptLine[] = [];
  const push = (text: string, o: Omit<ReceiptLine, 'text'> = {}) => out.push({ text, ...o });
  const rule = () => push('-'.repeat(w));
  const centered = (text: string, o: Omit<ReceiptLine, 'text'> = {}) => wrap(text, w).forEach((l) => push(center(l, w), o));

  centered(store.tenant_name, { bold: true, title: true });
  if (store.outlet_name && store.outlet_name !== store.tenant_name) centered(store.outlet_name);
  if (store.address) centered(store.address);
  if (store.phone) centered(store.phone);
  if (store.header) centered(store.header);
  rule();

  push(lr(t('pos.receipt.no'), sale.doc_no, w));
  push(lr(t('pos.receipt.date'), rc.local_time, w));
  push(lr(t('pos.receipt.cashier'), sale.cashier, w));
  if (sale.member) push(lr(t('pos.receipt.member'), sale.member.name, w));
  if (sale.salesperson) push(lr(t('pos.receipt.salesperson'), sale.salesperson.name, w));
  if (copy > 0) centered(t('pos.receipt.copy', { n: copy }), { bold: true });
  if (sale.status === 'void') centered(t('pos.receipt.void'), { bold: true });
  else if (sale.status === 'superseded') centered(t('pos.receipt.superseded'), { bold: true });
  rule();

  let lineDisc = 0n;
  for (const l of sale.lines) {
    wrap(l.name, w).forEach((s) => push(s));
    push(lr(`  ${qty(l.qty)} ${l.unit} x ${money(l.unit_price)}`, money(l.line_total), w));
    const d = toCents(l.discount);
    if (d > 0n) {
      lineDisc += d;
      push(lr(`  ${t('pos.receipt.lineDiscount')}`, `-${amt(d)}`, w));
    }
  }
  rule();

  push(lr(t('pos.receipt.subtotal'), money(sale.subtotal), w));
  const voucherCents = sale.vouchers.reduce((s, v) => s + toCents(v.amount), 0n);
  const redeem = toCents(sale.redeem_amount);
  const manual = toCents(sale.discount) - voucherCents - redeem;
  if (manual > 0n) push(lr(t('pos.receipt.discount'), `-${amt(manual)}`, w));
  for (const v of sale.vouchers) push(lr(t('pos.receipt.voucher', { code: v.code }), `-${money(v.amount)}`, w));
  if (redeem > 0n) push(lr(t('pos.receipt.redeem', { points: sale.points_redeemed }), `-${amt(redeem)}`, w));
  if (toCents(sale.tax_store) > 0n) push(lr(t('pos.receipt.taxStore', { pct: qty(sale.tax_store_pct) }), money(sale.tax_store), w));
  if (toCents(sale.tax_gov) > 0n) push(lr(t('pos.receipt.taxGov', { pct: qty(sale.tax_gov_pct) }), money(sale.tax_gov), w));
  if (sale.other_costs.length) for (const c of sale.other_costs) push(lr(c.name, money(c.amount), w));
  else if (toCents(sale.other_cost) > 0n) push(lr(t('pos.receipt.otherCost'), money(sale.other_cost), w));
  push(lr(t('pos.receipt.total'), money(sale.total), w), { bold: true, big: true });

  const surcharge = toCents(sale.surcharge);
  if (surcharge > 0n) {
    push(lr(t('pos.receipt.surcharge'), money(sale.surcharge), w));
    push(lr(t('pos.receipt.charged'), amt(toCents(sale.total) + surcharge), w), { bold: true });
  }
  rule();
  for (const p of sale.payments) {
    const fee = p.fee_bearer === 'customer' ? toCents(p.fee) : 0n;
    push(lr(p.method_name, amt(toCents(p.amount) + fee), w));
    if (p.ref_no) push(`  ${p.ref_no}`.slice(0, w));
  }
  if (toCents(sale.receivable) > 0n) {
    push(lr(t('pos.receipt.receivable'), money(sale.receivable), w), { bold: true });
    if (sale.credit?.due_date) push(lr(t('pos.receipt.due'), sale.credit.due_date.split('-').reverse().join('/'), w));
  } else {
    push(lr(t('pos.receipt.change'), money(sale.change), w), { bold: true });
  }
  if (lineDisc + manual + voucherCents + redeem > 0n) push(lr(t('pos.receipt.saved'), amt(lineDisc + manual + voucherCents + redeem), w));
  if (sale.member && sale.points_earned > 0) push(lr(t('pos.receipt.pointsEarned'), `+${sale.points_earned}`, w));

  if (sale.note) {
    rule();
    wrap(sale.note, w).forEach((s) => push(s));
  }
  if (store.footer) {
    rule();
    centered(store.footer);
  }
  return out;
}

const esc = (s: string) => s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!);

/** Dokumen HTML mandiri (tanpa CSS aplikasi) untuk dicetak. */
export function receiptHtml(lines: ReceiptLine[], paper: Paper): string {
  const body = lines
    .map((l) => `<div class="${[l.bold && 'b', l.big && 'g'].filter(Boolean).join(' ')}">${esc(l.text) || '&nbsp;'}</div>`)
    .join('');
  // Lebar cetak efektif printer thermal: ±48 mm (kertas 58) / ±72 mm (kertas 80). Ukuran huruf dihitung agar tepat
  // COLS karakter memenuhi lebar itu (lebar huruf monospace ≈ 0,6 em) — sama seperti font 12×24 printer ESC/POS.
  const width = paper === 80 ? 72 : 48;
  const font = width / COLS[paper] / 0.6;
  return `<!doctype html><html><head><meta charset="utf-8"><title>struk</title><style>
@page { size: ${paper}mm auto; margin: 0; }
html, body { margin: 0; padding: 0; background: #fff; color: #000; }
body { box-sizing: border-box; width: ${paper}mm; padding: 2mm ${(paper - width) / 2}mm 8mm; font: ${font.toFixed(3)}mm/1.3 'Courier New', 'Liberation Mono', ui-monospace, monospace; }
div { white-space: pre; overflow: hidden; }
.b { font-weight: 700; }
.g { transform: scaleY(1.5); transform-origin: 0 50%; margin: 0.8mm 0; }
</style></head><body>${body}</body></html>`;
}

/** Cetak HTML lewat iframe tersembunyi. Di Chrome `print()` menahan sampai cetak selesai/dialog ditutup. */
function printHtml(html: string): Promise<void> {
  return new Promise((resolve) => {
    const frame = document.createElement('iframe');
    frame.setAttribute('aria-hidden', 'true');
    frame.style.cssText = 'position:fixed;right:0;bottom:0;width:0;height:0;border:0;visibility:hidden';
    document.body.appendChild(frame);
    frame.onload = () => {
      try {
        frame.contentWindow?.focus();
        frame.contentWindow?.print();
      } finally {
        // Dibuang belakangan: browser yang mencetak secara asinkron masih membaca dokumen iframe.
        setTimeout(() => frame.remove(), 60_000);
        resolve();
      }
    };
    frame.srcdoc = html;
  });
}

/** Agent tidak bisa dihubungi (tidak berjalan / diblokir browser) — satu-satunya kasus "auto" boleh pindah ke browser. */
export class AgentUnreachable extends Error {}
/** Agent menjawab tetapi printer gagal (mati, kertas habis, nama printer salah). */
export class AgentPrintError extends Error {}

async function agentFetch(url: string, init: RequestInit, timeoutMs: number): Promise<Response> {
  try {
    return await fetch(url, { ...init, mode: 'cors', credentials: 'omit', signal: AbortSignal.timeout(timeoutMs) });
  } catch (e) {
    // TypeError = koneksi ditolak/diblokir; TimeoutError = tidak menjawab. Keduanya: anggap agent tidak ada.
    throw new AgentUnreachable(e instanceof Error ? e.message : String(e));
  }
}

/** Status agent untuk panel pengaturan: printer yang dipakai, atau null bila agent tidak berjalan. */
export async function agentStatus(url = loadReceiptSettings().agentUrl): Promise<{ version: string; printer: string } | null> {
  try {
    const res = await agentFetch(`${url}/status`, { method: 'GET' }, 1500);
    return res.ok ? ((await res.json()) as { version: string; printer: string }) : null;
  } catch {
    return null;
  }
}

async function sendToAgent(url: string, data: Uint8Array): Promise<void> {
  const res = await agentFetch(`${url}/print`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ data: toBase64(data) }) }, 20_000);
  if (res.status === 403) throw new AgentUnreachable('origin'); // agent belum mengizinkan alamat aplikasi ini
  if (!res.ok) {
    const body = (await res.json().catch(() => ({}))) as { error?: string };
    throw new AgentPrintError(body.error ?? `HTTP ${res.status}`);
  }
}

/** Cetak baris struk dengan cara sesuai pengaturan. Mengembalikan cara yang benar-benar dipakai. */
export async function printLines(lines: ReceiptLine[], settings = loadReceiptSettings()): Promise<'agent' | 'browser'> {
  if (settings.method !== 'browser') {
    try {
      await sendToAgent(settings.agentUrl, escposReceipt(lines, { cols: COLS[settings.paper], cut: settings.cut }));
      return 'agent';
    } catch (e) {
      // Printer menolak/gagal → jangan diam-diam pindah ke browser (bisa mencetak dua kali); tampilkan galatnya.
      if (settings.method === 'agent' || !(e instanceof AgentUnreachable)) throw e;
    }
  }
  await printHtml(receiptHtml(lines, settings.paper));
  return 'browser';
}

/**
 * Cetak struk nota. `reprint` = cetak ulang: dicatat dulu di server (audit) sehingga struk menampilkan nomor salinan;
 * bila pencatatan gagal, struk tidak dicetak (galat dilempar ke pemanggil).
 */
export async function printReceipt(saleId: string, opts: { reprint?: boolean } = {}): Promise<'agent' | 'browser'> {
  const settings = loadReceiptSettings();
  const copy = opts.reprint ? (await sales.reprint(saleId)).copy : 0;
  const rc = await sales.receipt(saleId);
  return printLines(receiptLines(rc, settings.paper, copy), settings);
}

/** Struk uji untuk tombol "Tes printer" di pengaturan. */
export function testLines(paper: Paper): ReceiptLine[] {
  const w = COLS[paper];
  return [
    { text: center('ARUS', w), bold: true, title: true },
    { text: center(t('pos.receipt.testTitle'), w) },
    { text: '-'.repeat(w) },
    { text: '1234567890'.repeat(5).slice(0, w) },
    { text: lr(t('pos.receipt.total'), '123.456', w), bold: true, big: true },
    { text: '-'.repeat(w) },
    { text: center(formatDateTime(new Date()), w) }
  ];
}

/** Pesan galat cetak untuk kasir. */
export function printErrorMessage(e: unknown): string {
  if (e instanceof AgentUnreachable) return t('pos.receipt.agentOffline');
  if (e instanceof AgentPrintError) return t('pos.receipt.printerFailed', { error: e.message });
  return t('pos.receipt.printFailed', { error: errorMessage(e) });
}
