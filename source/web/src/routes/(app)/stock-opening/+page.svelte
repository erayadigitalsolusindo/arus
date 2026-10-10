<script lang="ts">
  import DatePicker from '#lib/components/DatePicker.svelte';
  import { onMount } from 'svelte';
  import { opening, BUCKETS, type Bucket, type OpeningRow, type OpeningStatus } from '#lib/stock/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatDate, formatNumber, type MessageKey } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { ApiError } from '#lib/api/client.ts';
  import Modal from '#lib/components/Modal.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';

  const PAGE = 20;
  const BUCKET_STATS: [Bucket, string][] = [
    ['display', 'icon-store'],
    ['warehouse', 'icon-warehouse'],
    ['returns', 'icon-rotate-ccw']
  ];
  const GUIDE = ['stock.opening.guide.s1', 'stock.opening.guide.s2', 'stock.opening.guide.s3', 'stock.opening.guide.s4'] as const;

  type Cell = { draft: string; state: '' | 'saving' | 'saved' | 'error'; error: string };

  let rows = $state<OpeningRow[]>([]);
  let cells = $state<Record<string, Cell>>({});
  let total = $state(0);
  let offset = $state(0);
  let loading = $state(true);
  let loadError = $state('');
  let notice = $state('');
  let q = $state('');
  let status = $state<OpeningStatus | null>(null);
  let locking = $state<{ date: string; busy: boolean; error: string } | null>(null);

  const key = (id: string, b: Bucket) => `${id}:${b}`;
  const stats = $derived(status?.stats);
  const pct = $derived(stats && stats.items > 0 ? Math.min(100, Math.round((stats.filled / stats.items) * 100)) : 0);
  const editable = $derived(!!status && !status.locked && can('stock_opening', 'create'));

  let statusTimer: ReturnType<typeof setTimeout> | undefined;
  function scheduleStatus() {
    clearTimeout(statusTimer);
    statusTimer = setTimeout(() => void loadStatus(), 700);
  }

  async function loadStatus() {
    try {
      status = await opening.status();
    } catch (err) {
      loadError = errorMessage(err);
    }
  }

  let seq = 0; // hanya respons permintaan terbaru yang dipakai
  async function load() {
    const mine = ++seq;
    loading = true;
    loadError = '';
    try {
      const res = await opening.list({ q: q.trim(), limit: PAGE, offset });
      if (mine !== seq) return;
      rows = res.data;
      total = res.total;
      const next: Record<string, Cell> = {};
      for (const r of res.data) for (const b of BUCKETS) next[key(r.id, b)] = { draft: r[b], state: '', error: '' };
      cells = next;
    } catch (err) {
      if (mine === seq) loadError = errorMessage(err);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  $effect(() => {
    void q;
    const h = setTimeout(() => {
      offset = 0;
      void load();
    }, 250);
    return () => clearTimeout(h);
  });

  onMount(() => {
    void loadStatus();
    return () => clearTimeout(statusTimer);
  });

  function go(next: number) {
    offset = Math.max(0, next);
    void load();
  }

  async function commit(row: OpeningRow, b: Bucket) {
    const c = cells[key(row.id, b)];
    if (!c || c.state === 'saving') return;
    const raw = c.draft.trim().replace(',', '.');
    if (raw === '' || (Number.isFinite(Number(raw)) && Number(raw) === Number(row[b]))) {
      c.draft = row[b];
      return;
    }
    c.state = 'saving';
    c.error = '';
    try {
      const res = await opening.set(row.id, b, raw);
      row[b] = res.qty;
      c.draft = res.qty;
      c.state = 'saved';
      scheduleStatus();
    } catch (err) {
      c.state = 'error';
      c.error = err instanceof ApiError && err.code === 'VALIDATION' && err.fields.qty ? (fieldMessage(err.fields.qty) ?? errorMessage(err)) : errorMessage(err);
      if (err instanceof ApiError && err.code === 'OPENING_LOCKED') void loadStatus();
    }
  }

  function today(): string {
    const d = new Date();
    const p = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
  }

  async function confirmLock() {
    if (!locking) return;
    locking.busy = true;
    locking.error = '';
    try {
      await opening.lock(locking.date);
      await loadStatus();
      locking = null;
      notice = t('stock.opening.lock.done');
    } catch (err) {
      if (locking) {
        locking.error = errorMessage(err);
        locking.busy = false;
      }
    }
  }
</script>

<svelte:head><title>{t('stock.opening.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('stock.opening.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('stock.opening.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div class="max-w-3xl">
      <h1 class="font-display font-bold text-[19px]">{t('stock.opening.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('stock.opening.subtitle')}</p>
    </div>
    {#if status && !status.locked && can('stock_opening', 'approve')}
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={() => (locking = { date: today(), busy: false, error: '' })}>
        <i class="icon-lock text-[13px]"></i>{t('stock.opening.lock.button')}
      </button>
    {/if}
  </div>

  {#if status}
    <div class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] {status.locked ? 'badge-info' : 'badge-warning'}" role="status">
      <i class="{status.locked ? 'icon-lock' : 'icon-lock-open'} text-[14px] shrink-0"></i>
      <span>
        {#if status.locked && status.start_date}
          {t('stock.opening.status.lockedOn', { date: formatDate(new Date(`${status.start_date}T00:00:00`)) })}
        {:else}
          {t('stock.opening.status.open')}
        {/if}
      </span>
    </div>
  {/if}

  <div class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_340px]">
    <div class="grid gap-3 grid-cols-2 xl:grid-cols-4">
      <div class="surface-card !p-3.5 col-span-2 xl:col-span-1">
        <div class="flex items-center gap-2 text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]"><i class="icon-package text-[13px]"></i>{t('stock.opening.stats.filled')}</div>
        <div class="mt-1.5 font-display font-bold text-[22px] leading-none">{stats ? formatNumber(stats.filled) : '–'}<span class="text-[12px] font-medium text-[var(--text-tertiary)]"> {stats ? t('stock.opening.stats.filledOf', { total: formatNumber(stats.items) }) : ''}</span></div>
        <div class="mt-2.5 h-1.5 rounded-full bg-[var(--surface-sunken)] overflow-hidden" role="progressbar" aria-valuenow={pct} aria-valuemin="0" aria-valuemax="100" aria-label={t('stock.opening.stats.progress')}>
          <div class="h-full rounded-full bg-[var(--color-primary-600)] transition-[width] duration-500" style="width:{pct}%"></div>
        </div>
        <div class="mt-1.5 text-[11.5px] text-[var(--text-tertiary)]">
          {#if stats}{stats.filled >= stats.items && stats.items > 0 ? t('stock.opening.stats.allDone') : t('stock.opening.stats.remaining', { count: formatNumber(Math.max(0, stats.items - stats.filled)) })}{:else}&nbsp;{/if}
        </div>
      </div>
      {#each BUCKET_STATS as [b, ic] (b)}
        <div class="surface-card !p-3.5">
          <div class="flex items-center gap-2 text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]"><i class="{ic} text-[13px]"></i>{t('stock.opening.stats.qtyTotal', { bucket: t(`stock.opening.bucket.${b}` as MessageKey) })}</div>
          <div class="mt-1.5 font-display font-bold text-[22px] leading-none">{stats ? formatNumber(Number(stats[b])) : '–'}</div>
          <div class="mt-2.5 text-[11.5px] text-[var(--text-tertiary)]">{t('stock.opening.stats.qtyHint')}</div>
        </div>
      {/each}
    </div>
    <div class="surface-card !p-3.5">
      <div class="flex items-center gap-2 font-semibold text-[12.5px]"><i class="icon-lightbulb text-[14px] text-[var(--color-primary-600)]"></i>{t('stock.opening.guide.title')}</div>
      <ol class="mt-2 space-y-1.5 text-[12px] text-[var(--text-secondary)]">
        {#each GUIDE as k, i (k)}
          <li class="flex gap-2"><span class="shrink-0 w-4 h-4 mt-px rounded-full bg-[var(--surface-sunken)] text-[10px] font-bold grid place-items-center">{i + 1}</span><span>{t(k)}</span></li>
        {/each}
      </ol>
    </div>
  </div>

  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success">
      <i class="icon-circle-check text-[14px] shrink-0"></i><span>{notice}</span>
      <button type="button" class="ms-auto" aria-label={t('common.close')} onclick={() => (notice = '')}><i class="icon-x text-[13px]"></i></button>
    </div>
  {/if}
  {#if loadError}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('stock.opening.loadFailed')} {loadError}</span>
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-2 p-3 border-b border-[var(--border-subtle)]">
      <div class="relative grow sm:grow-0 sm:w-80">
        <i class="icon-search text-[13px] absolute start-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]"></i>
        <input type="search" class="w-full field-control !ps-8" placeholder={t('stock.opening.search')} aria-label={t('stock.opening.search')} bind:value={q} maxlength="200" />
      </div>
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[760px]">
        <thead>
          <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="px-3 py-2 text-start" scope="col">{t('stock.opening.col.code')}</th>
            <th class="px-3 py-2 text-start" scope="col">{t('stock.opening.col.name')}</th>
            <th class="px-3 py-2 text-start" scope="col">{t('stock.opening.col.unit')}</th>
            {#each BUCKETS as b (b)}
              <th class="px-3 py-2 text-end w-36" scope="col">{t(`stock.opening.bucket.${b}`)}</th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr class="border-t border-[var(--border-subtle)] align-middle hover:bg-[var(--surface-sunken)]">
              <td class="px-3 py-1 font-mono text-[12px]">{r.sku}</td>
              <td class="px-3 py-1 font-semibold">{r.name}</td>
              <td class="px-3 py-1">{r.unit}</td>
              {#each BUCKETS as b (b)}
                {@const c = cells[key(r.id, b)]}
                <td class="px-2 py-1 text-end">
                  {#if c}
                    <MoneyInput
                      class="field-control !text-end !py-1 w-28 {c.state === 'error' ? '!border-[var(--color-danger-600)]' : ''}"
                      decimals={3}
                      pad={false}
                      aria-label="{r.name} – {t(`stock.opening.bucket.${b}`)}"
                      aria-invalid={c.state === 'error'}
                      disabled={!editable}
                      bind:value={c.draft}
                      onfocus={(e) => e.currentTarget.select()}
                      onblur={() => commit(r, b)}
                      onkeydown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
                    />
                    {#if c.state === 'saving'}<div class="text-[10.5px] leading-3 text-[var(--text-tertiary)]">{t('stock.opening.saving')}</div>
                    {:else if c.state === 'saved'}<div class="text-[10.5px] leading-3 text-[var(--color-success-600,#16a34a)]"><i class="icon-check text-[10px]"></i> {t('stock.opening.saved')}</div>
                    {:else if c.state === 'error'}<div class="text-[10.5px] leading-3 text-[var(--color-danger-600)]">{c.error}</div>{/if}
                  {/if}
                </td>
              {/each}
            </tr>
          {:else}
            <tr><td colspan="6" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? '…' : q.trim() ? t('stock.opening.emptySearch') : t('stock.opening.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if total > 0}
      <div class="flex flex-wrap items-center justify-between gap-2 p-3 border-t border-[var(--border-subtle)] text-[12px] text-[var(--text-tertiary)]">
        <span>{t('stock.opening.range', { from: offset + 1, to: Math.min(offset + PAGE, total), total })}</span>
        <div class="flex gap-1.5">
          <button type="button" class="btn !text-[12px]" disabled={offset === 0 || loading} onclick={() => go(offset - PAGE)}>{t('stock.opening.prev')}</button>
          <button type="button" class="btn !text-[12px]" disabled={offset + PAGE >= total || loading} onclick={() => go(offset + PAGE)}>{t('stock.opening.next')}</button>
        </div>
      </div>
    {/if}
  </div>
</main>

{#if locking}
  <Modal title={t('stock.opening.lock.title')} onclose={() => (locking = null)}>
    <p class="text-[12.5px]">{t('stock.opening.lock.body')}</p>
    <div class="mt-3">
      <label for="start-date" class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t('stock.opening.lock.date')}</label>
      <DatePicker id="start-date" bind:value={locking.date} clearable={false} />
    </div>
    {#if locking.error}
      <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 mt-3 text-[12.5px] badge-danger">
        <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{locking.error}</span>
      </div>
    {/if}
    <div class="flex items-center gap-2 mt-5">
      <button type="button" class="btn btn-outline !text-[12.5px] flex-1" onclick={() => (locking = null)}>{t('stock.opening.lock.cancel')}</button>
      <button type="button" class="btn btn-primary !text-[12.5px] flex-1 disabled:opacity-60" disabled={locking.busy || !locking.date} onclick={confirmLock}>{t('stock.opening.lock.confirm')}</button>
    </div>
  </Modal>
{/if}
