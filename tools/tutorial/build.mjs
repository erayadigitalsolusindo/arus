// Merakit tutorial_out/index.html (statis, tanpa server) dari manifest.json + guides.json + screenshot.
// Sidebar mengikuti struktur menu aplikasi (nav.ts): Grup › Menu › Halaman › Panduan.
// Pakai: node build.mjs   → buka tutorial_out/index.html langsung di browser.
import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { guides as defs } from './guide-defs.mjs';
import './guide-defs-more.mjs';
import './guide-defs-core.mjs';
import './guide-defs-kasir.mjs';
import './guide-defs-item.mjs';
import './guide-defs-master.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const OUT = join(root, 'tutorial_out');
const manifest = JSON.parse(readFileSync(join(OUT, 'manifest.json'), 'utf8'));
const notesPath = join(here, 'notes.json'); // penjelasan tulisan tangan per href (opsional)
const notes = existsSync(notesPath) ? JSON.parse(readFileSync(notesPath, 'utf8')) : {};
const gpath = join(OUT, 'guides.json');
const guideData = existsSync(gpath) ? JSON.parse(readFileSync(gpath, 'utf8')).guides : [];

// --- pohon menu dari nav.ts (ARUS + SIAK) ---
const navSrc = readFileSync(join(root, 'source/web/src/lib/nav.ts'), 'utf8');
const idNav = readFileSync(join(root, 'source/web/src/lib/i18n/messages/id/nav.ts'), 'utf8');
const label = (key) => idNav.match(new RegExp(`^\\s*${key}:\\s*'([^']*)'`, 'm'))?.[1] ?? key;
const groups = [];
let grp = null;
let parent = null;
let pendingId = false;
const addEntry = (title, href) => parent.entries.push({ title, href, page: null, guides: [] });
for (const line of navSrc.split('\n')) {
  const g = line.match(/titleKey: 'nav\.group\.(\w+)'/);
  if (g) {
    grp = { title: idNav.match(new RegExp(`\\b${g[1]}:\\s*'([^']*)'`))?.[1] ?? g[1], parents: [] };
    groups.push(grp);
    continue;
  }
  if (!grp) continue;
  const single = line.match(/\{ id: '[^']+', labelKey: 'nav\.(\w+)'(.*)/);
  if (single) {
    parent = { title: label(single[1]), entries: [] };
    grp.parents.push(parent);
    const h = single[2].match(/href: '(\/[^']*)'/);
    if (h) addEntry(label(single[1]), h[1]);
    continue;
  }
  if (/^\s*id: '[^']+',\s*$/.test(line)) { pendingId = true; continue; }
  const lk = line.match(/^\s*labelKey: 'nav\.(\w+)',\s*$/);
  if (pendingId && lk) {
    parent = { title: label(lk[1]), entries: [] };
    grp.parents.push(parent);
    pendingId = false;
    continue;
  }
  const child = line.match(/\{ labelKey: 'nav\.(\w+)'.*?href: '(\/[^']*)'/);
  if (child && parent) addEntry(label(child[1]), child[2]);
}

const entries = groups.flatMap((g) => g.parents.flatMap((p) => p.entries.map((e) => ({ e, p }))));
const byHref = new Map(entries.map(({ e, p }) => [e.href, { e, p }]));
// Kasir Classic tidak ada di nav.ts: jadikan sub-menu tepat di bawah Kasir.
if (!byHref.has('/kasirb') && byHref.has('/kasir')) {
  const k = byHref.get('/kasir');
  const ce = { title: 'Kasir (klasik)', href: '/kasirb', page: null, guides: [] };
  k.p.entries.splice(k.p.entries.indexOf(k.e) + 1, 0, ce);
  byHref.set('/kasirb', { e: ce, p: k.p });
}
const home = { title: 'Mulai', parents: [{ title: 'Masuk', entries: [] }] };
const other = { title: 'Lainnya', parents: [{ title: 'Halaman lain', entries: [] }] };
const alias = {};
// Cari entri menu terdekat untuk href yang tidak ada di menu (mis. /items/new → /items).
const place = (href) => {
  const key = alias[href] ?? href;
  if (byHref.has(key)) return byHref.get(key);
  let best;
  for (const { e, p } of entries) if (key.startsWith(e.href + '/') && (!best || e.href.length > best.e.href.length)) best = { e, p };
  return best;
};

const pages = [];
const guides = [];
if (existsSync(join(OUT, 'desktop/00-login.png'))) {
  pages.push({ href: '/login', title: 'Masuk ke ARUS', desktop: 'desktop/00-login.png', mobile: 'mobile/00-login.png', buttons: [], columns: [], fields: [], note: 'Buka alamat ARUS, isi email dan kata sandi, lalu tekan tombol masuk. Lupa kata sandi? Gunakan tautan "Lupa kata sandi".' });
  const loginEntry = { title: 'Masuk ke ARUS', href: '/login', page: 0, guides: [] };
  home.parents[0].entries.push(loginEntry);
  byHref.set('/login', { e: loginEntry, p: home.parents[0] });
}
for (const m of manifest.pages.filter((x) => x.ok)) {
  const idx = pages.push({ href: m.href, title: m.title, desktop: m.file, mobile: m.mobileFile, heading: m.heading, buttons: m.buttons ?? [], columns: m.columns ?? [], fields: m.fields ?? [], note: notes[m.href] ?? '' }) - 1;
  const hit = byHref.get(m.href);
  if (hit) hit.e.page = idx;
  else {
    const near = place(m.href);
    const entry = { title: m.title, href: m.href, page: idx, guides: [] };
    (near ? near.p : other.parents[0]).entries.push(entry);
  }
}
for (const gd of guideData) {
  const at = defs.find((d) => d.id === gd.id)?.at;
  const idx = guides.push({ id: gd.id, title: gd.title, intro: gd.intro, shots: gd.shots, at }) - 1;
  const hit = at && (byHref.get(at) ?? place(at));
  if (hit) hit.e.guides.push(idx);
  else other.parents[0].entries.push({ title: gd.title, href: at ?? '', page: null, guides: [idx] });
}
// Hanya tampilkan entri yang punya halaman atau panduan.
const tree = [home, ...groups, other]
  .map((g) => ({ ...g, parents: g.parents.map((p) => ({ ...p, entries: p.entries.filter((e) => e.page !== null || e.guides.length) })).filter((p) => p.entries.length) }))
  .filter((g) => g.parents.length);

const html = `<!doctype html>
<html lang="id">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Tutorial ARUS</title>
<style>
:root{--bg:#f6f7f9;--panel:#fff;--ink:#16202e;--mute:#667085;--line:#e4e7ec;--acc:#1d4ed8;--accbg:#eaf0ff;--red:#e11d48}
@media (prefers-color-scheme:dark){:root{--bg:#0e141d;--panel:#151d29;--ink:#e6ebf2;--mute:#8d99ab;--line:#26303f;--acc:#7aa2ff;--accbg:#1b2a4a}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font:15px/1.55 system-ui,Segoe UI,Roboto,sans-serif}
.app{display:grid;grid-template-columns:320px 1fr;min-height:100vh}
nav{background:var(--panel);border-right:1px solid var(--line);padding:16px;position:sticky;top:0;height:100vh;overflow:auto}
nav h1{font-size:17px;margin:0 0 12px}
nav input{width:100%;padding:9px 11px;border:1px solid var(--line);border-radius:8px;background:var(--bg);color:var(--ink);margin-bottom:12px}
nav h2{font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:var(--mute);margin:18px 0 4px}
nav details{margin:1px 0}nav summary{cursor:pointer;padding:6px 9px;border-radius:7px;font-weight:600;font-size:14px;list-style:none}
nav summary::before{content:"▸";display:inline-block;width:14px;color:var(--mute)}nav details[open]>summary::before{content:"▾"}
nav summary:hover{background:var(--bg)}
nav a{display:block;padding:5px 9px;border-radius:7px;color:var(--ink);text-decoration:none;font-size:14px}
nav a:hover{background:var(--bg)}nav a.on{background:var(--accbg);color:var(--acc);font-weight:600}
nav .sub{margin-left:14px}nav .lvl2{margin-left:28px}
nav a.g::before{content:"①";color:var(--red);margin-right:6px;font-size:13px}
main{padding:28px;max-width:1100px;width:100%}
.crumb{color:var(--mute);font-size:13px}
h2.t{font-size:26px;margin:4px 0 6px}
.path{font:12px ui-monospace,Consolas,monospace;color:var(--mute)}
.tabs{display:flex;gap:6px;margin:18px 0 12px}
.tabs button{padding:7px 14px;border:1px solid var(--line);background:var(--panel);color:var(--ink);border-radius:999px;cursor:pointer}
.tabs button.on{background:var(--acc);border-color:var(--acc);color:#fff}
.shot{background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:10px;text-align:center}
.shot img{max-width:100%;border-radius:6px;border:1px solid var(--line)}
.shot.m img{max-width:340px}
.card{background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:16px 18px;margin-top:16px}
.card h3{margin:0 0 8px;font-size:14px}
.chips{display:flex;flex-wrap:wrap;gap:6px}.chips span{background:var(--bg);border:1px solid var(--line);border-radius:999px;padding:3px 10px;font-size:13px}
.step{display:flex;gap:10px;margin:8px 0;align-items:flex-start}.step b{flex:none;width:26px;height:26px;border-radius:50%;background:var(--red);color:#fff;text-align:center;line-height:26px;font-size:14px}
.pn{display:flex;justify-content:space-between;margin-top:22px}
.pn a{color:var(--acc);text-decoration:none;cursor:pointer}
.rel a{color:var(--acc);text-decoration:none;display:block;margin:2px 0}
@media(max-width:800px){.app{grid-template-columns:1fr}nav{position:static;height:auto;max-height:45vh}main{padding:16px}}
</style>
</head>
<body>
<div class="app">
<nav><h1>Tutorial ARUS</h1><input id="q" placeholder="Cari halaman atau panduan…" aria-label="Cari"><div id="menu"></div></nav>
<main id="view"></main>
</div>
<script>
const PAGES=${JSON.stringify(pages)};
const GUIDES=${JSON.stringify(guides)};
const TREE=${JSON.stringify(tree)};
const $=s=>document.querySelector(s);let mode='desktop';
const esc=s=>String(s).replace(/[&<>"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]));
// lokasi (Grup › Menu) tiap halaman/panduan untuk breadcrumb
const LOC={};TREE.forEach(g=>g.parents.forEach(p=>p.entries.forEach(e=>{const l=g.title+' › '+p.title;if(e.page!==null)LOC['p'+e.page]=l;e.guides.forEach(k=>LOC['g'+k]=l)})));
function menu(){const q=$('#q').value.toLowerCase();const open=!!q;let h='';
 for(const g of TREE){let gh='';
  for(const p of g.parents){let eh='';
   for(const e of p.entries){const hitE=!q||e.title.toLowerCase().includes(q)||(p.title.toLowerCase().includes(q));
    const gs=e.guides.filter(k=>!q||hitE||GUIDES[k].title.toLowerCase().includes(q));
    if(!hitE&&!gs.length)continue;
    const cls=p.entries.length>1?'sub':'';
    eh+=(e.page!==null?'<a class="'+cls+'" href="#p'+e.page+'" data-k="p'+e.page+'">'+esc(e.title)+'</a>':'<div class="'+cls+'" style="padding:5px 9px;font-size:14px;color:var(--mute)">'+esc(e.title)+'</div>')
     +gs.map(k=>'<a class="g '+(p.entries.length>1?'lvl2':'sub')+'" href="#g'+k+'" data-k="g'+k+'">'+esc(GUIDES[k].title)+'</a>').join('')}
   if(!eh)continue;
   gh+=p.entries.length===1?eh.replace(/class="sub"/,'class=""'):'<details'+(open?' open':'')+'><summary>'+esc(p.title)+'</summary>'+eh+'</details>'}
  if(gh)h+='<h2>'+esc(g.title)+'</h2>'+gh}
 $('#menu').innerHTML=h||'<p class="path">Tidak ada hasil.</p>';mark()}
function mark(){const k=(location.hash||'#p0').slice(1);document.querySelectorAll('nav a').forEach(a=>{const on=a.dataset.k===k;a.classList.toggle('on',on);if(on){const d=a.closest('details');if(d)d.open=true}})}
function chips(t,a){return a.length?'<div class="card"><h3>'+t+'</h3><div class="chips">'+a.map(x=>'<span>'+esc(x)+'</span>').join('')+'</div></div>':''}
function related(k,type){let out='';
 for(const g of TREE)for(const p of g.parents)for(const e of p.entries){if(type==='p'&&e.page===+k&&e.guides.length)out='<div class="card rel"><h3>Panduan untuk halaman ini</h3>'+e.guides.map(x=>'<a href="#g'+x+'">① '+esc(GUIDES[x].title)+'</a>').join('')+'</div>';
  if(type==='g'&&e.guides.includes(+k)&&e.page!==null)out='<div class="card rel"><h3>Halaman terkait</h3><a href="#p'+e.page+'">'+esc(PAGES[e.page].title)+' <span class="path">'+esc(PAGES[e.page].href)+'</span></a></div>'}
 return out}
function guide(k){const x=GUIDES[k];return '<div class="crumb">'+esc(LOC['g'+k]||'')+'  ·  Panduan</div><h2 class="t">'+esc(x.title)+'</h2><p>'+esc(x.intro)+'</p>'
 +x.shots.map((s,n)=>'<div class="card"><h3>Gambar '+(n+1)+': '+esc(s.title)+'</h3><div class="shot"><img loading="lazy" alt="'+esc(s.title)+'" src="'+s.file+'"></div>'+s.steps.map((t,i)=>'<div class="step"><b>'+(i+1)+'</b><span>'+esc(t)+'</span></div>').join('')+(s.tip?'<p class="path">💡 '+esc(s.tip)+'</p>':'')+'</div>').join('')+related(k,'g')}
function page(i){const p=PAGES[i];
 const intro=p.note||('Halaman <b>'+esc(p.title)+'</b>'+(p.heading&&p.heading!==p.title?' ('+esc(p.heading)+')':'')+'.'+(p.columns.length?' Data ditampilkan dalam tabel dengan kolom di bawah.':'')+(p.buttons.length?' Aksi yang tersedia ada pada tombol di bawah.':''));
 return '<div class="crumb">'+esc(LOC['p'+i]||'')+'</div><h2 class="t">'+esc(p.title)+'</h2><div class="path">'+esc(p.href)+'</div>'
 +'<p>'+(p.note?esc(intro):intro)+'</p>'
 +'<div class="tabs"><button data-m="desktop" class="'+(mode==='desktop'?'on':'')+'">Komputer</button><button data-m="mobile" class="'+(mode==='mobile'?'on':'')+'">HP</button></div>'
 +'<div class="shot '+(mode==='mobile'?'m':'')+'"><img loading="lazy" alt="Tampilan '+esc(p.title)+'" src="'+(mode==='mobile'?p.mobile:p.desktop)+'"></div>'
 +related(i,'p')+chips('Tombol &amp; aksi',p.buttons)+chips('Kolom tabel',p.columns)+chips('Isian formulir',p.fields)}
function show(){const h=(location.hash||'#p0').slice(1);const t=h[0],k=+h.slice(1);
 $('#view').innerHTML=(t==='g'&&GUIDES[k])?guide(k):page(PAGES[k]?k:0);
 document.querySelectorAll('.tabs button').forEach(b=>b.onclick=()=>{mode=b.dataset.m;show()});mark();scrollTo(0,0)}
$('#q').oninput=menu;addEventListener('hashchange',show);menu();show();
</script>
</body>
</html>`;
writeFileSync(join(OUT, 'index.html'), html);
const placed = tree.reduce((n, g) => n + g.parents.reduce((m, p) => m + p.entries.length, 0), 0);
console.log(`index.html: ${pages.length} halaman, ${guides.length} panduan, ${placed} entri menu`);
console.log('Tanpa tempat di menu (Lainnya):', other.parents[0].entries.map((e) => e.title).join(', ') || '-');
