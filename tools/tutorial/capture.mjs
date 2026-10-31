// Menangkap screenshot semua halaman ARUS (desktop + HP) dan menulis tutorial_out/manifest.json.
// Pakai: BASE_URL=http://localhost:5200 TUT_EMAIL=... TUT_PASSWORD=... node capture.mjs [--only=/items]
// Hanya MEMBACA halaman (tidak mengisi/menyimpan form), jadi aman untuk DB dev.
import { chromium } from 'playwright';
import { readFileSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const OUT = join(here, '..', '..', 'tutorial_out');
const BASE = process.env.BASE_URL ?? 'http://localhost:5200';
const EMAIL = process.env.TUT_EMAIL ?? 'aciraba.id@gmail.com';
const PASSWORD = process.env.TUT_PASSWORD ?? 'Salam1jiwa';
const only = process.argv.find((a) => a.startsWith('--only='))?.slice(7).replace(/^\/?/, '/'); // tanpa garis miring awal juga boleh (Git Bash mengubah path)

// --- daftar halaman dari menu sidebar (nav.ts) + label Indonesia (messages/id/nav.ts) ---
const navSrc = readFileSync(join(root, 'source/web/src/lib/nav.ts'), 'utf8');
const idNav = readFileSync(join(root, 'source/web/src/lib/i18n/messages/id/nav.ts'), 'utf8');
const label = (key) => idNav.match(new RegExp(`^\\s*${key}:\\s*'([^']*)'`, 'm'))?.[1] ?? key;

const pages = [];
const seen = new Set();
let group = '';
for (const line of navSrc.split('\n')) {
  const g = line.match(/titleKey: 'nav\.group\.(\w+)'/);
  if (g) group = idNav.match(new RegExp(`\\b${g[1]}:\\s*'([^']*)'`))?.[1] ?? g[1];
  const m = line.match(/labelKey: 'nav\.(\w+)'.*?href: '(\/[^']*)'/);
  if (m && !seen.has(m[2])) {
    seen.add(m[2]);
    pages.push({ href: m[2], title: label(m[1]), group });
  }
}
// Halaman yang tidak ada di menu / butuh bentuk khusus.
const extras = [
  ['/kasirb', 'Kasir (klasik)', 'Penjualan'],
  ['/items/new', 'Tambah barang', 'Master Data'],
  ['/members/new', 'Tambah member', 'Master Data'],
  ['/sales-returns/new', 'Retur penjualan baru', 'Penjualan'],
  ['/purchase-returns/new', 'Retur pembelian baru', 'Pembelian'],
  ['/accounting/journals/new', 'Jurnal baru', 'SIAK'],
  ['/stock-opname/langsung', 'Stok opname langsung', 'Penyesuaian'],
  ['/connect-claude', 'Hubungkan Claude', 'Sistem']
];
for (const [href, title, g] of extras) if (!seen.has(href)) pages.push({ href, title, group: g });
for (const p of pages.filter((p) => p.href.startsWith('/accounting')))
  if (!p.group || p.group === 'Utama') p.group = 'SIAK';

const slug = (href) => href.replace(/^\//, '').replace(/[\/]+/g, '-') || 'home';
const todo = pages.filter((p) => !only || p.href.startsWith(only));

async function login(page) {
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' });
  await page.locator('input[type="email"]').fill(EMAIL);
  await page.locator('input[type="password"]').fill(PASSWORD);
  await page.locator('button[type="submit"]').click();
  await page.waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 20000 });
}

async function settle(page) {
  await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {});
  await page.waitForTimeout(700);
}

// Teks yang terlihat, dipakai sebagai bahan penjelasan otomatis.
const probe = () => {
  const vis = (el) => {
    const r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0 && getComputedStyle(el).visibility !== 'hidden';
  };
  const txt = (el) => (el.innerText || el.getAttribute('aria-label') || el.getAttribute('title') || '').replace(/\s+/g, ' ').trim();
  const main = document.querySelector('main') ?? document.body;
  const uniq = (a) => [...new Set(a.filter(Boolean))];
  return {
    heading: txt(main.querySelector('h1,h2') ?? document.body).slice(0, 120),
    buttons: uniq([...main.querySelectorAll('button,a.btn,[role="button"]')].filter(vis).map(txt).filter((s) => s.length > 1 && s.length < 40)).slice(0, 14),
    columns: uniq([...main.querySelectorAll('th')].filter(vis).map(txt)).slice(0, 14),
    fields: uniq([...main.querySelectorAll('input,select,textarea')].filter(vis).map((e) => e.getAttribute('placeholder') || e.getAttribute('aria-label') || e.closest('label')?.innerText?.trim() || '')).filter((s) => s.length < 50).slice(0, 14)
  };
};

async function run(kind, viewport, isMobile) {
  const dir = join(OUT, kind);
  mkdirSync(dir, { recursive: true });
  const browser = await chromium.launch();
  const ctx = await browser.newContext({ viewport, isMobile, hasTouch: isMobile, deviceScaleFactor: isMobile ? 2 : 1, locale: 'id-ID', timezoneId: 'Asia/Jakarta' });
  const page = await ctx.newPage();
  const result = [];
  if (!only) {
    // halaman login diambil sebelum masuk
    await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' });
    await settle(page);
    await page.screenshot({ path: join(dir, '00-login.png') });
  }
  await login(page);
  let n = 0;
  for (const p of todo) {
    n++;
    const file = `${String(n).padStart(2, '0')}-${slug(p.href)}.png`;
    const entry = { ...p, file: `${kind}/${file}` };
    try {
      await page.goto(BASE + p.href, { waitUntil: 'domcontentloaded' });
      await settle(page);
      entry.finalPath = new URL(page.url()).pathname;
      entry.ok = entry.finalPath === p.href || entry.finalPath.startsWith(p.href);
      Object.assign(entry, await page.evaluate(probe));
      await page.screenshot({ path: join(dir, file) });
      console.log(`[${kind}] ${entry.ok ? 'ok ' : 'REDIRECT'} ${p.href}`);
    } catch (e) {
      entry.ok = false;
      entry.error = String(e).slice(0, 200);
      console.log(`[${kind}] FAIL ${p.href}: ${entry.error}`);
    }
    result.push(entry);
  }
  await browser.close();
  return result;
}

rmSync(join(OUT, 'desktop'), { recursive: true, force: true });
rmSync(join(OUT, 'mobile'), { recursive: true, force: true });
const desktop = await run('desktop', { width: 1440, height: 900 }, false);
const mobile = await run('mobile', { width: 390, height: 844 }, true);
const manifest = {
  generated: new Date().toISOString(),
  base: BASE,
  pages: desktop.map((d, i) => ({ ...d, mobileFile: mobile[i]?.file }))
};
mkdirSync(OUT, { recursive: true });
writeFileSync(join(OUT, 'manifest.json'), JSON.stringify(manifest, null, 2));
console.log(`Selesai: ${desktop.length} halaman, ${desktop.filter((d) => d.ok).length} ok.`);
