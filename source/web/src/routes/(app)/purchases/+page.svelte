<script lang="ts">
  import { onMount } from 'svelte';
  import Select from '#lib/components/Select.svelte';
  import DateRange from '#lib/components/DateRange.svelte';
  import PurchaseModal from '#lib/components/PurchaseModal.svelte';
  import { purchases as api, type PaymentType, type PurchaseRow, type PurchaseList } from '#lib/purchases/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatDate, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const PAGE = 50;
  const money = (v: string | number) => formatCurrency(Number(v), 'IDR', { maximumFractionDigits: 2 });
  const inputClass = 'h-9 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-[13px] outline-none focus:border-[var(--color-primary-500)]';
  const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

  const today = new Date();
  const monthAgo = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 30);

  let rows = $state<PurchaseRow[]>([]);
  let summary = $state<PurchaseList['summary'] | null>(null);
  let total = $state(0);
  let loading = $state(true);
  let error = $state('');
  let q = $state('');
  let from = $state(ymd(monthAgo));
  let to = $state(ymd(today));
  let type = $state<PaymentType | ''>('');
  let openId = $state<string | null>(null);

  const typeOptions = $derived([
    { value: '', label: t('purchases.allTypes') },
    { value: 'cash', label: t('purchases.type.cash') },
    { value: 'credit', label: t('purchases.type.credit') }
  ]);
  const canCreate = $derived(can('purchase_invoices', 'create'));
  const filtered = $derived(q.trim() !== '' || type !== '');

  let seq = 0; // hanya respons permintaan terbaru yang dipakai
  async function load(more = false) {
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await api.list({ from, to, q: q.trim(), payment_type: type, limit: PAGE, offset: more ? rows.length : 0 });
      if (mine !== seq) return;
      rows = more ? [...rows, ...res.data] : res.data;
      total = res.total;
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
    void type;
    void q;
    const h = setTimeout(() => void load(), 250);
    return () => clearTimeout(h);
  });

  onMount(() => {
    document.title = t('purchases.docTitle');
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
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('purchases.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('purchases.subtitle')}</p>
    </div>
    {#if canCreate}
      <a href="/purchases/new" class="btn btn-primary"><i class="icon-plus text-[14px]"></i>{t('purchases.newInvoice')}</a>
    {/if}
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('purchases.loadFailed')} {error}</span>
    </div>
  {/if}

  {#if summary}
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
      {@render stat('icon-truck', 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]', t('purchases.stat.total'), money(summary.total))}
      {@render stat('icon-wallet', 'bg-[var(--surface-sunken)] text-[var(--text-secondary)]', t('purchases.stat.credit'), money(summary.credit_total))}
      {@render stat('icon-receipt-text', 'bg-[var(--surface-sunken)] text-[var(--text-secondary)]', t('purchases.stat.count'), formatNumber(summary.count))}
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-end gap-3 p-3 border-b border-[var(--border-subtle)]">
      <div class="relative grow sm:grow-0 sm:w-80">
        <i class="icon-search text-[13px] absolute start-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]"></i>
        <input type="search" class="{inputClass} !ps-8" placeholder={t('purchases.search')} aria-label={t('purchases.search')} bind:value={q} maxlength="100" />
      </div>
      <DateRange bind:from bind:to ariaLabel={t('purchases.col.date')} class="w-64" />
      <Select class="!w-auto min-w-44" ariaLabel={t('purchases.col.type')} bind:value={type} options={typeOptions} />
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[900px] text-[12.5px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 py-3 text-start" scope="col">{t('purchases.col.docNo')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('purchases.col.date')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('purchases.col.supplier')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('purchases.col.invoice')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('purchases.col.type')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('purchases.col.lines')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('purchases.col.total')}</th>
            <th class="px-3 py-3 text-end" scope="col"><span class="sr-only">{t('purchases.col.action')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr class="border-b border-[var(--border-subtle)] align-middle last:border-0 hover:bg-[var(--color-primary)]/5">
              <td class="px-4 py-3 font-mono text-[12px] font-bold whitespace-nowrap">{r.doc_no}</td>
              <td class="px-3 py-3 whitespace-nowrap">{formatDate(r.purchase_date)}</td>
              <td class="px-3 py-3 font-medium">{r.supplier_name}</td>
              <td class="px-3 py-3 font-mono text-[12px]">{r.supplier_invoice_no || '—'}</td>
              <td class="px-3 py-3 whitespace-nowrap">
                <span class="badge-soft {r.payment_type === 'credit' ? 'badge-warning' : 'badge-success'}">{t(`purchases.type.${r.payment_type}`)}</span>
                {#if r.due_date}<div class="mt-0.5 text-[11px] text-[var(--text-tertiary)]">{t('purchases.col.due')}: {formatDate(r.due_date)}</div>{/if}
              </td>
              <td class="px-3 py-3 text-end tabular-nums">{formatNumber(r.lines)}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap font-bold">{money(r.total)}</td>
              <td class="px-3 py-3 text-end whitespace-nowrap">
                <button type="button" class="btn btn-sm" onclick={() => (openId = r.id)}>{t('purchases.detail')}</button>
              </td>
            </tr>
          {:else}
            <tr>
              <td colspan="8" class="px-4 py-16 text-center text-[var(--text-tertiary)]">
                {#if loading}…{:else if filtered}{t('purchases.emptySearch')}{:else}{t('purchases.empty')}{/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if rows.length < total}
      <div class="flex justify-center p-3 border-t border-[var(--border-subtle)]">
        <button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(true)}>{t('purchases.loadMore')}</button>
      </div>
    {/if}
  </div>
</main>

{#if openId}
  <PurchaseModal id={openId} onclose={() => (openId = null)} />
{/if}
