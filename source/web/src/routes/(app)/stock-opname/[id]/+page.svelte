<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { counts, type CountDetail, type CountLine, type OpeningRow } from '#lib/stock/api.ts';
  import { lookup } from '#lib/catalog/api.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t, formatDateTime, formatNumber, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { ApiError } from '#lib/api/client.ts';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import Modal from '#lib/components/Modal.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import MarkdownView from '#lib/components/MarkdownView.svelte';

  const id = page.params.id as string;
  const STEP = 200;

  type Cell = { draft: string; state: 'idle' | 'saving' | 'saved' | 'error'; error: string };

  let doc = $state<CountDetail | null>(null);
  let cells = $state<Record<string, Cell>>({});
  let loading = $state(true);
  let loadError = $state('');
  let notice = $state('');
  let actionError = $state('');
  let busy = $state(false);

  let q = $state('');
  let onlyDiff = $state(false);
  let onlyUncounted = $state(false);
  let shown = $state(STEP);

  let confirming = $state<'complete' | 'cancel' | null>(null);

  const isDraft = $derived(doc?.status === 'draft');
  const isQuick = $derived(doc?.kind === 'quick');
  // Dokumen selesai membandingkan stok sebelum → sesudah; draf membandingkan stok sistem → hasil hitung.
  const colSystem = $derived(doc?.status === 'completed' ? t('stock.count.col.before') : t('stock.count.col.system'));
  const colCounted = $derived(doc?.status === 'completed' ? t('stock.count.col.after') : t('stock.count.col.counted'));
  const colDiff = $derived(doc?.status === 'completed' ? t('stock.count.col.change') : t('stock.count.col.diff'));
  const canEdit = $derived(isDraft && can('stock_opname', 'create'));
  const canApprove = $derived(isDraft && can('stock_opname', 'approve'));
  const num = (s: string | null | undefined) => Number((s ?? '').replace(',', '.'));
  const qty = (n: number) => formatNumber(n, { maximumFractionDigits: 3 });
  const diffOf = (r: CountLine) => (r.counted == null ? null : num(r.counted) - num(r.snapshot));
  const sign = (n: number) => (n > 0 ? '+' : '');

  // Ringkasan dihitung ulang di klien dari baris yang tampil (pratinjau); server menghitung final saat selesai.
  const stats = $derived.by(() => {
    let counted = 0, diffs = 0, plus = 0, minus = 0;
    for (const r of doc?.items ?? []) {
      const d = diffOf(r);
      if (d == null) continue;
      counted++;
      if (Math.abs(d) > 0.0005) {
        diffs++;
        const v = d * num(r.unit_cost);
        if (v >= 0) plus += v;
        else minus += v;
      }
    }
    const lines = doc?.items.length ?? 0;
    return { lines, counted, diffs, plus, minus, value: plus + minus, pct: lines > 0 ? Math.round((counted / lines) * 100) : 0 };
  });

  const visible = $derived.by(() => {
    const needle = q.trim().toLowerCase();
    return (doc?.items ?? []).filter((r) => {
      if (needle && !r.name.toLowerCase().includes(needle) && !r.sku.toLowerCase().includes(needle)) return false;
      const d = diffOf(r);
      if (onlyUncounted && d != null) return false;
      if (onlyDiff && (d == null || Math.abs(d) <= 0.0005)) return false;
      return true;
    });
  });

  function adopt(d: CountDetail) {
    doc = d;
    const next: Record<string, Cell> = {};
    for (const r of d.items) next[r.item_id] = { draft: r.counted ?? '', state: 'idle', error: '' };
    cells = next;
  }

  async function load() {
    loading = true;
    loadError = '';
    try {
      adopt(await counts.get(id));
    } catch (err) {
      loadError = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  onMount(load);

  async function commit(r: CountLine) {
    const c = cells[r.item_id];
    if (!c || c.state === 'saving') return;
    const raw = c.draft.trim().replace(',', '.');
    const before = r.counted;
    if ((raw === '' && before == null) || (raw !== '' && before != null && Number(raw) === Number(before))) return;
    c.state = 'saving';
    c.error = '';
    try {
      await counts.setCounted(id, r.item_id, raw === '' ? null : raw);
      r.counted = raw === '' ? null : String(Number(raw));
      c.draft = r.counted ?? '';
      c.state = 'saved';
    } catch (err) {
      c.state = 'error';
      c.error = err instanceof ApiError && err.code === 'VALIDATION' && err.fields.counted_qty ? (fieldMessage(err.fields.counted_qty) ?? errorMessage(err)) : errorMessage(err);
      if (err instanceof ApiError && err.code === 'COUNT_NOT_DRAFT') void load();
    }
  }

  // ---- Tambah barang ----
  let pickId = $state('');
  let pickLabel = $state('');
  let catId = $state('');
  let catLabel = $state('');
  let brandId = $state('');
  let brandLabel = $state('');
  let adding = $state(false);

  async function searchItems(term: string): Promise<Option[]> {
    const res = await counts.items(term.trim());
    return res.data.map((r: OpeningRow) => ({ id: r.id, name: `${r.sku} — ${r.name}` }));
  }
  const searchCategories = (term: string) => lookup('categories').search(term);
  const searchBrands = (term: string) => lookup('brands').search(term);

  async function add(scope: Parameters<typeof counts.addItems>[1]) {
    if (adding) return;
    adding = true;
    actionError = '';
    try {
      const res = await counts.addItems(id, scope);
      notice = res.added > 0 ? t('stock.count.add.done', { count: res.added }) : t('stock.count.add.none');
      if (res.added > 0) adopt(await counts.get(id));
    } catch (err) {
      actionError = errorMessage(err);
    } finally {
      adding = false;
    }
  }

  $effect(() => {
    if (!pickId) return;
    const item = pickId;
    pickId = '';
    pickLabel = '';
    void add({ item_ids: [item] });
  });

  async function addAll() {
    if (confirm(t('stock.count.add.allConfirm'))) await add({ all: true });
  }

  async function remove(r: CountLine) {
    actionError = '';
    try {
      await counts.removeItem(id, r.item_id);
      if (doc) doc.items = doc.items.filter((x) => x.item_id !== r.item_id);
    } catch (err) {
      actionError = errorMessage(err);
      if (err instanceof ApiError && err.code === 'COUNT_NOT_DRAFT') void load();
    }
  }

  async function finish() {
    if (busy) return;
    busy = true;
    actionError = '';
    try {
      const d = await counts.complete(id);
      adopt(d);
      notice = t('stock.count.complete.done', { doc: d.doc_no });
      confirming = null;
    } catch (err) {
      actionError = errorMessage(err);
      confirming = null;
      if (err instanceof ApiError && err.code === 'COUNT_NOT_DRAFT') void load();
    } finally {
      busy = false;
    }
  }

  async function cancelCount() {
    if (busy) return;
    busy = true;
    actionError = '';
    try {
      await counts.cancel(id);
      confirming = null;
      notice = t('stock.count.cancel.done', { doc: doc?.doc_no ?? '' });
      adopt(await counts.get(id));
    } catch (err) {
      actionError = errorMessage(err);
      confirming = null;
    } finally {
      busy = false;
    }
  }

  const label = 'text-[11px] font-semibold uppercase tracking-wide block text-[var(--text-tertiary)]';
  const statusTone = {
    draft: { pill: 'badge-warning', color: 'warning', icon: 'icon-pencil-line' },
    completed: { pill: 'badge-success', color: 'success', icon: 'icon-clipboard-check' },
    cancelled: { pill: 'badge-neutral', color: 'neutral', icon: 'icon-circle-x' }
  } as const;
  const bucketIcon = { display: 'icon-store', warehouse: 'icon-warehouse', returns: 'icon-undo-2' } as const;
  const soft = (c: string) =>
    c === 'neutral'
      ? 'background: var(--surface-sunken); color: var(--text-tertiary)'
      : `background: color-mix(in srgb, var(--color-${c}-500, #3b82f6) 15%, transparent); color: var(--color-${c}-600, #2563eb)`;
</script>

<svelte:head><title>{doc ? t('stock.count.detail.docTitle', { doc: doc.doc_no }) : t('stock.count.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('stock.count.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <a href="/stock-opname" class="text-[var(--color-primary-600)]">{t('stock.count.title')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)] font-mono">{doc?.doc_no ?? '…'}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-3 max-w-[1400px] mx-auto w-full">
  {#if loadError}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('stock.count.detail.loadFailed')} {loadError}</span>
    </div>
  {:else if loading || !doc}
    <div class="space-y-3 animate-pulse" aria-busy="true">
      <div class="h-14 rounded-xl bg-[var(--surface-sunken)]"></div>
      <div class="h-14 rounded-xl bg-[var(--surface-sunken)]"></div>
      <div class="h-64 rounded-xl bg-[var(--surface-sunken)]"></div>
    </div>
  {:else}
    {@const st = statusTone[doc.status]}
    <!-- Header satu baris -->
    <section class="surface-card !p-3.5 flex flex-wrap items-center gap-x-4 gap-y-2">
      <a href="/stock-opname" class="header-icon-btn shrink-0" aria-label={t('stock.count.detail.back')} title={t('stock.count.detail.back')}><i class="icon-arrow-left text-[15px]"></i></a>
      <div class="min-w-0 grow">
        <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
          <h1 class="font-display font-bold text-[16px] font-mono leading-tight">{doc.doc_no}</h1>
          <span class="badge-soft {st.pill}">{t(`stock.count.status.${doc.status}`)}</span>
          {#if isQuick}<span class="badge-soft badge-warning inline-flex items-center gap-1"><i class="icon-zap text-[10px]"></i>{t('stock.count.kind.quick')}{doc.mode ? ` · ${t(`stock.count.mode.${doc.mode}`)}` : ''}</span>{/if}
        </div>
        <div class="mt-1 flex flex-wrap items-center gap-x-3.5 gap-y-0.5 text-[11.5px] text-[var(--text-tertiary)]">
          <span class="inline-flex items-center gap-1"><i class="{bucketIcon[doc.bucket]} text-[12px]"></i>{t(`stock.count.bucket.${doc.bucket}`)}</span>
          {#if session.outlet}<span class="inline-flex items-center gap-1"><i class="icon-store text-[12px]"></i>{session.outlet.name}</span>{/if}
          <span class="inline-flex items-center gap-1"><i class="icon-user text-[12px]"></i>{doc.created_by}</span>
          <span class="inline-flex items-center gap-1"><i class="icon-clock text-[12px]"></i>{formatDateTime(new Date(doc.created_at))}</span>
        </div>
      </div>
      {#if isDraft}
        <div class="flex flex-wrap gap-2">
          {#if canEdit}
            <button type="button" class="btn !text-[12.5px] !border !border-[var(--border-subtle)]" onclick={() => (confirming = 'cancel')}><i class="icon-x text-[13px]"></i>{t('stock.count.cancel.button')}</button>
          {/if}
          {#if canApprove}
            <button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={stats.counted === 0} onclick={() => (confirming = 'complete')}>
              <i class="icon-clipboard-check text-[13px]"></i>{t('stock.count.complete.button')}
            </button>
          {/if}
        </div>
      {/if}
    </section>

    {#if doc.note}<div class="surface-card !p-3 text-[12.5px]"><MarkdownView source={doc.note} /></div>{/if}

    {#if doc.status === 'completed'}
      <div class="flex items-start gap-2 rounded-lg px-3 py-2 text-[12.5px] badge-success">
        <i class="icon-circle-check text-[14px] mt-px shrink-0"></i><span>{isQuick ? t('stock.count.detail.quickInfo', { mode: doc.mode ? t(`stock.count.mode.${doc.mode}`) : '', date: formatDateTime(new Date(doc.completed_at!)), by: doc.completed_by ?? '' }) : t('stock.count.detail.completedInfo', { date: formatDateTime(new Date(doc.completed_at!)), by: doc.completed_by ?? '' })}</span>
      </div>
    {:else if doc.status === 'cancelled'}
      <div class="flex items-start gap-2 rounded-lg px-3 py-2 text-[12.5px] badge-neutral">
        <i class="icon-circle-x text-[14px] mt-px shrink-0"></i><span>{t('stock.count.detail.cancelledInfo', { date: formatDateTime(new Date(doc.cancelled_at!)), by: doc.cancelled_by ?? '' })}</span>
      </div>
    {/if}

    {#if notice}
      <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2 text-[12.5px] badge-success">
        <i class="icon-circle-check text-[14px] shrink-0"></i><span>{notice}</span>
        <button type="button" class="ms-auto" aria-label={t('common.close')} onclick={() => (notice = '')}><i class="icon-x text-[13px]"></i></button>
      </div>
    {/if}
    {#if actionError}
      <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2 text-[12.5px] badge-danger">
        <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{actionError}</span>
        <button type="button" class="ms-auto" aria-label={t('common.close')} onclick={() => (actionError = '')}><i class="icon-x text-[13px]"></i></button>
      </div>
    {/if}

    <!-- Ringkasan: satu strip -->
    <section class="surface-card !p-0 grid grid-cols-2 lg:grid-cols-4 divide-y lg:divide-y-0 lg:divide-x divide-[var(--border-subtle)]">
      <div class="px-4 py-2.5">
        <p class={label}>{t('stock.count.summary.lines')}</p>
        <p class="mt-0.5 font-display font-bold text-[18px] leading-tight tabular-nums">{formatNumber(stats.lines)}</p>
      </div>
      <div class="px-4 py-2.5">
        <p class={label}>{t('stock.count.summary.counted')}</p>
        <div class="mt-0.5 flex items-center gap-2.5">
          <p class="font-display font-bold text-[18px] leading-tight tabular-nums">{formatNumber(stats.counted)}<span class="text-[12px] font-semibold text-[var(--text-tertiary)]"> / {formatNumber(stats.lines)}</span></p>
          <div class="h-1.5 grow max-w-24 rounded-full bg-[var(--surface-sunken)] overflow-hidden" role="progressbar" aria-valuenow={stats.pct} aria-valuemin="0" aria-valuemax="100" aria-label={t('stock.count.percent', { percent: stats.pct })}>
            <div class="h-full rounded-full bg-[var(--color-primary-600)] transition-all" style="width: {stats.pct}%"></div>
          </div>
          <span class="text-[11px] tabular-nums text-[var(--text-tertiary)]">{stats.pct}%</span>
        </div>
      </div>
      <div class="px-4 py-2.5">
        <p class={label}>{t('stock.count.summary.diffs')}</p>
        <p class="mt-0.5 font-display font-bold text-[18px] leading-tight tabular-nums">{formatNumber(stats.diffs)}</p>
      </div>
      <div class="px-4 py-2.5 min-w-0">
        <p class={label}>{t('stock.count.summary.value')}</p>
        <p class="mt-0.5 font-display font-bold text-[18px] leading-tight tabular-nums {stats.value < 0 ? 'text-[var(--color-danger-600)]' : stats.value > 0 ? 'text-[var(--color-success-600,#16a34a)]' : ''}">
          {sign(stats.value)}{formatCurrency(stats.value)}
          <span class="ms-1 text-[11px] font-medium text-[var(--text-tertiary)]">▲ {formatCurrency(stats.plus)} · ▼ {formatCurrency(Math.abs(stats.minus))}</span>
        </p>
      </div>
    </section>

    <!-- Tabel hitung (kontrol tambah barang digabung di toolbar) -->
    <div class="surface-card !p-0 overflow-hidden">
      {#if canEdit}
        <div class="flex flex-wrap items-center gap-2 px-3.5 py-2.5 border-b border-[var(--border-subtle)]">
          <span class="inline-flex items-center gap-1.5 text-[12px] font-semibold text-[var(--text-tertiary)] me-1"><i class="icon-list-plus text-[14px] text-[var(--color-primary-600)]"></i>{t('stock.count.add.title')}</span>
          <div class="grow basis-56 max-w-sm"><Combobox bind:value={pickId} bind:label={pickLabel} search={searchItems} placeholder={t('stock.count.add.item')} /></div>
          <div class="flex gap-1 basis-48 grow max-w-56">
            <div class="grow min-w-0"><Combobox bind:value={catId} bind:label={catLabel} search={searchCategories} placeholder={t('stock.count.add.pickCategory')} /></div>
            <button type="button" class="btn !text-[12px] shrink-0 !border !border-[var(--border-subtle)]" disabled={!catId || adding} onclick={() => { const c = catId; catId = ''; catLabel = ''; void add({ category_id: c }); }}>{t('stock.count.add.add')}</button>
          </div>
          <div class="flex gap-1 basis-48 grow max-w-56">
            <div class="grow min-w-0"><Combobox bind:value={brandId} bind:label={brandLabel} search={searchBrands} placeholder={t('stock.count.add.pickBrand')} /></div>
            <button type="button" class="btn !text-[12px] shrink-0 !border !border-[var(--border-subtle)]" disabled={!brandId || adding} onclick={() => { const b = brandId; brandId = ''; brandLabel = ''; void add({ brand_id: b }); }}>{t('stock.count.add.add')}</button>
          </div>
          <button type="button" class="btn !text-[12px] !border !border-[var(--border-subtle)]" disabled={adding} onclick={addAll}><i class="icon-layers text-[13px]"></i>{t('stock.count.add.all')}</button>
        </div>
      {/if}
      <div class="flex flex-wrap items-center gap-x-3 gap-y-2 px-3.5 py-2 border-b border-[var(--border-subtle)]">
        <div class="relative grow basis-48 max-w-xs">
          <i class="icon-search absolute start-3 top-1/2 -translate-y-1/2 text-[13px] text-[var(--text-tertiary)]"></i>
          <input type="search" class="w-full field-control !ps-8 !py-1" placeholder={t('stock.count.filter.search')} aria-label={t('stock.count.filter.search')} bind:value={q} oninput={() => (shown = STEP)} maxlength="200" />
        </div>
        <label class="inline-flex items-center gap-1.5 text-[12px] cursor-pointer"><input type="checkbox" bind:checked={onlyDiff} onchange={() => (shown = STEP)} />{t('stock.count.filter.onlyDiff')}</label>
        <label class="inline-flex items-center gap-1.5 text-[12px] cursor-pointer"><input type="checkbox" bind:checked={onlyUncounted} onchange={() => (shown = STEP)} />{t('stock.count.filter.onlyUncounted')}</label>
        {#if canEdit}<span class="hidden xl:flex items-center gap-1.5 text-[11.5px] text-[var(--text-tertiary)] min-w-0"><i class="icon-info text-[12px] shrink-0"></i><span class="truncate" title={t('stock.count.detail.hint')}>{t('stock.count.detail.hint')}</span></span>{/if}
        <span class="ms-auto text-[12px] tabular-nums text-[var(--text-tertiary)]">{formatNumber(visible.length)} / {formatNumber(stats.lines)}</span>
      </div>
      <div class="overflow-x-auto scroll-thin max-h-[70vh]">
        <table class="w-full text-[12.5px] min-w-[760px]">
          <thead class="sticky top-0 z-10">
            <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
              <th class="px-3.5 py-2 text-start font-semibold" scope="col">{t('stock.count.col.name')}</th>
              <th class="px-3 py-2 text-end font-semibold" scope="col">{colSystem}</th>
              <th class="px-3 py-2 text-end font-semibold" scope="col">{colCounted}</th>
              <th class="px-3 py-2 text-end font-semibold" scope="col">{colDiff}</th>
              <th class="px-3 py-2 text-end font-semibold" scope="col">{t('stock.count.col.value')}</th>
              {#if canEdit}<th class="px-2 py-2 w-8" scope="col"><span class="sr-only">{t('stock.count.remove')}</span></th>{/if}
            </tr>
          </thead>
          <tbody>
            {#each visible.slice(0, shown) as r (r.item_id)}
              {@const d = diffOf(r)}
              {@const c = cells[r.item_id]}
              {@const neg = d != null && d < -0.0005}
              {@const pos = d != null && d > 0.0005}
              <tr class="border-t border-[var(--border-subtle)] align-middle transition-colors hover:bg-[var(--surface-sunken)]" style="box-shadow: inset 3px 0 0 {neg ? 'var(--color-danger-500,#ef4444)' : d != null ? 'var(--color-success-500,#22c55e)' : 'transparent'}">
                <td class="px-3.5 py-1.5">
                  <span class="font-semibold">{r.name}</span>
                  <span class="ms-2 font-mono text-[11px] text-[var(--text-tertiary)]">{r.sku} · {r.unit}</span>
                </td>
                <td class="px-3 py-1.5 text-end whitespace-nowrap tabular-nums">
                  {qty(num(r.snapshot))}
                  {#if isDraft && num(r.current) !== num(r.snapshot)}
                    <span class="ms-1 text-[11px] text-[var(--color-warning-600,#b45309)]" title={t('stock.count.changed', { qty: qty(num(r.current)) })}><i class="icon-triangle-alert text-[11px]"></i> {qty(num(r.current))}</span>
                  {/if}
                </td>
                <td class="px-3 py-1 text-end">
                  {#if canEdit && c}
                    <MoneyInput
                      class="field-control !text-end !py-1 w-24 font-semibold {c.state === 'error' ? '!border-[var(--color-danger-600)]' : ''}"
                      decimals={3}
                      pad={false}
                      aria-label="{r.name} – {t('stock.count.col.counted')}"
                      aria-invalid={c.state === 'error'}
                      title={c.state === 'error' ? c.error : c.state === 'saved' ? t('stock.opening.saved') : undefined}
                      bind:value={c.draft}
                      onfocus={(e) => e.currentTarget.select()}
                      onblur={() => commit(r)}
                      onkeydown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
                    />
                    {#if c.state === 'saving'}<span class="ms-1 text-[11px] text-[var(--text-tertiary)]">…</span>
                    {:else if c.state === 'saved'}<i class="icon-check text-[11px] ms-1 text-[var(--color-success-600,#16a34a)]"></i>
                    {:else if c.state === 'error'}<span class="block text-[11px] text-[var(--color-danger-600)]">{c.error}</span>{/if}
                  {:else}
                    <span class="whitespace-nowrap tabular-nums font-semibold">{r.counted == null ? '—' : qty(num(r.counted))}</span>
                  {/if}
                </td>
                <td class="px-3 py-1.5 text-end whitespace-nowrap">
                  {#if d == null}
                    <span class="text-[var(--text-tertiary)]">—</span>
                  {:else if neg || pos}
                    <span class="badge-soft {neg ? 'badge-danger' : 'badge-success'} tabular-nums"><i class="{neg ? 'icon-arrow-down' : 'icon-arrow-up'} text-[11px]"></i>{sign(d)}{qty(d)}</span>
                  {:else}
                    <span class="badge-soft badge-success"><i class="icon-check text-[11px]"></i>{t('stock.count.match')}</span>
                  {/if}
                </td>
                <td class="px-3 py-1.5 text-end whitespace-nowrap tabular-nums {neg ? 'text-[var(--color-danger-600)]' : pos ? 'text-[var(--color-success-600,#16a34a)]' : 'text-[var(--text-tertiary)]'}">
                  {d == null || (!neg && !pos) ? '' : formatCurrency(d * num(r.unit_cost))}
                </td>
                {#if canEdit}
                  <td class="px-2 py-1.5 text-end">
                    <button type="button" class="header-icon-btn" aria-label="{t('stock.count.remove')}: {r.name}" title={t('stock.count.remove')} onclick={() => remove(r)}><i class="icon-trash-2 text-[14px]"></i></button>
                  </td>
                {/if}
              </tr>
            {:else}
              <tr>
                <td colspan={canEdit ? 6 : 5} class="px-4 py-8 text-center text-[12.5px] text-[var(--text-tertiary)]">{doc.items.length === 0 ? t('stock.count.detail.empty') : t('stock.count.detail.emptyFilter')}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      {#if visible.length > shown}
        <div class="px-4 py-2 border-t border-[var(--border-subtle)] text-center">
          <button type="button" class="btn !text-[12px]" onclick={() => (shown += STEP)}>{t('stock.count.detail.more', { count: Math.min(STEP, visible.length - shown) })}</button>
        </div>
      {/if}
    </div>
  {/if}
</main>

{#if confirming === 'complete' && doc}
  <Modal title={t('stock.count.complete.title')} onclose={() => (confirming = null)}>
    <div class="space-y-3 text-[12.5px] leading-relaxed">
      {#if stats.diffs > 0}
        <p>{t('stock.count.complete.body', { count: stats.diffs })}</p>
        <div class="grid grid-cols-2 gap-2">
          <div class="rounded-lg p-3" style={soft('success')}><p class="text-[11px] font-semibold uppercase">▲</p><p class="font-display font-bold text-[15px] tabular-nums">{formatCurrency(stats.plus)}</p></div>
          <div class="rounded-lg p-3" style={soft('danger')}><p class="text-[11px] font-semibold uppercase">▼</p><p class="font-display font-bold text-[15px] tabular-nums">{formatCurrency(Math.abs(stats.minus))}</p></div>
        </div>
        <p class="font-semibold">{t('stock.count.complete.value', { value: `${sign(stats.value)}${formatCurrency(stats.value)}` })}</p>
      {:else}
        <p>{t('stock.count.complete.noDiff')}</p>
      {/if}
      {#if stats.lines > stats.counted}
        <p class="flex items-start gap-2 rounded-lg px-3 py-2 badge-warning"><i class="icon-triangle-alert text-[14px] mt-px shrink-0"></i><span>{t('stock.count.complete.uncounted', { count: stats.lines - stats.counted })}</span></p>
      {/if}
      <div class="flex justify-end gap-2 pt-1">
        <button type="button" class="btn !text-[12.5px]" onclick={() => (confirming = null)}>{t('stock.count.cancel.keep')}</button>
        <button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={busy} onclick={finish}>{busy ? t('stock.count.complete.working') : t('stock.count.complete.confirm')}</button>
      </div>
    </div>
  </Modal>
{:else if confirming === 'cancel'}
  <Modal title={t('stock.count.cancel.title')} onclose={() => (confirming = null)}>
    <div class="space-y-3 text-[12.5px] leading-relaxed">
      <p>{t('stock.count.cancel.body')}</p>
      <div class="flex justify-end gap-2 pt-1">
        <button type="button" class="btn !text-[12.5px]" onclick={() => (confirming = null)}>{t('stock.count.cancel.keep')}</button>
        <button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={busy} onclick={cancelCount}>{t('stock.count.cancel.confirm')}</button>
      </div>
    </div>
  </Modal>
{/if}
