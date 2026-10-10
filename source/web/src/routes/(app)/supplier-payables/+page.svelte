<script lang="ts">
  import { onMount } from 'svelte';
  import Select from '#lib/components/Select.svelte';
  import SettleModal from '#lib/components/SettleModal.svelte';
  import PayableModal from '#lib/components/PayableModal.svelte';
  import { payables as api, type Payable, type PayableFilter, type PayableSummary } from '#lib/payables/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatDate, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const PAGE = 50;
  const money = (v: string | number) => formatCurrency(Number(v));
  const inputClass = 'h-9 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-[13px] outline-none focus:border-[var(--color-primary-500)]';
  const statusClass = { open: 'badge-info', overdue: 'badge-danger', paid: 'badge-success' } as const;

  let rows = $state<Payable[]>([]);
  let summary = $state<PayableSummary | null>(null);
  let hasMore = $state(false);
  let loading = $state(true);
  let error = $state('');
  let nextCursor = ''; // paginasi keyset (big data): kursor dari halaman sebelumnya
  let q = $state('');
  let status = $state<PayableFilter>('open');
  let openId = $state<string | null>(null);
  let settling = $state(false);

  const statusOptions = $derived((['open', 'overdue', 'paid', 'all'] as const).map((s) => ({ value: s, label: t(`payables.status.${s}`) })));
  const canPay = $derived(can('supplier_payables', 'create'));

  let seq = 0; // hanya respons permintaan terbaru yang dipakai
  async function load(more = false) {
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await api.list({ status, q: q.trim(), limit: PAGE, cursor: more ? nextCursor : '' });
      if (mine !== seq) return;
      rows = more ? [...rows, ...res.data] : res.data;
      summary = res.summary;
      hasMore = res.has_more;
      nextCursor = res.next_cursor;
    } catch (e) {
      if (mine === seq) error = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  $effect(() => {
    void status;
    void q;
    const h = setTimeout(() => void load(), 250);
    return () => clearTimeout(h);
  });

  onMount(() => {
    document.title = t('payables.docTitle');
  });

  const agingBuckets = $derived(
    summary
      ? ([
          ['current', summary.aging.current, ''],
          ['d1_30', summary.aging.d1_30, 'text-[var(--color-warning-600)]'],
          ['d31_60', summary.aging.d31_60, 'text-[var(--color-danger-600)]'],
          ['d60_plus', summary.aging.d60_plus, 'text-[var(--color-danger-600)] font-extrabold']
        ] as const)
      : []
  );
</script>

{#snippet stat(icon: string, tone: string, label: string, value: string, valueClass: string = '')}
  <div class="surface-card !p-4 flex items-start gap-3">
    <span class="inline-flex size-10 shrink-0 items-center justify-center rounded-xl {tone}"><i class="{icon} text-[18px]"></i></span>
    <div class="min-w-0">
      <div class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{label}</div>
      <div class="mt-0.5 truncate font-display text-[20px] font-bold leading-tight tabular-nums {valueClass}">{value}</div>
    </div>
  </div>
{/snippet}

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('payables.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('payables.subtitle')}</p>
    </div>
    {#if canPay}
      <button type="button" class="btn btn-primary" onclick={() => (settling = true)}><i class="icon-banknote me-1"></i>{t('settle.payable.button')}</button>
    {/if}
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('payables.loadFailed')} {error}</span>
    </div>
  {/if}

  {#if summary}
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
      {@render stat('icon-wallet', 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]', t('payables.stat.outstanding'), money(summary.outstanding))}
      {@render stat('icon-alarm-clock', 'bg-[var(--color-danger-600,#dc2626)]/10 text-[var(--color-danger-600,#dc2626)]', t('payables.stat.overdue'), money(summary.overdue), Number(summary.overdue) > 0 ? 'text-[var(--color-danger-600,#dc2626)]' : '')}
      {@render stat('icon-receipt-text', 'bg-[var(--surface-sunken)] text-[var(--text-secondary)]', t('payables.stat.open'), formatNumber(summary.open_count))}
    </div>

    <section class="surface-card !p-4" aria-label={t('payables.aging.title')}>
      <h2 class="mb-2 text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('payables.aging.title')}</h2>
      <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {#each agingBuckets as [k, v, cls] (k)}
          <div class="rounded-lg bg-[var(--surface-sunken)] px-3 py-2.5">
            <div class="text-[11.5px] text-[var(--text-secondary)]">{t(`payables.aging.${k}`)}</div>
            <div class="mt-0.5 font-display text-[16px] font-bold tabular-nums {Number(v) > 0 ? cls : 'text-[var(--text-tertiary)]'}">{money(v)}</div>
          </div>
        {/each}
      </div>
    </section>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-end gap-3 p-3 border-b border-[var(--border-subtle)]">
      <div class="relative grow sm:grow-0 sm:w-80">
        <i class="icon-search text-[13px] absolute start-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]"></i>
        <input type="search" class="{inputClass}" style="padding-inline-start:2rem" placeholder={t('payables.search')} aria-label={t('payables.search')} bind:value={q} maxlength="100" />
      </div>
      <Select class="!w-auto min-w-44" ariaLabel={t('payables.col.status')} bind:value={status} options={statusOptions} />
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[960px] text-[12.5px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 py-3 text-start" scope="col">{t('payables.col.docNo')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('payables.col.supplier')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('payables.col.date')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('payables.col.due')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('payables.col.amount')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('payables.col.paid')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('payables.col.balance')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('payables.col.status')}</th>
            <th class="px-3 py-3 text-end" scope="col"><span class="sr-only">{t('payables.col.action')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr class="border-b border-[var(--border-subtle)] align-middle last:border-0 hover:bg-[var(--color-primary)]/5">
              <td class="px-4 py-3 whitespace-nowrap">
                <div class="font-mono text-[12px] font-bold">{r.doc_no}</div>
                {#if r.supplier_invoice_no}<div class="font-mono text-[11px] text-[var(--text-tertiary)]">{t('payables.col.invoice')}: {r.supplier_invoice_no}</div>{/if}
              </td>
              <td class="px-3 py-3"><div class="font-medium">{r.supplier_name}</div>{#if r.supplier_code}<div class="font-mono text-[11px] text-[var(--text-tertiary)]">{r.supplier_code}</div>{/if}</td>
              <td class="px-3 py-3 whitespace-nowrap">{formatDate(r.purchase_date)}</td>
              <td class="px-3 py-3 whitespace-nowrap">{r.due_date ? formatDate(r.due_date) : t('payables.noDue')}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap">{money(r.amount)}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap">{money(r.paid)}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap font-bold {r.status === 'paid' ? '' : 'text-[var(--color-danger-600)]'}">{money(r.balance)}</td>
              <td class="px-3 py-3"><span class="badge-soft {statusClass[r.status]}">{t(`payables.status.${r.status}`)}</span></td>
              <td class="px-3 py-3 text-end whitespace-nowrap">
                <button type="button" class="btn btn-sm {r.status !== 'paid' && canPay ? 'btn-primary' : ''}" onclick={() => (openId = r.id)}>{r.status !== 'paid' && canPay ? t('payables.pay') : t('payables.detail')}</button>
              </td>
            </tr>
          {:else}
            <tr>
              <td colspan="9" class="px-4 py-16 text-center text-[var(--text-tertiary)]">
                {#if loading}…{:else if q || status !== 'open'}{t('payables.emptySearch')}{:else}{t('payables.empty')}{/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if hasMore}
      <div class="flex justify-center p-3 border-t border-[var(--border-subtle)]">
        <button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(true)}>{t('payables.loadMore')}</button>
      </div>
    {/if}
  </div>
</main>

{#if settling}
  <SettleModal kind="payable" onclose={() => (settling = false)} onchanged={() => load()} />
{/if}

{#if openId}
  <PayableModal id={openId} onclose={() => (openId = null)} onchanged={() => load()} />
{/if}
