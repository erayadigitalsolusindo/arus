<script lang="ts">
  import Select from '#lib/components/Select.svelte';
  import { onMount } from 'svelte';
  import { fly } from 'svelte/transition';
  import { platformApi, type PlatformAuditItem } from '#lib/platform/api.ts';
  import { t, tryT, formatNumber, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const ACTIONS = [
    'platform.login', 'platform.setup', 'platform.impersonate', 'platform.tenant_status', 'platform.tenant_edit_window', 'platform.tenant_plan', 'platform.plan_save',
    'platform.admin_create', 'platform.admin_status', 'platform.admin_password', 'platform.mfa_enable', 'platform.mfa_disable', 'platform.mfa_reset', 'platform.mfa_recovery',
    'platform.apk_upload', 'platform.apk_delete', 'platform.ext_cred_create', 'platform.ext_cred_update', 'platform.ext_cred_rotate', 'platform.ext_cred_delete',
    'platform.ext_brand_save', 'platform.ext_brand_delete'
  ] as const;

  type Level = 'info' | 'warn' | 'crit';
  const LEVELS: Level[] = ['info', 'warn', 'crit'];

  let action = $state('');
  let items = $state<PlatformAuditItem[]>([]);
  let nextBefore = $state(0);
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

  async function load(more = false) {
    loading = true;
    error = '';
    try {
      const res = await platformApi.audit({ action, before: more ? nextBefore : undefined });
      items = more ? [...items, ...res.items] : res.items;
      nextBefore = res.next_before ?? 0;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    void load();
    return () => clearInterval(timer);
  });

  // Live tail: periksa entri baru tiap 5 detik dan taruh di atas (tanpa mengganggu halaman yang sudah dimuat).
  let timer: ReturnType<typeof setInterval> | undefined;
  $effect(() => {
    clearInterval(timer);
    if (!live) return;
    timer = setInterval(async () => {
      try {
        const res = await platformApi.audit({ action });
        const known = new Set(items.map((i) => i.id));
        const add = res.items.filter((i) => !known.has(i.id));
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

  const label = (a: string) => tryT(`platform.audit.actions.${a.replace('.', '_')}`) ?? a;
  function levelOf(it: PlatformAuditItem): Level {
    const a = it.action;
    if (a.includes('password') || a.includes('mfa_disable') || a.includes('mfa_reset') || a.endsWith('_delete') || (a.includes('tenant_status') && it.details?.active === false)) return 'crit';
    if (a.includes('impersonate') || a.includes('status') || a.includes('mfa_recovery') || a.includes('rotate') || a.includes('tenant_plan') || a.includes('edit_window') || a.includes('admin_create')) return 'warn';
    return 'info';
  }
  const levelLabel: Record<Level, string> = { info: 'INFO', warn: 'WARN', crit: 'CRIT' };

  const pad = (n: number, w = 2) => String(n).padStart(w, '0');
  function ts(iso: string) {
    const d = new Date(iso);
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`;
  }
  const compact = (o: Record<string, unknown>) =>
    Object.entries(o)
      .map(([k, v]) => `${k}=${typeof v === 'object' && v !== null ? JSON.stringify(v) : String(v)}`)
      .join(' ');
  const lineText = (it: PlatformAuditItem) =>
    `${ts(it.created_at)} ${levelLabel[levelOf(it)]} ${it.action} admin="${it.admin_name || '-'}" tenant="${it.tenant_name || it.tenant_id || '-'}" ip=${it.ip || '-'} ${compact(it.details ?? {})}`.trim();

  const shown = $derived.by(() => {
    const q = query.trim().toLowerCase();
    return items.filter((it) => (level === 'all' || levelOf(it) === level) && (!q || lineText(it).toLowerCase().includes(q) || label(it.action).toLowerCase().includes(q)));
  });
  const counts = $derived.by(() => {
    const c: Record<Level, number> = { info: 0, warn: 0, crit: 0 };
    for (const it of items) c[levelOf(it)]++;
    return c;
  });
  const admins = $derived(new Set(items.map((i) => i.admin_name).filter(Boolean)).size);

  async function copyLine(it: PlatformAuditItem) {
    try {
      await navigator.clipboard.writeText(lineText(it));
      copiedId = it.id;
      setTimeout(() => (copiedId = 0), 1200);
    } catch {
      /* clipboard diblokir */
    }
  }

  function exportLog() {
    const body = shown.map((it) => JSON.stringify({ id: it.id, time: it.created_at, level: levelOf(it), action: it.action, admin: it.admin_name, tenant_id: it.tenant_id, tenant: it.tenant_name, ip: it.ip, details: it.details })).join('\n');
    const url = URL.createObjectURL(new Blob([body], { type: 'application/x-ndjson' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = `platform-audit-${new Date().toISOString().slice(0, 10)}.ndjson`;
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
</script>

<svelte:head><title>{t('platform.audit.title')} | ACIRABA</title></svelte:head>

<div class="space-y-5">
  <div class="flex flex-wrap items-start justify-between gap-4">
    <div class="flex items-start gap-3.5">
      <span class="grid size-12 shrink-0 place-items-center rounded-2xl bg-[#0b1220] text-[#4ade80] shadow-sm font-mono text-[15px] font-bold">&gt;_</span>
      <div>
        <h1 class="font-display font-bold text-[22px] tracking-tight">{t('platform.audit.title')}</h1>
        <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)] max-w-2xl">{t('platform.audit.subtitle')}</p>
      </div>
    </div>
  </div>

  <!-- Ringkasan -->
  <div class="grid grid-cols-2 lg:grid-cols-4 gap-3">
    {#each [['all', formatNumber(items.length), t('platform.audit.sumEntries'), '#94a3b8'], ['info', formatNumber(counts.info), 'INFO', '#38bdf8'], ['warn', formatNumber(counts.warn), 'WARN', '#fbbf24'], ['crit', formatNumber(counts.crit), 'CRIT', '#f87171']] as const as [lv, n, lab, col] (lv)}
      <button type="button" class="stat {level === lv ? 'on' : ''}" onclick={() => (level = lv as typeof level)} aria-pressed={level === lv}>
        <span class="stat-dot" style="background:{col}"></span>
        <span class="text-start">
          <span class="block text-[10.5px] font-bold uppercase tracking-wider text-[var(--text-tertiary)]">{lab}</span>
          <span class="block font-display font-bold text-[20px] tabular-nums leading-tight">{n}</span>
        </span>
      </button>
    {/each}
  </div>

  <!-- Toolbar -->
  <div class="surface-card p-3 sm:p-4 flex flex-wrap items-center gap-3">
    <div class="relative grow min-w-52 max-w-md">
      <i class="icon-search absolute top-1/2 -translate-y-1/2 text-[14px] text-[var(--text-tertiary)]" style="inset-inline-start:0.8rem"></i>
      <input class="field-control w-full" style="padding-inline-start:2.4rem" type="search" bind:value={query} placeholder={t('platform.audit.search')} aria-label={t('platform.audit.search')} spellcheck="false" />
    </div>
    <Select class="!w-auto min-w-56" ariaLabel={t('platform.audit.action')} bind:value={action} onchange={() => load()} options={[{ value: '', label: t('platform.audit.all') }, ...ACTIONS.map((a) => ({ value: a as string, label: label(a) }))]} />
    <div class="ms-auto flex flex-wrap items-center gap-2">
      <button type="button" class="tool {live ? 'on' : ''}" aria-pressed={live} onclick={() => (live = !live)}>
        <span class="live-dot {live ? 'run' : ''}"></span>{t('platform.audit.live')}
      </button>
      <button type="button" class="tool {wrap ? 'on' : ''}" aria-pressed={wrap} onclick={() => (wrap = !wrap)}><i class="icon-wrap-text"></i>{t('platform.audit.wrap')}</button>
      <button type="button" class="tool" onclick={toggleAll}><i class="icon-{allOpen ? 'chevrons-down-up' : 'chevrons-up-down'}"></i>{allOpen ? t('platform.audit.collapseAll') : t('platform.audit.expandAll')}</button>
      <button type="button" class="tool" onclick={exportLog} disabled={shown.length === 0}><i class="icon-download"></i>{t('platform.audit.export')}</button>
    </div>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <!-- Konsol log -->
  <div class="term">
    <div class="term-bar">
      <span class="d" style="background:#ff5f56"></span><span class="d" style="background:#ffbd2e"></span><span class="d" style="background:#27c93f"></span>
      <span class="term-title">platform-audit.log</span>
      <span class="ms-auto text-[11px] text-[#64748b] tabular-nums">{t('platform.audit.loaded', { shown: formatNumber(shown.length), total: formatNumber(items.length), admins: formatNumber(admins) })}</span>
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
            <span class="c-act" title={label(it.action)}>{it.action}</span>
            <span class="c-kv"><span class="kk">admin</span>=<span class="vv">{it.admin_name || '-'}</span></span>
            {#if it.tenant_id}<span class="c-kv"><span class="kk">tenant</span>=<a class="vv lnk" href="/platform/tenants/{it.tenant_id}">{it.tenant_name || it.tenant_id}</a></span>{/if}
            <span class="c-kv"><span class="kk">ip</span>=<span class="vv">{it.ip || '-'}</span></span>
            {#if hasDetails(it.details) && !open[it.id]}<span class="c-det">{compact(it.details)}</span>{/if}
            {#if label(it.action) !== it.action}<span class="c-msg">— {label(it.action)}</span>{/if}
            <button type="button" class="cp" aria-label={t('platform.audit.copyLine')} title={t('platform.audit.copyLine')} onclick={() => copyLine(it)}><i class="icon-{copiedId === it.id ? 'check' : 'copy'}"></i></button>
          </div>
          {#if open[it.id]}
            <pre class="json" transition:fly={{ y: -4, duration: reduce ? 0 : 150 }}>{@html hl(it.details)}</pre>
          {/if}
        </div>
      {:else}
        <div class="empty">{loading ? t('platform.tenants.loading') : items.length ? t('platform.audit.noMatch') : t('platform.audit.empty')}</div>
      {/each}
      {#if live}<div class="tail"><span class="live-dot run"></span>{t('platform.audit.tailing')}<span class="blink">▍</span></div>{/if}
    </div>
  </div>

  {#if nextBefore}
    <div class="text-center">
      <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={loading} onclick={() => load(true)}>{t('platform.audit.loadMore')}</button>
    </div>
  {/if}
</div>

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
