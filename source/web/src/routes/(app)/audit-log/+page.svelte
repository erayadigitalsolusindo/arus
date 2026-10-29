<script lang="ts">
  import Select from '#lib/components/Select.svelte';
  import DatePicker from '#lib/components/DatePicker.svelte';
  import { onMount } from 'svelte';
  import { fly } from 'svelte/transition';
  import { audit as api, type AuditItem } from '#lib/audit/api.ts';
  import { t, tryT, formatNumber, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const ENTITIES = ['user', 'role', 'outlet', 'session', 'tenant', 'unit', 'category', 'brand', 'principal', 'supplier', 'item'] as const;
  const ACTIONS = [
    'auth.register', 'auth.login', 'auth.password_reset', 'auth.password_change', 'auth.email_verified', 'auth.outlet_switch',
    'user.create', 'user.update', 'user.password_reset', 'user.email_verify', 'role.create', 'role.update', 'role.delete', 'outlet.create', 'outlet.update', 'platform.impersonate', 'platform.tenant_status'
  ] as const;

  type Level = 'info' | 'warn' | 'crit';

  let entity = $state('');
  let action = $state('');
  let from = $state('');
  let to = $state('');

  let items = $state<AuditItem[]>([]);
  let cursor = $state('');
  let loading = $state(true);
  let error = $state('');
  let open = $state<Record<number, boolean>>({});
  let query = $state('');
  let level = $state<'all' | Level>('all');
  let live = $state(false);
  let wrap = $state(false);
  let copiedId = $state(0);
  let fresh = $state<Set<number>>(new Set());

  const reduce = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;

  // Tanggal (YYYY-MM-DD, zona waktu pengguna) → batas RFC3339. "Sampai" inklusif: awal hari berikutnya.
  const startOfDay = (d: string) => (d ? new Date(`${d}T00:00:00`).toISOString() : '');
  const endExclusive = (d: string) => {
    if (!d) return '';
    const x = new Date(`${d}T00:00:00`);
    x.setDate(x.getDate() + 1);
    return x.toISOString();
  };

  async function load(more = false) {
    loading = true;
    error = '';
    try {
      const page = await api.list({ entity, action, from: startOfDay(from), to: endExclusive(to), cursor: more ? cursor : '', limit: 50 });
      items = more ? [...items, ...page.items] : page.items;
      cursor = page.next_cursor;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  let timer: ReturnType<typeof setInterval> | undefined;
  onMount(() => {
    void load();
    return () => clearInterval(timer);
  });

  // Live tail: periksa entri baru tiap 5 detik (halaman pertama dengan filter yang sama), taruh di atas.
  $effect(() => {
    clearInterval(timer);
    if (!live) return;
    timer = setInterval(async () => {
      try {
        const page = await api.list({ entity, action, from: startOfDay(from), to: endExclusive(to), limit: 50 });
        const known = new Set(items.map((i) => i.id));
        const add = page.items.filter((i) => !known.has(i.id));
        if (add.length) {
          items = [...add, ...items];
          fresh = new Set(add.map((i) => i.id));
          setTimeout(() => (fresh = new Set()), 2500);
        }
      } catch {
        /* jaringan sesaat gagal: dicoba lagi pada putaran berikutnya */
      }
    }, 5000);
    return () => clearInterval(timer);
  });

  function resetFilters() {
    entity = action = from = to = '';
    void load();
  }

  const actionLabel = (a: string) => tryT(`audit.actions.${a.replace('.', '_')}`) ?? a;
  const entityLabel = (e: string) => tryT(`audit.entities.${e}`) ?? e;

  function levelOf(it: AuditItem): Level {
    const a = it.action;
    if (a.includes('password') || a.endsWith('_delete') || (a.includes('tenant_status') && it.details?.active === false)) return 'crit';
    if (a.includes('impersonate') || a.includes('status') || a.includes('void') || a.includes('override') || a.includes('line_discount') || a.includes('edit_window') || a.includes('superseded')) return 'warn';
    return 'info';
  }
  const levelLabel: Record<Level, string> = { info: 'INFO', warn: 'WARN', crit: 'CRIT' };

  const pad = (n: number, w = 2) => String(n).padStart(w, '0');
  function ts(iso: string) {
    const d = new Date(iso);
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`;
  }
  const compact = (o: Record<string, unknown>) =>
    Object.entries(o ?? {})
      .map(([k, v]) => `${k}=${typeof v === 'object' && v !== null ? JSON.stringify(v) : String(v)}`)
      .join(' ');
  const actor = (it: AuditItem) => it.actor_name || t('audit.system');
  const lineText = (it: AuditItem) =>
    `${ts(it.created_at)} ${levelLabel[levelOf(it)]} ${it.action} actor="${actor(it)}" entity=${it.entity}${it.entity_id ? `#${it.entity_id}` : ''} ip=${it.ip || '-'} ${compact(it.details)}`.trim();

  const shown = $derived.by(() => {
    const q = query.trim().toLowerCase();
    return items.filter((it) => (level === 'all' || levelOf(it) === level) && (!q || lineText(it).toLowerCase().includes(q) || actionLabel(it.action).toLowerCase().includes(q)));
  });
  const counts = $derived.by(() => {
    const c: Record<Level, number> = { info: 0, warn: 0, crit: 0 };
    for (const it of items) c[levelOf(it)]++;
    return c;
  });
  const actors = $derived(new Set(items.map((i) => i.actor_name).filter(Boolean)).size);

  async function copyLine(it: AuditItem) {
    try {
      await navigator.clipboard.writeText(lineText(it));
      copiedId = it.id;
      setTimeout(() => (copiedId = 0), 1200);
    } catch {
      /* clipboard diblokir */
    }
  }

  function exportLog() {
    const body = shown.map((it) => JSON.stringify({ id: it.id, time: it.created_at, level: levelOf(it), action: it.action, actor: it.actor_name, entity: it.entity, entity_id: it.entity_id, ip: it.ip, request_id: it.request_id, details: it.details })).join('\n');
    const url = URL.createObjectURL(new Blob([body], { type: 'application/x-ndjson' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = `audit-${new Date().toISOString().slice(0, 10)}.ndjson`;
    a.click();
    URL.revokeObjectURL(url);
  }

  // Sorot sintaks JSON ringan untuk panel detail.
  function hl(o: unknown) {
    const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    return esc(JSON.stringify(o, null, 2)).replace(
      /("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?)/g,
      (_m, str, colon, kw, num) => (str ? (colon ? `<span class="k">${str}</span>${colon}` : `<span class="s">${str}</span>`) : kw ? `<span class="b">${kw}</span>` : `<span class="n">${num}</span>`)
    );
  }
  const hasDetails = (o: Record<string, unknown>) => Object.keys(o ?? {}).length > 0;
  const allOpen = $derived(shown.length > 0 && shown.filter((i) => hasDetails(i.details)).every((i) => open[i.id]));
  function toggleAll() {
    const next = !allOpen;
    for (const it of shown) if (hasDetails(it.details)) open[it.id] = next;
  }

  const labelClass = 'text-[11px] font-semibold uppercase tracking-wide mb-1 block text-[var(--text-tertiary)]';
</script>

<svelte:head><title>{t('audit.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('audit.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('audit.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex items-start gap-3.5">
    <span class="grid size-12 shrink-0 place-items-center rounded-2xl bg-[#0b1220] text-[#4ade80] shadow-sm font-mono text-[15px] font-bold">&gt;_</span>
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('audit.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('audit.subtitle')}</p>
    </div>
  </div>

  <div class="grid grid-cols-2 lg:grid-cols-4 gap-3">
    {#each [['all', formatNumber(items.length), t('audit.sumEntries'), '#94a3b8'], ['info', formatNumber(counts.info), 'INFO', '#38bdf8'], ['warn', formatNumber(counts.warn), 'WARN', '#fbbf24'], ['crit', formatNumber(counts.crit), 'CRIT', '#f87171']] as const as [lv, n, lab, col] (lv)}
      <button type="button" class="stat {level === lv ? 'on' : ''}" onclick={() => (level = lv as typeof level)} aria-pressed={level === lv}>
        <span class="stat-dot" style="background:{col}"></span>
        <span class="text-start">
          <span class="block text-[10.5px] font-bold uppercase tracking-wider text-[var(--text-tertiary)]">{lab}</span>
          <span class="block font-display font-bold text-[20px] tabular-nums leading-tight">{n}</span>
        </span>
      </button>
    {/each}
  </div>

  <form class="surface-card p-4 grid grid-cols-2 lg:grid-cols-6 gap-3 items-end" onsubmit={(e) => (e.preventDefault(), load())}>
    <div>
      <label for="f-entity" class={labelClass}>{t('audit.entity')}</label>
      <Select id="f-entity" bind:value={entity} options={[{ value: '', label: t('audit.all') }, ...ENTITIES.map((e) => ({ value: e as string, label: entityLabel(e) }))]} />
    </div>
    <div class="lg:col-span-2">
      <label for="f-action" class={labelClass}>{t('audit.action')}</label>
      <Select id="f-action" bind:value={action} options={[{ value: '', label: t('audit.all') }, ...ACTIONS.map((a) => ({ value: a as string, label: actionLabel(a) }))]} />
    </div>
    <div>
      <label for="f-from" class={labelClass}>{t('audit.from')}</label>
      <DatePicker id="f-from" bind:value={from} max={to || undefined} />
    </div>
    <div>
      <label for="f-to" class={labelClass}>{t('audit.to')}</label>
      <DatePicker id="f-to" bind:value={to} min={from || undefined} />
    </div>
    <div class="flex gap-2">
      <button type="submit" class="btn btn-primary !text-[12.5px] flex-1" disabled={loading}>{t('audit.apply')}</button>
      <button type="button" class="btn btn-outline !text-[12.5px]" onclick={resetFilters}>{t('audit.reset')}</button>
    </div>
  </form>

  <div class="surface-card p-3 sm:p-4 flex flex-wrap items-center gap-3">
    <div class="relative grow min-w-52 max-w-md">
      <i class="icon-search absolute top-1/2 -translate-y-1/2 text-[14px] text-[var(--text-tertiary)]" style="inset-inline-start:0.8rem"></i>
      <input class="field-control w-full" style="padding-inline-start:2.4rem" type="search" bind:value={query} placeholder={t('audit.search')} aria-label={t('audit.search')} spellcheck="false" />
    </div>
    <div class="ms-auto flex flex-wrap items-center gap-2">
      <button type="button" class="tool {live ? 'on' : ''}" aria-pressed={live} onclick={() => (live = !live)}>
        <span class="live-dot {live ? 'run' : ''}"></span>{t('audit.live')}
      </button>
      <button type="button" class="tool {wrap ? 'on' : ''}" aria-pressed={wrap} onclick={() => (wrap = !wrap)}><i class="icon-wrap-text"></i>{t('audit.wrap')}</button>
      <button type="button" class="tool" onclick={toggleAll}><i class="icon-{allOpen ? 'chevrons-down-up' : 'chevrons-up-down'}"></i>{allOpen ? t('audit.collapseAll') : t('audit.expandAll')}</button>
      <button type="button" class="tool" onclick={exportLog} disabled={shown.length === 0}><i class="icon-download"></i>{t('audit.export')}</button>
    </div>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('audit.loadFailed')} {error}</span>
    </div>
  {/if}

  <!-- Konsol log -->
  <div class="term">
    <div class="term-bar">
      <span class="d" style="background:#ff5f56"></span><span class="d" style="background:#ffbd2e"></span><span class="d" style="background:#27c93f"></span>
      <span class="term-title">audit.log</span>
      <span class="ms-auto text-[11px] text-[#64748b] tabular-nums">{t('audit.loaded', { shown: formatNumber(shown.length), total: formatNumber(items.length), actors: formatNumber(actors) })}</span>
    </div>
    <div class="term-body scroll-thin" role="log" aria-live={live ? 'polite' : 'off'}>
      {#each shown as it (it.id)}
        {@const lv = levelOf(it)}
        <div class="row {fresh.has(it.id) ? 'fresh' : ''}" in:fly={{ y: -6, duration: reduce ? 0 : 200 }}>
          <div class="line {wrap ? 'wrap' : ''}">
            <button type="button" class="caret" class:inv={!hasDetails(it.details)} aria-expanded={!!open[it.id]} aria-label="details" onclick={() => hasDetails(it.details) && (open[it.id] = !open[it.id])}>
              <i class="icon-chevron-right {open[it.id] ? 'rot' : ''}"></i>
            </button>
            <span class="c-ts" title={formatDateTime(it.created_at, { dateStyle: 'full', timeStyle: 'long' })}>{ts(it.created_at)}</span>
            <span class="lv lv-{lv}">{levelLabel[lv]}</span>
            <span class="c-act" title={actionLabel(it.action)}>{it.action}</span>
            <span class="c-kv"><span class="kk">actor</span>=<span class="vv">{actor(it)}</span></span>
            <span class="c-kv"><span class="kk">entity</span>=<span class="vv" title={entityLabel(it.entity)}>{it.entity}{it.entity_id ? `#${it.entity_id.slice(0, 8)}` : ''}</span></span>
            <span class="c-kv"><span class="kk">ip</span>=<span class="vv">{it.ip || '-'}</span></span>
            {#if hasDetails(it.details) && !open[it.id]}<span class="c-det">{compact(it.details)}</span>{/if}
            {#if actionLabel(it.action) !== it.action}<span class="c-msg">— {actionLabel(it.action)}</span>{/if}
            <button type="button" class="cp" aria-label={t('audit.copyLine')} title={t('audit.copyLine')} onclick={() => copyLine(it)}><i class="icon-{copiedId === it.id ? 'check' : 'copy'}"></i></button>
          </div>
          {#if open[it.id]}
            <pre class="json" transition:fly={{ y: -4, duration: reduce ? 0 : 150 }}>{@html hl(it.details)}</pre>
          {/if}
        </div>
      {:else}
        <div class="empty">{loading ? t('audit.loading') : items.length ? t('audit.noMatch') : t('audit.empty')}</div>
      {/each}
      {#if live}<div class="tail"><span class="live-dot run"></span>{t('audit.tailing')}<span class="blink">▍</span></div>{/if}
    </div>
  </div>

  {#if cursor}
    <div class="text-center">
      <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={loading} onclick={() => load(true)}>{loading ? t('audit.loading') : t('audit.loadMore')}</button>
    </div>
  {/if}
</main>

<style>
  .stat { display: flex; align-items: center; gap: 0.8rem; padding: 0.85rem 1rem; border: 1px solid var(--border-subtle); border-radius: 0.85rem; background: var(--surface-base); transition: border-color 0.15s, box-shadow 0.15s, transform 0.15s; }
  .stat:hover { transform: translateY(-1px); }
  .stat.on { border-color: var(--color-primary-500); box-shadow: 0 0 0 1px var(--color-primary-500); }
  .stat-dot { width: 0.65rem; height: 0.65rem; border-radius: 9999px; box-shadow: 0 0 10px currentColor; }
  .tool { display: inline-flex; align-items: center; gap: 0.4rem; padding: 0.4rem 0.75rem; border: 1px solid var(--border-subtle); border-radius: 0.6rem; font-size: 12px; font-weight: 600; color: var(--text-secondary); transition: all 0.15s; }
  .tool:hover:not(:disabled) { border-color: var(--color-primary-400); color: var(--color-primary-600); }
  .tool.on { background: var(--color-primary-600); border-color: var(--color-primary-600); color: #fff; }
  .tool:disabled { opacity: 0.5; }
  .live-dot { width: 0.5rem; height: 0.5rem; border-radius: 9999px; background: #94a3b8; }
  .live-dot.run { background: #22c55e; animation: pulse 1.4s ease-in-out infinite; }
  @keyframes pulse { 0%, 100% { box-shadow: 0 0 0 0 rgb(34 197 94 / 0.55); } 50% { box-shadow: 0 0 0 5px rgb(34 197 94 / 0); } }

  /* ---- konsol (selalu gelap, tidak mengikuti tema) ---- */
  .term { overflow: hidden; border-radius: 0.9rem; border: 1px solid #1e293b; background: #0b1220; box-shadow: 0 10px 30px -18px rgb(2 6 23 / 0.6); }
  .term-bar { display: flex; align-items: center; gap: 0.4rem; padding: 0.55rem 0.9rem; background: #0f172a; border-bottom: 1px solid #1e293b; }
  .term-bar .d { width: 0.7rem; height: 0.7rem; border-radius: 9999px; }
  .term-title { margin-inline-start: 0.6rem; font: 600 12px ui-monospace, 'JetBrains Mono', 'Cascadia Code', Menlo, Consolas, monospace; color: #94a3b8; }
  .term-body { max-height: 68vh; overflow: auto; padding: 0.35rem 0; font: 12px/1.65 ui-monospace, 'JetBrains Mono', 'Cascadia Code', Menlo, Consolas, monospace; color: #cbd5e1; }
  .row { border-inline-start: 2px solid transparent; transition: background-color 0.2s; }
  .row:hover { background: #111a2e; }
  .row.fresh { background: rgb(34 197 94 / 0.1); border-inline-start-color: #22c55e; animation: fade 2.5s ease-out forwards; }
  @keyframes fade { to { background: transparent; border-inline-start-color: transparent; } }
  .line { display: flex; align-items: baseline; gap: 0.7rem; padding: 0.12rem 0.8rem 0.12rem 0.4rem; white-space: nowrap; }
  .line.wrap { flex-wrap: wrap; white-space: normal; row-gap: 0; }
  .caret { flex: none; width: 1.1rem; color: #64748b; font-size: 11px; }
  .caret.inv { visibility: hidden; }
  .caret i { display: inline-block; transition: transform 0.15s; }
  .caret i.rot { transform: rotate(90deg); }
  .c-ts { flex: none; color: #64748b; }
  .lv { flex: none; width: 3.1rem; padding: 0 0.3rem; border-radius: 0.25rem; text-align: center; font-weight: 700; font-size: 10.5px; line-height: 1.5; }
  .lv-info { background: rgb(56 189 248 / 0.14); color: #38bdf8; }
  .lv-warn { background: rgb(251 191 36 / 0.16); color: #fbbf24; }
  .lv-crit { background: rgb(248 113 113 / 0.18); color: #f87171; }
  .c-act { flex: none; color: #c4b5fd; font-weight: 600; }
  .c-kv { flex: none; }
  .kk { color: #7dd3fc; }
  .vv { color: #86efac; }
  .lnk { text-decoration: underline; text-underline-offset: 2px; }
  .lnk:hover { color: #bbf7d0; }
  .c-det { color: #fcd34d; opacity: 0.85; overflow: hidden; text-overflow: ellipsis; max-width: 38ch; }
  .wrap .c-det { max-width: none; overflow: visible; }
  .c-msg { color: #64748b; font-style: italic; margin-inline-start: auto; padding-inline-start: 1rem; }
  .cp { flex: none; opacity: 0; color: #94a3b8; padding: 0 0.3rem; transition: opacity 0.15s; }
  .row:hover .cp, .cp:focus-visible { opacity: 1; }
  .cp:hover { color: #fff; }
  .json { margin: 0.1rem 0.8rem 0.5rem 2.2rem; padding: 0.7rem 0.9rem; border-radius: 0.5rem; border: 1px solid #1e293b; background: #070d1a; font-size: 11.5px; line-height: 1.6; white-space: pre-wrap; word-break: break-word; color: #cbd5e1; }
  .json :global(.k) { color: #7dd3fc; }
  .json :global(.s) { color: #86efac; }
  .json :global(.n) { color: #fcd34d; }
  .json :global(.b) { color: #c4b5fd; }
  .empty { padding: 2.5rem 1rem; text-align: center; color: #64748b; }
  .tail { display: flex; align-items: center; gap: 0.6rem; padding: 0.4rem 0.9rem; color: #4ade80; }
  .blink { animation: blink 1s steps(2, start) infinite; }
  @keyframes blink { to { visibility: hidden; } }
  @media (prefers-reduced-motion: reduce) {
    .live-dot.run, .blink, .row.fresh { animation: none; }
    .stat, .caret i, .row { transition: none; }
  }
</style>
