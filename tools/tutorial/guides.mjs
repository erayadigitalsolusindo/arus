// Panduan berlangkah: menjalankan alur nyata di aplikasi dan memotret layar dengan badge angka (1, 2, 3 …)
// di elemen yang dimaksud. MENULIS data contoh ke tenant dev (barang, member, transaksi), sesuai izin pengguna.
// Pakai: node guides.mjs [--only=tambah-barang]   → tutorial_out/guides.json + tutorial_out/guides/<id>/NN.png
import { chromium } from 'playwright';
import { mkdirSync, writeFileSync, rmSync, readFileSync, existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { guides } from './guide-defs.mjs';
import './guide-defs-more.mjs';
import './guide-defs-core.mjs';
import './guide-defs-kasir.mjs';
import './guide-defs-item.mjs';
import './guide-defs-master.mjs';

const OUT = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'tutorial_out');
const BASE = process.env.BASE_URL ?? 'http://localhost:5200';
const EMAIL = process.env.TUT_EMAIL ?? 'aciraba.id@gmail.com';
const PASSWORD = process.env.TUT_PASSWORD ?? 'Salam1jiwa';
const only = process.argv.find((a) => a.startsWith('--only='))?.slice(7);

const settle = async (page) => {
  await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {});
  await page.waitForTimeout(600);
};

// Pasang outline + badge angka pada tiap target; mengembalikan teks langkah yang benar-benar ditemukan.
async function mark(page, marks) {
  const found = [];
  for (const m of marks) {
    const loc = page.locator(m.sel).locator('visible=true').first();
    const box = (await loc.count()) ? await loc.boundingBox() : null;
    if (box && box.y >= 0 && box.y + box.height <= page.viewportSize().height) found.push({ ...m, rect: box });
    else console.log(`   ! target tidak terlihat di layar: ${m.sel}`);
  }
  await page.evaluate(
    ({ items }) => {
      document.getElementById('__marks')?.remove();
      const layer = document.createElement('div');
      layer.id = '__marks';
      layer.style.cssText = 'position:fixed;inset:0;pointer-events:none;z-index:2147483647';
      items.forEach((it, i) => {
        const r = it.rect;
        const box = document.createElement('div');
        box.style.cssText = `position:fixed;left:${r.x - 3}px;top:${r.y - 3}px;width:${r.width + 6}px;height:${r.height + 6}px;border:2.5px solid #e11d48;border-radius:8px;box-shadow:0 0 0 3px rgba(225,29,72,.18)`;
        const b = document.createElement('div');
        b.textContent = String(i + 1);
        b.style.cssText = `position:fixed;left:${Math.max(2, r.x - 14)}px;top:${Math.max(2, r.y - 14)}px;width:26px;height:26px;border-radius:50%;background:#e11d48;color:#fff;font:700 14px/26px system-ui,sans-serif;text-align:center;box-shadow:0 2px 6px rgba(0,0,0,.35)`;
        layer.append(box, b);
      });
      document.body.append(layer);
    },
    { items: found }
  );
  return found;
}

async function run(kind, viewport, isMobile) {
  const browser = await chromium.launch();
  const ctx = await browser.newContext({ viewport, isMobile, hasTouch: isMobile, deviceScaleFactor: isMobile ? 2 : 1, locale: 'id-ID', timezoneId: 'Asia/Jakarta' });
  const authPage = await ctx.newPage();
  const page0 = authPage;
  const anonCtx = await browser.newContext({ viewport, locale: 'id-ID', timezoneId: 'Asia/Jakarta' });
  const anonPage = await anonCtx.newPage();
  const page = authPage;
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' });
  await page.locator('input[type="email"]').fill(EMAIL);
  await page.locator('input[type="password"]').fill(PASSWORD);
  await page.locator('button[type="submit"]').click();
  await page.waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 20000 });

  const result = [];
  for (const g of guides.filter((g) => !only || only.split(',').some((o) => (o.endsWith('*') ? g.id.startsWith(o.slice(0, -1)) : g.id === o)))) {
    const page = g.anon ? anonPage : page0;
    const dir = join(OUT, 'guides', kind, g.id);
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const entry = { id: g.id, title: g.title, group: g.group, intro: g.intro, shots: [] };
    let n = 0;
    try {
      for (const s of g.shots) {
        n++;
        if (s.goto) {
          await page.goto(BASE + s.goto, { waitUntil: 'domcontentloaded' });
          await settle(page);
        }
        await page.setViewportSize({ width: s.width ?? viewport.width, height: s.height ?? viewport.height });
        if (s.before) {
          await s.before(page);
          await settle(page);
        }
        await page.evaluate(() => document.querySelectorAll('*').forEach((e) => e.scrollLeft && (e.scrollLeft = 0)));
        const found = s.marks?.length ? await mark(page, s.marks) : [];
        const file = `${String(n).padStart(2, '0')}.png`;
        await page.screenshot({ path: join(dir, file) });
        await page.evaluate(() => document.getElementById('__marks')?.remove());
        entry.shots.push({ title: s.title, file: `guides/${kind}/${g.id}/${file}`, steps: found.map((m) => m.text), tip: s.tip });
        if (s.after) {
          await s.after(page);
          await settle(page);
        }
      }
      console.log(`[${kind}] ok   ${g.id}`);
    } catch (e) {
      console.log(`[${kind}] FAIL ${g.id}: ${String(e).slice(0, 300)}`);
      entry.error = String(e).slice(0, 300);
    }
    result.push(entry);
  }
  await browser.close();
  return result;
}

const desktop = await run('desktop', { width: 1440, height: 900 }, false);
mkdirSync(OUT, { recursive: true });
const outFile = join(OUT, 'guides.json');
// Gabung dengan hasil sebelumnya agar --only tidak menghapus panduan lain; urutan mengikuti guide-defs.
const prev = existsSync(outFile) ? JSON.parse(readFileSync(outFile, 'utf8')).guides : [];
const byId = new Map([...prev, ...desktop.filter((d) => !d.error)].map((x) => [x.id, x]));
const merged = guides.map((x) => byId.get(x.id)).filter(Boolean);
writeFileSync(outFile, JSON.stringify({ generated: new Date().toISOString(), guides: merged }, null, 2));
console.log(`Selesai: ${desktop.filter((d) => !d.error).length}/${desktop.length} panduan.`);
