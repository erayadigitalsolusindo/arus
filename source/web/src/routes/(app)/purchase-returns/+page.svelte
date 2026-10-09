<script lang="ts">
  import { onMount } from 'svelte';
  import Select from '#lib/components/Select.svelte';
  import DateRange from '#lib/components/DateRange.svelte';
  import ReturnModal from '#lib/components/PurchaseReturnModal.svelte';
  import { purchaseReturns as api, type ReturnRow, type ReturnList, type ReturnStatus } from '#lib/purchases/returns.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatDate, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const PAGE = 50;
  const money = (v: string | number) => formatCurrency(Number(v), 'IDR', { maximumFractionDigits: 2 });
  const inputClass = 'h-9 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-[13px] outline-none focus:border-[var(--color-primary-500)]';
  const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

  const today = new Date();
  const monthAgo = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 30);

  let rows = $state<ReturnRow[]>([]);
  let summary = $state<ReturnList['summary'] | null>(null);
  let hasMore = $state(false);
  let nextCursor = $state('');
  let loading = $state(true);
  let error = $state('');
  let q = $state('');
  let from = $state(ymd(monthAgo));
  let to = $state(ymd(today));
  let status = $state<ReturnStatus | ''>('');
  let openId = $state<string | null>(null);

  const statusOptions = $derived([
    { value: '', label: t('purchaseReturns.allStatus') },
    { value: 'completed', label: t('purchaseReturns.status.completed') },
    { value: 'void', label: t('purchaseReturns.status.void') }
  ]);
  const canCreate = $derived(can('purchase_returns', 'create'));
  const filtered = $derived(q.trim() !== '' || status !== '');

  let seq = 0; // hanya respons permintaan terbaru yang dipakai
  async function load(more = false) {
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await api.list({ from, to, q: q.trim(), status, limit: PAGE, cursor: more ? nextCursor : '' });
      if (mine !== seq) return;
      rows = more ? [...rows, ...res.data] : res.data;
      hasMore = res.has_more;
      nextCursor = res.next_cursor;
      summary = res.summary;
    } catch (e) {
      if (mine === seq) error = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  $effect(() => {
    void from;
    void to;
    void status;
    void q;
    void session.outlet?.id; // daftar per cabang aktif
    const h = setTimeout(() => void load(), 250);
    return () => clearTimeout(h);
  });

  onMount(() => {
    document.title = t('purchaseReturns.docTitle');
    const id = new URLSearchParams(location.search).get('open');
    if (id) openId = id;
  });
</script>

{#snippet stat(icon: string, tone: string, label: string, value: string)}
  <div class="surface-card !p-4 flex items-start gap-3">
    <span class="inline-flex size-10 shrink-0 items-center justify-center rounded-xl {tone}"><i class="{icon} text-[18px]"></i></span>
    <div class="min-w-0">
      <div class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{label}</div>
      <div class="mt-0.5 truncate font-display text-[20px] font-bold leading-tight tabular-nums">{value}</div>
    </div>
  </div>
{/snippet}

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div class="max-w-3xl">
      <h1 class="font-display font-bold text-[19px]">{t('purchaseReturns.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('purchaseReturns.subtitle')}</p>
    </div>
    {#if canCreate}
      <a href="/purchase-returns/new" class="btn btn-primary"><i class="icon-undo-2 text-[14px]"></i>{t('purchaseReturns.newReturn')}</a>
    {/if}
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('purchaseReturns.loadFailed')} {error}</span>
    </div>
  {/if}

  {#if summary}
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
      {@render stat('icon-undo-2', 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]', t('purchaseReturns.stat.total'), money(summary.total))}
      {@render stat('icon-wallet', 'bg-[var(--color-warning-500)]/10 text-[var(--color-warning-600)]', t('purchaseReturns.stat.cut'), money(summary.payable_cut))}
      {@render stat('icon-hand-coins', 'bg-[var(--color-success-500)]/10 text-[var(--color-success-600)]', t('purchaseReturns.stat.refund'), money(summary.refund))}
      {@render stat('icon-receipt-text', 'bg-[var(--surface-sunken)] text-[var(--text-secondary)]', t('purchaseReturns.stat.count'), formatNumber(summary.count))}
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-end gap-3 p-3 border-b border-[var(--border-subtle)]">
      <div class="relative grow sm:grow-0 sm:w-80">
        <i class="icon-search text-[13px] absolute top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]" style="inset-inline-start:0.75rem"></i>
        <input type="search" class={inputClass} style="padding-inline-start:2rem" placeholder={t('purchaseReturns.search')} aria-label={t('purchaseReturns.search')} bind:value={q} maxlength="100" />
      </div>
      <DateRange bind:from bind:to ariaLabel={t('purchaseReturns.col.date')} class="w-64" />
      <Select class="!w-auto min-w-44" ariaLabel={t('purchaseReturns.allStatus')} bind:value={status} options={statusOptions} />
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[960px] text-[12.5px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 py-3 text-start" scope="col">{t('purchaseReturns.col.docNo')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('purchaseReturns.col.date')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('purchaseReturns.col.purchase')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('purchaseReturns.col.supplier')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('purchaseReturns.col.lines')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('purchaseReturns.col.cut')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('purchaseReturns.col.refund')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('purchaseReturns.col.total')}</th>
            <th class="px-3 py-3 text-end" scope="col"><span class="sr-only">{t('purchaseReturns.col.action')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr class="border-b border-[var(--border-subtle)] align-middle last:border-0 hover:bg-[var(--color-primary)]/5 {r.status === 'void' ? 'opacity-60' : ''}">
              <td class="px-4 py-3 font-mono text-[12px] font-bold whitespace-nowrap">
                <button type="button" class="hover:underline" onclick={() => (openId = r.id)}>{r.doc_no}</button>
                {#if r.status === 'void'}<span class="badge-soft badge-danger ms-2 font-sans">{t('purchaseReturns.status.void')}</span>{/if}
              </td>
              <td class="px-3 py-3 whitespace-nowrap">
                {formatDate(r.return_date)}
                <div class="mt-0.5 text-[11px] text-[var(--text-tertiary)]">{t('purchaseReturns.entered', { at: formatDateTime(r.created_at), by: r.created_by || '—' })}</div>
              </td>
              <td class="px-3 py-3 font-mono text-[12px]">{r.purchase_doc_no}</td>
              <td class="px-3 py-3 font-medium">{r.supplier_name}</td>
              <td class="px-3 py-3 text-end tabular-nums">{formatNumber(r.lines)}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap">{Number(r.payable_cut) > 0 ? money(r.payable_cut) : '—'}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap">{Number(r.refund) > 0 ? money(r.refund) : '—'}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap font-bold {r.status === 'void' ? 'line-through' : ''}">{money(r.total)}</td>
              <td class="px-3 py-3 text-end whitespace-nowrap">
                <button type="button" class="btn btn-sm" onclick={() => (openId = r.id)}>{t('purchaseReturns.detail')}</button>
              </td>
            </tr>
          {:else}
            <tr>
              <td colspan="9" class="px-4 py-16 text-center text-[var(--text-tertiary)]">
                {#if loading}…{:else if filtered}{t('purchaseReturns.emptySearch')}{:else}{t('purchaseReturns.empty')}{/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if hasMore}
      <div class="flex justify-center p-3 border-t border-[var(--border-subtle)]">
        <button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(true)}>{t('purchaseReturns.loadMore')}</button>
      </div>
    {/if}
  </div>
</main>

{#if openId}
  <ReturnModal id={openId} onclose={() => (openId = null)} onchanged={() => load()} />
{/if}
