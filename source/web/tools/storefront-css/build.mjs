// Membangun stylesheet Tailwind v4 untuk landing page tenant (server Go tidak bisa mengompilasi Tailwind).
// Hasil: source/backend/internal/storefront/tailwind.css (di-embed ke binary API). Jalankan ulang bila daftar di bawah diubah:
//   bun run build:storefront-css
// Hanya utility yang dihasilkan di sini yang berfungsi pada halaman tenant; CSS tambahan lain ditulis tenant di editor.
import { compile } from '@tailwindcss/node';
import { writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const out = resolve(here, '../../../backend/internal/storefront/tailwind.css');
const webRoot = resolve(here, '../..');

const colors = ['slate', 'gray', 'zinc', 'neutral', 'stone', 'red', 'orange', 'amber', 'yellow', 'lime', 'green', 'emerald', 'teal', 'cyan', 'sky', 'blue', 'indigo', 'violet', 'purple', 'fuchsia', 'pink', 'rose'];
const shades = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950];
const hoverShades = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900];
const spacing = ['0', 'px', '0.5', '1', '1.5', '2', '2.5', '3', '3.5', '4', '5', '6', '7', '8', '9', '10', '11', '12', '14', '16', '20', '24', '28', '32', '36', '40', '44', '48', '52', '56', '60', '64', '72', '80', '96'];
const sizes = ['xs', 'sm', 'base', 'lg', 'xl', '2xl', '3xl', '4xl', '5xl', '6xl', '7xl', '8xl', '9xl'];
const bps = ['sm', 'md', 'lg', 'xl'];
const c = new Set();
const add = (...v) => v.flat().forEach((x) => c.add(x));
const both = (arr, f) => arr.map(f);

// Warna: latar, teks, garis, gradasi, divider, ring; hover untuk latar/teks/garis.
for (const col of colors) {
  for (const s of shades) {
    for (const p of ['bg', 'text', 'border', 'from', 'via', 'to', 'divide', 'ring', 'fill', 'stroke', 'decoration', 'outline', 'placeholder']) add(`${p}-${col}-${s}`);
  }
  for (const s of hoverShades) {
    for (const p of ['bg', 'text', 'border']) add(`hover:${p}-${col}-${s}`);
  }
  for (const s of [500, 600]) add(`focus:ring-${col}-${s}`, `focus:border-${col}-${s}`);
}
for (const base of ['white', 'black', 'transparent', 'current']) {
  for (const p of ['bg', 'text', 'border', 'from', 'via', 'to', 'divide', 'ring', 'fill']) add(`${p}-${base}`);
  add(`hover:bg-${base}`, `hover:text-${base}`);
}
for (const base of ['white', 'black']) for (const a of [5, 10, 20, 30, 40, 50, 60, 70, 80, 90]) add(`bg-${base}/${a}`, `text-${base}/${a}`, `border-${base}/${a}`);
for (const col of colors) for (const a of [10, 20, 30, 50, 70, 90]) add(`bg-${col}-500/${a}`, `bg-${col}-600/${a}`, `bg-${col}-900/${a}`);
for (const col of colors) for (const a of [50, 60, 70, 80, 90]) add(`bg-${col}-950/${a}`, `bg-${col}-50/${a}`, `bg-${col}-100/${a}`);

// Jarak dan ukuran.
for (const s of spacing) {
  for (const p of ['p', 'px', 'py', 'pt', 'pb', 'pl', 'pr', 'ps', 'pe', 'm', 'mx', 'my', 'mt', 'mb', 'ml', 'mr', 'ms', 'me', 'gap', 'gap-x', 'gap-y', 'space-x', 'space-y', 'w', 'h', 'size', 'min-h', 'min-w', 'top', 'bottom', 'left', 'right', 'inset', 'inset-x', 'inset-y', 'basis', 'scroll-m', 'scroll-p']) add(`${p}-${s}`);
  if (s !== '0' && s !== 'px') for (const p of ['m', 'mx', 'my', 'mt', 'mb', 'ml', 'mr', 'top', 'left', 'right', 'bottom', 'inset', 'space-x', 'space-y']) add(`-${p}-${s}`);
}
for (const f of ['1/2', '1/3', '2/3', '1/4', '2/4', '3/4', '1/5', '2/5', '3/5', '4/5', '1/6', '5/6', '1/12', '5/12', '7/12', '11/12', 'full', 'screen', 'svw', 'dvh', 'auto', 'min', 'max', 'fit']) add(`w-${f}`, `h-${f}`, `min-w-${f}`, `min-h-${f}`, `max-h-${f}`, `max-w-${f}`);
add(['p', 'm', 'px', 'py', 'mx', 'my', 'mt', 'mb', 'ml', 'mr', 'pt', 'pb'].map((p) => `${p}-auto`));
add(['xs', 'sm', 'md', 'lg', 'xl', '2xl', '3xl', '4xl', '5xl', '6xl', '7xl', 'prose', 'screen-sm', 'screen-md', 'screen-lg', 'screen-xl', 'none'].map((s) => `max-w-${s}`));
add('container', 'mx-auto', 'ms-auto', 'me-auto');

// Tipografi.
add(sizes.map((s) => `text-${s}`), ['thin', 'extralight', 'light', 'normal', 'medium', 'semibold', 'bold', 'extrabold', 'black'].map((w) => `font-${w}`));
add(['none', 'tight', 'snug', 'normal', 'relaxed', 'loose', '3', '4', '5', '6', '7', '8', '9', '10'].map((l) => `leading-${l}`), ['tighter', 'tight', 'normal', 'wide', 'wider', 'widest'].map((t) => `tracking-${t}`));
add('text-left', 'text-center', 'text-right', 'text-justify', 'uppercase', 'lowercase', 'capitalize', 'normal-case', 'italic', 'not-italic', 'underline', 'no-underline', 'line-through', 'truncate', 'text-ellipsis', 'text-balance', 'text-pretty', 'whitespace-nowrap', 'whitespace-normal', 'break-words', 'break-all', 'antialiased', 'font-sans', 'font-serif', 'font-mono', 'list-none', 'list-disc', 'list-decimal', 'list-inside', 'align-middle', 'underline-offset-4', 'hover:underline', 'hover:no-underline');
add([1, 2, 3, 4, 5, 6].map((n) => `line-clamp-${n}`));

// Tata letak.
add('block', 'inline-block', 'inline', 'flex', 'inline-flex', 'grid', 'inline-grid', 'hidden', 'contents', 'table', 'flow-root', 'flex-row', 'flex-row-reverse', 'flex-col', 'flex-col-reverse', 'flex-wrap', 'flex-nowrap', 'flex-1', 'flex-auto', 'flex-none', 'grow', 'grow-0', 'shrink', 'shrink-0',
  'items-start', 'items-center', 'items-end', 'items-baseline', 'items-stretch', 'justify-start', 'justify-center', 'justify-end', 'justify-between', 'justify-around', 'justify-evenly', 'self-start', 'self-center', 'self-end', 'self-stretch', 'place-items-center', 'place-content-center', 'content-center',
  'relative', 'absolute', 'fixed', 'sticky', 'static', 'z-0', 'z-10', 'z-20', 'z-30', 'z-40', 'z-50', 'overflow-hidden', 'overflow-auto', 'overflow-x-auto', 'overflow-y-auto', 'overflow-visible', 'object-cover', 'object-contain', 'object-center', 'object-top', 'aspect-square', 'aspect-video', 'aspect-auto', 'aspect-4/3', 'aspect-3/2',
  'columns-2', 'columns-3', 'snap-x', 'snap-mandatory', 'snap-start', 'snap-center', 'scroll-smooth', 'sr-only', 'pointer-events-none', 'cursor-pointer', 'select-none', 'inset-0', 'top-0', 'bottom-0', 'left-0', 'right-0');
for (let n = 1; n <= 12; n++) add(`grid-cols-${n}`, `col-span-${n}`, `grid-rows-${n}`, `row-span-${n}`, `order-${n}`);
add('grid-cols-none', 'col-span-full', 'col-auto', 'order-first', 'order-last', 'auto-rows-fr', 'grid-flow-row', 'grid-flow-col');

// Bingkai, bayangan, efek.
const radius = ['none', 'xs', 'sm', 'md', 'lg', 'xl', '2xl', '3xl', '4xl', 'full'];
add('rounded', radius.map((r) => `rounded-${r}`), ['t', 'b', 'l', 'r', 'tl', 'tr', 'bl', 'br', 's', 'e'].flatMap((d) => ['', ...radius.map((r) => `-${r}`)].map((r) => `rounded-${d}${r}`)));
add('border', 'border-0', 'border-2', 'border-4', 'border-8', ['t', 'b', 'l', 'r', 'x', 'y', 's', 'e'].flatMap((d) => [`border-${d}`, `border-${d}-0`, `border-${d}-2`, `border-${d}-4`]), 'border-solid', 'border-dashed', 'border-dotted', 'border-none',
  'divide-x', 'divide-y', 'divide-x-2', 'divide-y-2', 'ring', 'ring-0', 'ring-1', 'ring-2', 'ring-4', 'ring-inset', 'outline-none', 'outline', 'shadow', 'shadow-none', ['xs', 'sm', 'md', 'lg', 'xl', '2xl', 'inner'].map((s) => `shadow-${s}`),
  ['sm', 'md', 'lg', 'xl', '2xl', '3xl'].map((s) => `hover:shadow-${s}`), ['sm', 'md', 'lg'].map((s) => `backdrop-blur-${s}`), 'backdrop-blur', 'blur', 'blur-sm', 'blur-md', 'grayscale', 'sepia', 'invert', 'brightness-90', 'brightness-110');
for (let o = 0; o <= 100; o += 5) add(`opacity-${o}`, `hover:opacity-${o}`);
add('transition', 'transition-all', 'transition-colors', 'transition-opacity', 'transition-transform', 'transition-shadow', [75, 100, 150, 200, 300, 500, 700, 1000].map((d) => `duration-${d}`), ['linear', 'in', 'out', 'in-out'].map((e) => `ease-${e}`),
  'transform', ['50', '75', '90', '95', '100', '105', '110', '125', '150'].flatMap((s) => [`scale-${s}`, `hover:scale-${s}`]), ['1', '2', '3', '6', '12', '45', '90', '180'].map((r) => `rotate-${r}`), ['0', '1', '2', '4', '8'].flatMap((y) => [`hover:-translate-y-${y}`, `-translate-y-${y}`, `translate-y-${y}`]), 'animate-pulse', 'animate-spin', 'animate-bounce', 'animate-ping');
add('bg-gradient-to-t', 'bg-gradient-to-b', 'bg-gradient-to-l', 'bg-gradient-to-r', 'bg-gradient-to-tr', 'bg-gradient-to-br', 'bg-gradient-to-tl', 'bg-gradient-to-bl', 'bg-linear-to-t', 'bg-linear-to-b', 'bg-linear-to-l', 'bg-linear-to-r', 'bg-linear-to-tr', 'bg-linear-to-br', 'bg-linear-to-tl', 'bg-linear-to-bl',
  'bg-cover', 'bg-contain', 'bg-center', 'bg-no-repeat', 'bg-fixed', 'bg-clip-text', 'text-transparent', 'mix-blend-multiply', 'prose');

// Responsif: hanya utility tata letak/tipografi yang lazim.
const responsive = ['block', 'inline-block', 'flex', 'inline-flex', 'grid', 'hidden', 'flex-row', 'flex-col', 'flex-wrap', 'items-center', 'items-start', 'justify-between', 'justify-center', 'justify-start', 'text-left', 'text-center', 'text-right', 'w-full', 'w-auto', 'w-1/2', 'w-1/3', 'w-2/3', 'w-1/4', 'w-3/4', 'mx-auto', 'container', 'order-first', 'order-last', 'col-span-full', 'object-cover', 'sticky', 'relative', 'static', 'aspect-video', 'aspect-square',
  ...sizes.map((s) => `text-${s}`), ...[1, 2, 3, 4, 5, 6, 7, 8, 12].flatMap((n) => [`grid-cols-${n}`, `col-span-${n}`]), ...['0', '1', '2', '3', '4', '5', '6', '8', '10', '12', '16', '20', '24', '28', '32', '36', '40'].flatMap((n) => [`p-${n}`, `px-${n}`, `py-${n}`, `pt-${n}`, `pb-${n}`, `m-${n}`, `mt-${n}`, `mb-${n}`, `gap-${n}`, `space-y-${n}`, `space-x-${n}`, `h-${n}`, `w-${n}`]),
  ...['xs', 'sm', 'md', 'lg', 'xl', '2xl', '3xl', '4xl', '5xl', '6xl', '7xl'].map((s) => `max-w-${s}`), ...['h-screen', 'h-auto', 'h-full', 'min-h-screen'], 'rounded-none', 'rounded-lg', 'rounded-xl', 'rounded-2xl', 'rounded-full'];
for (const bp of bps) for (const u of responsive) add(`${bp}:${u}`);
for (const col of colors) add(`md:bg-${col}-50`, `md:bg-${col}-500`, `md:text-${col}-600`);
add('md:bg-white', 'md:bg-transparent', 'md:border-0', 'md:border-b', 'md:border-t', 'md:flex-1', 'md:shadow-lg');
for (const u of ['first:mt-0', 'last:mb-0', 'last:border-0', 'first:rounded-t-xl', 'last:rounded-b-xl', 'odd:bg-slate-50', 'even:bg-white', 'group', 'group-hover:opacity-100', 'group-hover:scale-105', 'group-hover:text-emerald-600', 'peer', 'focus:outline-none', 'focus:ring-2', 'active:scale-95', 'disabled:opacity-50', 'focus-within:ring-2']) add(u);

// Urutan layer WAJIB theme < base < utilities: preflight (base) tidak boleh menimpa utility seperti text-4xl / p-6.
const source = `@layer theme, base, components, utilities;\n@import "tailwindcss/theme.css" layer(theme);\n@import "tailwindcss/preflight.css" layer(base);\n@import "tailwindcss/utilities.css" layer(utilities);\n`;
const compiler = await compile(source, { base: webRoot, onDependency: () => {} });
const css = compiler.build([...c]);
writeFileSync(out, `/* Dibuat oleh web/tools/storefront-css/build.mjs; jangan diedit tangan. ${c.size} kandidat utility. */\n${css}`);
console.log(`ok ${out} (${(css.length / 1024).toFixed(0)} KB, ${c.size} kandidat)`);
