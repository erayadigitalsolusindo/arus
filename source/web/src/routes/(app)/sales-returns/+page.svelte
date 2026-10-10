<script lang="ts">
  import { onMount } from 'svelte';
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';
  import SaleReturnModal from '#lib/components/SaleReturnModal.svelte';
  import { salesReturns, type SaleReturnList, type SaleReturnRow } from '#lib/sales/returns.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatDate, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const PAGE = 50;
  const money = (v: string) => formatCurrency(Number(v), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  const inputClass = 'h-9 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-[13px] outline-none focus:border-[var(--color-primary-500)]';
  const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const today = new Date();

  let rows = $state<SaleReturnRow[]>([]);
  let hasMore = $state(false);
  let nextCursor = $state('');
  let loading = $state(true);
  let error = $state('');
  let query = $state('');
  let from = $state(ymd(new Date(today.getFullYear(), today.getMonth(), today.getDate() - 30)));
  let to = $state(ymd(today));
  let openId = $state<string | null>(null);
  let status = $state<'' | 'completed' | 'void'>('');
  const statusOptions = $derived([
    { value: '' as const, label: t('sales.returnFlow.status.all') },
    { value: 'completed' as const, label: t('sales.returnFlow.status.completed') },
    { value: 'void' as const, label: t('sales.returnFlow.status.void') }
  ]);

  const canCreate = $derived(can('sales_returns', 'create'));
  const filtered = $derived(query.trim() !== '');
  let seq = 0;

  async function load(more = false) {
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const result = await salesReturns.list({ from, to, q: query.trim(), status, limit: PAGE, cursor: more ? nextCursor : '' });
      if (mine !== seq) return;
      rows = more ? [...rows, ...result.data] : result.data;
      hasMore = result.has_more;
      nextCursor = result.next_cursor;
    } catch (err) {
      if (mine === seq) {
        error = errorMessage(err);
        if (!more) rows = [];
      }
    } finally {
      if (mine === seq) loading = false;
    }
  }

  $effect(() => {
    void from;
    void to;
    void query;
    void status;
    void session.outlet?.id;
    const timer = setTimeout(() => void load(), 250);
    return () => clearTimeout(timer);
  });

  onMount(() => {
    document.title = t('sales.returnFlow.docTitle');
    const id = new URLSearchParams(location.search).get('open');
    if (id) openId = id;
  });
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div class="max-w-3xl">
      <h1 class="font-display font-bold text-[19px]">{t('sales.returnFlow.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('sales.returnFlow.subtitle')}</p>
    </div>
    {#if canCreate}
      <a href="/sales-returns/new" class="btn btn-primary"><i class="icon-undo-2 text-[14px]"></i>{t('sales.returnFlow.newReturn')}</a>
    {/if}
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('sales.returnFlow.loadFailed')} {error}</span>
    </div>
  {/if}

  <section class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-end gap-3 p-3 border-b border-[var(--border-subtle)]">
      <div class="relative grow sm:grow-0 sm:w-80">
        <i class="icon-search text-[13px] absolute top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]" style="inset-inline-start:0.75rem"></i>
        <input type="search" class={inputClass} style="padding-inline-start:2rem" placeholder={t('sales.returnFlow.search')} aria-label={t('sales.returnFlow.search')} bind:value={query} maxlength="100" />
      </div>
      <DateRange bind:from bind:to ariaLabel={t('sales.returnFlow.col.date')} class="w-64" />
      <Select bind:value={status} ariaLabel={t('sales.returnFlow.status.label')} options={statusOptions} class="w-44" />
      <span class="ms-auto text-[11.5px] text-[var(--text-tertiary)]">{formatNumber(rows.length)}{hasMore ? '+' : ''}</span>
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[900px] text-[12.5px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 py-3 text-start" scope="col">{t('sales.returnFlow.col.docNo')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('sales.returnFlow.col.date')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('sales.returnFlow.col.sale')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('sales.returnFlow.col.member')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('sales.returnFlow.col.lines')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('sales.returnFlow.col.cut')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('sales.returnFlow.col.refund')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('sales.returnFlow.col.total')}</th>
            <th class="w-8" scope="col"><span class="sr-only">{t('sales.returnFlow.col.action')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.id)}
            <tr class="border-b border-[var(--border-subtle)] align-middle last:border-0 hover:bg-[var(--color-primary)]/5">
              <td class="px-4 py-3 font-mono text-[12px] font-bold whitespace-nowrap">
                <button type="button" class="hover:underline" onclick={() => (openId = row.id)}>{row.doc_no}</button>
                {#if row.status === 'void'}<span class="ms-1 badge-soft badge-danger font-sans">{t('sales.returnFlow.status.void')}</span>{/if}
              </td>
              <td class="px-3 py-3 whitespace-nowrap">
                {formatDate(row.return_date)}
                <div class="mt-0.5 text-[11px] text-[var(--text-tertiary)]">{t('sales.returnFlow.entered', { at: formatDateTime(row.created_at), by: row.created_by || '—' })}</div>
              </td>
              <td class="px-3 py-3 font-mono text-[12px]">{row.sale_doc_no}</td>
              <td class="px-3 py-3 font-medium">{row.member_name || '—'}</td>
              <td class="px-3 py-3 text-end tabular-nums">{formatNumber(row.lines)}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap">{Number(row.receivable_cut) > 0 ? money(row.receivable_cut) : '—'}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap">{Number(row.refund) > 0 ? money(row.refund) : '—'}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap font-bold">{money(row.total)}</td>
              <td class="px-3 py-3 text-end"><button type="button" class="btn btn-sm" onclick={() => (openId = row.id)}>{t('sales.returnFlow.detail')}</button></td>
            </tr>
          {:else}
            <tr><td colspan="9" class="px-4 py-16 text-center text-[var(--text-tertiary)]">{#if loading}…{:else if filtered}{t('sales.returnFlow.emptySearch')}{:else}{t('sales.returnFlow.empty')}{/if}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if hasMore}
      <div class="flex justify-center p-3 border-t border-[var(--border-subtle)]">
        <button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(true)}>{t('sales.returnFlow.loadMore')}</button>
      </div>
    {/if}
  </section>
</main>

{#if openId}
  <SaleReturnModal id={openId} onclose={() => (openId = null)} onchanged={() => void load()} />
{/if}
