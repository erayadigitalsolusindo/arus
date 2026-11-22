<script lang="ts">
  import { platformApi, type BulkTenantItem, type BulkTenantResult } from '#lib/platform/api.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import type { MessageKey } from '#lib/i18n/types.ts';
  import StatCard from '#lib/platform/StatCard.svelte';

  const CHUNK = 100; // batas server per permintaan; file lebih besar dikirim bertahap
  const DEFAULT_PASSWORD = 'SayaCintaIndonesia1945';
  const HEADER = 'nama_toko,email,nama_pemilik,nama_outlet,telepon';
  const SAMPLE = [
    'Warung Bu Ani,ani.rahayu@gmail.com,Ani Rahayu,Warung Bu Ani Pusat,081234567890',
    'Toko Maju Jaya,budi.santoso@gmail.com,Budi Santoso,,',
    'Kopi Senja,citra.dewi@gmail.com,,Kopi Senja Dago,0812-3456-7891'
  ].join('\n');
  const k = (s: string) => `platform.tenants.bulk.${s}` as MessageKey;

  type Row = { line: number; shop: string; email: string; owner: string; outlet: string; phone: string; dupOf?: number };

  let cells = $state<string[][]>([]); // hasil baca file/tempel: satu array per baris
  let text = $state(''); // isi kotak tempel (sumber CSV)
  let fileName = $state('');
  let dragging = $state(false);
  let busy = $state(false);
  let checking = $state(false);
  let error = $state('');
  let checks = $state<Record<number, string | null>>({}); // nomor baris → kode galat server, null = lolos
  let results = $state<Record<number, BulkTenantResult>>({});
  let done = $state(false);
  let filter = $state<'all' | 'ok' | 'bad'>('all');

  // Pemisah dideteksi dari baris pertama (Excel Indonesia sering titik koma; tempel dari sel = tab).
  function splitLine(l: string, d: string): string[] {
    const out: string[] = [];
    let cur = '', q = false;
    for (let i = 0; i < l.length; i++) {
      const c = l[i];
      if (c === '"') {
        if (q && l[i + 1] === '"') { cur += '"'; i++; } else q = !q;
      } else if (c === d && !q) { out.push(cur.trim()); cur = ''; } else cur += c;
    }
    out.push(cur.trim());
    return out;
  }
  function parseText(s: string): string[][] {
    const lines = s.replace(/^﻿/, '').split(/\r?\n/);
    const first = lines.find((l) => l.trim()) ?? '';
    const d = ['\t', ';', ','].find((c) => first.includes(c)) ?? ',';
    return lines.map((l) => (l.trim() ? splitLine(l, d) : []));
  }

  const rows = $derived.by<Row[]>(() => {
    const seen = new Map<string, number>();
    const out: Row[] = [];
    cells.forEach((c, i) => {
      if (!c.some(Boolean)) return;
      if (out.length === 0 && c[1] && !c[1].includes('@') && i === cells.findIndex((x) => x.some(Boolean))) return; // baris judul
      const r: Row = { line: i + 1, shop: c[0] ?? '', email: (c[1] ?? '').toLowerCase(), owner: c[2] ?? '', outlet: c[3] ?? '', phone: c[4] ?? '' };
      if (r.email && seen.has(r.email)) r.dupOf = seen.get(r.email);
      else if (r.email) seen.set(r.email, r.line);
      out.push(r);
    });
    return out;
  });
  const toItem = (r: Row): BulkTenantItem => ({ business_name: r.shop, email: r.email, owner_name: r.owner, outlet_name: r.outlet, phone: r.phone });

  // Pemeriksaan sungguhan ke server (validasi + email terpakai), ditunda sejenak saat data berubah.
  let timer: ReturnType<typeof setTimeout>;
  let seq = 0;
  $effect(() => {
    const send = rows.filter((r) => !r.dupOf);
    clearTimeout(timer);
    if (done) return; // sesudah proses, status baris dibaca dari `results`
    checks = {};
    if (send.length === 0) { checking = false; return; }
    checking = true;
    const my = ++seq;
    timer = setTimeout(async () => {
      try {
        const next: Record<number, string | null> = {};
        for (let i = 0; i < send.length; i += CHUNK) {
          const part = send.slice(i, i + CHUNK);
          const res = (await platformApi.bulkCreateTenants(part.map(toItem), true)).results;
          part.forEach((r, j) => (next[r.line] = res[j]?.ok ? null : (res[j]?.error ?? 'INTERNAL')));
          if (my !== seq) return;
        }
        checks = next;
        error = '';
      } catch (err) {
        if (my === seq) error = errorMessage(err) || t(k('checkFail'));
      } finally {
        if (my === seq) checking = false;
      }
    }, 450);
    return () => clearTimeout(timer);
  });

  function problem(r: Row, code: string | null | undefined = checks[r.line]): string | null {
    if (r.dupOf) return t(k('dupe'), { line: r.dupOf });
    const c = code;
    if (!c) return null;
    if (c === 'EMAIL_TAKEN') return t(k('taken'));
    if (c.startsWith('INVALID:')) {
      const [, f, code] = c.split(':');
      return t(k('invalid'), { field: t(k('field' + f.charAt(0).toUpperCase() + f.slice(1))), reason: t(k('r' + code)) });
    }
    return t(k('internal'));
  }
  const status = (r: Row) => {
    const res = results[r.line];
    if (res) return res.ok ? { s: 'ok', note: t(k('noteCreated')) } : { s: 'bad', note: problem(r, res.error ?? 'INTERNAL') ?? t(k('internal')) };
    const p = problem(r);
    if (p) return { s: 'fix', note: p };
    return checks[r.line] === null ? { s: 'ready', note: t(k('noteReady')) } : { s: 'wait', note: t(k('checking')) };
  };

  const valid = $derived(rows.filter((r) => checks[r.line] === null && !r.dupOf));
  const invalid = $derived(rows.filter((r) => r.dupOf || (checks[r.line] ?? null) !== null));
  const dupes = $derived(rows.filter((r) => r.dupOf).length);
  const okCount = $derived(Object.values(results).filter((r) => r.ok).length);
  const failCount = $derived(Object.values(results).filter((r) => !r.ok).length);
  const shown = $derived(done ? rows.filter((r) => filter === 'all' || (filter === 'ok') === !!results[r.line]?.ok) : rows);

  function load(c: string[][], name = '', raw?: string) {
    cells = c;
    text = raw ?? c.map((r) => r.join(',')).join('\n');
    fileName = name;
    results = {};
    done = false;
    error = '';
  }
  async function onFile(f?: File | null) {
    if (!f) return;
    try {
      if (/\.xlsx$/i.test(f.name)) {
        const { readSheet } = await import('read-excel-file/browser');
        const sheet = await readSheet(f);
        // Excel menyimpan telepon sebagai angka (0 depan hilang): kembalikan 0 lokal.
        const c = sheet.map((row) => [0, 1, 2, 3, 4].map((i) => {
          const v = row[i];
          if (v == null) return '';
          return i === 4 && typeof v === 'number' ? (String(v).startsWith('62') ? '+' : '0') + String(v) : String(v).trim();
        }));
        load(c, f.name);
      } else {
        const raw = await f.text();
        load(parseText(raw), f.name, raw);
      }
    } catch {
      error = t(k('readFail'));
    }
  }

  function download(name: string, body: string) {
    const a = document.createElement('a');
    a.href = URL.createObjectURL(new Blob(['﻿' + body], { type: 'text/csv;charset=utf-8' }));
    a.download = name;
    a.click();
    URL.revokeObjectURL(a.href);
  }
  const csvCell = (v: string | number) => (/[",\n;]/.test(String(v)) ? `"${String(v).replace(/"/g, '""')}"` : String(v));
  function exportResult() {
    const lines = rows.map((r) => [r.line, r.shop, r.email, t(k('st' + ({ ok: 'Ok', bad: 'Fail', fix: 'Fix', ready: 'Ready', wait: 'Wait' } as const)[status(r).s as 'ok'])), status(r).note].map(csvCell).join(','));
    download('hasil-daftar-massal.csv', ['baris,nama_toko,email,status,keterangan', ...lines].join('\n'));
  }

  async function submit() {
    if (!valid.length || busy) return;
    busy = true;
    error = '';
    try {
      const next: Record<number, BulkTenantResult> = {};
      for (let i = 0; i < valid.length; i += CHUNK) {
        const part = valid.slice(i, i + CHUNK);
        const res = (await platformApi.bulkCreateTenants(part.map(toItem))).results;
        part.forEach((r, j) => (next[r.line] = res[j] ?? { row: j, email: r.email, ok: false, error: 'INTERNAL' }));
        results = { ...next }; // tampilkan progres tiap potongan
      }
      done = true;
      filter = 'all';
    } catch (err) {
      error = errorMessage(err);
      done = Object.keys(results).length > 0;
    } finally {
      busy = false;
    }
  }

  const tone = { ok: 'badge-success', bad: 'badge-danger', fix: 'badge-danger', ready: 'badge-info', wait: 'badge-warning' } as const;
  const label = { ok: 'stOk', bad: 'stFail', fix: 'stFix', ready: 'stReady', wait: 'stWait' } as const;
</script>

<div class="flex flex-wrap items-end justify-between gap-3">
  <div>
    <h1 class="font-display font-bold text-[22px] tracking-tight">{t(k('title'))}</h1>
    <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)] max-w-2xl">{t(k('subtitle'))}</p>
  </div>
  <a href="/platform/tenants" class="btn btn-outline !text-[12.5px]"><i class="icon-chevron-left text-[13px]"></i>{t(k('back'))}</a>
</div>

<div class="grid grid-cols-2 xl:grid-cols-4 gap-3">
  <StatCard label={t(k('statRows'))} value={String(rows.length)} icon="store" tone="primary" hint={fileName || t(k('statNone'))} />
  <StatCard label={done ? t(k('statOk')) : t(k('statReady'))} value={String(done ? okCount : valid.length)} icon="circle-check" tone="success" hint={done ? t(k('statOkHint'), { count: Object.keys(results).length }) : t(k('statReadyHint'))} />
  <StatCard label={done ? t(k('statFail')) : t(k('statFix'))} value={String(done ? failCount : invalid.length)} icon="circle-alert" tone="danger" hint={done ? t(k('statFailHint')) : t(k('statFixHint'))} />
  <StatCard label={t(k('statDup'))} value={String(dupes)} icon="users" tone="warning" hint={t(k('statDupHint'))} />
</div>

<div class="grid gap-4 xl:grid-cols-3">
  <div class="xl:col-span-2 surface-card space-y-3">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h2 class="font-semibold text-[14px]"><span class="inline-grid size-6 place-items-center rounded-full bg-[var(--color-primary-50)] text-[var(--color-primary-700)] text-[12px] me-1.5">1</span>{t(k('step1'))}</h2>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-outline !text-[12px]" onclick={() => download('template-tenant.csv', HEADER + '\n' + SAMPLE)}><i class="icon-download text-[13px]"></i>{t(k('template'))}</button>
        <button type="button" class="btn btn-outline !text-[12px]" onclick={() => load(parseText(HEADER + '\n' + SAMPLE), 'contoh', HEADER + '\n' + SAMPLE)}>{t(k('sample'))}</button>
        {#if rows.length}<button type="button" class="btn btn-outline !text-[12px]" onclick={() => load([], '', '')}>{t(k('clear'))}</button>{/if}
      </div>
    </div>

    <label
      class="flex flex-col items-center justify-center gap-1 rounded-xl border-2 border-dashed px-4 py-6 text-center cursor-pointer transition-colors {dragging ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-50)]' : 'border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]'}"
      ondragover={(e) => { e.preventDefault(); dragging = true; }}
      ondragleave={() => (dragging = false)}
      ondrop={(e) => { e.preventDefault(); dragging = false; void onFile(e.dataTransfer?.files?.[0]); }}
    >
      <i class="icon-download text-[22px] text-[var(--text-tertiary)]"></i>
      <span class="text-[13px] font-semibold">{t(k('drop'))}</span>
      <span class="text-[11.5px] text-[var(--text-tertiary)]">{fileName ? t(k('loaded'), { name: fileName }) : t(k('dropHint'))}</span>
      <input type="file" accept=".csv,.txt,.xlsx,text/csv" class="sr-only" onchange={(e) => onFile(e.currentTarget.files?.[0])} />
    </label>

    <details open={!rows.length}>
      <summary class="cursor-pointer text-[12px] font-semibold text-[var(--text-tertiary)]">{t(k('paste'))}</summary>
      <textarea
        value={text}
        oninput={(e) => load(parseText(e.currentTarget.value), '', e.currentTarget.value)}
        rows="6"
        class="mt-2 w-full field-control font-mono text-[12px]"
        aria-label={t(k('step1'))}
        placeholder={HEADER + '\n' + SAMPLE}
      ></textarea>
    </details>

    {#if error}<div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>{/if}
  </div>

  <div class="surface-card space-y-3 text-[12.5px]">
    <h2 class="font-semibold text-[14px]">{t(k('colTitle'))}</h2>
    <table class="w-full text-[12px]">
      <thead>
        <tr class="text-[10.5px] uppercase tracking-wider text-[var(--text-tertiary)]"><th class="py-1 text-start">{t(k('colName'))}</th><th class="py-1 text-start">{t(k('colRequired'))}</th><th class="py-1 text-start">{t(k('colDefault'))}</th></tr>
      </thead>
      <tbody>
        {#each [['nama_toko', true, 'defNone'], ['email', true, 'defNone'], ['nama_pemilik', false, 'defOwner'], ['nama_outlet', false, 'defOutlet'], ['telepon', false, 'defPhone']] as [name, req, def] (name)}
          <tr class="border-t border-[var(--border-subtle)]"><td class="py-1.5 font-mono">{name}</td><td class="py-1.5">{req ? t(k('yes')) : t(k('no'))}</td><td class="py-1.5 text-[var(--text-tertiary)]">{t(k(String(def)))}</td></tr>
        {/each}
      </tbody>
    </table>
    <p class="text-[11.5px] text-[var(--text-tertiary)]">{t(k('colNote'))}</p>

    <h2 class="font-semibold text-[14px] pt-1">{t(k('exampleTitle'))}</h2>
    <pre class="rounded-lg bg-[var(--surface-sunken)] p-2.5 text-[11px] leading-relaxed overflow-x-auto scroll-thin">{HEADER}
{SAMPLE}</pre>

    <h2 class="font-semibold text-[14px] pt-1">{t(k('afterTitle'))}</h2>
    <ul class="list-disc ps-4 space-y-1 text-[var(--text-secondary,inherit)]">
      <li>{t(k('after1'))}</li>
      <li>{t(k('after2'))}</li>
      <li>{t(k('after3'), { password: DEFAULT_PASSWORD })}</li>
      <li>{t(k('after4'))}</li>
    </ul>
  </div>
</div>

<div class="surface-card !p-0 overflow-hidden">
  <div class="p-3 sm:p-4 flex flex-wrap items-center justify-between gap-3 border-b border-[var(--border-subtle)]">
    <h2 class="font-semibold text-[14px]"><span class="inline-grid size-6 place-items-center rounded-full bg-[var(--color-primary-50)] text-[var(--color-primary-700)] text-[12px] me-1.5">2</span>{done ? t(k('stepResult')) : t(k('step2'))}</h2>
    <div class="flex flex-wrap items-center gap-2">
      {#if done}
        {#each [['all', 'fAll'], ['ok', 'fOk'], ['bad', 'fBad']] as [f, l] (f)}
          <button type="button" class="btn !text-[12px] {filter === f ? 'btn-primary' : 'btn-outline'}" onclick={() => (filter = f as typeof filter)}>{t(k(l))}</button>
        {/each}
        <button type="button" class="btn btn-outline !text-[12px]" onclick={exportResult}><i class="icon-download text-[13px]"></i>{t(k('export'))}</button>
      {:else}
        {#if checking}<span class="text-[12px] text-[var(--text-tertiary)]">{t(k('checking'))}</span>
        {:else if invalid.length}<span class="text-[12px] text-[var(--text-tertiary)]">{t(k('skipped'), { count: invalid.length })}</span>{/if}
        <button type="button" class="btn btn-primary disabled:opacity-60" disabled={busy || checking || !valid.length} onclick={submit}>
          {busy ? t(k('busy')) : t(k('submit'), { count: valid.length })}
        </button>
      {/if}
    </div>
  </div>

  <div class="overflow-x-auto scroll-thin">
    <table class="w-full text-[12.5px] min-w-[860px]">
      <thead>
        <tr class="text-[11px] uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
          {#each ['thRow', 'thShop', 'thEmail', 'thOwner', 'thOutlet', 'thPhone', 'thStatus'] as h (h)}
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t(k(h))}</th>
          {/each}
        </tr>
      </thead>
      <tbody>
        {#each shown as r (r.line)}
          {@const s = status(r)}
          <tr class="border-t border-[var(--border-subtle)] align-top">
            <td class="px-4 py-2.5 tabular-nums text-[var(--text-tertiary)]">{r.line}</td>
            <td class="px-4 py-2.5 font-semibold">{r.shop || '—'}</td>
            <td class="px-4 py-2.5 font-mono text-[12px]">{r.email || '—'}</td>
            <td class="px-4 py-2.5">{r.owner || r.email.split('@')[0] || '—'}{#if !r.owner}<span class="text-[10.5px] text-[var(--text-tertiary)]"> {t(k('auto'))}</span>{/if}</td>
            <td class="px-4 py-2.5">{r.outlet || r.shop || '—'}{#if !r.outlet}<span class="text-[10.5px] text-[var(--text-tertiary)]"> {t(k('auto'))}</span>{/if}</td>
            <td class="px-4 py-2.5 font-mono text-[12px]">{r.phone || '—'}</td>
            <td class="px-4 py-2.5">
              <span class="inline-flex rounded-full px-2.5 py-0.5 text-[11px] font-semibold {tone[s.s as keyof typeof tone]}">{t(k(label[s.s as keyof typeof label]))}</span>
              <span class="block mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{s.note}</span>
            </td>
          </tr>
        {:else}
          <tr>
            <td colspan="7" class="px-4 py-14 text-center">
              <span class="mx-auto mb-3 grid size-12 place-items-center rounded-full bg-[var(--surface-sunken)] text-[var(--text-tertiary)]"><i class="icon-store text-[22px]"></i></span>
              <p class="font-semibold">{t(k('emptyTitle'))}</p>
              <p class="text-[12px] text-[var(--text-tertiary)] mt-1">{t(k('emptyHint'))}</p>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</div>
