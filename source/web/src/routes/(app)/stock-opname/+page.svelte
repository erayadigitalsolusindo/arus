<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { counts, BUCKETS, type Bucket, type CountKind, type CountStatus, type CountSummary, type CountsOverview } from '#lib/stock/api.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t, formatDateTime, formatCurrency, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';
  import Select from '#lib/components/Select.svelte';

  const PAGE = 25;

  let rows = $state<CountSummary[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let status = $state<CountStatus | ''>('');
  let kind = $state<CountKind | ''>('');
  let search = $state('');
  let overview = $state<CountsOverview | null>(null);
  let timer: ReturnType<typeof setTimeout> | undefined;
  let loading = $state(true);
  let loadError = $state('');

  let creating = $state(false);
  let bucket = $state<Bucket>('display');
  let note = $state('');
  let busy = $state(false);
  let formError = $state('');

  const canCreate = $derived(can('stock_opname', 'create'));
  const canQuick = $derived(can('stock_opname', 'approve'));
  const filtered = $derived(status !== '' || kind !== '' || search.trim() !== '');

  const pill: Record<CountStatus, string> = { draft: 'badge-warning', completed: 'badge-success', cancelled: 'badge-neutral' };
  const net = $derived(overview ? Number(overview.plus) + Number(overview.minus) : 0);
  const signed = (n: number) => (n > 0 ? '+' : '') + formatCurrency(n);
  const tone = (n: number) => (n < 0 ? 'text-[var(--color-danger-600)]' : n > 0 ? 'text-[var(--color-success-600,#16a34a)]' : 'text-[var(--text-tertiary)]');
  const pct = (r: CountSummary) => (r.lines > 0 ? Math.round((r.counted / r.lines) * 100) : 0);

  async function load() {
    loading = true;
    loadError = '';
    try {
      const res = await counts.list({ status, kind, q: search.trim(), limit: PAGE, offset });
      rows = res.data;
      total = res.total;
      overview = res.summary;
    } catch (err) {
      loadError = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  onMount(load);

  // Muat ulang saat outlet aktif berpindah (daftar per outlet).
  let lastOutlet = session.outlet?.id;
  $effect(() => {
    const id = session.outlet?.id;
    if (id !== lastOutlet) {
      lastOutlet = id;
      offset = 0;
      void load();
    }
  });

  function filterChanged() {
    offset = 0;
    void load();
  }

  function searchChanged() {
    clearTimeout(timer);
    timer = setTimeout(filterChanged, 350);
  }

  function resetFilters() {
    status = '';
    kind = '';
    search = '';
    filterChanged();
  }

  function go(next: number) {
    offset = Math.max(0, next);
    void load();
  }

  async function create(e: SubmitEvent) {
    e.preventDefault();
    if (busy) return;
    busy = true;
    formError = '';
    try {
      const d = await counts.create(bucket, note.trim());
      await goto(`/stock-opname/${d.id}`);
    } catch (err) {
      formError = errorMessage(err);
      busy = false;
    }
  }

  const label = 'text-[11.5px] font-semibold uppercase tracking-wide block text-[var(--text-tertiary)]';
  const th = 'px-3 py-2 font-semibold whitespace-nowrap';
</script>

<svelte:head><title>{t('stock.count.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('stock.count.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('stock.count.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-3 max-w-[1400px] mx-auto w-full">
  <!-- Ringkasan 30 hari: satu strip ringkas -->
  <section class="surface-card !p-0 grid grid-cols-2 lg:grid-cols-4 divide-y lg:divide-y-0 lg:divide-x divide-[var(--border-subtle)]" aria-label={t('stock.count.kpi.period')}>
    {#each [{ k: 'open', v: overview?.open }, { k: 'done', v: overview?.done }, { k: 'diffs', v: overview?.diff_lines }, { k: 'net', v: net }] as const as c (c.k)}
      <div class="px-4 py-2.5 min-w-0">
        <p class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] truncate">{t(`stock.count.kpi.${c.k}`)}{c.k !== 'open' ? ` · ${t('stock.count.kpi.period')}` : ''}</p>
        <p class="mt-0.5 font-display font-bold text-[18px] leading-tight tabular-nums {c.k === 'net' ? tone(net) : ''}">
          {#if c.v == null}<span class="inline-block h-4 w-12 rounded bg-[var(--surface-sunken)] animate-pulse align-middle"></span>
          {:else if c.k === 'net'}{signed(net)}{#if overview}<span class="ms-2 text-[11px] font-medium text-[var(--text-tertiary)]">▲ {formatCurrency(Number(overview.plus))} · ▼ {formatCurrency(Math.abs(Number(overview.minus)))}</span>{/if}
          {:else}{formatNumber(Number(c.v))}{/if}
        </p>
      </div>
    {/each}
  </section>

  <!-- Toolbar -->
  <div class="flex flex-wrap items-center gap-2">
    <div class="relative grow basis-56 max-w-xs">
      <i class="icon-search absolute start-3 top-1/2 -translate-y-1/2 text-[13px] text-[var(--text-tertiary)]"></i>
      <input type="search" class="w-full field-control !ps-8" placeholder={t('stock.count.filter.searchDoc')} aria-label={t('stock.count.filter.searchDoc')} maxlength="64" bind:value={search} oninput={searchChanged} />
    </div>
    <Select
      class="!w-auto min-w-36"
      ariaLabel={t('stock.count.list.doc')}
      bind:value={status}
      onchange={filterChanged}
      options={[
        { value: '', label: t('stock.count.filter.all') },
        { value: 'draft', label: t('stock.count.status.draft') },
        { value: 'completed', label: t('stock.count.status.completed') },
        { value: 'cancelled', label: t('stock.count.status.cancelled') }
      ]}
    />
    <Select
      class="!w-auto min-w-32"
      ariaLabel={t('stock.count.filter.allKinds')}
      bind:value={kind}
      onchange={filterChanged}
      options={[
        { value: '', label: t('stock.count.filter.allKinds') },
        { value: 'session', label: t('stock.count.kind.session') },
        { value: 'quick', label: t('stock.count.kind.quick') }
      ]}
    />
    {#if filtered}<button type="button" class="text-[12px] text-[var(--color-primary-600)] inline-flex items-center gap-1" onclick={resetFilters}><i class="icon-x text-[12px]"></i>{t('stock.count.filter.clear')}</button>{/if}
    <div class="ms-auto flex items-center gap-2">
      {#if session.outlet}<span class="hidden md:inline-flex badge-soft badge-neutral items-center gap-1"><i class="icon-store text-[11px]"></i>{session.outlet.name}</span>{/if}
      {#if canQuick}
        <a href="/stock-opname/langsung" class="btn !text-[12.5px] !border !border-[var(--border-subtle)]"><i class="icon-zap text-[13px]"></i>{t('stock.count.newQuick')}</a>
      {/if}
      {#if canCreate}
        <button type="button" class="btn btn-primary !text-[12.5px]" onclick={() => { creating = true; formError = ''; }}>
          <i class="icon-plus text-[13px]"></i>{t('stock.count.new')}
        </button>
      {/if}
    </div>
  </div>

  {#if loadError}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('stock.count.list.loadFailed')} {loadError}</span>
    </div>
  {/if}

  <!-- Daftar dokumen -->
  <div class="surface-card !p-0 overflow-hidden">
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[860px]">
        <thead>
          <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)] bg-[var(--surface-sunken)] text-start">
            <th class="{th} text-start" scope="col">{t('stock.count.list.doc')}</th>
            <th class="{th} text-start" scope="col">{t('stock.count.list.type')}</th>
            <th class="{th} text-start" scope="col">{t('stock.count.list.bucket')}</th>
            <th class="{th} text-start" scope="col">{t('stock.count.list.date')}</th>
            <th class="{th} text-start" scope="col">{t('stock.count.list.by')}</th>
            <th class="{th} text-start" scope="col">{t('stock.count.list.progress')}</th>
            <th class="{th} text-end" scope="col">{t('stock.count.list.diffs')}</th>
            <th class="{th} text-end" scope="col">{t('stock.count.list.value')}</th>
            <th class="{th} text-start" scope="col">{t('stock.count.list.status')}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            {@const v = Number(r.diff_amount)}
            <tr class="border-t border-[var(--border-subtle)] cursor-pointer transition-colors hover:bg-[var(--surface-sunken)]" onclick={() => goto(`/stock-opname/${r.id}`)}>
              <td class="px-3 py-2">
                <a href="/stock-opname/{r.id}" class="font-mono font-bold text-[var(--color-primary-600)]" onclick={(e) => e.stopPropagation()}>{r.doc_no}</a>
                {#if r.note}<div class="text-[11px] text-[var(--text-tertiary)] truncate max-w-[220px]" title={r.note}>{r.note}</div>{/if}
              </td>
              <td class="px-3 py-2 whitespace-nowrap">
                {#if r.kind === 'quick'}
                  <span class="inline-flex items-center gap-1 font-medium"><i class="icon-zap text-[12px] text-[var(--color-warning-600,#b45309)]"></i>{t('stock.count.kind.quick')}</span>
                  {#if r.mode}<div class="text-[11px] text-[var(--text-tertiary)]">{t(`stock.count.mode.${r.mode}`)}</div>{/if}
                {:else}
                  <span class="font-medium">{t('stock.count.kind.session')}</span>
                {/if}
              </td>
              <td class="px-3 py-2 whitespace-nowrap">{t(`stock.count.bucket.${r.bucket}`)}</td>
              <td class="px-3 py-2 whitespace-nowrap text-[var(--text-tertiary)]">{formatDateTime(new Date(r.created_at))}</td>
              <td class="px-3 py-2 whitespace-nowrap">{r.created_by}</td>
              <td class="px-3 py-2 whitespace-nowrap">
                {#if r.kind === 'quick'}
                  <span class="tabular-nums">{t('stock.count.list.items', { count: r.lines })}</span>
                {:else}
                  <div class="flex items-center gap-2">
                    <div class="h-1.5 w-20 rounded-full bg-[var(--surface-sunken)] overflow-hidden"><div class="h-full rounded-full {r.status === 'cancelled' ? 'bg-[var(--text-tertiary)]' : 'bg-[var(--color-primary-600)]'}" style="width: {pct(r)}%"></div></div>
                    <span class="tabular-nums text-[11.5px]">{r.counted}/{r.lines}</span>
                  </div>
                {/if}
              </td>
              <td class="px-3 py-2 text-end tabular-nums">
                {#if r.status !== 'cancelled' && r.counted > 0}{r.differences}{:else}<span class="text-[var(--text-tertiary)]">—</span>{/if}
              </td>
              <td class="px-3 py-2 text-end tabular-nums whitespace-nowrap font-semibold {tone(v)}">
                {#if r.status !== 'cancelled' && r.differences > 0}{signed(v)}{:else}<span class="font-normal text-[var(--text-tertiary)]">—</span>{/if}
              </td>
              <td class="px-3 py-2 whitespace-nowrap"><span class="badge-soft {pill[r.status]}">{t(`stock.count.status.${r.status}`)}</span></td>
            </tr>
          {:else}
            <tr>
              <td colspan="9" class="px-4 py-10 text-center text-[12.5px] text-[var(--text-tertiary)]">
                {loading ? '…' : filtered ? t('stock.count.list.noMatch') : t('stock.count.list.empty')}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <div class="flex flex-wrap items-center justify-between gap-2 px-3 py-2 border-t border-[var(--border-subtle)] text-[12px] text-[var(--text-tertiary)]">
      <span class="tabular-nums">{total > 0 ? t('stock.opening.range', { from: offset + 1, to: Math.min(offset + PAGE, total), total }) : ''}</span>
      {#if total > PAGE}
        <div class="flex gap-1.5">
          <button type="button" class="btn !text-[12px]" disabled={offset === 0 || loading} onclick={() => go(offset - PAGE)}>{t('stock.opening.prev')}</button>
          <button type="button" class="btn !text-[12px]" disabled={offset + PAGE >= total || loading} onclick={() => go(offset + PAGE)}>{t('stock.opening.next')}</button>
        </div>
      {/if}
    </div>
  </div>
</main>

{#if creating}
  <Modal title={t('stock.count.create.title')} onclose={() => (creating = false)}>
    <form class="space-y-4" onsubmit={create} novalidate>
      <div class="space-y-1.5">
        <label for="oc-bucket" class={label}>{t('stock.count.create.bucket')}</label>
        <Select id="oc-bucket" bind:value={bucket} options={BUCKETS.map((b) => ({ value: b, label: t(`stock.count.bucket.${b}`) }))} />
      </div>
      <div class="space-y-1.5">
        <label for="oc-note" class={label}>{t('stock.count.create.note')}</label>
        <textarea id="oc-note" class="field-control w-full" rows="3" maxlength="1000" bind:value={note}></textarea>
      </div>
      <p class="text-[11.5px] leading-relaxed text-[var(--text-tertiary)]">{t('stock.count.create.hint')}</p>
      {#if formError}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{formError}</span>
        </div>
      {/if}
      <div class="flex justify-end gap-2">
        <button type="button" class="btn !text-[12.5px]" onclick={() => (creating = false)}>{t('common.close')}</button>
        <button type="submit" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={busy}>{busy ? t('stock.count.create.submitting') : t('stock.count.create.submit')}</button>
      </div>
    </form>
  </Modal>
{/if}
