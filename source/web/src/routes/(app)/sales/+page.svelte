<script lang="ts">
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';
  import SaleDetailDrawer from '#lib/components/SaleDetailDrawer.svelte';
  import { sales as api, PAY_METHODS, type SaleAllRow, type SaleAllSummary } from '#lib/sales/api.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { outletScope, supportAllOutlets } from '#lib/outlets/store.svelte.ts';
  import { t, formatCurrency, formatDate, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  // Mode "Semua Cabang": hanya cabang yang boleh diakses pengguna; kolom Cabang muncul.
  supportAllOutlets();
  const allMode = $derived(outletScope.all);

  let rows = $state<SaleAllRow[]>([]);
  let summary = $state<SaleAllSummary | null>(null);
  let nextCursor = $state<string | undefined>();
  let loading = $state(true);
  let error = $state('');
  let q = $state('');
  let from = $state('');
  let to = $state('');
  let status = $state('');
  let method = $state('');
  let detailId = $state<string | null>(null);

  const money = (v: string | number) => formatCurrency(Number(v));
  const hasCost = $derived(summary?.cost !== undefined);
  const colCount = $derived(10 + (allMode ? 1 : 0) + (hasCost ? 2 : 0));

  const statusOptions = $derived([
    { value: '', label: t('sales.status.all') },
    { value: 'completed', label: t('sales.status.completed') },
    { value: 'void', label: t('sales.status.void') }
  ]);
  const methodOptions = $derived([{ value: '', label: t('sales.method.all') }, ...PAY_METHODS.map((m) => ({ value: m, label: t(`sales.method.${m}`) }))]);

  let seq = 0; // hanya respons permintaan terbaru yang dipakai
  async function load(more = false) {
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await api.listAll({ from, to, q: q.trim(), status, method, allOutlets: allMode, cursor: more ? nextCursor : null });
      if (mine !== seq) return;
      rows = more ? [...rows, ...res.data] : res.data;
      summary = res.summary;
      nextCursor = res.next_cursor;
      // Tampilkan periode bawaan server (hari ini) di pemilih tanggal tanpa memuat ulang.
      if (!from) from = res.from;
      if (!to) to = res.to;
    } catch (err) {
      if (mine !== seq) return;
      error = errorMessage(err);
      if (!more) {
        rows = [];
        summary = null;
        nextCursor = undefined;
      }
    } finally {
      if (mine === seq) loading = false;
    }
  }

  // Ubah filter / cabang → muat dari awal (pencarian ditunda sebentar). Tanggal lewat onchange DateRange.
  $effect(() => {
    void q;
    void status;
    void method;
    void allMode;
    void session.outlet?.id;
    const h = setTimeout(() => void load(), 250);
    return () => clearTimeout(h);
  });

  const METHOD_ICON: Record<string, string> = { cash: 'icon-banknote', debit: 'icon-credit-card', credit_card: 'icon-credit-card', ewallet: 'icon-wallet', transfer: 'icon-arrow-right' };
  const timeOf = (iso: string) => new Date(iso).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  const inputClass = 'w-full field-control';
  const taxAndFees = (r: SaleAllRow) => Number(r.tax_store) + Number(r.tax_gov) + Number(r.other_cost);
  const totalDiscount = (r: SaleAllRow) => Number(r.discount) + Number(r.line_discount);
  const lossClass = 'text-[var(--color-danger-600,#dc2626)]';
  const profitClass = 'text-[var(--color-success-600,#16a34a)]';
</script>

<svelte:head><title>{t('sales.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('sales.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('sales.title')}</span>
    </div>
  </div>
</div>

{#snippet stat(icon: string, tone: string, label: string, value: string, sub: string, valueClass: string = '')}
  <div class="surface-card !p-4 flex items-start gap-3">
    <span class="inline-flex size-10 shrink-0 items-center justify-center rounded-xl {tone}"><i class="{icon} text-[18px]"></i></span>
    <div class="min-w-0">
      <div class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{label}</div>
      <div class="mt-0.5 truncate font-display text-[20px] font-bold leading-tight tabular-nums {valueClass}">{value}</div>
      <div class="mt-0.5 min-h-4 truncate text-[11.5px] text-[var(--text-tertiary)]">{sub}</div>
    </div>
  </div>
{/snippet}

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('sales.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('sales.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('sales.loadFailed')} {error}</span>
    </div>
  {/if}

  {#if summary}
    {@const avg = summary.completed_count > 0 ? Number(summary.total) / summary.completed_count : 0}
    {@const gross = Number(summary.total) + Number(summary.discount)}
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 {hasCost ? 'xl:grid-cols-5' : 'xl:grid-cols-3'}">
      {@render stat('icon-receipt-text', 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]', t('sales.summary.count'), formatNumber(summary.count), summary.count > summary.completed_count ? t('sales.summary.voided', { count: summary.count - summary.completed_count }) : '')}
      {@render stat('icon-banknote', 'bg-[var(--color-success-600,#16a34a)]/10 text-[var(--color-success-600,#16a34a)]', t('sales.summary.total'), money(summary.total), t('sales.summary.average', { amount: money(avg) }))}
      {@render stat('icon-percent', 'bg-[var(--color-warning-600,#d97706)]/10 text-[var(--color-warning-600,#d97706)]', t('sales.summary.discount'), money(summary.discount), gross > 0 ? t('sales.summary.discountShare', { pct: formatNumber((Number(summary.discount) / gross) * 100, { maximumFractionDigits: 1 }) }) : '')}
      {#if summary.cost !== undefined && summary.profit !== undefined}
        {@const net = Number(summary.profit) + Number(summary.cost)}
        {@render stat('icon-boxes', 'bg-[var(--surface-sunken)] text-[var(--text-secondary)]', t('sales.summary.cost'), money(summary.cost), '')}
        {@render stat('icon-trending-up', 'bg-[var(--color-success-600,#16a34a)]/10 text-[var(--color-success-600,#16a34a)]', t('sales.summary.profit'), money(summary.profit), net > 0 ? t('sales.summary.margin', { pct: formatNumber((Number(summary.profit) / net) * 100, { maximumFractionDigits: 1 }) }) : '', Number(summary.profit) < 0 ? lossClass : '')}
      {/if}
    </div>
    {#if Object.keys(summary.methods).length || Number(summary.receivable) > 0}
      <div class="flex flex-wrap items-center gap-2 text-[12px]">
        <span class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('sales.summary.methods')}</span>
        {#each summary.by_method as m (m.method_id)}
          <span class="inline-flex items-center gap-1.5 rounded-full border border-[var(--border-subtle)] bg-[var(--surface-raised)] px-2.5 py-1 font-medium"><i class="{METHOD_ICON[m.kind]} text-[12px] text-[var(--text-tertiary)]"></i>{m.name}<span class="tabular-nums text-[var(--text-secondary)]">{money(m.amount)}</span>{#if Number(m.fee) > 0}<span class="tabular-nums text-[var(--color-warning-600)]">· {t('sales.summary.fee', { amount: money(m.fee) })}</span>{/if}{#if Number(m.surcharge) > 0}<span class="tabular-nums text-[var(--color-success-600)]">· {t('sales.summary.surcharge', { amount: money(m.surcharge) })}</span>{/if}</span>
        {/each}
        {#if Number(summary.receivable) > 0}
          <span class="inline-flex items-center gap-1.5 rounded-full border border-[var(--border-subtle)] bg-[var(--surface-raised)] px-2.5 py-1 font-medium"><i class="icon-hourglass text-[12px] text-[var(--text-tertiary)]"></i>{t('sales.credit')}<span class="tabular-nums text-[var(--color-danger-600)]">{money(summary.receivable)}</span></span>
        {/if}
      </div>
    {/if}
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-end gap-3 p-3 border-b border-[var(--border-subtle)]">
      <div class="relative grow sm:grow-0 sm:w-80">
        <i class="icon-search text-[13px] absolute start-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]"></i>
        <input type="search" class="{inputClass} !ps-8" placeholder={t('sales.search')} aria-label={t('sales.search')} bind:value={q} maxlength="60" />
      </div>
      <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('sales.period')} />
      <Select class="!w-auto min-w-40" ariaLabel={t('sales.status.label')} bind:value={status} options={statusOptions} />
      <Select class="!w-auto min-w-40" ariaLabel={t('sales.method.label')} bind:value={method} options={methodOptions} />
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[1180px] text-[12.5px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 py-3 text-start" scope="col">{t('sales.col.time')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('sales.col.docNo')}</th>
            {#if allMode}<th class="px-3 py-3 text-start" scope="col">{t('sales.col.outlet')}</th>{/if}
            <th class="px-3 py-3 text-start" scope="col">{t('sales.col.cashier')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('sales.col.member')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('sales.col.items')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('sales.col.subtotal')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('sales.col.discount')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('sales.col.tax')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('sales.col.total')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('sales.col.methods')}</th>
            {#if hasCost}
              <th class="px-3 py-3 text-end" scope="col">{t('sales.col.cost')}</th>
              <th class="px-3 py-3 text-end" scope="col">{t('sales.col.profit')}</th>
            {/if}
            <th class="w-8" scope="col"><span class="sr-only">{t('sales.detail.open')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            {@const voided = r.status === 'void'}
            <tr class="group cursor-pointer border-b border-[var(--border-subtle)] align-top transition-colors last:border-0 hover:bg-[var(--color-primary)]/5 {voided ? 'opacity-60' : ''}" onclick={(e) => !(e.target as HTMLElement).closest('button') && (detailId = r.id)}>
              <td class="px-4 py-3.5 whitespace-nowrap">
                <div class="font-medium">{formatDate(r.created_at)}</div>
                <div class="text-[11.5px] tabular-nums text-[var(--text-tertiary)]">{timeOf(r.created_at)}</div>
              </td>
              <td class="px-3 py-3.5 whitespace-nowrap">
                <button type="button" class="font-mono text-[12px] font-bold text-[var(--color-primary)] hover:underline" title={t('sales.detail.open')} aria-label="{t('sales.detail.open')}: {r.doc_no}" onclick={() => (detailId = r.id)}>{r.doc_no}</button>
                {#if voided}<div class="mt-1"><span class="badge-soft badge-danger">{t('sales.badge.voidDoc')}</span></div>{:else if r.revision > 1}<div class="mt-1"><span class="badge-soft badge-warning" title={t('sales.badge.revisedTip', { n: r.revision })}>{t('sales.badge.revised', { n: r.revision })}</span></div>{/if}
              </td>
              {#if allMode}<td class="px-3 py-3.5"><div class="font-medium">{r.outlet.name}</div><div class="font-mono text-[11px] text-[var(--text-tertiary)]">{r.outlet.code}</div></td>{/if}
              <td class="px-3 py-3.5">
                <div class="flex items-center gap-2">
                  <span class="inline-flex size-7 shrink-0 items-center justify-center rounded-full bg-[var(--surface-sunken)] text-[11px] font-bold text-[var(--text-secondary)]" aria-hidden="true">{(r.cashier || '?').trim().charAt(0).toUpperCase()}</span>
                  <span class="font-medium">{r.cashier}</span>
                </div>
              </td>
              <td class="px-3 py-3.5">{#if r.member}<div class="flex items-center gap-1.5"><i class="icon-user text-[12px] text-[var(--text-tertiary)]"></i>{r.member}</div>{:else}<span class="text-[var(--text-tertiary)]">—</span>{/if}</td>
              <td class="px-3 py-3.5 text-end tabular-nums">{r.line_count}</td>
              <td class="px-3 py-3.5 text-end whitespace-nowrap tabular-nums">{money(r.subtotal)}</td>
              <td class="px-3 py-3.5">
                {#if totalDiscount(r) > 0}
                  <div class="whitespace-nowrap font-semibold tabular-nums text-[var(--color-warning-600,#d97706)]">−{money(totalDiscount(r))}</div>
                  <div class="mt-1.5 flex flex-wrap gap-1">
                    {#if Number(r.line_discount) > 0}<span class="badge-soft badge-info" title={t('sales.tip.line')}>{t('sales.badge.line')} {money(r.line_discount)}</span>{/if}
                    {#if Number(r.manual_discount) > 0}<span class="badge-soft badge-info" title={t('sales.tip.manual')}>{t('sales.badge.manual')} {money(r.manual_discount)}</span>{/if}
                    {#each r.voucher_codes as code (code)}<span class="badge-soft badge-success inline-flex items-center gap-1" title={t('sales.tip.voucher', { code })}><i class="icon-ticket text-[10px]"></i>{code}</span>{/each}
                    {#if r.points_redeemed > 0}<span class="badge-soft badge-warning inline-flex items-center gap-1" title={t('sales.tip.points', { points: r.points_redeemed })}><i class="icon-star text-[10px]"></i>{r.points_redeemed} · {money(r.redeem_amount)}</span>{/if}
                  </div>
                {:else}
                  <span class="text-[var(--text-tertiary)]">—</span>
                {/if}
                {#if r.price_overrides > 0}<div class="mt-1.5"><span class="badge-soft badge-warning inline-flex items-center gap-1" title={t('sales.tip.override', { count: r.price_overrides })}><i class="icon-shield-check text-[10px]"></i>{t('sales.badge.override')} ×{r.price_overrides}</span></div>{/if}
              </td>
              <td class="px-3 py-3.5 text-end whitespace-nowrap tabular-nums">{taxAndFees(r) > 0 ? '+' + money(taxAndFees(r)) : '—'}</td>
              <td class="px-3 py-3.5 text-end whitespace-nowrap tabular-nums text-[13.5px] font-bold {voided ? 'line-through' : ''}">{money(r.total)}</td>
              <td class="px-3 py-3.5">
                <div class="flex flex-col items-start gap-1">
                  {#each PAY_METHODS.filter((m) => r.methods[m] !== undefined) as m (m)}
                    <span class="inline-flex items-center gap-1.5 whitespace-nowrap text-[12px]"><i class="{METHOD_ICON[m]} text-[12px] text-[var(--text-tertiary)]"></i><span class="text-[var(--text-secondary)]">{t(`sales.method.${m}`)}</span><span class="font-medium tabular-nums">{money(r.methods[m] ?? '0')}</span></span>
                  {/each}
                  {#if Number(r.receivable) > 0}
                    <span class="inline-flex items-center gap-1.5 whitespace-nowrap text-[12px]"><i class="icon-hourglass text-[12px] text-[var(--text-tertiary)]"></i><span class="text-[var(--text-secondary)]">{t('sales.credit')}</span><span class="font-medium tabular-nums text-[var(--color-danger-600)]">{money(r.receivable)}</span></span>
                  {/if}
                </div>
              </td>
              {#if hasCost && r.cost !== undefined && r.profit !== undefined}
                <td class="px-3 py-3.5 text-end whitespace-nowrap tabular-nums text-[var(--text-secondary)]">{money(r.cost)}</td>
                <td class="px-3 py-3.5 text-end whitespace-nowrap tabular-nums font-semibold {Number(r.profit) < 0 ? lossClass : profitClass}">{money(r.profit)}</td>
              {/if}
              <td class="pe-3 py-3.5 text-[var(--text-tertiary)] opacity-0 transition-opacity group-hover:opacity-100"><i class="icon-chevron-right text-[14px]"></i></td>
            </tr>
          {:else}
            <tr>
              <td colspan={colCount} class="px-4 py-16 text-center">
                {#if loading}
                  <span class="text-[var(--text-tertiary)]">…</span>
                {:else}
                  <span class="mx-auto mb-3 inline-flex size-12 items-center justify-center rounded-2xl bg-[var(--surface-sunken)] text-[var(--text-tertiary)]"><i class="icon-receipt-text text-[22px]"></i></span>
                  <div class="text-[13px] font-semibold">{q.trim() || status || method ? t('sales.emptySearch') : t('sales.empty')}</div>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if summary && rows.length > 0}
      <div class="flex flex-wrap items-center justify-between gap-2 border-t border-[var(--border-subtle)] bg-[var(--surface-sunken)] px-4 py-2.5 text-[12px] text-[var(--text-tertiary)]">
        <span>{t('sales.showing', { shown: formatNumber(rows.length), total: formatNumber(summary.count) })}</span>
        {#if nextCursor}<button type="button" class="btn !text-[12px]" disabled={loading} onclick={() => load(true)}>{t('sales.loadMore')}</button>{/if}
      </div>
    {/if}
  </div>
</main>

{#if detailId}
  <SaleDetailDrawer saleId={detailId} onclose={() => (detailId = null)} onswitch={(id) => (detailId = id)} onchanged={() => load()} />
{/if}
