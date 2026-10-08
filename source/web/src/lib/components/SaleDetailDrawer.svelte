<script lang="ts">
  // Panel detail nota (slide-over kanan): barang yang dibeli, harga daftar → harga jual, HPP/laba (bila berizin),
  // sumber potongan (manual/kupon/poin), pembayaran, dampak stok, dan riwayat audit. Hanya baca.
  import { onMount } from 'svelte';
  import { sales as api, PAY_METHODS, type SaleDetail } from '#lib/sales/api.ts';
  import { t, tryT, formatCurrency, formatNumber, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let { saleId, onclose }: { saleId: string; onclose: () => void } = $props();

  type Tab = 'items' | 'discounts' | 'payments' | 'stock' | 'history';
  let tab = $state<Tab>('items');
  let d = $state<SaleDetail | null>(null);
  let error = $state('');
  let panel = $state<HTMLElement>();

  onMount(() => {
    const previous = document.activeElement as HTMLElement | null;
    panel?.focus();
    api
      .detail(saleId)
      .then((res) => (d = res))
      .catch((err) => (error = errorMessage(err)));
    return () => previous?.focus?.();
  });

  const money = (v: string | number) => formatCurrency(Number(v));
  const qty = (v: string) => formatNumber(Number(v), { maximumFractionDigits: 3 });
  const num = (v: string | undefined) => Number(v ?? 0);
  const profitClass = (v: string | undefined) => (num(v) < 0 ? 'text-[var(--color-danger-600,#dc2626)]' : 'text-[var(--color-success-600,#16a34a)]');
  const actionLabel = (a: string) => tryT(`audit.actions.${a.replace('.', '_')}`) ?? a;

  const voided = $derived(d?.status === 'void');
  const hasCost = $derived(d?.cost !== undefined);
  const margin = $derived(d && d.cost !== undefined && d.profit !== undefined && num(d.subtotal) - num(d.discount) > 0 ? (num(d.profit) / (num(d.subtotal) - num(d.discount))) * 100 : null);
  const discountTotal = $derived(d ? num(d.discount) + num(d.line_discount) : 0);
  const netPaid = $derived(d ? num(d.paid) - num(d.change) : 0);
  const overrides = $derived(d ? d.lines.filter((l) => l.price_override).length : 0);

  const tabs = $derived<{ id: Tab; label: string; count?: number }[]>([
    { id: 'items', label: t('sales.detail.tabs.items'), count: d?.lines.length },
    { id: 'discounts', label: t('sales.detail.tabs.discounts'), count: d ? (num(d.line_discount) > 0 ? 1 : 0) + (num(d.manual_discount) > 0 ? 1 : 0) + d.vouchers.length + (d.points_redeemed > 0 ? 1 : 0) : undefined },
    { id: 'payments', label: t('sales.detail.tabs.payments'), count: d?.payments.length },
    { id: 'stock', label: t('sales.detail.tabs.stock'), count: d?.stock.length },
    { id: 'history', label: t('sales.detail.tabs.history'), count: d?.events.length }
  ]);

  const BUCKET_ICON: Record<string, string> = { display: 'icon-store', warehouse: 'icon-boxes', returns: 'icon-rotate-ccw' };
  const METHOD_ICON: Record<string, string> = { cash: 'icon-banknote', debit: 'icon-credit-card', credit_card: 'icon-credit-card', ewallet: 'icon-wallet', transfer: 'icon-arrow-right' };

  // Detail audit yang dikenal: penyetuju + baris yang diubah (sku, harga normal → harga, atau potongan).
  type EvLine = { sku?: string; list_price?: string; price?: string; discount?: string; qty?: string };
  const evApprover = (details: Record<string, unknown> | null) => (typeof details?.approver === 'string' ? details.approver : '');
  const evLines = (details: Record<string, unknown> | null): EvLine[] => (Array.isArray(details?.lines) ? (details.lines as EvLine[]) : []);

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && !e.defaultPrevented) onclose();
  }
  function tabKeys(e: KeyboardEvent) {
    const i = tabs.findIndex((x) => x.id === tab);
    const next = e.key === 'ArrowRight' ? i + 1 : e.key === 'ArrowLeft' ? i - 1 : null;
    if (next === null) return;
    e.preventDefault();
    tab = tabs[(next + tabs.length) % tabs.length].id;
    (document.getElementById(`sd-tab-${tab}`) as HTMLElement | null)?.focus();
  }

  const label = 'text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
</script>

<svelte:window {onkeydown} />

<div class="fixed inset-0 z-[60] flex justify-end bg-black/45 backdrop-blur-[2px] sd-fade" role="presentation" onmousedown={(e) => e.target === e.currentTarget && onclose()}>
  <div
    bind:this={panel}
    class="sd-slide flex h-full w-full max-w-[980px] flex-col bg-[var(--surface-canvas)] shadow-2xl outline-none"
    role="dialog"
    aria-modal="true"
    aria-label={t('sales.detail.title')}
    tabindex="-1"
  >
    <!-- Kepala: identitas nota -->
    <header class="border-b border-[var(--border-subtle)] bg-[var(--surface-raised)] px-5 pt-4 pb-3">
      <div class="flex items-start gap-3">
        <span class="inline-flex size-11 shrink-0 items-center justify-center rounded-xl bg-[var(--color-primary-600)]/10 text-[var(--color-primary-600)]"><i class="icon-receipt text-[20px]"></i></span>
        <div class="min-w-0 grow">
          <div class="flex flex-wrap items-center gap-2">
            <h2 class="font-mono text-[17px] font-bold tracking-tight">{d?.doc_no ?? '…'}</h2>
            {#if d}<span class="badge-soft {voided ? 'badge-danger' : 'badge-success'}">{voided ? t('sales.status.void') : t('sales.status.completed')}</span>{/if}
            {#if overrides > 0}<span class="badge-soft badge-warning">{t('sales.badge.override')} ×{overrides}</span>{/if}
          </div>
          {#if d}<div class="mt-0.5 text-[12px] text-[var(--text-tertiary)]">{formatDateTime(d.created_at)}</div>{/if}
        </div>
        <button type="button" class="header-icon-btn" onclick={onclose} aria-label={t('common.close')}><i class="icon-x text-[16px]"></i></button>
      </div>

      {#if d}
        <dl class="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-[12.5px] sm:grid-cols-4">
          <div><dt class={label}>{t('sales.detail.info.outlet')}</dt><dd class="mt-0.5 font-medium">{d.outlet.name} <span class="font-mono text-[11px] font-normal text-[var(--text-tertiary)]">{d.outlet.code}</span></dd></div>
          <div><dt class={label}>{t('sales.detail.info.cashier')}</dt><dd class="mt-0.5 font-medium">{d.cashier || '—'}</dd></div>
          <div><dt class={label}>{t('sales.detail.info.member')}</dt><dd class="mt-0.5 font-medium">{d.member ? d.member.name : '—'}{#if d.member}<span class="font-mono text-[11px] font-normal text-[var(--text-tertiary)]"> {d.member.code}</span>{/if}</dd></div>
          <div><dt class={label}>{t('sales.detail.info.salesperson')}</dt><dd class="mt-0.5 font-medium">{d.salesperson?.name ?? '—'}</dd></div>
        </dl>
      {/if}
    </header>

    {#if error}
      <div class="p-5"><div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('sales.detail.loadFailed')} {error}</span></div></div>
    {:else if !d}
      <div class="space-y-3 p-5" aria-busy="true">
        <div class="grid grid-cols-3 gap-3">{#each [0, 1, 2] as i (i)}<div class="h-[68px] animate-pulse rounded-xl bg-[var(--surface-sunken)]"></div>{/each}</div>
        {#each [0, 1, 2, 3] as i (i)}<div class="h-10 animate-pulse rounded-lg bg-[var(--surface-sunken)]"></div>{/each}
      </div>
    {:else}
      <!-- Kartu ringkas -->
      <div class="grid grid-cols-2 gap-3 px-5 pt-4 {hasCost ? 'sm:grid-cols-4' : 'sm:grid-cols-3'}">
        <div class="surface-card !p-3.5 col-span-2 sm:col-span-1">
          <div class={label}>{t('sales.detail.stat.total')}</div>
          <div class="mt-1 font-display text-[22px] font-bold tabular-nums leading-tight {voided ? 'line-through opacity-60' : ''}">{money(d.total)}</div>
          <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t('sales.detail.stat.items', { count: d.lines.length, qty: qty(d.base_qty_total) })}</div>
        </div>
        <div class="surface-card !p-3.5">
          <div class={label}>{t('sales.detail.stat.paid')}</div>
          <div class="mt-1 font-display text-[17px] font-bold tabular-nums">{money(d.paid)}</div>
          <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t('sales.detail.stat.change')} {money(d.change)}</div>
        </div>
        <div class="surface-card !p-3.5">
          <div class={label}>{t('sales.summary.discount')}</div>
          <div class="mt-1 font-display text-[17px] font-bold tabular-nums">{money(discountTotal)}</div>
          <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{d.vouchers.length ? d.vouchers.map((v) => v.code).join(', ') : '—'}</div>
        </div>
        {#if hasCost && d.profit !== undefined && d.cost !== undefined}
          <div class="surface-card !p-3.5">
            <div class={label}>{t('sales.detail.stat.profit')}</div>
            <div class="mt-1 font-display text-[17px] font-bold tabular-nums {profitClass(d.profit)}">{money(d.profit)}</div>
            <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t('sales.detail.stat.cost')} {money(d.cost)}{#if margin !== null} · {formatNumber(margin, { maximumFractionDigits: 1 })}%{/if}</div>
          </div>
        {/if}
      </div>

      <!-- Tab -->
      <div class="mt-4 flex gap-1 overflow-x-auto border-b border-[var(--border-subtle)] px-5 scroll-thin" role="tablist" aria-label={t('sales.detail.title')} tabindex="-1" onkeydown={tabKeys}>
        {#each tabs as tb (tb.id)}
          <button
            id="sd-tab-{tb.id}"
            type="button"
            role="tab"
            aria-selected={tab === tb.id}
            aria-controls="sd-panel"
            tabindex={tab === tb.id ? 0 : -1}
            class="relative -mb-px flex shrink-0 items-center gap-1.5 border-b-2 px-3 py-2.5 text-[12.5px] font-semibold transition-colors !bg-transparent {tab === tb.id ? 'border-[var(--color-primary)] !text-[var(--color-primary)]' : 'border-transparent !text-[var(--text-tertiary)] hover:!text-[var(--text-primary,inherit)]'}"
            onclick={() => (tab = tb.id)}
          >
            {tb.label}{#if tb.count !== undefined}<span class="rounded-full bg-[var(--surface-sunken)] px-1.5 py-px text-[10.5px] tabular-nums">{tb.count}</span>{/if}
          </button>
        {/each}
      </div>

      <div id="sd-panel" role="tabpanel" aria-labelledby="sd-tab-{tab}" class="grow overflow-y-auto p-5 scroll-thin" tabindex="0">
        {#if tab === 'items'}
          <div class="surface-card !p-0 overflow-hidden">
            <div class="overflow-x-auto scroll-thin">
              <table class="w-full min-w-[600px] text-[12.5px]">
                <thead>
                  <tr class="bg-[var(--surface-sunken)] text-[11px] uppercase tracking-wide text-[var(--text-secondary)]">
                    <th class="px-3 py-2.5 text-start" scope="col">{t('sales.detail.items.item')}</th>
                    <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.items.qty')}</th>
                    <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.items.price')}</th>
                    <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.items.discount')}</th>
                    <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.items.total')}</th>
                    {#if hasCost}
                      <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.items.cost')}</th>
                      <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.items.profit')}</th>
                    {/if}
                  </tr>
                </thead>
                <tbody>
                  {#each d.lines as l (l.position)}
                    <tr class="border-t border-[var(--border-subtle)] align-top">
                      <td class="px-3 py-3">
                        <div class="font-semibold">{l.name}</div>
                        <div class="font-mono text-[11px] text-[var(--text-tertiary)]">{l.sku}</div>
                        {#if Number(l.factor) !== 1}<div class="mt-0.5 text-[11px] text-[var(--text-tertiary)]">{t('sales.detail.items.baseQty', { qty: qty(l.base_qty) })}</div>{/if}
                        {#if l.note}<div class="mt-1 rounded-md bg-[var(--surface-sunken)] px-2 py-1 text-[11.5px]"><span class="font-semibold">{t('sales.detail.items.lineNote')}:</span> {l.note}</div>{/if}
                      </td>
                      <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums">{qty(l.qty)} <span class="text-[var(--text-tertiary)]">{l.unit}</span></td>
                      <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums">
                        {#if l.price_override || num(l.list_price) !== num(l.unit_price)}<div class="text-[11px] text-[var(--text-tertiary)] line-through">{money(l.list_price)}</div>{/if}
                        <div class="font-medium">{money(l.unit_price)}</div>
                        {#if l.price_override}<span class="badge-soft badge-warning mt-1 inline-block">{t('sales.detail.items.overridden')}</span>{/if}
                      </td>
                      <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums">{num(l.discount) > 0 ? '−' + money(l.discount) : '—'}</td>
                      <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums font-semibold">{money(l.line_total)}</td>
                      {#if hasCost && l.unit_cost !== undefined && l.profit !== undefined}
                        <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums text-[var(--text-secondary)]">{money(l.unit_cost)}</td>
                        <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums font-medium {profitClass(l.profit)}">{money(l.profit)}</td>
                      {/if}
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          </div>
          <!-- Ringkasan angka ala invoice -->
          <div class="mt-4 flex justify-end">
            <dl class="surface-card !p-4 w-full max-w-[360px] space-y-2 text-[12.5px] tabular-nums">
              <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.detail.discounts.subtotal')}</dt><dd class="font-medium">{money(d.subtotal)}</dd></div>
              {#if num(d.discount) > 0}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.summary.discount')}</dt><dd class="text-[var(--color-warning-600,#d97706)]">−{money(d.discount)}</dd></div>{/if}
              {#if num(d.tax_store) + num(d.tax_gov) > 0}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.col.tax')}</dt><dd>+{money(num(d.tax_store) + num(d.tax_gov))}</dd></div>{/if}
              {#if num(d.other_cost) > 0}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.detail.discounts.otherCost')}</dt><dd>+{money(d.other_cost)}</dd></div>{#each d.other_costs as c, ci (ci)}<div class="flex justify-between gap-3 text-[11.5px] text-[var(--text-tertiary)]"><dt class="ps-3">{c.name || t('sales.detail.discounts.unnamedCost')}</dt><dd>{money(c.amount)}</dd></div>{/each}{/if}
              <div class="flex items-baseline justify-between gap-3 border-t border-[var(--border-subtle)] pt-3 text-[15px] font-bold"><dt>{t('sales.detail.discounts.total')}</dt><dd>{money(d.total)}</dd></div>
            </dl>
          </div>
          {#if d.note}<p class="mt-3 rounded-lg bg-[var(--surface-sunken)] px-3 py-2 text-[12.5px]"><span class={label}>{t('sales.detail.info.note')}</span><br />{d.note}</p>{/if}
        {:else if tab === 'discounts'}
          <div class="grid gap-4 md:grid-cols-2">
            <section>
              <h3 class="mb-2 text-[12.5px] font-bold">{t('sales.detail.discounts.sources')}</h3>
              {#if discountTotal <= 0}
                <p class="rounded-lg border border-dashed border-[var(--border-subtle)] p-4 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('sales.detail.discounts.none')}</p>
              {:else}
                <ul class="space-y-2">
                  {#if num(d.line_discount) > 0}
                    <li class="surface-card !p-3 flex items-center gap-3">
                      <span class="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-[var(--color-primary-600)]/10 text-[var(--color-primary-600)]"><i class="icon-tag text-[15px]"></i></span>
                      <div class="min-w-0 grow"><div class="font-semibold text-[12.5px]">{t('sales.detail.discounts.lineDiscountItems')}</div><div class="truncate text-[11.5px] text-[var(--text-tertiary)]">{d.lines.filter((l) => num(l.discount) > 0).map((l) => l.name).join(', ')}</div></div>
                      <div class="font-semibold tabular-nums">−{money(d.line_discount)}</div>
                    </li>
                  {/if}
                  {#if num(d.manual_discount) > 0}
                    <li class="surface-card !p-3 flex items-center gap-3">
                      <span class="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-[var(--color-primary-600)]/10 text-[var(--color-primary-600)]"><i class="icon-percent text-[15px]"></i></span>
                      <div class="grow font-semibold text-[12.5px]">{t('sales.detail.discounts.manual')}</div>
                      <div class="font-semibold tabular-nums">−{money(d.manual_discount)}</div>
                    </li>
                  {/if}
                  {#each d.vouchers as v (v.code)}
                    <li class="surface-card !p-3 flex items-center gap-3">
                      <span class="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-[var(--color-success-600,#16a34a)]/10 text-[var(--color-success-600,#16a34a)]"><i class="icon-ticket text-[15px]"></i></span>
                      <div class="min-w-0 grow">
                        <div class="font-mono text-[12.5px] font-bold">{v.code}</div>
                        <div class="truncate text-[11.5px] text-[var(--text-tertiary)]">{v.name} · {v.kind === 'percent' ? t('sales.detail.discounts.kindPercent', { value: formatNumber(Number(v.value), { maximumFractionDigits: 2 }) }) : t('sales.detail.discounts.kindAmount')}</div>
                      </div>
                      <div class="font-semibold tabular-nums">−{money(v.amount)}</div>
                    </li>
                  {/each}
                  {#if d.points_redeemed > 0}
                    <li class="surface-card !p-3 flex items-center gap-3">
                      <span class="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-[var(--color-warning-600,#d97706)]/10 text-[var(--color-warning-600,#d97706)]"><i class="icon-star text-[15px]"></i></span>
                      <div class="min-w-0 grow"><div class="font-semibold text-[12.5px]">{t('sales.detail.discounts.points')}</div><div class="text-[11.5px] text-[var(--text-tertiary)]">{t('sales.detail.discounts.pointsDetail', { points: formatNumber(d.points_redeemed) })}</div></div>
                      <div class="font-semibold tabular-nums">−{money(d.redeem_amount)}</div>
                    </li>
                  {/if}
                </ul>
              {/if}
              {#if d.points_earned > 0}<p class="mt-3 flex items-center gap-2 text-[12px] text-[var(--text-secondary)]"><i class="icon-star text-[13px]"></i>{t('sales.detail.discounts.earned')}: <strong>{formatNumber(d.points_earned)}</strong></p>{/if}
            </section>

            <!-- Rincian hitung ala struk -->
            <section>
              <h3 class="mb-2 text-[12.5px] font-bold">{t('sales.detail.discounts.calc')}</h3>
              <div class="surface-card !p-4 text-[12.5px]">
                <dl class="space-y-2 tabular-nums">
                  <div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.subtotal')}</dt><dd class="font-medium">{money(d.subtotal)}</dd></div>
                  {#if num(d.line_discount) > 0}<div class="flex justify-between gap-3 text-[var(--text-tertiary)]"><dt class="ps-3 text-[11.5px]">{t('sales.detail.discounts.lineDiscount')}</dt><dd class="text-[11.5px]">{money(d.line_discount)}</dd></div>{/if}
                  {#if num(d.manual_discount) > 0}<div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.manual')}</dt><dd>−{money(d.manual_discount)}</dd></div>{/if}
                  {#if num(d.voucher_amount) > 0}<div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.voucher')}</dt><dd>−{money(d.voucher_amount)}</dd></div>{/if}
                  {#if num(d.redeem_amount) > 0}<div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.points')}</dt><dd>−{money(d.redeem_amount)}</dd></div>{/if}
                  <div class="flex justify-between gap-3 border-t border-dashed border-[var(--border-subtle)] pt-2"><dt>{t('sales.detail.discounts.afterDiscount')}</dt><dd class="font-medium">{money(num(d.subtotal) - num(d.discount))}</dd></div>
                  {#if num(d.tax_store) > 0}<div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.taxStore', { pct: formatNumber(Number(d.tax_store_pct), { maximumFractionDigits: 2 }) })}</dt><dd>+{money(d.tax_store)}</dd></div>{/if}
                  {#if num(d.tax_gov) > 0}<div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.taxGov', { pct: formatNumber(Number(d.tax_gov_pct), { maximumFractionDigits: 2 }) })}</dt><dd>+{money(d.tax_gov)}</dd></div>{/if}
                  {#if num(d.other_cost) > 0}<div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.otherCost')}</dt><dd>+{money(d.other_cost)}</dd></div>{#each d.other_costs as c, ci (ci)}<div class="flex justify-between gap-3 text-[11.5px] text-[var(--text-tertiary)]"><dt class="ps-3">{c.name || t('sales.detail.discounts.unnamedCost')}</dt><dd>{money(c.amount)}</dd></div>{/each}{/if}
                  <div class="flex items-baseline justify-between gap-3 border-t border-[var(--border-subtle)] pt-3 text-[15px] font-bold"><dt>{t('sales.detail.discounts.total')}</dt><dd>{money(d.total)}</dd></div>
                </dl>
              </div>
            </section>
          </div>
        {:else if tab === 'payments'}
          {#if d.payments.length === 0}
            <p class="rounded-lg border border-dashed border-[var(--border-subtle)] p-6 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('sales.detail.payments.empty')}</p>
          {:else}
            <ul class="space-y-2">
              {#each d.payments as p, i (i)}
                <li class="surface-card !p-3.5 flex items-center gap-3">
                  <span class="inline-flex size-10 shrink-0 items-center justify-center rounded-lg bg-[var(--color-primary-600)]/10 text-[var(--color-primary-600)]"><i class="{METHOD_ICON[p.method] ?? 'icon-wallet'} text-[17px]"></i></span>
                  <div class="min-w-0 grow">
                    <div class="font-semibold text-[13px]">{PAY_METHODS.includes(p.method) ? t(`sales.method.${p.method}`) : p.method}</div>
                    {#if p.ref_no}<div class="font-mono text-[11.5px] text-[var(--text-tertiary)]">{t('sales.detail.payments.ref')}: {p.ref_no}</div>{/if}
                  </div>
                  <div class="font-display text-[16px] font-bold tabular-nums">{money(p.amount)}</div>
                </li>
              {/each}
            </ul>
            <div class="surface-card !p-4 mt-3 text-[12.5px]">
              <dl class="space-y-2 tabular-nums">
                <div class="flex justify-between"><dt>{t('sales.detail.payments.paid')}</dt><dd class="font-medium">{money(d.paid)}</dd></div>
                <div class="flex justify-between"><dt>{t('sales.detail.payments.change')}</dt><dd>−{money(d.change)}</dd></div>
                <div class="flex justify-between border-t border-[var(--border-subtle)] pt-2 text-[14px] font-bold"><dt>{t('sales.detail.payments.net')}</dt><dd>{money(netPaid)}</dd></div>
              </dl>
            </div>
          {/if}
        {:else if tab === 'stock'}
          <p class="mb-3 text-[12px] text-[var(--text-tertiary)]">{t('sales.detail.stock.intro')}</p>
          {#if d.stock.length === 0}
            <p class="rounded-lg border border-dashed border-[var(--border-subtle)] p-6 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('sales.detail.stock.empty')}</p>
          {:else}
            <div class="surface-card !p-0 overflow-hidden">
              <div class="overflow-x-auto scroll-thin">
                <table class="w-full min-w-[560px] text-[12.5px]">
                  <thead>
                    <tr class="bg-[var(--surface-sunken)] text-[11px] uppercase tracking-wide text-[var(--text-secondary)]">
                      <th class="px-3 py-2.5 text-start" scope="col">{t('sales.detail.stock.item')}</th>
                      <th class="px-3 py-2.5 text-start" scope="col">{t('sales.detail.stock.type')}</th>
                      <th class="px-3 py-2.5 text-start" scope="col">{t('sales.detail.stock.bucket')}</th>
                      <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.stock.change')}</th>
                      <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.stock.after')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each d.stock as m (m.id)}
                      <tr class="border-t border-[var(--border-subtle)]">
                        <td class="px-3 py-3"><div class="font-semibold">{m.name}</div><div class="font-mono text-[11px] text-[var(--text-tertiary)]">{m.sku}</div></td>
                        <td class="px-3 py-3"><span class="badge-soft {m.type === 'SALE' ? 'badge-info' : 'badge-warning'}">{t(`sales.detail.stock.types.${m.type}`)}</span><div class="mt-1 text-[11px] text-[var(--text-tertiary)]">{formatDateTime(m.at)}</div></td>
                        <td class="px-3 py-3 whitespace-nowrap"><i class="{BUCKET_ICON[m.bucket]} me-1 text-[12px] text-[var(--text-tertiary)]"></i>{t(`sales.detail.stock.bucketNames.${m.bucket}`)}</td>
                        <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums font-semibold {num(m.delta) < 0 ? 'text-[var(--color-danger-600,#dc2626)]' : 'text-[var(--color-success-600,#16a34a)]'}">{num(m.delta) > 0 ? '+' : ''}{qty(m.delta)} <span class="font-normal text-[var(--text-tertiary)]">{m.unit}</span></td>
                        <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums {num(m.balance_after) < 0 ? 'text-[var(--color-danger-600,#dc2626)]' : ''}">{qty(m.balance_after)}</td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            </div>
          {/if}
        {:else}
          <p class="mb-3 text-[12px] text-[var(--text-tertiary)]">{t('sales.detail.history.intro')}</p>
          {#if d.events.length === 0}
            <p class="rounded-lg border border-dashed border-[var(--border-subtle)] p-6 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('sales.detail.history.empty')}</p>
          {:else}
            <ol class="relative ms-3 space-y-5 border-s border-[var(--border-subtle)] ps-6">
              {#each d.events as ev, i (i)}
                <li class="relative">
                  <span class="absolute -start-[31px] top-0.5 inline-flex size-[11px] rounded-full border-2 border-[var(--surface-canvas)] bg-[var(--color-primary-600)] ring-1 ring-[var(--color-primary-600)]/40"></span>
                  <div class="flex flex-wrap items-baseline gap-x-2">
                    <span class="text-[13px] font-semibold">{actionLabel(ev.action)}</span>
                    <span class="text-[11.5px] text-[var(--text-tertiary)]">{formatDateTime(ev.at)}{#if ev.actor} · {t('sales.detail.history.by', { actor: ev.actor })}{/if}</span>
                  </div>
                  {#if evApprover(ev.details)}<span class="badge-soft badge-warning mt-1.5 inline-flex items-center gap-1"><i class="icon-shield-check text-[11px]"></i>{t('sales.detail.history.approver', { name: evApprover(ev.details) })}</span>{/if}
                  {#each evLines(ev.details) as el, j (j)}
                    <div class="mt-1.5 flex flex-wrap items-center gap-2 rounded-md bg-[var(--surface-sunken)] px-2.5 py-1.5 text-[12px]">
                      <span class="font-mono text-[11px]">{el.sku}</span>
                      {#if el.list_price !== undefined && el.price !== undefined}<span class="tabular-nums"><span class="text-[var(--text-tertiary)] line-through">{money(el.list_price)}</span> → <strong>{money(el.price)}</strong></span>{/if}
                      {#if el.discount !== undefined}<span class="tabular-nums">−{money(el.discount)}</span>{/if}
                    </div>
                  {/each}
                </li>
              {/each}
            </ol>
          {/if}
        {/if}
      </div>
    {/if}
  </div>
</div>

<style>
  .sd-fade {
    animation: sd-fade 160ms ease-out;
  }
  .sd-slide {
    animation: sd-slide 220ms cubic-bezier(0.22, 1, 0.36, 1);
  }
  @keyframes sd-fade {
    from {
      opacity: 0;
    }
  }
  @keyframes sd-slide {
    from {
      transform: translateX(32px);
      opacity: 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .sd-fade,
    .sd-slide {
      animation: none;
    }
  }
</style>
